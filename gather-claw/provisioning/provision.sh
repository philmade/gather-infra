#!/bin/bash
set -euo pipefail

# Claw user provisioning script
# Usage: ./provision.sh <username> [--zai-key <key>] [--telegram-token <token>]
#                       [--telegram-chat-id <id>] [--pb-admin-token <token>]

CLAW_ROOT="/srv/claw"
USERS_DIR="$CLAW_ROOT/users"
NGINX_DIR="/etc/nginx/claw-users"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_DIR="$(dirname "$SCRIPT_DIR")"

POCKETBASE_URL="http://127.0.0.1:8090"
PB_ADMIN_TOKEN="${PB_ADMIN_TOKEN:-}"

# --- Parse arguments ---
USERNAME=""
ZAI_KEY=""
TELEGRAM_TOKEN=""
TELEGRAM_CHAT_ID=""

while [[ $# -gt 0 ]]; do
    case "$1" in
        --zai-key)          ZAI_KEY="$2"; shift 2 ;;
        --telegram-token)   TELEGRAM_TOKEN="$2"; shift 2 ;;
        --telegram-chat-id) TELEGRAM_CHAT_ID="$2"; shift 2 ;;
        --pb-admin-token)   PB_ADMIN_TOKEN="$2"; shift 2 ;;
        -*)                 echo "Unknown option: $1" >&2; exit 1 ;;
        *)
            if [ -z "$USERNAME" ]; then
                USERNAME="$1"
            else
                echo "Unexpected argument: $1" >&2; exit 1
            fi
            shift ;;
    esac
done

if [ -z "$USERNAME" ]; then
    echo "Usage: $0 <username> [--zai-key <key>] [--telegram-token <token>] [--telegram-chat-id <id>] [--pb-admin-token <token>]" >&2
    exit 1
fi

# --- Validate username ---
if ! [[ "$USERNAME" =~ ^[a-z0-9]{3,20}$ ]]; then
    echo "Error: Username must be 3-20 lowercase alphanumeric characters" >&2
    exit 1
fi

# --- PocketBase helpers ---
pb_api() {
    local method="$1" path="$2"
    shift 2
    curl -s -f -X "$method" \
        -H "Authorization: Bearer $PB_ADMIN_TOKEN" \
        -H "Content-Type: application/json" \
        "$POCKETBASE_URL$path" "$@"
}

# --- Check for duplicate workspace (if PB available) ---
if [ -n "$PB_ADMIN_TOKEN" ]; then
    EXISTING=$(pb_api GET "/api/collections/workspaces/records?filter=subdomain%3D'${USERNAME}'" 2>/dev/null || echo '{"totalItems":0}')
    if [ "$(echo "$EXISTING" | jq -r '.totalItems')" -gt 0 ]; then
        echo "Error: Workspace '$USERNAME' already exists in PocketBase" >&2
        exit 1
    fi
fi

# --- Allocate port ---
if [ -n "$PB_ADMIN_TOKEN" ]; then
    HIGHEST_PORT=$(pb_api GET "/api/collections/workspaces/records?sort=-port&perPage=1" 2>/dev/null | jq -r '.items[0].port // 9999')
else
    # Fallback: scan existing user dirs for ports
    HIGHEST_PORT=9999
    for compose_file in "$USERS_DIR"/*/docker-compose.yml; do
        if [ -f "$compose_file" ]; then
            port=$(grep -oP '127\.0\.0\.1:\K[0-9]+' "$compose_file" 2>/dev/null | head -1)
            if [ -n "$port" ] && [ "$port" -gt "$HIGHEST_PORT" ]; then
                HIGHEST_PORT=$port
            fi
        fi
    done
fi
PORT=$((HIGHEST_PORT + 1))

# --- Ensure directories ---
mkdir -p "$USERS_DIR" "$NGINX_DIR"

# --- Create user directory ---
USER_DIR="$USERS_DIR/$USERNAME"
if [ -d "$USER_DIR" ]; then
    echo "Error: User directory already exists: $USER_DIR" >&2
    exit 1
fi
mkdir -p "$USER_DIR/data" "$USER_DIR/soul"

# --- Copy default soul files ---
if [ -d "$SCRIPT_DIR/soul-template" ]; then
    cp "$SCRIPT_DIR/soul-template/"* "$USER_DIR/soul/" 2>/dev/null || true
    echo "  Default soul files copied"
fi

# --- Generate Gather identity ---
echo "Generating Gather agent identity..."
GATHER_PRIVATE_KEY=""
GATHER_PUBLIC_KEY=""
if command -v openssl > /dev/null 2>&1; then
    TMPKEY=$(mktemp)
    openssl genpkey -algorithm Ed25519 -out "$TMPKEY" 2>/dev/null
    openssl pkey -in "$TMPKEY" -pubout -out "${TMPKEY}.pub" 2>/dev/null
    GATHER_PRIVATE_KEY=$(base64 -w0 "$TMPKEY" 2>/dev/null || base64 "$TMPKEY")
    GATHER_PUBLIC_KEY=$(base64 -w0 "${TMPKEY}.pub" 2>/dev/null || base64 "${TMPKEY}.pub")
    rm -f "$TMPKEY" "${TMPKEY}.pub"
    echo "  Ed25519 keypair generated"
fi

# --- Write docker-compose ---
sed -e "s/__USERNAME__/$USERNAME/g" \
    -e "s/__PORT__/$PORT/g" \
    -e "s|__ZAI_API_KEY__|${ZAI_KEY:-}|g" \
    -e "s|__TELEGRAM_BOT_TOKEN__|${TELEGRAM_TOKEN:-}|g" \
    -e "s|__TELEGRAM_CHAT_ID__|${TELEGRAM_CHAT_ID:-}|g" \
    -e "s|__GATHER_PRIVATE_KEY__|${GATHER_PRIVATE_KEY}|g" \
    -e "s|__GATHER_PUBLIC_KEY__|${GATHER_PUBLIC_KEY}|g" \
    "$SCRIPT_DIR/docker-compose.user.yml.tpl" > "$USER_DIR/docker-compose.yml"

# --- Write NGINX config ---
sed -e "s/__USERNAME__/$USERNAME/g" \
    -e "s/__PORT__/$PORT/g" \
    "$SCRIPT_DIR/nginx-user.conf.tpl" > "$NGINX_DIR/$USERNAME.conf"

# --- Create workspace record in PocketBase (if available) ---
if [ -n "$PB_ADMIN_TOKEN" ]; then
    echo "Creating workspace record in PocketBase..."
    CONTAINER_NAME="claw-${USERNAME}"
    pb_api POST "/api/collections/workspaces/records" \
        -d "{
            \"subdomain\": \"$USERNAME\",
            \"port\": $PORT,
            \"server\": \"$(curl -s ifconfig.me 2>/dev/null || echo 'unknown')\",
            \"status\": \"provisioning\",
            \"container_id\": \"$CONTAINER_NAME\"
        }" > /dev/null
fi

# --- Start container ---
echo "Starting container for $USERNAME on port $PORT..."
cd "$USER_DIR" && docker compose up -d

# --- Update workspace status (if PB available) ---
if [ -n "$PB_ADMIN_TOKEN" ]; then
    WORKSPACE_ID=$(pb_api GET "/api/collections/workspaces/records?filter=subdomain%3D'${USERNAME}'" | jq -r '.items[0].id')
    pb_api PATCH "/api/collections/workspaces/records/$WORKSPACE_ID" \
        -d '{"status": "running"}' > /dev/null
fi

# --- Reload NGINX ---
echo "Reloading NGINX..."
nginx -s reload 2>/dev/null || echo "  (NGINX reload skipped — not running or no permission)"

# --- Print summary ---
echo ""
echo "========================================="
echo " Claw provisioned: $USERNAME"
echo "========================================="
echo " URL:       https://$USERNAME.claw.gather.is"
echo " ADK API:   http://127.0.0.1:$PORT"
echo " Port:      $PORT"
echo " Container: claw-$USERNAME"
echo "========================================="
