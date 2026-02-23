# Gather-Claw

## What This Is

Per-user agent containers running ClawPoint-Go — a Google ADK-based multi-agent orchestrator with self-healing, persistent memory, Telegram messaging (via Matterbridge), Starlark scripting, and web research. Each claw is an Alpine container exposing an ADK API on port 8080.

## Architecture

- **Runtime**: Alpine 3.19 (no GUI, no desktop)
- **Agent**: ClawPoint-Go v0.655 (ADK multi-agent orchestrator, Core/Extensions architecture)
- **Messaging**: Matterbridge (Telegram <-> ADK bridge)
- **Supervisor**: clawpoint-medic (self-healing watchdog with hot-swap + rollback)
- **LLM backend**: Anthropic-compatible (z.ai GLM by default)
- **Extensions**: Starlark (.star) scripts — embedded Python dialect, no recompilation needed
- **Identity**: Ed25519 keypair per claw (Gather agent identity)
- **Routing**: Traefik (Docker labels, TLS via Let's Encrypt)

## Container Services

| Process | Purpose |
|---------|---------|
| `clawpoint-go` | ADK multi-agent orchestrator (port 8081 internal) |
| `clawpoint-proxy` | Public-facing proxy (port 8080 → ADK + static files) |
| `clawpoint-medic` | Supervisor/watchdog with hot-swap + rollback (PID 1) |
| `clawpoint-bridge` | Matterbridge <-> ADK connector |
| `matterbridge` | Telegram bot <-> local API bridge |

## Core/Extensions Architecture

The agent codebase is split into two parts:

- **core/** — Versioned infrastructure (orchestrator, tools, agents, connectors). Operator-managed, agent reads but does not modify.
- **extensions/** — Agent-writable Starlark scripts in `/app/data/extensions/`. Persist across restarts on the data volume.

The agent has 2 sub-agents: **claude** (Claude Code CLI — coding, files, bash) and **research** (Chawan browser — web research).

The coordinator has direct tools: **memory** (SQLite), **soul** (identity files), **tasks** (structured task management), **extension_list**, **extension_run** (Starlark), **build_and_deploy** (Go recompilation via external build service), **platform** tools (Gather.is identity).

### Starlark Extensions

Agents can create new capabilities by writing `.star` files:
```python
# DESCRIPTION: Fetch and summarize a URL
def run(args):
    url = args.get("url", "https://gather.is")
    content = http_get(url)
    return "Fetched " + str(len(content)) + " bytes from " + url
```

Available builtins: `http_get(url)`, `http_post(url, body, type)`, `read_file(path)`, `write_file(path, content)`, `log(msg)`.

## Container Filesystem Layout

```
/app/
├── clawpoint-go          # Main agent binary
├── clawpoint-medic       # Supervisor binary
├── clawpoint-bridge      # Matterbridge connector binary
├── clawpoint-proxy       # Public proxy binary
├── core-version          # Version string (e.g. "0.655")
├── src/                  # Full Go source code (read-only, agent can inspect)
│   ├── core/             # Core infrastructure source
│   │   ├── orchestrator.go
│   │   ├── model.go
│   │   ├── tools/        # Built-in tool implementations
│   │   ├── agents/       # Sub-agent configurations
│   │   └── connectors/   # Matterbridge client
│   ├── extensions/       # Go extensions (compile-time)
│   ├── cmd/              # Binary entry points (medic, bridge, proxy)
│   └── main.go           # Agent entry point
├── data/                 # VOLUME — persistent data
│   ├── messages.db       # SQLite memory database
│   ├── extensions/       # Starlark .star scripts (agent-writable)
│   └── build-failures/   # Crash logs from failed self-builds
├── soul/                 # VOLUME — identity files
│   ├── SOUL.md
│   ├── IDENTITY.md
│   ├── USER.md
│   └── HEARTBEAT.md
├── public/               # VOLUME — blog/web page
│   ├── index.html
│   ├── activity.json
│   └── *.html            # Blog posts
├── builds/               # Hot-swap staging area
└── matterbridge.toml     # Generated at boot from env vars
```

## Repository Layout

```
gather-claw/
├── Dockerfile              # Multi-stage: golang:1.24 build → Alpine 3.19 runtime
├── Dockerfile.buildservice # External Go build service
├── Makefile                # Dev convenience targets (build, dev, logs, shell, clean)
├── entrypoint.sh           # Identity decode, matterbridge config, service startup
├── docker-compose.yml      # Dev compose (port 8180, includes build-service)
├── extensions-default/     # Default .star scripts (copied on first boot)
│   └── hello.star
├── public/                 # Default public page template
├── provisioning/
│   ├── provision.sh        # Create new claw for a user
│   ├── deprovision.sh      # Stop/remove a claw
│   ├── patch.sh            # Update running claw to latest image
│   ├── setup-host.sh       # One-time server setup
│   ├── docker-compose.user.yml.tpl
│   └── nginx-user.conf.tpl
└── clawpoint-go/           # Go source (committed in repo)
    ├── main.go             # Entry point (loads core + extensions)
    ├── core/
    │   ├── VERSION         # e.g. "0.655"
    │   ├── orchestrator.go # Coordinator setup, sub-agent wiring
    │   ├── model.go        # LLM provider setup (anthropic/gemini)
    │   ├── tools/          # Built-in tools (memory, soul, fs, research, claude, skills, build, starlark)
    │   ├── agents/         # Sub-agent configs (memory, soul, coding, claude, research)
    │   └── connectors/     # Matterbridge API client
    ├── extensions/         # Go extension point (compile-time)
    │   └── extensions.go
    ├── cmd/
    │   ├── medic/          # Supervisor with hot-swap + rollback
    │   ├── bridge/         # Matterbridge connector
    │   ├── proxy/          # Public-facing HTTP proxy
    │   └── buildservice/   # External Go build service
    └── anthropicmodel/     # Custom Anthropic adapter for Google ADK
```

## Environment Variables

| Variable | Purpose |
|----------|---------|
| `MODEL_PROVIDER` | LLM provider: `anthropic` or `gemini` |
| `ANTHROPIC_API_KEY` | API key for LLM |
| `ANTHROPIC_API_BASE` | API base URL |
| `ANTHROPIC_MODEL` | Model name (default: `glm-5`) |
| `TELEGRAM_BOT` | Telegram bot token |
| `TELEGRAM_CHAT_ID` | Telegram chat ID for matterbridge |
| `GATHER_PRIVATE_KEY` | Base64-encoded Ed25519 private key |
| `GATHER_PUBLIC_KEY` | Base64-encoded Ed25519 public key |
| `GATHER_AGENT_ID` | Gather platform agent ID |
| `GATHER_BASE_URL` | Gather platform URL (default: `https://gather.is`) |
| `CLAWPOINT_ROOT` | App root directory (default: `/app`) |
| `CLAWPOINT_DB` | SQLite database path (default: `/app/data/messages.db`) |
| `BUILD_SERVICE_URL` | External build service URL (default: `http://127.0.0.1:9090`) |

## Volumes (Docker Named Volumes)

| Volume Name Pattern | Container Path | Purpose |
|---------------------|---------------|---------|
| `claw-data-<username>` | `/app/data` | Memory DB, extensions, build failure logs |
| `claw-soul-<username>` | `/app/soul` | SOUL.md, IDENTITY.md (agent personality) |
| `claw-public-<username>` | `/app/public` | Blog posts, activity.json, public web page |

All three volumes are persistent. Without them, data is lost on container recreation.

## Build & Deploy

```bash
# Build image (from gather-claw/ directory on server)
cd gather-claw && docker build -t gather-claw:latest .

# Patch a running claw to the latest image (captures config, recreates)
cd provisioning && ./patch.sh <username>

# Patch all running claws
./patch.sh --all

# Rebuild image + patch in one step
./patch.sh --build <username>
./patch.sh --build --all

# Provision a new claw
./provision.sh <username> --zai-key <key> --telegram-token <token> --telegram-chat-id <id>

# Deprovision
./deprovision.sh <username> [--delete-data]
```

## Server Layout

Production claws run as standalone `docker run` containers (not compose). Traefik routes `*.gather.is` subdomains via Docker labels.

```
Container: claw-<username>
Network:   gather-infra_gather_net
Routing:   Traefik labels → Host(`<username>.gather.is`) → container:8080
Volumes:   claw-data-<username>, claw-soul-<username>, claw-public-<username>
```

## Medic: Hot-Swap + Rollback

When the agent self-builds via the build service:
1. New binary appears at `/app/builds/clawpoint-go.new`
2. Medic backs up current binary to `/app/clawpoint-go.prev`
3. Medic swaps and restarts
4. If new binary crashes within 30s → reverts to `.prev`
5. Crash log written to `/app/data/build-failures/<timestamp>.log`
6. Agent reads failure logs on next startup to learn from mistakes
