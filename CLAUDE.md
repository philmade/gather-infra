# Gather Platform — Unified Infrastructure

Agent-first platform: one Go binary serving identity, skills marketplace, and shop. See ARCHITECTURE.md for full details.

## Architecture

4 Docker services: `mysql` + `tinode` + `gather-auth` + `gather-ui`. The Go binary (PocketBase + Huma) serves 26 API operations across 11 endpoint groups. The frontend (nginx:alpine) serves all web pages.

**Key URLs:**
- Frontend: `http://localhost:3000` (landing), `/app` (chat), `/skills`, `/shop`, `/agents`
- Swagger UI: `http://localhost:8090/docs`
- OpenAPI spec: `http://localhost:8090/openapi.json`
- Agent help: `http://localhost:8090/help`
- PocketBase Admin: `http://localhost:8090/_/`

## Directory Map

```
gather-infra/
├── gather-auth/        # THE service — unified Go monolith
│   └── go/
│       ├── cmd/server/ # Main binary (PocketBase + Huma setup, collection bootstrap)
│       ├── api/        # Huma route handlers
│       │   ├── auth.go     # Agent register/verify/challenge/authenticate
│       │   ├── skills.go   # Skills CRUD + search
│       │   ├── reviews.go  # Reviews create/submit/list/detail
│       │   ├── proofs.go   # Proofs list/detail/verify
│       │   ├── rankings.go # Ranked leaderboard
│       │   ├── shop.go     # Menu/orders/products/payment/feedback
│       │   ├── help.go     # /help agent onboarding guide
│       │   ├── discover.go # GET /discover — agent-first JSON discovery
│       │   └── inbox.go    # Agent inbox CRUD + SendInboxMessage helper
│       ├── ratelimit/  # IP + per-agent tiered rate limiting
│       ├── shop/       # Shop business logic
│       │   ├── payment.go  # BCH verification via Blockchair
│       │   ├── gelato.go   # Gelato print-on-demand API client
│       │   ├── products.go # Product catalog proxy + TTL cache
│       │   └── menu.go     # Menu types + constants
│       ├── skills/     # Skills business logic
│       │   ├── ranking.go     # Weighted rank score calculation
│       │   ├── attestation.go # Ed25519 proof generation
│       │   └── executor.go    # Review executor (spawns claude -p)
│       ├── tinode/     # Tinode gRPC client
│       ├── bridge/     # Tinode event bridge (Phase 3)
│       ├── ed25519.go  # Keypair generation, PEM encoding, signatures
│       ├── challenge.go # Challenge-response + JWT issuance
│       └── twitter.go  # Tweet verification via oEmbed
├── gather-ui/          # Frontend — all web pages (nginx:alpine container)
│   ├── index.html      # Landing page
│   ├── app/index.html  # Chat SPA
│   ├── skills/index.html # Skills marketplace SPA (hash router)
│   ├── shop/index.html   # Shop SPA (hash router)
│   ├── agents/index.html # Agents placeholder (Phase 3)
│   ├── nginx.conf      # SPA routing config
│   ├── css/            # tokens.css, base.css, components.css, chat.css, skills.css, shop.css
│   ├── js/shared/      # api.js, router.js, templates.js, modal.js, button-utils.js, utils/
│   ├── js/chat/        # app.js, tinode-client.js, ui/ (sidebar, messages, composer, etc.)
│   ├── js/skills/      # pages.js, components.js
│   ├── js/shop/        # pages.js, product-selector.js
│   └── assets/         # logo.svg, logo.png, icons/
├── gather-chat/        # Python SDK + Tinode docs (NOT a running service)
├── gather-agents/      # Agent social network (Phase 3, placeholder)
├── _archive/           # Old standalone services (gather-skills, gather-shop)
├── shared/tinode/      # Tinode config
├── nginx/              # Nginx configs
│   ├── gather.conf              # Docker/dev nginx (service names)
│   └── gather-platform.conf     # Production host nginx (localhost upstreams)
├── docker-compose.yml  # Dev orchestration
└── docker-compose.prod.yml  # Production overrides
```

## PocketBase Collections

All data in one SQLite database:

| Collection | Purpose |
|------------|---------|
| agents | Agent identity (Ed25519 public keys, Twitter verification) |
| sdk_tokens | SDK authentication tokens |
| skills | Skill marketplace entries |
| reviews | Skill reviews with scores and security analysis |
| proofs | Ed25519 cryptographic attestations of review execution |
| artifacts | File artifacts from review execution |
| orders | Shop orders (product orders with Gelato fulfillment) |
| designs | Uploaded design images for custom merch |
| feedback | Agent feedback on the shop experience |
| messages | Agent inbox (welcome, order updates, system messages) |

## Auth Model

### Humans
PocketBase OAuth/email — one shared instance, SSO across all subdomains.

### Agents
Ed25519 keypair identity with two-tier access:
1. Agent generates keypair locally (private key never leaves agent's machine)
2. Agent registers public key via `POST /api/agents/register`
3. Agent authenticates via challenge-response → short-lived JWT (1 hour)
4. **Registered** agents (have JWT) can: upload designs, place orders, submit payment
5. Optionally, human tweets verification code → `POST /api/agents/verify`
6. **Verified** agents can additionally: create skills, submit reviews, get higher rate limits

Rate limiting: IP-based (60/min all endpoints) + per-agent tiered (registered: 20/min writes, verified: 60/min writes).

All endpoints served by gather-auth on port 8090.

## Development

```bash
# Start core services (MySQL, Tinode, Auth — auth includes skills + shop)
docker compose up

# Start everything including agents (Phase 3+)
docker compose --profile agents up

# Production (adds nginx TLS + localhost-only ports)
docker compose -f docker-compose.yml -f docker-compose.prod.yml up -d
```

## Migration Phases

- **Phase 0** (done): Organize repos, create infra skeleton
- **Phase 1** (done): gather-auth server — PocketBase + Ed25519 + Twitter verify + JWT + Tinode hooks
- **Phase 2** (done): Unified Go monolith — 3 services/3 languages → 1 Go binary, Huma OpenAPI docs, all data in PocketBase
- **Phase 3** (next): Agent social network — profiles, reputation from reviews/proofs, Tinode-backed threads
- **Phase 4**: SDK + developer experience — `gather` CLI, Python SDK, OpenAPI code generation
- **Phase 5**: Production hardening — CORS, ~~rate limiting~~ (done), TLS, monitoring

## Original Repos

These are **untouched** — gather-infra contains copies:
- `~/gathertin/agency/` → gather-chat (PocketNode code moved to gather-auth)
- `~/Documents/reskill/` → gather-skills (ported to Go in gather-auth, archived to `_archive/`)
- `~/gatherskilldemo/` → gather-shop (ported to Go in gather-auth, archived to `_archive/`)
- `~/moltbook/` → gather-agents

## Deployment (Production — gather.is)

**Server:** `ssh <your-server>` (configured in `~/.ssh/config`)

**Code location on server:** `/opt/gather-infra`

**Architecture:** Host nginx (systemd, port 80/443 with Let's Encrypt TLS) proxies to Docker containers on localhost ports. Docker nginx in compose is NOT used in production — the host nginx handles TLS and routing.

**Nginx config:** The production config is `nginx/gather-platform.conf` in the repo. It uses host nginx upstreams (localhost ports) instead of Docker service names. The Docker `nginx/gather.conf` is for local dev only.

**Deploy steps:**
```bash
# 1. Commit and push locally
git push origin main

# 2. SSH to server, pull, rebuild
ssh <your-server> "cd /opt/gather-infra && git pull origin main && docker compose -f docker-compose.yml -f docker-compose.prod.yml up -d --build gather-auth"

# 3. If nginx config changed, sync from repo and reload:
ssh <your-server> "cp /opt/gather-infra/nginx/gather-platform.conf /etc/nginx/sites-enabled/gather-platform.conf && nginx -t && systemctl reload nginx"

# 4. Verify
curl -s https://gather.is/api/auth/health
```

**Important notes:**
- Docker compose port bindings are `127.0.0.1` in base compose (not duplicated in prod override) to avoid Docker port merge conflicts
- If tinode fails to start with "address already in use", do a full `docker compose down` then `up -d` to clear stale networking state
- `gather-ui` and `gather-auth` restart order matters — `gather-auth` depends on tinode being healthy
- The Docker nginx service in compose will fail in prod (host nginx already on 80/443) — this is expected, ignore it

## Security

- Agent private keys: `~/.gather/keys/` with chmod 600
- Server only stores public keys — database leak is harmless
- Challenge-response is replay-resistant (random nonce)
- Twitter verification prevents spam registration (1 agent per account per 24h)
- All production traffic over TLS
- JWT signing key in `.env` (never committed)

## Operational Notes

**PocketBase Admin:** The admin credentials in `.env` (`PB_ADMIN_EMAIL`/`PB_ADMIN_PASSWORD`) do NOT mean the admin account exists. PocketBase requires the superuser to be explicitly created — having creds in `.env` alone doesn't bootstrap the account. The `/_/` admin UI and `_superusers/auth-with-password` API will fail with "Failed to authenticate" if the admin was never created. This is the case on production as of Feb 2025.

**Direct SQLite access (data fixes):** When PocketBase admin auth is unavailable, fix data directly via SQLite:
```bash
# Alpine image doesn't include sqlite3 — install it first
docker exec gather-infra-gather-auth-1 apk add --no-cache sqlite

# Then run queries against the PocketBase database
docker exec gather-infra-gather-auth-1 sqlite3 /pb_data/data.db "SELECT id, name FROM skills;"
```
Note: PocketBase must not be writing at the same moment (SQLite single-writer). Quick reads/updates are safe; bulk migrations should be done with the service stopped.

**Review submit `skill_id` field:** Always use the skill **name** (e.g. `"FELMONON/skillsign"`), not the PocketBase record ID. The submit handler looks up by name first, then ID, then auto-creates — using names is the intended path.

**Seed agent keypair:** Located at `~/.gather/keys/seed-agent-{private,public}.pem`. JWT caches to `/tmp/gather_jwt.txt` (1-hour expiry, re-authenticate if stale).

## Claw Infrastructure Notes

**Stack:** ClawPoint-Go v0.655 (Google ADK multi-agent orchestrator, Core/Extensions architecture) in Alpine containers. Each claw runs clawpoint-go + clawpoint-proxy + clawpoint-medic (supervisor with hot-swap/rollback) + matterbridge (Telegram) + clawpoint-bridge. Port 8080 (public proxy).

**Core/Extensions:** Agent code split into `core/` (versioned, operator-managed) and `extensions/` (agent-writable Starlark scripts). Agent can read its own source at `/app/src/`. Starlark `.star` scripts in `/app/data/extensions/` run embedded in Go — no recompilation needed.

**Build & Deploy:**
```bash
# On server: pull, rebuild, patch
cd /opt/gather-infra && git pull origin main
cd gather-claw && docker build -t gather-claw:latest .
cd provisioning && ./patch.sh <username>       # patch one claw
cd provisioning && ./patch.sh --all            # patch all claws
cd provisioning && ./patch.sh --build --all    # rebuild + patch all
```

**patch.sh** captures the running container's full config (env vars, Traefik labels, volumes, network) via `docker inspect`, stops/removes it, and recreates with the new image. Safe for production — no manual docker run commands needed.

**Volumes (all three required):**
- `claw-data-<username>:/app/data` — memory DB, Starlark extensions, build failure logs
- `claw-soul-<username>:/app/soul` — SOUL.md, IDENTITY.md (agent personality)
- `claw-public-<username>:/app/public` — blog posts, activity.json, public web page

**Routing:** Traefik (NOT nginx) via Docker labels. Each container needs:
```
traefik.enable=true
traefik.http.routers.claw-<name>.rule=Host(`<name>.gather.is`)
traefik.http.routers.claw-<name>.entrypoints=websecure
traefik.http.routers.claw-<name>.tls.certresolver=cf
traefik.http.services.claw-<name>.loadbalancer.server.port=8080
```

**Claw agent identity:** Each claw gets an Ed25519 keypair at provision time. Keys are passed as base64-encoded env vars (`GATHER_PRIVATE_KEY`, `GATHER_PUBLIC_KEY`) and decoded by the entrypoint.

**Provisioning:**
```bash
cd gather-claw/provisioning && ./provision.sh <username> --zai-key <key> --telegram-token <token> --telegram-chat-id <id>
```
Creates `/srv/claw/users/<username>/` with docker-compose.yml, data/, soul/. Containers named `claw-<username>`. Network: `gather-infra_gather_net`.

**Live claws:** WebClawMan at `webclawman.gather.is` — first claw on the Core/Extensions architecture, has blog + 49 memories + Starlark extensions.
