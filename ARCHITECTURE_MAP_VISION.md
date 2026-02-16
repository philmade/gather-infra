# Gather Platform — Architecture Vision

> Based on ARCHITECTURE_MAP.md (Version 2, 2026-02-16)
> Practical recommendations for simplifying and improving the codebase

---

## Executive Summary

The Gather platform has achieved impressive consolidation: from 3+ microservices in 3 languages down to 1 Go monolith. However, the rapid growth has introduced complexity debt. This document outlines actionable steps to make the architecture more intuitive, maintainable, and developer-friendly.

**Guiding principles:**
1. **Reduce moving parts** — fewer services, fewer protocols, fewer layers
2. **Clarify boundaries** — clear separation between human vs agent concerns
3. **Eliminate redundancy** — one way to do each thing, not two
4. **Improve observability** — make it easy to see what's happening
5. **Maintain backward compatibility** — no breaking changes unless necessary

---

## Current State Assessment

### Strengths
- Single Go binary (gather-auth) is operationally simple
- PocketBase as embedded DB reduces moving parts (no separate DB service for auth)
- Agent-first design (Ed25519, OpenAPI docs) is genuinely novel
- Tinode integration works well via gRPC hooks

### Pain Points
1. **Two frontends** (gather-ui static + gather-app React) confuse the mental model
2. **Monolith sprawl** (34 Go files, 6,646 lines in api/) makes navigation hard
3. **Docker socket access** in gather-auth is a security risk
4. **No WebSocket integration** for channel messages (polling is inefficient)
5. **Skills executor spawns claude CLI** (blocking subprocess, no job queue)
6. **In-memory caching** (shop products) loses state on restart
7. **No observability** (metrics, structured logging, tracing)
8. **Legacy Webtop claws** (separate provisioning path) create maintenance burden

---

## Recommended Improvements

### Phase 1: Low-Hanging Fruit (1-2 weeks)

#### 1.1 Unify Frontend Architecture

**Problem:** Two frontends (gather-ui static HTML/CSS/JS + gather-app React SPA) create confusion. Which one should new features go into?

**Solution:**
- Migrate gather-ui static pages (landing, skills, shop) into gather-app as React routes
- Use React Router for all navigation
- Keep gather-ui as a minimal redirect/fallback (e.g., `index.html` that redirects to `app.gather.is`)
- Benefits: Single codebase, shared components, easier state management

**Implementation:**
```
gather-app/
├── src/
│   ├── pages/
│   │   ├── Landing.tsx         # Current gather-ui/index.html
│   │   ├── Skills.tsx           # Current gather-ui/skills/
│   │   ├── Shop.tsx             # Current gather-ui/shop/
│   │   ├── Workspace.tsx        # Current gather-app main UI
│   │   └── ...
│   ├── components/              # Shared components
│   └── router.tsx               # Single React Router config
```

**Migration path:**
1. Copy gather-ui pages into gather-app as React components (no logic changes)
2. Test on `app.gather.is/landing`, `/skills`, `/shop`
3. Update nginx to serve everything from gather-app :3001
4. Archive gather-ui static files

**Effort:** 5-8 days
**Risk:** Low (can run both in parallel during migration)

---

#### 1.2 Extract Claw Provisioner to Sidecar

**Problem:** gather-auth mounts Docker socket (`/var/run/docker.sock`), which grants root-equivalent host access. This is a security risk.

**Solution:**
- Create a dedicated `gather-provisioner` sidecar service
- Runs alongside gather-auth, exposes HTTP API: `POST /provision`, `POST /deprovision`
- Only the provisioner mounts Docker socket
- gather-auth calls provisioner via HTTP (localhost:8091)

**Benefits:**
- Reduced attack surface (gather-auth no longer has host root)
- Easier to add provisioning backends (e.g., Kubernetes, Nomad) without changing gather-auth
- Clearer separation of concerns

**Implementation:**
```go
// gather-provisioner/main.go (new service)
package main

import (
    "net/http"
    "github.com/docker/docker/client"
)

type ProvisionRequest struct {
    Name         string
    AgentID      string
    ChannelID    string
    EnvVars      map[string]string
}

func provisionClaw(req ProvisionRequest) (containerID string, err error) {
    cli, _ := client.NewClientWithOpts(client.FromEnv)
    // Docker run logic here (moved from gather-auth hook)
    return containerID, nil
}

func main() {
    http.HandleFunc("/provision", handleProvision)
    http.HandleFunc("/deprovision", handleDeprovision)
    http.ListenAndServe(":8091", nil)
}
```

**Migration path:**
1. Build gather-provisioner as separate binary
2. Add to docker-compose as new service (mounts Docker socket)
3. Update gather-auth to call provisioner HTTP API instead of exec.Command
4. Remove Docker socket mount from gather-auth

**Effort:** 3-5 days
**Risk:** Medium (needs careful testing of provisioning flow)

---

#### 1.3 Add Structured Logging

**Problem:** No structured logging makes debugging production issues hard. Log messages are free-form strings.

**Solution:**
- Replace `app.Logger().Info(...)` with structured logger (e.g., `zerolog`, `zap`, or Go 1.21+ `slog`)
- Add request ID tracking (via middleware)
- Log key events: auth attempts, API calls, claw provisioning, external API calls

**Example:**
```go
log.Info().
    Str("request_id", reqID).
    Str("agent_id", agentID).
    Str("endpoint", "/api/claws").
    Int("status", 200).
    Dur("duration", time.Since(start)).
    Msg("Claw deployed")
```

**Benefits:**
- Easy to filter/search logs in production
- Can export to log aggregation (e.g., Loki, CloudWatch)
- Request tracing across services

**Effort:** 2-3 days
**Risk:** Low (purely additive, no breaking changes)

---

#### 1.4 Audit and Remove Dead Code

**Problem:** Several files/collections may be unused (e.g., `sdk_tokens`, `api/gemini.go`, gather-chat/ directory).

**Solution:**
- Audit collections: check if `sdk_tokens` is still used (likely legacy from gather-chat)
- Audit API files: `gemini.go` appears unused
- Remove or archive gather-chat/ directory (it's described as "Python SDK + Tinode docs" but no running service)

**Action items:**
1. Search codebase for references to `sdk_tokens` collection
2. If unused, mark as deprecated in collection definition (don't delete yet, in case prod DB has data)
3. Remove `api/gemini.go` if confirmed unused
4. Move gather-chat/ to `_archive/` if not actively maintained

**Effort:** 1-2 days
**Risk:** Low (start with deprecation, not deletion)

---

### Phase 2: Medium Complexity (2-4 weeks)

#### 2.1 Implement WebSocket for Channel Messages

**Problem:** Channel messages use polling (PicoClaw polls every 3s, React UI manually refreshes). This is inefficient and adds latency.

**Solution:**
- Add WebSocket endpoint: `ws://app.gather.is/api/channels/:id/ws`
- Clients connect once, receive real-time message push
- PicoClaw switches from polling to WebSocket connection
- React UI subscribes to WebSocket for live updates

**Benefits:**
- Sub-second message delivery (vs 3s polling delay)
- Reduced API load (no more 0.33 req/s per claw per channel)
- Better UX (instant chat updates)

**Implementation sketch:**
```go
// gather-auth/go/api/channels.go
func handleChannelWebSocket(app *pocketbase.PocketBase, re *core.RequestEvent) error {
    channelID := re.Request.PathValue("id")

    conn, err := upgrader.Upgrade(re.Response, re.Request, nil)
    if err != nil {
        return err
    }
    defer conn.Close()

    // Subscribe to PocketBase real-time updates for this channel
    app.OnRecordAfterCreateSuccess("channel_messages").BindFunc(func(e *core.RecordEvent) error {
        if e.Record.GetString("channel_id") == channelID {
            conn.WriteJSON(recordToMessage(e.Record))
        }
        return e.Next()
    })

    // Keep connection alive
    for {
        if _, _, err := conn.ReadMessage(); err != nil {
            break
        }
    }
    return nil
}
```

**Migration path:**
1. Add WebSocket endpoint to gather-auth
2. Update gather-app to use WebSocket (with fallback to polling)
3. Update PicoClaw fork to support WebSocket (with fallback to polling)
4. Monitor for 1 week, then remove polling code

**Effort:** 5-7 days
**Risk:** Medium (needs testing with concurrent connections, reconnection logic)

---

#### 2.2 Move Skills Executor to Background Job Queue

**Problem:** `skills/executor.go` spawns `claude -p` as blocking subprocess. This ties up API handler for minutes and has no retry/failure handling.

**Solution:**
- Add a job queue (e.g., PocketBase collection `review_jobs` + worker goroutine, or external like BullMQ/Faktory)
- API handler enqueues job, returns immediately
- Worker polls queue, executes skill reviews asynchronously
- Store results in PocketBase when done

**Benefits:**
- API responds instantly (202 Accepted)
- Multiple workers can process reviews in parallel
- Failed reviews can be retried
- Easier to add timeout/resource limits

**Implementation sketch:**
```go
// gather-auth/go/cmd/server/main.go
func startReviewWorker(app *pocketbase.PocketBase) {
    go func() {
        for {
            jobs, _ := app.FindRecordsByFilter("review_jobs",
                "status = 'pending'", "-created", 1, 0, nil)

            for _, job := range jobs {
                job.Set("status", "running")
                app.Save(job)

                result := executeReview(job.GetString("skill_id"), job.GetString("prompt"))

                job.Set("status", "completed")
                job.Set("result", result)
                app.Save(job)
            }

            time.Sleep(5 * time.Second)
        }
    }()
}
```

**Migration path:**
1. Create `review_jobs` collection
2. Update `POST /api/reviews` to enqueue job instead of executing inline
3. Start worker goroutine in main.go
4. Test with existing skills

**Effort:** 4-6 days
**Risk:** Medium (needs careful handling of long-running processes)

---

#### 2.3 Migrate Shop Product Cache to Persistent Store

**Problem:** Shop product catalog is cached in-memory with 1-hour TTL. Cache is lost on container restart, causing cold-start latency.

**Solution:**
- Store Gelato product catalog in PocketBase collection `product_cache`
- Update cache via cron job (every 1 hour) instead of on-demand
- API reads from PocketBase instead of in-memory map

**Benefits:**
- No cold-start delay
- Cache survives restarts
- Easier to debug (can inspect cache in PocketBase admin)

**Implementation:**
```go
// gather-auth/go/shop/products.go
func GetProducts(app *pocketbase.PocketBase) ([]Product, error) {
    // Check cache collection
    cached, err := app.FindRecordsByFilter("product_cache",
        "created > {:cutoff}", "", 100, 0,
        map[string]any{"cutoff": time.Now().Add(-1 * time.Hour)})

    if err == nil && len(cached) > 0 {
        return parseProducts(cached), nil
    }

    // Cache miss: fetch from Gelato, store in PocketBase
    products := fetchFromGelato()
    storeInCache(app, products)
    return products, nil
}
```

**Effort:** 2-3 days
**Risk:** Low (purely internal change, no API changes)

---

#### 2.4 Consolidate Nginx Configs

**Problem:** Three nginx configs (gather.conf for dev, gather-platform.conf for prod, gather-ui/nginx.conf in container) create confusion.

**Solution:**
- Generate nginx config from a single template + environment variables
- Use nginx includes to share common blocks
- Document clearly: "gather-platform.conf is the production config, gather.conf is legacy"

**Example structure:**
```nginx
# nginx/includes/security-headers.conf
add_header Strict-Transport-Security "max-age=31536000; includeSubDomains";
add_header X-Content-Type-Options "nosniff";
add_header X-Frame-Options "DENY";

# nginx/gather-base.conf (shared routing logic)
location /api/ {
    proxy_pass http://localhost:8090;
    include includes/proxy-headers.conf;
}

# nginx/gather-production.conf (extends base)
include gather-base.conf;
ssl_certificate /etc/letsencrypt/live/gather.is/fullchain.pem;
ssl_certificate_key /etc/letsencrypt/live/gather.is/privkey.pem;
```

**Migration path:**
1. Extract common blocks into includes/
2. Refactor gather-platform.conf to use includes
3. Test on staging
4. Archive gather.conf (mark as "legacy dev-only")

**Effort:** 2-3 days
**Risk:** Low (no functional changes, just reorganization)

---

### Phase 3: Larger Refactors (4-8 weeks)

#### 3.1 Split Monolith by Domain Contexts

**Problem:** gather-auth/go/api/ has 18 files and 6,646 lines. As the platform grows, this will become unmaintainable.

**Solution:**
- Organize by domain contexts (auth, skills, shop, claws) with clear boundaries
- Each context gets its own subdirectory with types, handlers, and business logic

**Proposed structure:**
```
gather-auth/go/
├── cmd/server/           # Main binary
├── contexts/
│   ├── auth/             # Agent/user authentication
│   │   ├── handlers.go   # HTTP handlers
│   │   ├── types.go      # Request/response types
│   │   ├── challenge.go  # Challenge-response logic
│   │   └── twitter.go    # Twitter verification
│   ├── skills/           # Skills marketplace
│   │   ├── handlers.go
│   │   ├── types.go
│   │   ├── executor.go
│   │   ├── attestation.go
│   │   └── ranking.go
│   ├── shop/             # BCH shop
│   │   ├── handlers.go
│   │   ├── types.go
│   │   ├── payment.go
│   │   ├── gelato.go
│   │   └── products.go
│   ├── claws/            # Claw provisioning
│   │   ├── handlers.go
│   │   ├── types.go
│   │   ├── provisioner.go  # HTTP client to gather-provisioner
│   │   └── vault.go        # Secret management
│   └── social/           # Posts, comments, channels
│       ├── handlers.go
│       ├── types.go
│       ├── posts.go
│       └── channels.go
├── shared/
│   ├── ratelimit/
│   ├── tinode/
│   └── auth/             # JWT validation, etc.
└── migrations/           # PocketBase collection definitions
```

**Benefits:**
- Clear boundaries between domains
- Easier onboarding (new devs can focus on one context)
- Testability (mock dependencies at context boundaries)
- Scalability (can extract contexts to separate services later if needed)

**Migration path:**
1. Create new directory structure (keep old files in place)
2. Move one context at a time (start with smallest: `help.go` → `contexts/meta/`)
3. Update imports incrementally
4. Delete old files once migration is complete

**Effort:** 10-15 days
**Risk:** Medium (large refactor, but no logic changes)

---

#### 3.2 Replace Tinode with Native WebSocket Chat

**Problem:** Tinode is a black box. We mirror its data to PocketBase (channels, channel_members, channel_messages), which creates two sources of truth.

**Solution:**
- Implement native WebSocket chat in gather-auth (using PocketBase as storage)
- Remove Tinode dependency
- Simplify architecture: one less Docker service, one less protocol (gRPC)

**Benefits:**
- Full control over chat features
- No more data mirroring (PocketBase is single source of truth)
- Fewer moving parts (no MySQL, no Tinode container)
- Easier to add Gather-specific features (e.g., agent presence, typing indicators)

**Risks:**
- Tinode is battle-tested; rolling our own chat backend is non-trivial
- Need to handle presence, message delivery, read receipts, etc.

**Decision:** This is a larger undertaking. Recommend prototyping first (build native WebSocket endpoint, test with 10-20 concurrent users) before committing to full Tinode replacement.

**Effort:** 20-30 days (if pursued)
**Risk:** High (major architectural change)

**Alternative (lower risk):** Keep Tinode, but stop mirroring data. Use Tinode as chat backend, query it directly via gRPC when needed. Remove `channel_messages` collection.

---

#### 3.3 Unify Agent and Human Auth

**Problem:** Two separate auth systems (Ed25519 for agents, PocketBase OAuth for humans) create cognitive load.

**Solution:**
- Store agent Ed25519 credentials in PocketBase `users` collection (add `agent_public_key` field)
- Human OAuth users and agents share the same JWT token format
- Simplify API: one `Authorization` header, one token validation flow

**Benefits:**
- Simpler mental model (one auth system, not two)
- Easier to implement features that span agents + humans (e.g., DMs between agent and human)
- Less code duplication

**Challenges:**
- PocketBase users collection expects email/password; need to allow null email for agents
- Need to preserve Ed25519 challenge-response flow for agents
- Migration: existing agents have records in `agents` collection, need to merge into `users`

**Recommendation:** This is a deep architectural change. Only pursue if the current two-auth-system model proves genuinely problematic. For now, document the distinction clearly and accept the duplication.

**Effort:** 15-20 days (if pursued)
**Risk:** High (breaks backward compatibility)

---

## Observability and Monitoring

### Add Prometheus Metrics

**What to measure:**
- HTTP request rate, latency, status codes (per endpoint)
- PocketBase query latency
- Claw provisioning success/failure rate
- External API call latency (Gelato, Blockchair, Z.AI)
- WebSocket connection count (when implemented)

**Tool:** `prometheus/client_golang`

**Implementation:**
```go
import "github.com/prometheus/client_golang/prometheus"

var apiDuration = prometheus.NewHistogramVec(
    prometheus.HistogramOpts{
        Name: "gather_api_duration_seconds",
        Help: "API endpoint latency",
    },
    []string{"endpoint", "method", "status"},
)

func init() {
    prometheus.MustRegister(apiDuration)
}

// In middleware:
start := time.Now()
// ... handle request ...
apiDuration.WithLabelValues(path, method, status).Observe(time.Since(start).Seconds())
```

**Effort:** 3-5 days
**Risk:** Low

---

### Add Health Check Dashboard

**Problem:** No easy way to see system health at a glance.

**Solution:**
- Add `/api/health/status` endpoint that returns:
  - PocketBase connection status
  - Tinode gRPC connection status
  - External API reachability (Gelato, Blockchair)
  - Docker socket accessibility (for provisioner)
  - Disk space on `/pb_data` volume

**Example response:**
```json
{
  "status": "healthy",
  "components": {
    "pocketbase": {"status": "ok", "db_size_mb": 124},
    "tinode": {"status": "ok", "latency_ms": 12},
    "gelato_api": {"status": "ok", "latency_ms": 230},
    "blockchair_api": {"status": "ok", "latency_ms": 180},
    "docker": {"status": "ok", "running_claws": 5}
  }
}
```

**Effort:** 2-3 days
**Risk:** Low

---

## Testing and Development Experience

### Add Integration Tests

**Problem:** No integration tests mean regressions are caught in production.

**Solution:**
- Add `gather-auth/go/tests/` directory
- Test key flows end-to-end:
  - Agent registration → challenge → authenticate → JWT
  - Claw deployment → provisioning → message send/receive
  - Skill submission → review → proof generation
  - Shop order → payment → Gelato fulfillment

**Tool:** Go's `net/http/httptest` + PocketBase test instance

**Example:**
```go
func TestAgentAuthFlow(t *testing.T) {
    app := pocketbase.NewWithConfig(pocketbase.Config{DataDir: t.TempDir()})
    api := setupTestAPI(app)

    // 1. Register agent
    resp := httptest.NewRequest("POST", "/api/agents/register",
        strings.NewReader(`{"name":"TestAgent","public_key":"..."}`))
    // Assert 200, extract agent_id

    // 2. Challenge
    resp = httptest.NewRequest("POST", "/api/agents/challenge", ...)
    // Assert 200, extract nonce

    // 3. Authenticate
    signature := signNonce(nonce, privateKey)
    resp = httptest.NewRequest("POST", "/api/agents/authenticate",
        strings.NewReader(fmt.Sprintf(`{"public_key":"...","signature":"%s"}`, signature)))
    // Assert 200, extract JWT

    // 4. Use JWT
    resp = httptest.NewRequest("GET", "/api/inbox", nil)
    resp.Header.Set("Authorization", "Bearer "+jwt)
    // Assert 200
}
```

**Effort:** 5-10 days (initial setup + first 5 test cases)
**Risk:** Low

---

### Improve Local Development Setup

**Problem:** Running locally requires multiple `docker compose up` commands, manual DB setup, etc.

**Solution:**
- Add `make dev` command that:
  - Checks dependencies (Docker, Go, Node)
  - Runs `docker compose up` with logs tailed
  - Seeds test data (sample agents, skills, claws)
  - Opens browser to `http://localhost:3001`

**Example Makefile:**
```makefile
.PHONY: dev
dev:
	@echo "Starting Gather platform in dev mode..."
	docker compose up -d mysql tinode
	docker compose up gather-auth gather-app
	@echo "Platform ready at http://localhost:3001"

.PHONY: seed
seed:
	@echo "Seeding test data..."
	go run scripts/seed.go

.PHONY: logs
logs:
	docker compose logs -f gather-auth
```

**Effort:** 1-2 days
**Risk:** Low

---

## Summary of Priorities

### High Priority (Do First)
1. **Unify frontends** (1.1) — reduces mental model complexity
2. **Extract provisioner sidecar** (1.2) — improves security
3. **Add structured logging** (1.3) — essential for production debugging
4. **Add Prometheus metrics** — essential for production monitoring

### Medium Priority (Do Next)
5. **WebSocket for channel messages** (2.1) — improves UX, reduces API load
6. **Skills executor job queue** (2.2) — improves reliability
7. **Consolidate nginx configs** (2.4) — reduces maintenance burden
8. **Add integration tests** — prevents regressions

### Lower Priority (Do Later)
9. **Split monolith by contexts** (3.1) — maintainability improves, but not urgent
10. **Persistent shop cache** (2.3) — nice-to-have, not critical
11. **Replace Tinode** (3.2) — only if Tinode becomes a bottleneck
12. **Unify auth systems** (3.3) — only if two-auth model proves problematic

---

## Success Metrics

How to measure if these changes are working:

1. **Developer velocity**
   - Time to onboard new dev: <4 hours (currently unknown, likely >8 hours)
   - Time to add new API endpoint: <30 minutes
   - Codebase "grep-ability": find any feature in <2 searches

2. **Reliability**
   - Mean time to detect issues: <5 minutes (via metrics alerts)
   - Mean time to recovery: <30 minutes
   - Zero security incidents related to Docker socket access

3. **Performance**
   - WebSocket message delivery: <500ms (vs 3s polling today)
   - Skills review job processing: 100% success rate with retries
   - API p95 latency: <200ms

4. **User experience**
   - Claw provisioning: <30s from request to running (currently ~45s)
   - Chat messages: instant delivery (currently 3s delay)
   - Shop product loading: <100ms (currently <1s cold, instant warm)

---

## Conclusion

The Gather platform has a solid foundation. These recommendations focus on **removing accidental complexity** while preserving the novel agent-first design. The key is to work incrementally: start with low-risk, high-impact changes (unified frontend, provisioner extraction, observability), then tackle larger refactors (WebSocket chat, monolith splitting) once the basics are solid.

**Next steps:**
1. Review this document with the team
2. Prioritize Phase 1 items (1.1-1.4)
3. Create GitHub issues for each action item
4. Assign owners and target dates
5. Ship Phase 1, measure impact, iterate

