#!/bin/bash
set -euo pipefail

# Patch a running claw container to the latest gather-claw:latest image.
# Captures the container's full config (env, labels, volumes, network)
# and recreates it with the new image.
#
# Usage:
#   ./patch.sh <username>              # patch a single claw
#   ./patch.sh --all                   # patch all claw-* containers
#   ./patch.sh --build <username>      # rebuild image first, then patch
#   ./patch.sh --build --all           # rebuild image, then patch all

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_DIR="$(dirname "$SCRIPT_DIR")"
IMAGE="gather-claw:latest"

BUILD_FIRST=false
PATCH_ALL=false
USERNAME=""

while [[ $# -gt 0 ]]; do
    case "$1" in
        --build) BUILD_FIRST=true; shift ;;
        --all)   PATCH_ALL=true; shift ;;
        -*)      echo "Unknown option: $1" >&2; exit 1 ;;
        *)
            if [ -z "$USERNAME" ]; then
                USERNAME="$1"
            else
                echo "Unexpected argument: $1" >&2; exit 1
            fi
            shift ;;
    esac
done

if [ "$PATCH_ALL" = false ] && [ -z "$USERNAME" ]; then
    echo "Usage: $0 [--build] <username>"
    echo "       $0 [--build] --all"
    exit 1
fi

# --- Rebuild image if requested ---
if [ "$BUILD_FIRST" = true ]; then
    echo "[PATCH] Building $IMAGE from $REPO_DIR ..."
    docker build -t "$IMAGE" "$REPO_DIR"
    echo "[PATCH] Image built."
fi

# --- Patch a single container ---
patch_container() {
    local CONTAINER="$1"

    echo ""
    echo "============================================"
    echo "[PATCH] Patching: $CONTAINER"
    echo "============================================"

    # Check it exists
    if ! docker inspect "$CONTAINER" > /dev/null 2>&1; then
        echo "[PATCH] ERROR: Container $CONTAINER not found. Skipping."
        return 1
    fi

    # Extract config via docker inspect
    echo "[PATCH] Capturing config..."

    # Environment variables
    local ENV_ARGS=""
    while IFS= read -r env; do
        # Skip PATH — Docker sets this automatically
        if [[ "$env" == PATH=* ]]; then continue; fi
        ENV_ARGS="$ENV_ARGS -e $(printf '%q' "$env")"
    done < <(docker inspect "$CONTAINER" --format '{{range .Config.Env}}{{println .}}{{end}}' | grep -v '^$')

    # Labels
    local LABEL_ARGS=""
    while IFS= read -r label; do
        [ -z "$label" ] && continue
        LABEL_ARGS="$LABEL_ARGS -l $(printf '%q' "$label")"
    done < <(docker inspect "$CONTAINER" --format '{{range $k, $v := .Config.Labels}}{{$k}}={{$v}}{{"\n"}}{{end}}' | grep -v '^$')

    # Volume mounts
    local VOLUME_ARGS=""
    while IFS= read -r bind; do
        [ -z "$bind" ] && continue
        VOLUME_ARGS="$VOLUME_ARGS -v $bind"
    done < <(docker inspect "$CONTAINER" --format '{{range .HostConfig.Binds}}{{println .}}{{end}}' | grep -v '^$')

    # Network
    local NETWORK
    NETWORK=$(docker inspect "$CONTAINER" --format '{{range $k, $v := .NetworkSettings.Networks}}{{$k}}{{end}}')

    # Restart policy
    local RESTART
    RESTART=$(docker inspect "$CONTAINER" --format '{{.HostConfig.RestartPolicy.Name}}')
    if [ -z "$RESTART" ] || [ "$RESTART" = "no" ]; then
        RESTART="unless-stopped"
    fi

    # Add ForwardAuth to main claw router + explicit service link
    local USERNAME="${CONTAINER#claw-}"
    local MAIN_ROUTER="claw-${USERNAME}"
    local DEBUG_ROUTER="claw-${USERNAME}-debug"
    LABEL_ARGS="$LABEL_ARGS -l $(printf '%q' "traefik.http.routers.${MAIN_ROUTER}.middlewares=gather-forward-auth")"
    LABEL_ARGS="$LABEL_ARGS -l $(printf '%q' "traefik.http.routers.${MAIN_ROUTER}.service=${MAIN_ROUTER}")"

    # Debug: path-based (/debug) with ForwardAuth + StripPrefix → port 8081
    LABEL_ARGS="$LABEL_ARGS -l $(printf '%q' "traefik.http.routers.${DEBUG_ROUTER}.rule=Host(\`${USERNAME}.gather.is\`) && PathPrefix(\`/debug\`)")"
    LABEL_ARGS="$LABEL_ARGS -l $(printf '%q' "traefik.http.routers.${DEBUG_ROUTER}.entrypoints=websecure")"
    LABEL_ARGS="$LABEL_ARGS -l $(printf '%q' "traefik.http.routers.${DEBUG_ROUTER}.tls.certresolver=cf")"
    LABEL_ARGS="$LABEL_ARGS -l $(printf '%q' "traefik.http.routers.${DEBUG_ROUTER}.middlewares=gather-forward-auth,claw-debug-strip")"
    LABEL_ARGS="$LABEL_ARGS -l $(printf '%q' "traefik.http.routers.${DEBUG_ROUTER}.service=${DEBUG_ROUTER}")"
    LABEL_ARGS="$LABEL_ARGS -l $(printf '%q' "traefik.http.services.${DEBUG_ROUTER}.loadbalancer.server.port=8081")"

    echo "[PATCH] Config captured:"
    echo "  Network:  $NETWORK"
    echo "  Restart:  $RESTART"
    echo "  Volumes:  $(echo "$VOLUME_ARGS" | tr -s ' ')"
    echo "  Labels:   $(echo "$LABEL_ARGS" | wc -w) label args"
    echo "  Env vars: $(echo "$ENV_ARGS" | grep -o '\-e' | wc -l) vars"

    # Stop and remove
    echo "[PATCH] Stopping $CONTAINER..."
    docker stop "$CONTAINER" > /dev/null
    docker rm "$CONTAINER" > /dev/null

    # Recreate with new image
    echo "[PATCH] Starting $CONTAINER with $IMAGE..."
    local CMD="docker run -d --name $CONTAINER --network $NETWORK --restart $RESTART $VOLUME_ARGS $ENV_ARGS $LABEL_ARGS $IMAGE"
    eval $CMD > /dev/null

    # Wait and verify
    sleep 3
    local STATUS
    STATUS=$(docker ps --filter "name=^${CONTAINER}$" --format '{{.Status}}')
    if [ -z "$STATUS" ]; then
        echo "[PATCH] ERROR: $CONTAINER failed to start!"
        docker logs "$CONTAINER" 2>&1 | tail -20
        return 1
    fi

    echo "[PATCH] OK: $CONTAINER — $STATUS"
}

# --- Execute ---
if [ "$PATCH_ALL" = true ]; then
    echo "[PATCH] Finding all claw-* containers..."
    CONTAINERS=$(docker ps --format '{{.Names}}' | grep '^claw-' | grep -v '^claw-build-service$' | sort)
    if [ -z "$CONTAINERS" ]; then
        echo "[PATCH] No claw-* containers found."
        exit 0
    fi
    echo "[PATCH] Found: $CONTAINERS"
    FAILED=0
    for c in $CONTAINERS; do
        patch_container "$c" || FAILED=$((FAILED + 1))
    done
    echo ""
    echo "[PATCH] Done. Failures: $FAILED"
else
    CONTAINER="claw-${USERNAME}"
    patch_container "$CONTAINER"
fi
