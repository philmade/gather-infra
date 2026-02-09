"""
Tinode WebSocket client for Agency SDK

Handles the low-level WebSocket communication with Tinode:
    - Authentication (basic auth with bot credentials)
    - Topic subscription (workspace channels, DMs)
    - Message receiving and sending
    - Deduplication of messages (handles reconnect history)

Each agent bot gets its own TinodeClient instance with unique credentials.
"""

import asyncio
import base64
import json
import logging
from typing import Callable, Optional, Any
from dataclasses import dataclass, field

import websockets
from websockets.client import WebSocketClientProtocol

logger = logging.getLogger(__name__)


@dataclass
class Message:
    """Represents a chat message from Tinode"""
    topic: str
    from_user: str
    content: str
    seq: int
    mentions: list[str] = field(default_factory=list)
    is_dm: bool = False

    @classmethod
    def from_data(cls, topic: str, data: dict) -> "Message":
        """Parse message from Tinode {data} packet"""
        content = data.get("content", "")

        # Extract mentions from message format
        # Tinode uses fmt array with mention references
        mentions = []
        if "fmt" in data:
            for fmt in data.get("fmt", []):
                if fmt.get("tp") == "MN":  # Mention type
                    ref = fmt.get("key")
                    if ref:
                        mentions.append(ref)

        # Check if this is a P2P (DM) topic
        # DMs can be either:
        # - p2p... topics (traditional P2P)
        # - usr... topics (when subscribed via user ID)
        is_dm = topic.startswith("p2p") or topic.startswith("usr")

        return cls(
            topic=topic,
            from_user=data.get("from", ""),
            content=content if isinstance(content, str) else str(content),
            seq=data.get("seq", 0),
            mentions=mentions,
            is_dm=is_dm
        )


class TinodeClient:
    """
    WebSocket client for connecting to Tinode as a bot user.

    Each bot agent gets its own client instance with its own credentials.
    """

    def __init__(
        self,
        server_url: str,
        login: str,
        password: str,
        api_key: str = "AQEAAAABAAD_rAp4DJh05a1HAwFT3A6K"
    ):
        self.server_url = server_url
        self.login = login
        self.password = password
        self.api_key = api_key

        self.ws: Optional[WebSocketClientProtocol] = None
        self.user_id: Optional[str] = None
        self.msg_id = 0

        # Callbacks
        self.on_message: Optional[Callable[[Message], None]] = None
        self.on_connected: Optional[Callable[[], None]] = None
        self.on_disconnected: Optional[Callable[[], None]] = None

        # Subscribed topics
        self.subscriptions: set[str] = set()

        # Pending requests (for request-response pattern)
        self._pending: dict[str, asyncio.Future] = {}

        # Track processed messages to avoid duplicates
        self._processed_messages: set[str] = set()

    def _next_id(self) -> str:
        """Generate next message ID"""
        self.msg_id += 1
        return str(self.msg_id)

    async def connect(self) -> None:
        """Connect to Tinode server"""
        # Add API key to URL for authentication
        url = self.server_url
        if "?" in url:
            url = f"{url}&apikey={self.api_key}"
        else:
            url = f"{url}?apikey={self.api_key}"

        logger.info(f"Connecting to {self.server_url}")
        self.ws = await websockets.connect(url)

        # Send hello
        await self._hello()

        # Login
        await self._login()

        if self.on_connected:
            self.on_connected()

        logger.info(f"Connected as {self.user_id}")

    async def _hello(self) -> dict:
        """Send hello handshake"""
        msg_id = self._next_id()
        await self._send({
            "hi": {
                "id": msg_id,
                "ver": "0.22",
                "ua": "AgencySDK/0.1.0",
                "lang": "en"
            }
        })
        return await self._wait_for_ctrl(msg_id)

    async def _login(self) -> dict:
        """Authenticate with basic auth"""
        msg_id = self._next_id()
        # Tinode expects base64-encoded secret for basic auth
        secret = f"{self.login}:{self.password}"
        secret_b64 = base64.b64encode(secret.encode()).decode()

        await self._send({
            "login": {
                "id": msg_id,
                "scheme": "basic",
                "secret": secret_b64
            }
        })

        result = await self._wait_for_ctrl(msg_id)
        self.user_id = result.get("params", {}).get("user")
        return result

    async def subscribe(self, topic: str, get_recent: bool = True) -> dict:
        """Subscribe to a topic

        Args:
            topic: The topic to subscribe to
            get_recent: If True, request recent messages (useful for P2P topics)
        """
        msg_id = self._next_id()

        # For P2P topics (DMs), request recent messages to catch up on any missed messages
        get_what = "desc sub data"
        if topic.startswith("p2p") and get_recent:
            # Request last 10 messages for DMs to catch any we missed
            get_params = {
                "what": "desc sub data",
                "data": {"limit": 10}
            }
        else:
            get_params = {"what": get_what}

        await self._send({
            "sub": {
                "id": msg_id,
                "topic": topic,
                "get": get_params
            }
        })

        result = await self._wait_for_ctrl(msg_id)
        self.subscriptions.add(topic)
        logger.info(f"Subscribed to {topic}")
        return result

    async def subscribe_to_me(self) -> list[str]:
        """
        Subscribe to 'me' topic to get list of subscriptions,
        then subscribe to all those topics to receive messages.
        Returns list of topics subscribed to.
        """
        # Collect topics from meta messages
        self._pending_subs = []

        msg_id = self._next_id()
        await self._send({
            "sub": {
                "id": msg_id,
                "topic": "me",
                "get": {
                    "what": "sub"
                }
            }
        })

        await self._wait_for_ctrl(msg_id)

        # Receive additional messages (meta with subscriptions list)
        # Keep receiving until we get a meta message with subs
        for _ in range(10):  # Max 10 iterations
            try:
                raw = await asyncio.wait_for(self.ws.recv(), timeout=0.5)
                await self._handle_message(raw)
                if self._pending_subs:
                    break  # Got subscriptions
            except asyncio.TimeoutError:
                break

        # Now subscribe to each topic we're a member of
        topics = self._pending_subs.copy()
        self._pending_subs = []

        logger.info(f"Found {len(topics)} topic(s) to subscribe to")

        for topic in topics:
            if topic.startswith("grp") or topic.startswith("p2p"):
                try:
                    await self.subscribe(topic)
                except Exception as e:
                    logger.warning(f"Failed to subscribe to {topic}: {e}")

        return topics

    async def discover_and_subscribe_channels(self, workspace_id: str) -> list[str]:
        """
        Discover channels in a workspace and subscribe to them.

        Channels are group topics with public.type = "channel" and
        public.parent = workspace_id.

        Returns list of channel topic IDs subscribed to.
        """
        channels = []

        # Subscribe to workspace to get its subscriber list (which includes channels)
        msg_id = self._next_id()
        await self._send({
            "sub": {
                "id": msg_id,
                "topic": workspace_id,
                "get": {
                    "what": "sub desc"
                }
            }
        })

        try:
            await self._wait_for_ctrl(msg_id)
        except Exception as e:
            logger.warning(f"Failed to subscribe to workspace: {e}")
            return channels

        # Now query for channel topics using fnd
        # Search for topics tagged with parent:<workspace_id>
        fnd_msg_id = self._next_id()
        await self._send({
            "sub": {
                "id": fnd_msg_id,
                "topic": "fnd"
            }
        })

        try:
            await self._wait_for_ctrl(fnd_msg_id)
        except Exception as e:
            logger.debug(f"fnd subscribe: {e}")

        # Set search query
        search_query = f"parent:{workspace_id}"
        set_msg_id = self._next_id()
        await self._send({
            "set": {
                "id": set_msg_id,
                "topic": "fnd",
                "desc": {
                    "public": search_query
                }
            }
        })

        try:
            await self._wait_for_ctrl(set_msg_id)
        except Exception as e:
            logger.debug(f"fnd set: {e}")

        # Get search results
        self._fnd_results = []
        get_msg_id = self._next_id()
        await self._send({
            "get": {
                "id": get_msg_id,
                "topic": "fnd",
                "what": "sub"
            }
        })

        # Collect results
        for _ in range(10):
            try:
                raw = await asyncio.wait_for(self.ws.recv(), timeout=0.5)
                await self._handle_message(raw)
            except asyncio.TimeoutError:
                break

        channels = self._fnd_results.copy() if hasattr(self, '_fnd_results') else []
        self._fnd_results = []

        logger.info(f"Found {len(channels)} channel(s) in workspace")

        # Subscribe to each channel
        for channel in channels:
            try:
                await self.subscribe(channel)
                logger.info(f"Subscribed to channel {channel}")
            except Exception as e:
                logger.warning(f"Failed to subscribe to channel {channel}: {e}")

        # Leave fnd topic
        leave_msg_id = self._next_id()
        await self._send({
            "leave": {
                "id": leave_msg_id,
                "topic": "fnd"
            }
        })

        return channels

    async def send_agent_ready(self, workspace_id: str, handle: str, name: str) -> None:
        """
        Send agent:ready event to notify the system this agent is online.

        PocketNode's bridge will process this event and ensure the agent
        is properly set up in the workspace.
        """
        event = {
            "type": "agent:ready",
            "handle": handle,
            "name": name,
            "workspace": workspace_id,
            "bot_uid": self.user_id
        }

        # Publish to workspace topic
        await self.publish(workspace_id, json.dumps(event))
        logger.info(f"Sent agent:ready event for @{handle}")

    async def update_profile(self, name: str, handle: str) -> None:
        """
        Update bot's public profile with name and handle.
        """
        msg_id = self._next_id()
        await self._send({
            "set": {
                "id": msg_id,
                "topic": "me",
                "desc": {
                    "public": {
                        "fn": name,
                        "bot": True,
                        "handle": handle
                    }
                }
            }
        })

        try:
            await self._wait_for_ctrl(msg_id)
            logger.info(f"Updated profile: name={name}, handle={handle}")
        except Exception as e:
            logger.warning(f"Failed to update profile: {e}")

    async def publish(self, topic: str, content: str, wait_for_ack: bool = False) -> dict:
        """Publish a message to a topic

        Args:
            topic: The topic to publish to
            content: The message content
            wait_for_ack: If True, wait for server acknowledgment (may cause issues if called from message handler)
        """
        msg_id = self._next_id()
        await self._send({
            "pub": {
                "id": msg_id,
                "topic": topic,
                "noecho": True,
                "content": content
            }
        })
        if wait_for_ack:
            return await self._wait_for_ctrl(msg_id)
        return {"code": 202, "text": "Accepted (no ack)"}

    async def publish_formatted(
        self,
        topic: str,
        text: str,
        fmt: Optional[list] = None,
        ent: Optional[list] = None
    ) -> dict:
        """Publish a formatted message (with mentions, links, etc.)"""
        msg_id = self._next_id()
        msg = {
            "pub": {
                "id": msg_id,
                "topic": topic,
                "noecho": True,
                "content": text
            }
        }

        if fmt:
            msg["pub"]["head"] = {"mime": "text/x-drafty"}
            msg["pub"]["content"] = {"txt": text, "fmt": fmt, "ent": ent or []}

        await self._send(msg)
        return await self._wait_for_ctrl(msg_id)

    async def _send(self, msg: dict) -> None:
        """Send a message to the server"""
        if self.ws is None:
            raise RuntimeError("Not connected")
        data = json.dumps(msg)
        logger.debug(f">>> {data}")
        await self.ws.send(data)

    async def _wait_for_ctrl(self, msg_id: str, timeout: float = 10.0) -> dict:
        """Wait for a ctrl response with matching ID"""
        future = asyncio.get_event_loop().create_future()
        self._pending[msg_id] = future

        async def receive_until_resolved():
            while not future.done():
                try:
                    raw = await asyncio.wait_for(self.ws.recv(), timeout=1.0)
                    await self._handle_message(raw)
                except asyncio.TimeoutError:
                    continue

        try:
            # Start receiving messages until we get our response
            receive_task = asyncio.create_task(receive_until_resolved())
            result = await asyncio.wait_for(future, timeout)
            receive_task.cancel()
            return result
        except asyncio.TimeoutError:
            raise Exception(f"Timeout waiting for response to message {msg_id}")
        finally:
            self._pending.pop(msg_id, None)

    async def _handle_message(self, raw: str) -> None:
        """Handle incoming WebSocket message"""
        logger.debug(f"<<< {raw}")
        msg = json.loads(raw)

        # Control message (response to our requests)
        if "ctrl" in msg:
            ctrl = msg["ctrl"]
            msg_id = ctrl.get("id")
            if msg_id and msg_id in self._pending:
                future = self._pending[msg_id]
                if ctrl.get("code", 0) >= 400:
                    future.set_exception(
                        Exception(f"Error {ctrl.get('code')}: {ctrl.get('text')}")
                    )
                else:
                    future.set_result(ctrl)

        # Data message (chat message)
        elif "data" in msg:
            data = msg["data"]
            topic = data.get("topic", "")
            seq = data.get("seq", 0)
            from_user = data.get("from", "")

            logger.debug(f"Data message: topic={topic}, from={from_user}, seq={seq}")

            # Don't process our own messages
            if from_user == self.user_id:
                logger.debug(f"Ignoring own message in {topic}")
                return

            # Deduplicate: track by topic + seq number
            msg_key = f"{topic}:{seq}"
            if msg_key in self._processed_messages:
                logger.debug(f"Skipping duplicate message: {msg_key}")
                return
            self._processed_messages.add(msg_key)

            # Limit memory - keep only last 1000 message keys
            if len(self._processed_messages) > 1000:
                # Remove oldest entries (set doesn't maintain order, so clear half)
                to_remove = list(self._processed_messages)[:500]
                for key in to_remove:
                    self._processed_messages.discard(key)

            message = Message.from_data(topic, data)
            logger.debug(f"Processing: {message.topic} is_dm={message.is_dm}")

            if self.on_message:
                try:
                    self.on_message(message)
                except Exception as e:
                    logger.error(f"Error in message handler: {e}")

        # Meta message (topic info, subscriptions)
        elif "meta" in msg:
            meta = msg["meta"]
            # Collect subscriptions from 'me' topic
            if meta.get("topic") == "me" and "sub" in meta:
                for sub in meta["sub"]:
                    topic = sub.get("topic")
                    if topic and hasattr(self, '_pending_subs'):
                        self._pending_subs.append(topic)
                        logger.debug(f"Found subscription: {topic}")

            # Collect fnd search results
            if meta.get("topic") == "fnd" and "sub" in meta:
                if not hasattr(self, '_fnd_results'):
                    self._fnd_results = []
                for sub in meta["sub"]:
                    topic = sub.get("topic")
                    if topic:
                        self._fnd_results.append(topic)
                        logger.debug(f"Found channel: {topic}")

        # Presence message
        elif "pres" in msg:
            pres = msg["pres"]
            await self._handle_presence(pres)

    async def _handle_presence(self, pres: dict) -> None:
        """Handle presence notifications"""
        topic = pres.get("topic", "")
        what = pres.get("what", "")
        src = pres.get("src", "")  # The topic or user this is about

        logger.debug(f"Presence: topic={topic}, what={what}, src={src}")

        # Handle DM notifications on 'me' topic
        # When someone sends a DM, we get: topic='me', what='msg', src=<user_id>
        # We need to subscribe to the user ID - Tinode will resolve it to the P2P topic
        if topic == "me" and what == "msg" and src:
            # src is a user ID (e.g., "usr-L1uZzRJrx4"), not a p2p topic
            # Subscribing to a user ID will get us the P2P topic
            if src.startswith("usr") and src != self.user_id:
                if not hasattr(self, '_pending_dm_subscriptions'):
                    self._pending_dm_subscriptions = set()
                if src not in self._pending_dm_subscriptions:
                    self._pending_dm_subscriptions.add(src)
                    logger.info(f"Queued DM subscription to user: {src}")

        # Also handle P2P topics directly if we somehow get them
        if topic == "me":
            if src and src.startswith("p2p") and src not in self.subscriptions:
                if not hasattr(self, '_pending_dm_subscriptions'):
                    self._pending_dm_subscriptions = set()
                self._pending_dm_subscriptions.add(src)
                logger.info(f"Queued DM for subscription: {src}")

    async def subscribe_nonblocking(self, topic: str) -> None:
        """
        Subscribe to a topic without waiting for response.
        Used during message handling to avoid WebSocket concurrency issues.
        The ctrl response will be processed by the main message loop.
        """
        if topic in self.subscriptions:
            return

        msg_id = self._next_id()

        # For P2P topics, request recent messages
        if topic.startswith("p2p"):
            get_params = {
                "what": "desc sub data",
                "data": {"limit": 10}
            }
        else:
            get_params = {"what": "desc sub data"}

        await self._send({
            "sub": {
                "id": msg_id,
                "topic": topic,
                "get": get_params
            }
        })

        # Mark as subscribed immediately (optimistic)
        self.subscriptions.add(topic)
        logger.debug(f"Sent non-blocking subscription for {topic}")

    async def process_pending_dm_subscriptions(self) -> None:
        """Process any pending DM subscriptions using non-blocking subscribe

        Handles both:
        - User IDs (e.g., "usr-L1uZzRJrx4") - Tinode resolves to P2P topic
        - P2P topics directly (e.g., "p2pABC123")
        """
        if not hasattr(self, '_pending_dm_subscriptions') or not self._pending_dm_subscriptions:
            return

        to_subscribe = list(self._pending_dm_subscriptions)
        self._pending_dm_subscriptions.clear()

        for target in to_subscribe:
            # Skip if we're already subscribed (for user IDs, check both the ID and potential P2P topics)
            if target in self.subscriptions:
                continue

            try:
                logger.info(f"Auto-subscribing to DM: {target}")
                await self.subscribe_nonblocking(target)
            except Exception as e:
                logger.warning(f"Failed to send subscription for DM {target}: {e}")

    async def run(self) -> None:
        """Main message loop - run until disconnected"""
        if self.ws is None:
            raise RuntimeError("Not connected")

        try:
            async for raw in self.ws:
                await self._handle_message(raw)
                # Process any pending DM subscriptions after each message
                await self.process_pending_dm_subscriptions()
        except websockets.ConnectionClosed:
            logger.info("Connection closed")
            if self.on_disconnected:
                self.on_disconnected()

    async def disconnect(self) -> None:
        """Disconnect from server"""
        if self.ws:
            await self.ws.close()
            self.ws = None
