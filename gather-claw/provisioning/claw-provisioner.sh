#!/bin/bash
set -euo pipefail

# Claw Provisioner — polls gather-auth for "provisioning" deployments and
# creates the actual containers. Runs on the host (not in Docker).
#
# Usage:
#   CLAW_PROVISIONER_KEY=secret ./claw-provisioner.sh          # one-shot
#   CLAW_PROVISIONER_KEY=secret ./claw-provisioner.sh --loop   # poll every 30s
#
# Required env:
#   CLAW_PROVISIONER_KEY  — shared secret matching gather-auth's env var
#
# Optional env:
#   GATHER_API_URL        — default: http://127.0.0.1:8090
#   CLAW_ROOT             — default: /srv/claw
#   ZAI_API_KEY           — z.ai API key for clawpoint-go
#   TELEGRAM_BOT_TOKEN    — Telegram bot token
#   TELEGRAM_CHAT_ID      — Telegram chat ID

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

GATHER_API_URL="${GATHER_API_URL:-http://127.0.0.1:8090}"
CLAW_ROOT="${CLAW_ROOT:-/srv/claw}"
USERS_DIR="$CLAW_ROOT/users"
NGINX_DIR="/etc/nginx/claw-users"

if [ -z "${CLAW_PROVISIONER_KEY:-}" ]; then
    echo "Error: CLAW_PROVISIONER_KEY is required" >&2
    exit 1
fi

log() { echo "[$(date '+%H:%M:%S')] $*"; }

# Fetch pending claws from the API
fetch_pending() {
    curl -sf \
        -H "X-Provisioner-Key: $CLAW_PROVISIONER_KEY" \
        "$GATHER_API_URL/api/claws/pending" 2>/dev/null || echo '{"claws":null,"total":0}'
}

# Report provisioning result back to the API
report_result() {
    local id="$1" status="$2" container_id="${3:-}" error_msg="${4:-}"
    curl -sf -X POST \
        -H "X-Provisioner-Key: $CLAW_PROVISIONER_KEY" \
        -H "Content-Type: application/json" \
        "$GATHER_API_URL/api/claws/$id/provision-result" \
        -d "{\"status\":\"$status\",\"container_id\":\"$container_id\",\"error_message\":\"$error_msg\"}" \
        > /dev/null 2>&1
}

# Provision a single claw
provision_claw() {
    local id="$1" name="$2" subdomain="$3" port="$4" instructions="$5"

    log "Provisioning: $name (subdomain=$subdomain, port=$port)"

    local user_dir="$USERS_DIR/$subdomain"
    local container_name="claw-$subdomain"

    # Create directory structure
    mkdir -p "$user_dir/data" "$user_dir/soul"

    # Write instructions to soul if provided
    if [ -n "$instructions" ] && [ "$instructions" != "null" ]; then
        echo "$instructions" > "$user_dir/soul/SOUL.md"
    fi

    # Write docker-compose.yml from template
    sed -e "s/__USERNAME__/$subdomain/g" \
        -e "s/__PORT__/$port/g" \
        -e "s|__ZAI_API_KEY__|${ZAI_API_KEY:-}|g" \
        -e "s|__TELEGRAM_BOT_TOKEN__|${TELEGRAM_BOT_TOKEN:-}|g" \
        -e "s|__TELEGRAM_CHAT_ID__|${TELEGRAM_CHAT_ID:-}|g" \
        -e "s|__GATHER_PRIVATE_KEY__|${GATHER_PRIVATE_KEY:-}|g" \
        -e "s|__GATHER_PUBLIC_KEY__|${GATHER_PUBLIC_KEY:-}|g" \
        "$SCRIPT_DIR/docker-compose.user.yml.tpl" > "$user_dir/docker-compose.yml"

    # Write nginx config
    mkdir -p "$NGINX_DIR"
    sed -e "s/__USERNAME__/$subdomain/g" \
        -e "s/__PORT__/$port/g" \
        "$SCRIPT_DIR/nginx-user.conf.tpl" > "$NGINX_DIR/$subdomain.conf"

    # Start container
    cd "$user_dir" && docker compose up -d

    # Reload nginx
    nginx -s reload 2>/dev/null || nginx -t && systemctl reload nginx 2>/dev/null || true

    log "Provisioned: $name → https://$subdomain.claw.gather.is (port $port)"
    return 0
}

# Process all pending claws
process_pending() {
    local response
    response=$(fetch_pending)

    local total
    total=$(echo "$response" | jq -r '.total // 0')

    if [ "$total" -eq 0 ]; then
        return
    fi

    log "Found $total pending claw(s)"

    echo "$response" | jq -c '.claws[]?' | while read -r claw; do
        local id name subdomain port instructions
        id=$(echo "$claw" | jq -r '.id')
        name=$(echo "$claw" | jq -r '.name')
        subdomain=$(echo "$claw" | jq -r '.subdomain')
        port=$(echo "$claw" | jq -r '.port')
        instructions=$(echo "$claw" | jq -r '.instructions // empty')

        if [ -z "$subdomain" ] || [ "$subdomain" = "null" ]; then
            log "Skipping $id — no subdomain assigned"
            continue
        fi

        if provision_claw "$id" "$name" "$subdomain" "$port" "$instructions"; then
            report_result "$id" "running" "claw-$subdomain" ""
            log "Reported running: $id"
        else
            report_result "$id" "failed" "" "Container provisioning failed"
            log "Reported failed: $id"
        fi
    done
}

# Main
if [ "${1:-}" = "--loop" ]; then
    log "Starting provisioner loop (poll every 30s)"
    while true; do
        process_pending || log "Error during processing (continuing)"
        sleep 30
    done
else
    process_pending
fi
