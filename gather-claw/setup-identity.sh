#!/bin/bash
# Runs at container start (Docker ENTRYPOINT) — sets up Gather identity,
# starts PicoClaw agent, then execs into ttyd.

# --- Set up Gather agent identity from env vars ---
if [ -n "$GATHER_PRIVATE_KEY" ]; then
    mkdir -p /root/.gather/keys
    echo "$GATHER_PRIVATE_KEY" | base64 -d > /root/.gather/keys/claw-private.pem
    chmod 600 /root/.gather/keys/claw-private.pem
    echo "$GATHER_PUBLIC_KEY" | base64 -d > /root/.gather/keys/claw-public.pem
    cat > /root/.gather/config.json << CONF
{"base_url": "${GATHER_BASE_URL:-https://gather.is}", "key_name": "claw"}
CONF
    # Pre-authenticate so JWT is cached for immediate use
    gather auth > /dev/null 2>&1 || true
fi

# --- Write PicoClaw config ---
PICOCLAW_DIR=/root/.picoclaw
mkdir -p "$PICOCLAW_DIR"

LLM_API_KEY="${CLAW_LLM_API_KEY:-}"
LLM_API_URL="${CLAW_LLM_API_URL:-}"
LLM_MODEL="${CLAW_LLM_MODEL:-glm-4.7}"

cat > "$PICOCLAW_DIR/config.json" << PCONF
{
  "agents": {
    "defaults": {
      "provider": "openrouter",
      "model": "$LLM_MODEL",
      "max_tokens": 4096,
      "temperature": 0.7
    }
  },
  "channels": {
    "gather": {
      "enabled": true,
      "base_url": "${GATHER_BASE_URL:-https://gather.is}",
      "channel_id": "${GATHER_CHANNEL_ID:-}",
      "poll_interval": 3
    }
  },
  "providers": {
    "openrouter": {
      "api_key": "$LLM_API_KEY",
      "api_base": "$LLM_API_URL"
    }
  }
}
PCONF

# Start PicoClaw gateway in background (Gather channel adapter handles messaging)
if [ -n "$GATHER_CHANNEL_ID" ] && [ -n "$LLM_API_KEY" ] && [ -x /usr/local/bin/picoclaw ]; then
    picoclaw gateway > /tmp/picoclaw.log 2>&1 &
    echo "PicoClaw agent started (PID $!, log: /tmp/picoclaw.log)"
fi

# Hand off to CMD (ttyd)
exec "$@"
