#!/bin/bash
# claw-daemon — bridges Gather channel messages to PicoClaw AI agent
# Polls the claw's Gather channel for user messages, passes them to
# `picoclaw chat`, and posts the response back.
#
# Required env:
#   GATHER_CHANNEL_ID   — channel to watch
# Optional env:
#   LLM_API_KEY         — API key for PicoClaw's LLM provider
#   LLM_API_URL         — OpenAI-compatible base URL (default: https://open.z.ai/v1)
#   LLM_MODEL           — model name (default: glm-5)
#   CLAW_SYSTEM_PROMPT  — system prompt for the agent
#   CLAW_NAME           — display name

set -euo pipefail

CHANNEL_ID="${GATHER_CHANNEL_ID:-}"
CLAW="${CLAW_NAME:-Claw}"
POLL_INTERVAL=3
LOG="/tmp/claw-daemon.log"

log() { echo "[$(date '+%H:%M:%S')] $*" >> "$LOG"; }

if [ -z "$CHANNEL_ID" ]; then
    log "ERROR: GATHER_CHANNEL_ID not set, daemon exiting"
    exit 0
fi

# Wait for gather auth to be ready
sleep 2
gather auth > /dev/null 2>&1 || {
    log "ERROR: gather auth failed, daemon exiting"
    exit 0
}

# Set up PicoClaw config if LLM key is available
PICOCLAW_DIR="/root/.picoclaw"
mkdir -p "$PICOCLAW_DIR"

LLM_API_KEY="${LLM_API_KEY:-}"
LLM_API_URL="${LLM_API_URL:-https://open.z.ai/v1}"
LLM_MODEL="${LLM_MODEL:-glm-5}"
SYSTEM_PROMPT="${CLAW_SYSTEM_PROMPT:-You are ${CLAW}, a helpful AI assistant running inside a Gather workspace. Be concise and helpful.}"

if [ -n "$LLM_API_KEY" ]; then
    cat > "$PICOCLAW_DIR/config.json" << CONF
{
  "agents": {
    "default": {
      "provider": "llm",
      "model": "$LLM_MODEL",
      "system_prompt": $(printf '%s' "$SYSTEM_PROMPT" | jq -Rs .)
    }
  },
  "providers": {
    "llm": {
      "type": "openai",
      "api_key": "$LLM_API_KEY",
      "base_url": "$LLM_API_URL"
    }
  }
}
CONF
    HAS_LLM=true
    log "PicoClaw configured: model=$LLM_MODEL url=$LLM_API_URL"
else
    HAS_LLM=false
    log "WARN: No LLM_API_KEY — will echo messages only"
fi

# Generate response for a user message
respond() {
    local message="$1"

    if [ "$HAS_LLM" = "true" ] && command -v picoclaw > /dev/null 2>&1; then
        # Use PicoClaw for AI response
        local reply
        reply=$(timeout 30 picoclaw chat "$message" 2>/dev/null) || reply=""
        if [ -n "$reply" ]; then
            echo "$reply"
            return
        fi
        log "PicoClaw returned empty, falling back"
    fi

    # Fallback: echo mode
    echo "Received: $message (LLM not configured — set LLM_API_KEY to enable AI responses)"
}

# REST API helpers
BASE_URL="${GATHER_BASE_URL:-https://gather.is}"

get_token() {
    cat /tmp/gather_jwt.txt 2>/dev/null || echo ""
}

TOKEN=$(get_token)
last_refresh=$(date +%s)

refresh_token() {
    local now
    now=$(date +%s)
    if [ $((now - last_refresh)) -gt 3000 ]; then
        gather auth > /dev/null 2>&1 || true
        TOKEN=$(get_token)
        last_refresh=$now
        log "Token refreshed"
    fi
}

# Initialize watermark to now (only respond to NEW messages, not history)
watermark=$(date -u '+%Y-%m-%d %H:%M:%S.000Z')
log "Daemon started: channel=$CHANNEL_ID (watermark: $watermark)"

while true; do
    refresh_token

    # Fetch new messages since watermark
    encoded_wm=$(printf '%s' "$watermark" | jq -Rr @uri)
    response=$(curl -s --max-time 10 \
        "${BASE_URL}/api/channels/${CHANNEL_ID}/messages?limit=10&since=${encoded_wm}" \
        -H "Authorization: Bearer $TOKEN" 2>/dev/null || echo "{}")

    messages=$(echo "$response" | jq -r '.messages // []' 2>/dev/null)
    count=$(echo "$messages" | jq 'length' 2>/dev/null || echo "0")

    if [ "$count" != "0" ] && [ "$count" != "null" ]; then
        # Process oldest first (API returns newest first)
        for i in $(seq $((count - 1)) -1 0); do
            author_id=$(echo "$messages" | jq -r ".[$i].author_id" 2>/dev/null)
            author_name=$(echo "$messages" | jq -r ".[$i].author_name" 2>/dev/null)
            body=$(echo "$messages" | jq -r ".[$i].body" 2>/dev/null)
            created=$(echo "$messages" | jq -r ".[$i].created" 2>/dev/null)

            # Only respond to user messages (author_id starts with "user:")
            if [[ "$author_id" == user:* ]]; then
                log "Message from $author_name: $body"

                reply=$(respond "$body")
                log "Reply: $reply"

                # Post response back to channel
                post_payload=$(jq -n --arg body "$reply" '{"body": $body}')
                curl -s --max-time 10 \
                    "${BASE_URL}/api/channels/${CHANNEL_ID}/messages" \
                    -H "Authorization: Bearer $TOKEN" \
                    -H "Content-Type: application/json" \
                    -d "$post_payload" > /dev/null 2>&1
            fi

            watermark="$created"
        done
    fi

    sleep "$POLL_INTERVAL"
done
