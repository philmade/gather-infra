#!/bin/bash
set -e
cd /app

# --- Gather identity (if keys provided) ---
if [ -n "$GATHER_PRIVATE_KEY" ]; then
    mkdir -p /root/.gather/keys
    echo "$GATHER_PRIVATE_KEY" | base64 -d > /root/.gather/keys/private.pem
    echo "$GATHER_PUBLIC_KEY" | base64 -d > /root/.gather/keys/public.pem
    chmod 600 /root/.gather/keys/*.pem
fi

# --- Matterbridge config (if Telegram token provided) ---
if [ -n "$TELEGRAM_BOT" ]; then
    cat > /app/matterbridge.toml <<MBEOF
[telegram.claw]
Token="$TELEGRAM_BOT"
RemoteNickFormat="[{PROTOCOL}] <{NICK}> "

[api.claw]
BindAddress="127.0.0.1:4242"

[[gateway]]
name="claw"
enable=true

[[gateway.inout]]
account="telegram.claw"
channel="${TELEGRAM_CHAT_ID:-0}"

[[gateway.inout]]
account="api.claw"
channel="api"
MBEOF
    echo "Starting matterbridge..."
    matterbridge -conf /app/matterbridge.toml > /tmp/matterbridge.log 2>&1 &
fi

# --- Soul files (copy defaults if not mounted) ---
if [ ! -f /app/soul/SOUL.md ]; then
    echo "# Soul" > /app/soul/SOUL.md
    echo "# Identity" > /app/soul/IDENTITY.md
fi

# --- Public page (copy defaults on first boot) ---
if [ ! -f /app/public/index.html ]; then
    cp /app/public-default/* /app/public/ 2>/dev/null || true
fi

# --- Port layout: proxy on :8080 (public), ADK on :8081 (internal) ---
export ADK_PORT=8081

# --- Start clawpoint-go (internal, port 8081) ---
echo "Starting clawpoint-go on :${ADK_PORT}..."
PORT=${ADK_PORT} ./clawpoint-go web api webui > /tmp/adk-go.log 2>&1 &

# --- Start proxy (public-facing, port 8080 → ADK on 8081) ---
echo "Starting clawpoint-proxy on :8080..."
PROXY_ADDR=":8080" ADK_INTERNAL="http://127.0.0.1:${ADK_PORT}" PUBLIC_DIR="/app/public" \
    ./clawpoint-proxy > /tmp/proxy.log 2>&1 &

# --- Start bridge (if matterbridge running) ---
if [ -n "$TELEGRAM_BOT" ]; then
    echo "Starting clawpoint-bridge..."
    ADK_URL="http://127.0.0.1:${ADK_PORT}" ./clawpoint-bridge > /tmp/bridge.log 2>&1 &
fi

# --- Medic as foreground supervisor ---
echo "Starting clawpoint-medic..."
exec ./clawpoint-medic
