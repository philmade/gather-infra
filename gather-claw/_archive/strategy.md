# BuyClaw Strategy

## Vision

A turnkey "AI workspace in a box" - customers get a browser-accessible Linux desktop with an autonomous AI agent (OpenClaw) pre-configured and ready to go. No setup, no API keys to manage, no terminal knowledge needed. Open browser, start working with your AI agent.

## Phase 1: Build for Phill (Current)

**Goal**: Get a single working instance on Hetzner that Phill can use daily.

**What gets built**:
- Hetzner server provisioned and hardened
- NGINX reverse proxy with Let's Encrypt SSL
- Custom Webtop Docker container with pre-installed:
  - OpenClaw (AI agent gateway)
  - GLM-5 configured as the LLM backend (via z.ai API)
  - Firefox browser
  - xfce4-terminal
  - Telegram channel adapter for OpenClaw
  - Wallet integration (Privy - parked for now)
- Docker Compose file for easy start/stop/rebuild
- Accessible at a domain over HTTPS

**What does NOT get built yet**:
- User management / auth
- Billing / subscriptions
- Multi-tenant container orchestration
- Landing page / marketing site

## Phase 2: Multi-User SaaS (Future)

**Goal**: Let other people sign up, pay $50/month, and get their own container.

**What gets added**:
- Simple web app for signup/login
- Stripe billing integration
- Docker API orchestration (provision/destroy containers per user)
- NGINX dynamic routing (map user to their container port)
- User dashboard (start/stop/status of their workspace)
- Persistent storage per user (optional)

**Estimated per-user costs at scale**:
| Component | Monthly Cost |
|-----------|-------------|
| Hetzner compute (~4GB RAM, 2 CPU per user) | ~$5-8 |
| GLM-5 API usage (moderate) | ~$5-15 |
| Kasm/Webtop licensing | $0 (open source) |
| Privy wallet (if used) | TBD |
| **Total per user** | **~$10-23** |
| **Revenue per user** | **$50** |
| **Margin per user** | **$27-40 (~55-80%)** |

## Technology Decisions

### Why Webtop over Kasm
- Kasm Community Edition: free but limited to 5 sessions, non-commercial license
- Kasm Starter: $5+/user/month licensing fee
- Webtop: fully open source (GPL), no licensing costs, uses Selkies/WebRTC (better streaming than Kasm's VNC)
- Tradeoff: we build our own orchestration layer in Phase 2 (~1-2 days of work)

### Why GLM-5 over Claude/GPT
- $1.00/1M input tokens vs Claude's higher pricing
- OpenAI-compatible API (drop-in replacement)
- 200K context window, competitive benchmarks (77.8% SWE-bench)
- Open-weight MIT license expected Q1 2026
- Users can swap to Claude/GPT if they prefer (OpenClaw is model-agnostic)

### Why OpenClaw over Nanobot
- 180K+ developer community, 100+ AgentSkills
- Native integrations: Telegram, WhatsApp, Signal, Discord, Slack, GitHub, etc.
- Production-grade memory (SQLite vector + FTS5)
- Privy wallet skill already available on ClawHub
- Security concerns mitigated by container isolation
- Nanobot (3,510 lines Python) remains a fallback if OpenClaw proves too heavy

### Why Privy for Wallet (when we get there)
- Already has an OpenClaw skill on ClawHub
- Server-side key management (keys never in agent memory)
- Multi-chain: Ethereum + Solana
- Policy-driven guardrails (spending limits, transaction validation)
- Alternative: Coinbase Agentic Wallets (launched Feb 2026, Base chain only - too new)

### Why Telegram as First Messenger
- OpenClaw has a mature Telegram channel adapter
- Telegram Bot API is free and well-documented
- No phone number verification hassles (unlike WhatsApp)
- Can add WhatsApp/Signal later via additional channel adapters

## Competitive Landscape

The "AI agent workspace" space is nascent. Closest comparisons:
- **DigitalOcean OpenClaw Marketplace**: one-click deploy, but no desktop/browser - just the agent
- **Cloudflare MoltWorker**: OpenClaw on Workers, no desktop
- **Various VPS providers**: sell compute, but no pre-configuration

BuyClaw's differentiator: the complete package. Desktop + agent + wallet + messenger, ready to go.
