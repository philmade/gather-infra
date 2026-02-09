"""
Agency SDK - Main orchestrator

Bridges local ADK agents to GatherTin workspaces via Tinode chat.

Architecture:
    ┌─────────────┐    ┌─────────────┐    ┌─────────────┐    ┌─────────┐
    │   User      │    │   Tinode    │    │  AgencySDK  │    │   ADK   │
    │  (Web UI)   │───▶│   Server    │───▶│   Bridge    │───▶│  Server │
    └─────────────┘    └─────────────┘    └─────────────┘    └─────────┘
                                                                  │
                                                                  ▼
                                                             ┌─────────┐
                                                             │  Gemini │
                                                             └─────────┘

Key Features:
    - Full ADK session management (sessions persist across messages)
    - ADK debugging UI available at http://localhost:8000
    - Automatic @mention detection (responds to @agent_handle)
    - DM support (always responds to direct messages)
    - Message deduplication (handles Tinode history on reconnect)

Session Mapping:
    - ADK app_name = agent folder name (e.g., "hello_agent")
    - ADK user_id = Tinode user ID who sent the message
    - ADK session_id = "topic_{tinode_topic}" (conversation context)
"""

import asyncio
import json
import logging
from pathlib import Path
from typing import Optional, Any
from dataclasses import dataclass, field
from urllib.parse import urlparse

import httpx

from .client import TinodeClient, Message
from .discovery import discover_agents, DiscoveredAgent

logger = logging.getLogger(__name__)


@dataclass
class AgentConfig:
    """Configuration for a single agent"""
    handle: str
    bot_login: str
    bot_password: str


@dataclass
class SDKConfig:
    """SDK configuration loaded from gather.config.json

    Supports PocketBase authentication:
    - auth_token: Pre-obtained PocketBase JWT token
    - email/password: Credentials for PocketBase login
    """
    pocketnode_url: str
    workspace: str
    channels: list[str] = field(default_factory=list)  # Channel IDs for agent subscription
    auth_token: Optional[str] = None  # PocketBase JWT token
    email: Optional[str] = None  # PocketBase email
    password: Optional[str] = None  # PocketBase password
    server: Optional[str] = None  # Tinode WebSocket URL (fetched from PocketNode)
    agents: dict[str, AgentConfig] = field(default_factory=dict)

    @classmethod
    def from_file(cls, path: str | Path) -> "SDKConfig":
        """Load config from JSON file"""
        with open(path) as f:
            data = json.load(f)

        pocketnode_url = data.get("pocketnode_url", "http://localhost:8090")
        workspace = data.get("workspace") or data.get("workspace_id")

        if not workspace:
            raise ValueError("workspace is required in config file")

        return cls(
            pocketnode_url=pocketnode_url,
            workspace=workspace,
            channels=data.get("channels", []),
            auth_token=data.get("auth_token"),
            email=data.get("email"),
            password=data.get("password"),
            server=data.get("server"),
        )


class AgentRunner:
    """
    Runs a single agent with its own Tinode connection.

    Bridges messages to ADK web server for proper session management.
    This enables full ADK debugging at http://localhost:8000

    Handles:
    - Connecting as the agent's bot user
    - Subscribing to appropriate topics
    - Routing messages (always respond to DMs, only @mentions in channels)
    - Calling ADK web server's /run API
    """

    def __init__(
        self,
        agent: DiscoveredAgent,
        config: AgentConfig,
        server: str,
        api_key: str,
        workspace_id: str,
        adk_server_url: str = "http://localhost:8000"
    ):
        self.agent = agent
        self.config = config
        self.workspace_id = workspace_id
        self.adk_server_url = adk_server_url
        self.client = TinodeClient(
            server_url=server,
            login=config.bot_login,
            password=config.bot_password,
            api_key=api_key
        )

        # Session locks to prevent concurrent ADK requests to same session
        # This fixes "stale session" errors when multiple messages arrive simultaneously
        self._session_locks: dict[str, asyncio.Lock] = {}

        # Set up message handler
        self.client.on_message = self._handle_message

    async def start(self) -> None:
        """Connect and start listening"""
        await self.client.connect()
        logger.info(f"Agent @{self.agent.handle} connected as {self.client.user_id}")

        # 1. Update bot profile with name and handle
        display_name = self.agent.name or self.agent.handle.replace("_", " ").title()
        await self.client.update_profile(display_name, self.agent.handle)

        # 2. Subscribe to 'me' to get our existing subscriptions
        await self.client.subscribe_to_me()

        # 3. Discover and subscribe to channels in the workspace
        channels = await self.client.discover_and_subscribe_channels(self.workspace_id)
        logger.info(f"Agent @{self.agent.handle} subscribed to {len(channels)} channel(s)")

        # 4. Send agent:ready event to notify PocketNode
        await self.client.send_agent_ready(
            self.workspace_id,
            self.agent.handle,
            display_name
        )

    async def run(self) -> None:
        """Run the message loop"""
        await self.client.run()

    def _handle_message(self, message: Message) -> None:
        """Handle incoming message - decide whether to respond"""
        should_respond = False
        reason = ""

        logger.debug(f"@{self.agent.handle}: message in {message.topic} (is_dm={message.is_dm})")

        if message.is_dm:
            # Always respond to DMs
            should_respond = True
            reason = "DM"
        elif self.client.user_id and self.client.user_id in message.mentions:
            # Respond when @mentioned via Tinode's formal mention system
            should_respond = True
            reason = f"mentioned in {message.topic}"
        else:
            # Fallback: check for @handle in plain text
            # This handles cases where the frontend sends plain @handle text
            handle_mention = f"@{self.agent.handle}"
            if handle_mention.lower() in message.content.lower():
                should_respond = True
                reason = f"handle mentioned in {message.topic}"

        if should_respond:
            logger.info(
                f"@{self.agent.handle}: responding to {message.from_user} ({reason})"
            )
            # Run async response in background
            asyncio.create_task(self._respond(message))
        else:
            logger.debug(
                f"@{self.agent.handle}: ignoring message in {message.topic} (not mentioned)"
            )

    async def _respond(self, message: Message) -> None:
        """Invoke agent and send response"""
        try:
            # Invoke the ADK agent with proper session context
            response = await self._invoke_agent(
                content=message.content,
                user_id=message.from_user,
                topic=message.topic
            )

            # Send response to the same topic
            if response:
                await self.client.publish(message.topic, response)

        except Exception as e:
            logger.error(f"@{self.agent.handle}: error responding - {e}")
            # Optionally send error message
            await self.client.publish(
                message.topic,
                f"Sorry, I encountered an error: {str(e)}"
            )

    def _get_session_lock(self, session_id: str) -> asyncio.Lock:
        """Get or create a lock for a session to serialize requests"""
        if session_id not in self._session_locks:
            self._session_locks[session_id] = asyncio.Lock()
        return self._session_locks[session_id]

    async def _invoke_agent(self, content: str, user_id: str, topic: str) -> Optional[str]:
        """
        Invoke the ADK agent via ADK web server's REST API.

        This enables full ADK debugging/sessions - view at http://localhost:8000

        Maps:
        - app_name: agent handle (e.g., "hello_agent")
        - user_id: Tinode user ID (who sent the message)
        - session_id: Tinode topic (conversation context)
        """
        app_name = self.agent.handle
        session_id = f"topic_{topic}"

        # Serialize requests to the same session to prevent "stale session" errors
        async with self._get_session_lock(session_id):
            try:
                async with httpx.AsyncClient(timeout=60.0) as client:
                    # Ensure session exists (create if needed)
                    session_url = f"{self.adk_server_url}/apps/{app_name}/users/{user_id}/sessions/{session_id}"
                    session_resp = await client.post(session_url)
                    if session_resp.status_code not in [200, 409]:  # 409 = already exists
                        logger.debug(f"Session creation: {session_resp.status_code}")

                    # Call ADK web server's /run endpoint
                    response = await client.post(
                        f"{self.adk_server_url}/run",
                        json={
                            "app_name": app_name,
                            "user_id": user_id,
                            "session_id": session_id,
                            "new_message": {
                                "parts": [{"text": content}],
                                "role": "user"
                            },
                            "streaming": False
                        }
                    )

                    if response.status_code != 200:
                        logger.error(f"ADK server error: {response.status_code} - {response.text}")
                        return f"Error: ADK server returned {response.status_code}"

                    # Parse response - array of events
                    events = response.json()

                    # Extract text from final response event
                    response_text = ""
                    for event in events:
                        # Look for content with text parts
                        if "content" in event and event["content"]:
                            content_obj = event["content"]
                            if "parts" in content_obj:
                                for part in content_obj["parts"]:
                                    if "text" in part:
                                        response_text += part["text"]

                    if response_text:
                        logger.debug(f"@{self.agent.handle} response: {response_text[:100]}...")
                        return response_text

                    logger.warning(f"@{self.agent.handle} produced no response from ADK")
                    return None

            except httpx.ConnectError:
                logger.error(f"Cannot connect to ADK server at {self.adk_server_url}")
                logger.error("Make sure 'adk web' is running: adk web --port 8000")
                return "Error: ADK server not running. Start it with: adk web"
            except Exception as e:
                logger.error(f"ADK invocation failed: {e}")
                raise



class AgencySDK:
    """
    Main SDK class - discovers agents and bridges them to Tinode.

    Architecture:
        This SDK bridges Tinode ↔ ADK Web Server

        Run 'adk web' separately to get full ADK debugging at http://localhost:8000

    Usage:
        # Terminal 1: Start ADK web server
        adk web --port 8000

        # Terminal 2: Start GatherSDK bridge
        sdk = AgencySDK.from_config("gather.config.json")
        await sdk.discover("./agents")
        await sdk.run()
    """

    def __init__(self, config: SDKConfig, adk_server_url: str = "http://localhost:8000"):
        self.config = config
        self.adk_server_url = adk_server_url
        self.agents: list[DiscoveredAgent] = []
        self.runners: list[AgentRunner] = []

    @classmethod
    def from_config(cls, path: str | Path, adk_server_url: str = "http://localhost:8000") -> "AgencySDK":
        """Create SDK from config file"""
        config = SDKConfig.from_file(path)
        return cls(config, adk_server_url=adk_server_url)

    async def discover(self, root_dir: str | Path = ".") -> list[DiscoveredAgent]:
        """Discover agents in directory and register them with PocketNode"""
        self.agents = discover_agents(root_dir)

        if not self.agents:
            return self.agents

        # Register agents via PocketNode API using PocketBase auth
        await self._register_agents_via_api()

        return self.agents

    async def _get_auth_token(self, client: httpx.AsyncClient) -> str:
        """Get PocketBase auth token (from config or by logging in)"""
        if self.config.auth_token:
            return self.config.auth_token

        if self.config.email and self.config.password:
            # Login to PocketBase to get token
            logger.info("Authenticating with PocketBase...")

            # Try regular users collection first, then superusers
            for collection in ["users", "_superusers"]:
                response = await client.post(
                    f"{self.config.pocketnode_url}/api/collections/{collection}/auth-with-password",
                    json={
                        "identity": self.config.email,
                        "password": self.config.password
                    },
                    timeout=30.0
                )

                if response.status_code == 200:
                    data = response.json()
                    return data["token"]

                # If 404 (collection not found) or 400 (wrong credentials for this collection), try next
                if response.status_code not in [400, 404]:
                    response.raise_for_status()

            raise Exception("Invalid email or password")

        raise Exception(
            "Authentication required. Provide either:\n"
            "  - auth_token: A PocketBase JWT token\n"
            "  - email + password: PocketBase login credentials"
        )

    async def _register_agents_via_api(self) -> None:
        """Register discovered agents via PocketNode API"""
        handles = [agent.handle for agent in self.agents]

        logger.info(f"Registering {len(handles)} agent(s) with PocketNode...")

        async with httpx.AsyncClient() as client:
            try:
                # Get auth token
                auth_token = await self._get_auth_token(client)

                response = await client.post(
                    f"{self.config.pocketnode_url}/api/sdk/register-agents",
                    headers={"Authorization": f"Bearer {auth_token}"},
                    json={
                        "workspace": self.config.workspace,
                        "channels": self.config.channels,  # Include channels for subscription
                        "handles": handles
                    },
                    timeout=30.0
                )

                if response.status_code == 401:
                    raise Exception("Authentication failed - invalid or expired token")

                response.raise_for_status()
                data = response.json()

                if not data.get("success"):
                    raise Exception(data.get("message", "Registration failed"))

                # Update config with server info
                self.config.server = data["server"]

                # Create agent configs from response
                for agent_data in data["agents"]:
                    handle = agent_data["handle"]
                    self.config.agents[handle] = AgentConfig(
                        handle=handle,
                        bot_login=agent_data["bot_login"],
                        bot_password=agent_data["bot_password"]
                    )
                    logger.info(f"  Registered @{handle}")

            except httpx.HTTPError as e:
                logger.error(f"Failed to register agents: {e}")
                raise

        # Create runners for registered agents
        self._setup_from_config()

    def _setup_from_config(self) -> None:
        """Create runners that bridge to ADK web server"""
        for agent in self.agents:
            if agent.handle not in self.config.agents:
                logger.warning(
                    f"Agent @{agent.handle} not registered - skipping"
                )
                continue

            agent_config = self.config.agents[agent.handle]
            runner = AgentRunner(
                agent=agent,
                config=agent_config,
                server=self.config.server,
                api_key="AQEAAAABAAD_rAp4DJh05a1HAwFT3A6K",  # Default API key
                workspace_id=self.config.workspace,
                adk_server_url=self.adk_server_url
            )
            self.runners.append(runner)

    async def run(self) -> None:
        """Connect all agents and run until interrupted"""
        if not self.runners:
            logger.error("No agents to run. Did you call discover()?")
            return

        # Connect all agents
        logger.info(f"Starting {len(self.runners)} agent(s)...")
        await asyncio.gather(*[r.start() for r in self.runners])

        logger.info("All agents connected. Listening for messages...")
        logger.info("Press Ctrl+C to stop")

        # Run all message loops
        await asyncio.gather(*[r.run() for r in self.runners])

    async def stop(self) -> None:
        """Disconnect all agents"""
        for runner in self.runners:
            await runner.client.disconnect()
