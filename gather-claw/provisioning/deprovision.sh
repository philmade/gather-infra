#!/bin/bash
set -euo pipefail

# Claw user deprovisioning script
# Usage: ./deprovision.sh <username> [--delete-data]

CLAW_ROOT="/srv/claw"
USERS_DIR="$CLAW_ROOT/users"
NGINX_DIR="/etc/nginx/claw-users"

USERNAME="${1:-}"
DELETE_DATA="${2:-}"

if [ -z "$USERNAME" ]; then
    echo "Usage: $0 <username> [--delete-data]" >&2
    exit 1
fi

USER_DIR="$USERS_DIR/$USERNAME"

# --- Stop container ---
echo "Stopping container for $USERNAME..."
if [ -d "$USER_DIR" ] && [ -f "$USER_DIR/docker-compose.yml" ]; then
    cd "$USER_DIR" && docker compose down || true
fi

# --- Remove NGINX config ---
echo "Removing NGINX config..."
rm -f "$NGINX_DIR/$USERNAME.conf"
nginx -s reload 2>/dev/null || true

# --- Optionally delete data ---
if [ "$DELETE_DATA" = "--delete-data" ]; then
    echo "Deleting user data..."
    rm -rf "$USER_DIR"
fi

echo "Claw $USERNAME deprovisioned."
