# Gather-Claw

## What This Is

Per-user agent containers running ClawPoint-Go — a Google ADK-based multi-agent orchestrator with self-healing, persistent memory, Telegram messaging (via Matterbridge), and web research. Each claw is an Alpine container (~50MB) exposing an ADK API on port 8080.

## Architecture

- **Runtime**: Alpine 3.19 (no GUI, no desktop)
- **Agent**: ClawPoint-Go (ADK multi-agent orchestrator)
- **Messaging**: Matterbridge (Telegram <-> ADK bridge)
- **Supervisor**: clawpoint-medic (self-healing watchdog)
- **LLM backend**: Anthropic-compatible (z.ai GLM-5 by default)
- **Identity**: Ed25519 keypair per claw (Gather agent identity)
- **Reverse proxy**: NGINX with Let's Encrypt SSL per `*.claw.gather.is`

## Container Services

| Process | Purpose |
|---------|---------|
| `clawpoint-go` | ADK multi-agent orchestrator (port 8080) |
| `clawpoint-medic` | Supervisor/watchdog (foreground, PID 1) |
| `clawpoint-bridge` | Matterbridge <-> ADK connector |
| `matterbridge` | Telegram bot <-> local API bridge |

## Directory Layout

```
gather-claw/
├── Dockerfile              # Multi-stage: build clawpoint-go from source, Alpine runtime
├── entrypoint.sh           # Gather identity + service startup
├── docker-compose.yml      # Dev compose for local testing
├── provisioning/
│   ├── provision.sh        # Create new claw for a user
│   ├── deprovision.sh      # Stop/remove a claw
│   ├── setup-host.sh       # One-time server setup (NGINX, certs)
│   ├── docker-compose.user.yml.tpl  # Per-user compose template
│   └── nginx-user.conf.tpl          # Per-user NGINX template
├── zeroclaw-gather-channel/ # Rust Gather channel adapter
├── strategy.md             # Strategic notes
├── .gather/                # Gather agent identity
└── clawpoint-go/           # Source (synced from external repo, not committed)
```

## Environment Variables

| Variable | Purpose |
|----------|---------|
| `MODEL_PROVIDER` | LLM provider (default: `anthropic`) |
| `ANTHROPIC_API_KEY` | API key for LLM |
| `ANTHROPIC_API_BASE` | API base URL |
| `ANTHROPIC_MODEL` | Model name (default: `glm-5`) |
| `TELEGRAM_BOT` | Telegram bot token |
| `TELEGRAM_CHAT_ID` | Telegram chat ID for matterbridge |
| `GATHER_PRIVATE_KEY` | Base64-encoded Ed25519 private key |
| `GATHER_PUBLIC_KEY` | Base64-encoded Ed25519 public key |
| `CLAWPOINT_ROOT` | App root directory (default: `/app`) |
| `CLAWPOINT_DB` | SQLite database path (default: `/app/data/messages.db`) |

## Volumes

| Mount | Container Path | Purpose |
|-------|---------------|---------|
| `./data` | `/app/data` | Persistent memory DB, wallet files |
| `./soul` | `/app/soul` | SOUL.md, IDENTITY.md (agent personality) |

## Build & Deploy

```bash
# 1. Sync clawpoint-go source into build context
rsync -av /path/to/clawpoint-go/ gather-claw/clawpoint-go/

# 2. Build image
cd gather-claw && docker build -t gather-claw:latest .

# 3. Provision a claw (on server)
cd provisioning && ./provision.sh <username> --zai-key <key> --telegram-token <token> --telegram-chat-id <id>

# 4. Deprovision
cd provisioning && ./deprovision.sh <username> [--delete-data]
```

## Server Layout

```
/srv/claw/users/<username>/
├── docker-compose.yml   # Generated from template
├── data/                # Mounted to /app/data (persistent)
└── soul/                # Mounted to /app/soul (personality)
```

NGINX configs: `/etc/nginx/claw-users/<username>.conf`

## Migration from BuyClaw/PicoClaw

Old containers (`buyclaw-*`) used Webtop (4GB RAM, Chrome, XFCE). New containers (`claw-*`) use Alpine (~50MB, no GUI). To migrate:

1. Stop old: `docker stop buyclaw-<name>`
2. Copy soul files: `cp -r /srv/buyclaw/users/<name>/config/.openclaw/workspace/soul/ /srv/claw/users/<name>/soul/`
3. Provision new claw with same Telegram token
4. Update NGINX (port change, remove auth_request)
5. Verify agent responds on Telegram
6. Remove old: `docker rm buyclaw-<name>`
