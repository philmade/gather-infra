"""
Agency SDK CLI - Bridge ADK agents to Gather.is workspaces

Commands:
    gathersdk init       Initialize a new agent project
    gathersdk serve      Start the SDK bridge (auto-starts ADK web if needed)
    gathersdk discover   List agents found in directory

The serve command automatically starts 'adk web' if not already running.
Debug your agents at http://localhost:8000
"""

import asyncio
import logging
import signal
import subprocess
import sys
from pathlib import Path

import click
import httpx

from .sdk import AgencySDK
from .discovery import discover_agents


# Global reference to ADK subprocess so we can clean it up
_adk_process: subprocess.Popen | None = None


@click.group()
@click.option("--verbose", "-v", is_flag=True, help="Enable debug logging")
def cli(verbose: bool):
    """Agency SDK - Connect ADK agents to Tinode workspaces"""
    level = logging.DEBUG if verbose else logging.INFO
    logging.basicConfig(
        level=level,
        format="%(asctime)s [%(levelname)s] %(name)s: %(message)s",
        datefmt="%H:%M:%S"
    )


async def _check_adk_running(url: str) -> bool:
    """Check if ADK web is running at the given URL.

    Returns True only if the /list-apps endpoint responds successfully,
    confirming it's actually ADK web and not some other service on the port.
    """
    try:
        async with httpx.AsyncClient(timeout=2.0) as client:
            resp = await client.get(f"{url}/list-apps")
            return resp.status_code == 200
    except (httpx.ConnectError, httpx.TimeoutException):
        return False


def _start_adk_web(port: int = 8000) -> subprocess.Popen:
    """Start ADK web server as a subprocess."""
    global _adk_process

    click.echo(click.style("\n  Starting ADK web server...", fg="yellow"))

    # Start adk web in the background
    # Redirect stdout/stderr to DEVNULL to keep our output clean
    _adk_process = subprocess.Popen(
        ["adk", "web", "--port", str(port)],
        stdout=subprocess.DEVNULL,
        stderr=subprocess.DEVNULL,
    )

    return _adk_process


def _cleanup_adk_process():
    """Cleanup ADK subprocess on exit."""
    global _adk_process
    if _adk_process is not None:
        click.echo("\n  Stopping ADK web server...")
        _adk_process.terminate()
        try:
            _adk_process.wait(timeout=5)
        except subprocess.TimeoutExpired:
            _adk_process.kill()
        _adk_process = None


@cli.command()
@click.option(
    "--config", "-c",
    default="gather.config.json",
    help="Path to config file"
)
@click.option(
    "--dir", "-d",
    default=".",
    help="Directory to search for agents"
)
@click.option(
    "--adk-url",
    default="http://localhost:8000",
    help="ADK web server URL"
)
@click.option(
    "--no-auto-adk",
    is_flag=True,
    help="Don't auto-start ADK web if not running"
)
def serve(config: str, dir: str, adk_url: str, no_auto_adk: bool):
    """
    Start the SDK bridge connecting your agents to Gather.is.

    Automatically starts 'adk web' if not already running on port 8000.
    Debug your agents at http://localhost:8000
    """
    config_path = Path(config)

    if not config_path.exists():
        click.echo(f"Config file not found: {config}", err=True)
        click.echo("\nTo get a config file:")
        click.echo("  1. Go to your workspace settings in the web UI")
        click.echo("  2. Click 'SDK Access'")
        click.echo("  3. Download gather.config.json")
        sys.exit(1)

    try:
        sdk = AgencySDK.from_config(config_path, adk_server_url=adk_url)
    except Exception as e:
        click.echo(f"Failed to load config: {e}", err=True)
        sys.exit(1)

    # Run the async discover and serve
    try:
        asyncio.run(_serve_async(sdk, dir, adk_url, auto_start_adk=not no_auto_adk))
    except KeyboardInterrupt:
        click.echo("\nShutting down...")
    finally:
        # Clean up ADK subprocess if we started it
        _cleanup_adk_process()


async def _serve_async(sdk: AgencySDK, dir: str, adk_url: str, auto_start_adk: bool = True):
    """Async portion of serve command"""
    from .discovery import discover_agents

    # First discover agents locally
    agents = discover_agents(dir)

    if not agents:
        raise click.ClickException(
            f"No agents found!\n\n"
            f"Searched in: {Path(dir).resolve()}\n\n"
            f"Expected structure:\n"
            f"  agents/\n"
            f"      my_agent/\n"
            f"          __init__.py  # from .agent import root_agent\n"
            f"          agent.py     # root_agent = Agent(...)"
        )

    click.echo(f"Found {len(agents)} agent(s): {', '.join(f'@{a.handle}' for a in agents)}")

    # Check ADK web server and auto-start if needed
    click.echo(f"\nChecking ADK server at {adk_url}...")

    adk_running = await _check_adk_running(adk_url)
    adk_started_by_us = False

    if adk_running:
        click.echo(click.style("  ADK server is running", fg="green"))
    elif auto_start_adk:
        # Extract port from URL for starting ADK
        from urllib.parse import urlparse
        parsed = urlparse(adk_url)
        port = parsed.port or 8000

        _start_adk_web(port)
        adk_started_by_us = True

        # Wait for ADK to be ready (up to 10 seconds)
        click.echo("  Waiting for ADK server to be ready...")
        for i in range(20):
            await asyncio.sleep(0.5)
            if await _check_adk_running(adk_url):
                click.echo(click.style("  ADK server started successfully", fg="green"))
                break
        else:
            click.echo(click.style("  WARNING: ADK server didn't start in time", fg="red"))
            click.echo("  Try running 'adk web' manually in another terminal")
    else:
        click.echo(click.style("  WARNING: ADK server not reachable!", fg="red"))
        click.echo()
        click.echo("  Start the ADK web server in another terminal:")
        click.echo(click.style(f"    adk web --port 8000", fg="cyan"))
        click.echo()
        click.echo("  Or remove --no-auto-adk to start it automatically")
        click.echo()

    # Register with PocketNode
    click.echo(f"\nRegistering with {sdk.config.pocketnode_url}...")
    await sdk.discover(dir)

    if not sdk.runners:
        raise click.ClickException(
            "No agents could be registered!\n"
            "Check that your SDK token is valid."
        )

    # Run
    click.echo(f"\nConnecting to {sdk.config.server}")
    click.echo(f"Workspace: {sdk.config.workspace}")
    click.echo(f"Agents: {', '.join(f'@{r.agent.handle}' for r in sdk.runners)}")
    click.echo()

    # Show ADK status
    if adk_started_by_us:
        click.echo(click.style("ADK web server: ", fg="white") + click.style("started automatically", fg="yellow"))
    else:
        click.echo(click.style("ADK web server: ", fg="white") + click.style("already running", fg="green"))

    click.echo(click.style("Debug UI: ", fg="white") + click.style(adk_url, fg="cyan"))
    click.echo()
    click.echo("Press Ctrl+C to stop")
    click.echo()

    await sdk.run()


@cli.command()
@click.option(
    "--dir", "-d",
    default=".",
    help="Directory to search for agents"
)
def discover(dir: str):
    """Discover agents in a directory (without connecting)"""
    agents = discover_agents(dir)

    if not agents:
        click.echo("No agents found.")
        click.echo(f"\nSearched in: {Path(dir).resolve()}")
        return

    click.echo(f"Found {len(agents)} agent(s):\n")
    for agent in agents:
        click.echo(f"  @{agent.handle}")
        click.echo(f"    Path: {agent.path}")
        click.echo(f"    Agent: {type(agent.root_agent).__name__}")
        click.echo()


@cli.command()
@click.option(
    "--name", "-n",
    required=True,
    help="Name for your agent (e.g., my_agent)"
)
def init(name: str):
    """
    Initialize a new Gather.is agent project.

    Creates the agent folder structure, config placeholder, and .env example.
    """
    import json
    import re

    # Validate agent name
    if not re.match(r'^[a-z][a-z0-9_]*$', name):
        click.echo(click.style("Error: ", fg="red") + "Agent name must be lowercase, start with a letter, and contain only letters, numbers, and underscores.")
        click.echo(f"  Example: my_agent, support_bot, billing_helper")
        sys.exit(1)

    click.echo(click.style("Creating Gather.is agent project...", fg="cyan"))
    click.echo()

    # Create agent directory
    agent_dir = Path(name)
    if agent_dir.exists():
        click.echo(click.style(f"Error: ", fg="red") + f"Directory '{name}' already exists.")
        sys.exit(1)

    agent_dir.mkdir()

    # Create __init__.py
    init_file = agent_dir / "__init__.py"
    init_file.write_text("from .agent import root_agent\n")
    click.echo(f"  Created {name}/__init__.py")

    # Create agent.py
    agent_file = agent_dir / "agent.py"
    agent_code = f'''from google.adk.agents import Agent

root_agent = Agent(
    name="{name}",
    model="gemini-2.5-flash",
    description="A helpful assistant agent",
    instruction="""You are a helpful assistant.

Answer questions clearly and concisely.
Be friendly and professional.
""",
)
'''
    agent_file.write_text(agent_code)
    click.echo(f"  Created {name}/agent.py")

    # Create placeholder gather.config.json if it doesn't exist
    config_file = Path("gather.config.json")
    if not config_file.exists():
        placeholder_config = {
            "_comment": "REPLACE THIS FILE - Download from app.gather.is → Workspace → SDK Settings",
            "pocketnode_url": "https://app.gather.is",
            "workspace": "YOUR_WORKSPACE_ID",
            "auth_token": "YOUR_AUTH_TOKEN"
        }
        config_file.write_text(json.dumps(placeholder_config, indent=2) + "\n")
        click.echo(f"  Created gather.config.json (placeholder)")

    # Create .env.example if it doesn't exist
    env_example = Path(".env.example")
    if not env_example.exists():
        env_example.write_text("GOOGLE_API_KEY=your_google_api_key_here\n")
        click.echo(f"  Created .env.example")

    click.echo()
    click.echo(click.style("Project created!", fg="green"))
    click.echo()
    click.echo("Next steps:")
    click.echo()
    click.echo("  1. Get your config file:")
    click.echo(click.style("     → Go to https://app.gather.is", fg="cyan"))
    click.echo(click.style("     → Workspace dropdown → SDK Settings", fg="cyan"))
    click.echo(click.style("     → Download and replace gather.config.json", fg="cyan"))
    click.echo()
    click.echo("  2. Set up your API key:")
    click.echo(click.style("     cp .env.example .env", fg="cyan"))
    click.echo(click.style("     # Edit .env with your key from https://aistudio.google.com/apikey", fg="cyan"))
    click.echo()
    click.echo("  3. Run your agent:")
    click.echo(click.style("     source .env && gathersdk serve", fg="cyan"))
    click.echo()
    click.echo("  4. Chat with your agent:")
    click.echo(click.style(f"     → Go to app.gather.is and type: @{name} hello!", fg="cyan"))
    click.echo()


def main():
    cli()


if __name__ == "__main__":
    main()
