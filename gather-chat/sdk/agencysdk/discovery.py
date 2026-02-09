"""
Agent discovery - finds ADK agents in the local directory tree

Follows the Google ADK pattern:
  parent_folder/
      agent_one/
          __init__.py   # Must contain: from .agent import root_agent
          agent.py      # Must define: root_agent = Agent(...)
      agent_two/
          __init__.py
          agent.py
"""

import importlib.util
import logging
import sys
from pathlib import Path
from typing import Any, Optional
from dataclasses import dataclass

logger = logging.getLogger(__name__)


@dataclass
class DiscoveredAgent:
    """An agent discovered in the local directory"""
    name: str           # Folder name (used as handle)
    path: Path          # Path to agent folder
    root_agent: Any     # The imported root_agent object

    @property
    def handle(self) -> str:
        """The @mention handle for this agent"""
        return self.name


def discover_agents(root_dir: str | Path = ".") -> list[DiscoveredAgent]:
    """
    Discover all ADK agents in the given directory.

    Looks for folders containing __init__.py that exports 'root_agent'.

    Args:
        root_dir: Directory to search (default: current directory)

    Returns:
        List of discovered agents
    """
    root_path = Path(root_dir).resolve()
    agents = []

    logger.info(f"Discovering agents in {root_path}")

    # Look for immediate subdirectories with __init__.py
    for folder in root_path.iterdir():
        if not folder.is_dir():
            continue

        # Skip hidden folders and common non-agent directories
        if folder.name.startswith(".") or folder.name in (
            "__pycache__", "node_modules", "venv", ".venv", "env", ".git"
        ):
            continue

        init_file = folder / "__init__.py"
        agent_file = folder / "agent.py"

        # Must have __init__.py
        if not init_file.exists():
            continue

        # Try to import and find root_agent
        agent = _try_import_agent(folder)
        if agent:
            agents.append(agent)
            logger.info(f"  Found agent: @{agent.handle} ({folder.name}/)")

    logger.info(f"Discovered {len(agents)} agent(s)")
    return agents


def _try_import_agent(folder: Path) -> Optional[DiscoveredAgent]:
    """
    Try to import an agent from a folder.

    Looks for root_agent export in __init__.py
    """
    module_name = f"_agency_agent_{folder.name}"

    try:
        # Add parent to path so relative imports work
        parent = str(folder.parent)
        if parent not in sys.path:
            sys.path.insert(0, parent)

        # Import the package
        spec = importlib.util.spec_from_file_location(
            module_name,
            folder / "__init__.py",
            submodule_search_locations=[str(folder)]
        )

        if spec is None or spec.loader is None:
            return None

        module = importlib.util.module_from_spec(spec)
        sys.modules[module_name] = module
        spec.loader.exec_module(module)

        # Look for root_agent
        root_agent = getattr(module, "root_agent", None)
        if root_agent is None:
            logger.debug(f"  {folder.name}/: no root_agent export")
            return None

        return DiscoveredAgent(
            name=folder.name,
            path=folder,
            root_agent=root_agent
        )

    except Exception as e:
        logger.warning(f"  {folder.name}/: failed to import - {e}")
        return None


def validate_agent(agent: DiscoveredAgent) -> bool:
    """
    Validate that a discovered agent has required methods.

    ADK agents should have an invoke or run method.
    """
    root = agent.root_agent

    # Check for common ADK agent interfaces
    if hasattr(root, "invoke"):
        return True
    if hasattr(root, "run"):
        return True
    if hasattr(root, "__call__"):
        return True

    logger.warning(
        f"Agent @{agent.handle} has no invoke/run method - may not work correctly"
    )
    return False
