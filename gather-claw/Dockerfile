# Clay — autonomous AI agent container
# Build context: gather-claw/ (clay/ source committed in repo)
#
# Build: docker build -t gather-claw:latest .

# === Stage 1: Build clay binaries ===
FROM golang:1.24-alpine AS builder
RUN apk add --no-cache git
WORKDIR /build
COPY clay/ .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o clay .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o clay-medic ./cmd/medic
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o clay-bridge ./cmd/bridge
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o clay-proxy ./cmd/proxy

# === Stage 2: Runtime ===
FROM alpine:3.19

RUN apk add --no-cache matterbridge ca-certificates bash curl jq python3

WORKDIR /app

# Clay binaries
COPY --from=builder /build/clay /app/
COPY --from=builder /build/clay-medic /app/
COPY --from=builder /build/clay-bridge /app/
COPY --from=builder /build/clay-proxy /app/

# Full source code (agent can read to understand itself)
COPY clay/ /app/src/

# Core version (readable by the agent at runtime)
COPY clay/core/VERSION /app/core-version

# Entrypoint (gather identity + service startup)
COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

ENV CLAY_ROOT=/app
ENV CLAY_DB=/app/data/messages.db

# Default public page template (copied to /app/public on first boot)
COPY public/ /app/public-default/

# Default Starlark extensions (copied to /app/data/extensions on first boot)
COPY extensions-default/ /app/extensions-default/

RUN mkdir -p /app/data /app/soul /app/public /app/builds /app/data/build-failures /app/data/extensions /var/log

EXPOSE 8080

ENTRYPOINT ["/entrypoint.sh"]
