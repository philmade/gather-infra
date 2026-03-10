# Multi-Agent Provisioning: Clay + Hermes + DeerFlow

## Status: PARKED

Started implementation on 2026-03-10, then parked. Partial changes were made to two files (see "What's already done" below).

## What's already done

### 1. `gather-auth/go/api/claws.go`
- Added `AgentType` field to `ClawDeployment` struct
- Added backwards-compat default in `recordToClawDeployment()`
- Added `AgentType` field to `DeployClawInput` struct
- Added validation in POST /api/claws handler (accepts `clay`, `hermes`, `deerflow`, defaults to `clay`)
- Stores `agent_type` on the record

### 2. `gather-auth/go/cmd/server/main.go`
- Added `agent_type` field migration to `ensureClawDeploymentsCollection()`
- Added `agent_type` field to fresh collection creation

### What's NOT done yet (tasks 2-7)

Everything below still needs implementation.

---

## Context

The claw provisioning pipeline currently deploys only Clay agents (Go/ADK). We want to support multiple agent frameworks — starting with Hermes (NousResearch) and DeerFlow (ByteDance) — so users can choose their agent type at deploy time. Everything else stays the same: Traefik routing, ForwardAuth, LLM proxy, agent identity, messaging UI, channels, heartbeats.

## What stays the same for ALL agent types

- Traefik routing: `{subdomain}.gather.is` → container port 8080
- ForwardAuth: session cookie validation via gather-auth
- LLM proxy: proxy token injection + usage metering
- Agent identity: Ed25519 keypair + agent record in `agents` collection
- Channel creation: default channel per claw
- Messaging: gather-auth streams to/from container on port 8080
- Trial timer + tier enforcement
- `patch.sh` capture/replay pattern

## What changes per agent type

| Concern | Clay | Hermes | DeerFlow |
|---------|------|--------|----------|
| Docker image | `gather-claw:latest` | `gather-hermes:latest` | `gather-deerflow:latest` |
| Public port | 8080 (clay-proxy) | 8080 (gather-bridge) | 8080 (nginx) |
| Debug UI port | 8081 (ADK `/ui/`) | None | None (has own UI at `/`) |
| LLM env vars | `ANTHROPIC_API_KEY` + `ANTHROPIC_API_BASE` | `OPENAI_API_KEY` + `OPENAI_BASE_URL` | Config file injection |
| Data volume | `/app/data` | `/app/data` (hermes home) | `/app/data` (backend state) |
| Config method | Env vars | `.env` + `config.yaml` | `config.yaml` + `.env` |
| Processes | 5 (clay, proxy, medic, bridge, matterbridge) | 2 (bridge + hermes gateway) | 5 (nginx, frontend, gateway, langgraph, bridge) |

## Architecture: The Gather Bridge

The critical integration point: gather-auth's streaming code (`HandleClawStream`) POSTs messages to the claw container at `http://container:8080/msg` and expects SSE responses. Each agent image must speak this protocol.

**Solution: `gather-bridge`** — a thin Go binary (reuse/adapt clay-bridge) that:
1. Listens on port 8080
2. Accepts `POST /msg` with `{text, user_id, username, protocol}`
3. Forwards to the agent's native API (ADK, Hermes AIAgent, DeerFlow LangGraph)
4. Streams SSE responses back
5. Serves static files (agent's public page) at `/`

For Clay, this is already `clay-bridge` + `clay-proxy`. For Hermes and DeerFlow, we build framework-specific bridge adapters.

## Remaining Implementation Plan

### Task 2: Deploy UI — agent type selector

File: `gather-app/src/components/DeployModal/DeployAgentModal.tsx`
- Add `agentType` to `DeployConfig` interface (default `"clay"`)
- Reset it when modal opens

File: `gather-app/src/components/DeployModal/StepConfigure.tsx`
- Add three cards/buttons before the name input:
  - **Clay** — "Autonomous Go agent with build/ops/research loops" (default)
  - **Hermes** — "Python agent with tools, memory, and messaging"
  - **DeerFlow** — "Multi-agent Python framework with web UI"

File: `gather-app/src/components/DeployModal/StepDeploying.tsx`
- Pass `agent_type: config.agentType` in the POST body

### Task 3: OpenAI-compatible LLM proxy endpoint

File: `gather-auth/go/api/llm_proxy.go`

Add `POST /api/llm/v1/chat/completions` that:
1. Validates proxy token (same `x-api-key` header check)
2. Enforces tier quotas (same logic)
3. Forwards to upstream (same upstream URL — most LLM proxies like z.ai support both formats)
4. Records usage

Register in `RegisterLLMProxyRoutes`:
```go
mux.HandleFunc("POST /api/llm/v1/chat/completions", handleOpenAIProxy(app))
```

The handler is identical to `handleLLMProxy` except:
- Upstream path: `/v1/chat/completions` instead of `/v1/messages`
- Headers: `Authorization: Bearer <key>` instead of `x-api-key`
- Usage response format: OpenAI's `usage.prompt_tokens`/`usage.completion_tokens` instead of `usage.input_tokens`/`usage.output_tokens`
- Error format: OpenAI JSON errors instead of Anthropic format

### Task 4: Generalize provisionClaw()

File: `gather-auth/go/cmd/server/main.go` — in `provisionClaw()`

**4a. Image selection:**
```go
agentType := record.GetString("agent_type")
if agentType == "" {
    agentType = "clay"
}

imageMap := map[string]string{
    "clay":     getEnv("CLAW_IMAGE_CLAY", "gather-claw:latest"),
    "hermes":   getEnv("CLAW_IMAGE_HERMES", "gather-hermes:latest"),
    "deerflow": getEnv("CLAW_IMAGE_DEERFLOW", "gather-deerflow:latest"),
}
image := imageMap[agentType]
```

**4b. Per-type env vars:**
```go
// Common env vars (all types)
envMap := map[string]string{
    "GATHER_PRIVATE_KEY":  privB64,
    "GATHER_PUBLIC_KEY":   pubB64,
    "GATHER_AGENT_ID":     agentRec.Id,
    "GATHER_CHANNEL_ID":   channelID,
    "GATHER_BASE_URL":     baseURL,
    "CLAW_NAME":           clawDisplayName,
}

// LLM proxy env vars (per-type)
switch agentType {
case "clay":
    envMap["MODEL_PROVIDER"] = "anthropic"
    envMap["CLAY_ROOT"] = "/app"
    envMap["CLAY_DB"] = "/app/data/messages.db"
    envMap["ANTHROPIC_API_KEY"] = proxyToken
    envMap["ANTHROPIC_API_BASE"] = "http://gather-auth:8090/api/llm"
    envMap["ADK_WEBUI_ADDRESS"] = "https://" + subdomain + ".gather.is/api"
case "hermes":
    envMap["OPENAI_API_KEY"] = proxyToken
    envMap["OPENAI_BASE_URL"] = "http://gather-auth:8090/api/llm/v1"
    envMap["HERMES_HOME"] = "/app/data"
case "deerflow":
    envMap["LLM_PROXY_TOKEN"] = proxyToken
    envMap["LLM_PROXY_URL"] = "http://gather-auth:8090/api/llm"
}
```

**4c. Per-type Traefik labels:**
```go
// All types get the main router
labels := map[string]string{
    "traefik.enable": "true",
    "traefik.http.routers." + routerName + ".rule":             "Host(`" + subdomain + ".gather.is`)",
    "traefik.http.routers." + routerName + ".entrypoints":      "websecure",
    "traefik.http.routers." + routerName + ".tls.certresolver": "cf",
    "traefik.http.routers." + routerName + ".middlewares":      "gather-forward-auth",
    "traefik.http.routers." + routerName + ".service":          routerName,
    "traefik.http.services." + routerName + ".loadbalancer.server.port": "8080",
}

// Debug router only for Clay
if agentType == "clay" {
    labels["traefik.http.routers."+routerName+"-debug.rule"] = "Host(`"+subdomain+".gather.is`) && PathPrefix(`/debug`)"
    // ... rest of debug labels
}
```

**4d. Per-type volumes:**
```go
mounts := []mount.Mount{
    {Type: mount.TypeVolume, Source: dataVolume, Target: "/app/data"},
}
// Clay gets extra soul + public volumes
if agentType == "clay" {
    // Note: provision.sh creates these, provisionClaw() currently only creates data volume
    // May need to add soul + public volume creation here too
}
```

**4e. Vault secret proxy key protection:**
```go
// Per-type: block the relevant proxy key from vault overrides
switch agentType {
case "clay":
    if key == "ANTHROPIC_API_KEY" || key == "ANTHROPIC_API_BASE" { continue }
case "hermes":
    if key == "OPENAI_API_KEY" || key == "OPENAI_BASE_URL" { continue }
case "deerflow":
    if key == "LLM_PROXY_TOKEN" || key == "LLM_PROXY_URL" { continue }
}
```

### Task 5: gather-hermes Docker image

New directory: `gather-hermes/`

**Dockerfile:**
```dockerfile
FROM python:3.11-slim
RUN apt-get update && apt-get install -y git nodejs npm curl && rm -rf /var/lib/apt/lists/*
WORKDIR /app
RUN git clone --recurse-submodules https://github.com/NousResearch/hermes-agent.git /app/hermes
RUN cd /app/hermes && pip install -e ".[all]" && pip install -e ./mini-swe-agent
COPY bridge/ /app/bridge/
COPY entrypoint.sh /app/
RUN chmod +x /app/entrypoint.sh
EXPOSE 8080
ENTRYPOINT ["/app/entrypoint.sh"]
```

**Bridge adapter** (`bridge/main.py`): FastAPI service on port 8080:
- `POST /msg` → creates `AIAgent` instance, calls `.chat(text)`, streams SSE
- `GET /` → static public page
- Session state per user (conversation history)
- Reads `OPENAI_API_KEY` and `OPENAI_BASE_URL` from env

**Entrypoint:**
1. Write `config.yaml` from env vars
2. Decode Gather identity keys
3. Start bridge on port 8080

### Task 6: gather-deerflow Docker image

New directory: `gather-deerflow/`

**Dockerfile:**
```dockerfile
FROM python:3.12-slim
RUN apt-get update && apt-get install -y curl nginx supervisor nodejs npm && rm -rf /var/lib/apt/lists/*
RUN npm install -g pnpm
WORKDIR /app
RUN git clone https://github.com/bytedance/deer-flow.git /app/deerflow
RUN cd /app/deerflow/backend && pip install uv && uv sync
RUN cd /app/deerflow/frontend && pnpm install --frozen-lockfile && pnpm build
COPY bridge/ /app/bridge/
COPY entrypoint.sh supervisord.conf nginx.conf /app/
EXPOSE 8080
ENTRYPOINT ["/app/entrypoint.sh"]
```

**Bridge adapter**: Routes `POST /msg` to DeerFlow's LangGraph API.

**nginx.conf**: Internal routing — frontend static at `/`, gateway API, langgraph backend.

**supervisord.conf**: Manages nginx + frontend + gateway + langgraph + bridge.

### Task 7: Update patch.sh

File: `gather-claw/provisioning/patch.sh`

Detect agent type from container image and skip debug router for non-Clay:

```bash
IMAGE=$(docker inspect "$CONTAINER" --format '{{.Config.Image}}')
case "$IMAGE" in
    *gather-claw*)    AGENT_TYPE="clay" ;;
    *gather-hermes*)  AGENT_TYPE="hermes" ;;
    *gather-deerflow*) AGENT_TYPE="deerflow" ;;
    *)                AGENT_TYPE="clay" ;;
esac

# Only add debug router for Clay
if [ "$AGENT_TYPE" = "clay" ]; then
    LABEL_ARGS="$LABEL_ARGS -l ..."  # debug router labels
fi
```

Also: use the detected image name instead of hardcoded `$IMAGE` for the `docker run` command.

## Files Modified (done)

| File | Change |
|------|--------|
| `gather-auth/go/api/claws.go` | `AgentType` field + validation + storage |
| `gather-auth/go/cmd/server/main.go` | `agent_type` field in collection schema |

## Files to modify (remaining)

| File | Change |
|------|--------|
| `gather-auth/go/cmd/server/main.go` | provisionClaw() generalization |
| `gather-auth/go/api/llm_proxy.go` | OpenAI-compatible endpoint |
| `gather-app/src/components/DeployModal/DeployAgentModal.tsx` | agentType in config |
| `gather-app/src/components/DeployModal/StepConfigure.tsx` | Agent type selector cards |
| `gather-app/src/components/DeployModal/StepDeploying.tsx` | Pass agent_type to API |
| `gather-claw/provisioning/patch.sh` | Multi-image detection |

## Files/Directories to create (remaining)

| Path | Contents |
|------|----------|
| `gather-hermes/Dockerfile` | Hermes agent container |
| `gather-hermes/bridge/main.py` | Python bridge (Gather protocol → Hermes) |
| `gather-hermes/entrypoint.sh` | Config generation + startup |
| `gather-deerflow/Dockerfile` | DeerFlow container |
| `gather-deerflow/bridge/main.py` | Python bridge (Gather protocol → LangGraph) |
| `gather-deerflow/entrypoint.sh` | Config generation + supervisord startup |
| `gather-deerflow/supervisord.conf` | Process manager config |
| `gather-deerflow/nginx.conf` | Internal routing |

## Verification checklist

1. Deploy a Clay claw via UI → works exactly as before
2. Deploy a Hermes claw via UI → container starts, `/msg` endpoint responds, can chat
3. Deploy a DeerFlow claw via UI → container starts, DeerFlow UI loads, can chat
4. LLM proxy metering works for all three types
5. `patch.sh --all` correctly handles mixed containers
6. ForwardAuth works for all agent types
