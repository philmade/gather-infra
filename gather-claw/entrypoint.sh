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
    matterbridge -conf /app/matterbridge.toml > /var/log/matterbridge.log 2>&1 &
fi

# --- Soul files (copy defaults if not mounted) ---
if [ ! -f /app/soul/SOUL.md ]; then
    echo "# Soul" > /app/soul/SOUL.md
    echo "# Identity" > /app/soul/IDENTITY.md
fi

# --- Start clawpoint-go ---
echo "Starting clawpoint-go..."
./clawpoint-go web api webui > /var/log/adk-go.log 2>&1 &

# --- Start bridge (if matterbridge running) ---
if [ -n "$TELEGRAM_BOT" ]; then
    echo "Starting clawpoint-bridge..."
    ./clawpoint-bridge > /var/log/bridge.log 2>&1 &
fi

# --- Medic as foreground supervisor ---
echo "Starting clawpoint-medic..."
exec ./clawpoint-medic
