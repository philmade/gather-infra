# GatherSDK

Connect Google ADK agents to [Gather.is](https://gather.is) workspaces.

## Installation

```bash
pip install gathersdk google-adk
```

## Quick Start

### 1. Initialize your project

```bash
mkdir my-agents && cd my-agents
python -m venv .venv && source .venv/bin/activate
pip install gathersdk google-adk
gathersdk init --name hello_agent
```

### 2. Get your config

1. Go to [app.gather.is](https://app.gather.is)
2. Create or open a workspace
3. Click workspace dropdown → **SDK Settings**
4. Download `gather.config.json` and replace the placeholder

### 3. Set your API key

```bash
cp .env.example .env
# Edit .env and add your Google API key from https://aistudio.google.com/apikey
```

### 4. Run

```bash
source .env && gathersdk serve
```

The SDK automatically starts `adk web` if not already running.

### 5. Chat with your agent

Go to [app.gather.is](https://app.gather.is) and type `@hello_agent hello!`

## Commands

```bash
# Initialize a new agent project
gathersdk init --name my_agent

# Start the SDK (auto-starts ADK web server)
gathersdk serve

# Start without auto-starting ADK
gathersdk serve --no-auto-adk

# Discover agents without connecting
gathersdk discover

# Verbose logging
gathersdk -v serve
```

## Project Structure

```
my-agents/
├── gather.config.json   # From app.gather.is SDK Settings
├── .env                 # GOOGLE_API_KEY=your_key
└── hello_agent/
    ├── __init__.py      # from .agent import root_agent
    └── agent.py         # root_agent = Agent(...)
```

## Agent Code

```python
# hello_agent/agent.py
from google.adk import Agent

root_agent = Agent(
    name="hello_agent",
    model="gemini-2.5-flash",
    instruction="You are a helpful assistant."
)
```

```python
# hello_agent/__init__.py
from .agent import root_agent
```

## Debugging

The ADK debug UI is available at http://localhost:8000 when running. View:
- Session history and state
- Message flow and events
- Tool calls and responses

## Links

- **App**: https://app.gather.is
- **Docs**: https://app.gather.is/docs
- **API Keys**: https://aistudio.google.com/apikey
