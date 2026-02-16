#!/bin/bash
# Runs at container start (Docker ENTRYPOINT) — sets up Gather identity
# then execs into ttyd so the terminal session has keys available.

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

# Hand off to CMD (ttyd)
exec "$@"
