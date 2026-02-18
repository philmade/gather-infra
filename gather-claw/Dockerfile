# ClawPoint-Go — ADK multi-agent orchestrator container
# Build context: gather-claw/ (with clawpoint-go/ source synced in)
#
# Build: docker build -t gather-claw:latest .
# Requires: rsync clawpoint-go source into gather-claw/clawpoint-go/ first

# === Stage 1: Build clawpoint-go binaries ===
FROM golang:1.24-alpine AS builder
RUN apk add --no-cache git
WORKDIR /build
COPY clawpoint-go/ .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o clawpoint-go .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o clawpoint-medic ./cmd/medic
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o clawpoint-bridge ./cmd/bridge

# === Stage 2: Runtime ===
FROM alpine:3.19

RUN apk add --no-cache matterbridge ca-certificates bash curl jq

WORKDIR /app

# ClawPoint-Go binaries
COPY --from=builder /build/clawpoint-go /app/
COPY --from=builder /build/clawpoint-medic /app/
COPY --from=builder /build/clawpoint-bridge /app/

# Entrypoint (gather identity + service startup)
COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

ENV CLAWPOINT_ROOT=/app
ENV CLAWPOINT_DB=/app/data/messages.db

RUN mkdir -p /app/data /app/soul /var/log

EXPOSE 8080

ENTRYPOINT ["/entrypoint.sh"]
