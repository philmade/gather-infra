# ClawPoint-Go — ADK multi-agent orchestrator container
# Build context: gather-claw/ (clawpoint-go/ source committed in repo)
#
# Build: docker build -t gather-claw:latest .

# === Stage 1: Build clawpoint-go binaries ===
FROM golang:1.24-alpine AS builder
RUN apk add --no-cache git
WORKDIR /build
COPY clawpoint-go/ .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o clawpoint-go .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o clawpoint-medic ./cmd/medic
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o clawpoint-bridge ./cmd/bridge
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o clawpoint-proxy ./cmd/proxy

# === Stage 2: Runtime ===
FROM alpine:3.19

RUN apk add --no-cache matterbridge ca-certificates bash curl jq

WORKDIR /app

# ClawPoint-Go binaries
COPY --from=builder /build/clawpoint-go /app/
COPY --from=builder /build/clawpoint-medic /app/
COPY --from=builder /build/clawpoint-bridge /app/
COPY --from=builder /build/clawpoint-proxy /app/

# Full source code (agent can read to understand itself)
COPY clawpoint-go/ /app/src/

# Core version (readable by the agent at runtime)
COPY clawpoint-go/core/VERSION /app/core-version

# Entrypoint (gather identity + service startup)
COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

ENV CLAWPOINT_ROOT=/app
ENV CLAWPOINT_DB=/app/data/messages.db

# Default public page template (copied to /app/public on first boot)
COPY public/ /app/public-default/

# Default Starlark extensions (copied to /app/data/extensions on first boot)
COPY extensions-default/ /app/extensions-default/

RUN mkdir -p /app/data /app/soul /app/public /app/builds /app/data/build-failures /app/data/extensions /var/log

EXPOSE 8080

ENTRYPOINT ["/entrypoint.sh"]
