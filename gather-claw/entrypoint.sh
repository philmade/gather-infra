#!/bin/bash
set -e
cd /app

# --- User-configured environment variables (from UI settings) ---
if [ -f /app/data/.env ]; then
    set -a
    source /app/data/.env
    set +a
fi

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
RemoteNickFormat=""

[api.claw]
BindAddress="127.0.0.1:4242"

[[gateway]]
name="clawpoint"
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

# --- Starlark extensions (copy defaults on first boot) ---
mkdir -p /app/data/extensions
if [ ! -f /app/data/extensions/hello.star ]; then
    cp /app/extensions-default/*.star /app/data/extensions/ 2>/dev/null || true
fi

# --- Port layout: proxy on :8080 (public), ADK on :8081 (internal) ---
export ADK_PORT=8081

# --- Start clawpoint-go (internal, port 8081) ---
# ADK_WEBUI_ADDRESS lets the web UI know where the API is from the browser's perspective.
# For local dev with exposed port 8181, set ADK_WEBUI_ADDRESS=http://localhost:8181/api
WEBUI_FLAG=""
if [ -n "$ADK_WEBUI_ADDRESS" ]; then
    WEBUI_FLAG="-api_server_address ${ADK_WEBUI_ADDRESS}"
fi
echo "Starting clawpoint-go on :${ADK_PORT}..."
./clawpoint-go web -port ${ADK_PORT} -write-timeout 10m api -sse-write-timeout 10m webui ${WEBUI_FLAG} > /tmp/adk-go.log 2>&1 &

# --- Start proxy (public-facing, port 8080 → ADK on 8081) ---
echo "Starting clawpoint-proxy on :8080..."
PROXY_ADDR=":8080" ADK_INTERNAL="http://127.0.0.1:${ADK_PORT}" PUBLIC_DIR="/app/public" \
    ./clawpoint-proxy > /tmp/proxy.log 2>&1 &

# --- Start bridge (always — serves Gather UI messages + Telegram if configured) ---
echo "Starting clawpoint-bridge..."
ADK_URL="http://127.0.0.1:${ADK_PORT}" ./clawpoint-bridge > /tmp/bridge.log 2>&1 &

# --- Medic as foreground supervisor ---
echo "Starting clawpoint-medic..."
exec ./clawpoint-medic
