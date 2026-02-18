#!/bin/bash
set -euo pipefail

# Claw host setup script (one-time, run on server)
# Installs NGINX, certbot, creates directory structure, builds Docker image

CLAW_ROOT="/srv/claw"
REPO_DIR="${1:-/opt/gather-infra/gather-claw}"
DOMAIN="claw.gather.is"

echo "=== Claw Host Setup ==="

# --- Install NGINX + Certbot ---
echo "Installing NGINX and Certbot..."
apt-get update
apt-get install -y nginx certbot python3-certbot-dns-cloudflare

# --- Create directory structure ---
echo "Creating directories..."
mkdir -p "$CLAW_ROOT/users"
mkdir -p /etc/nginx/claw-users

# --- Write main NGINX config ---
echo "Writing NGINX config..."
cat > /etc/nginx/sites-available/claw <<'NGINX'
# HTTP -> HTTPS redirect
server {
    listen 80;
    server_name *.claw.gather.is;
    return 301 https://$host$request_uri;
}

# Include per-user configs
include /etc/nginx/claw-users/*.conf;

# Catch-all for unknown subdomains
server {
    listen 443 ssl http2 default_server;
    server_name *.claw.gather.is;
    ssl_certificate /etc/letsencrypt/live/claw.gather.is/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/claw.gather.is/privkey.pem;
    return 404;
}
NGINX

ln -sf /etc/nginx/sites-available/claw /etc/nginx/sites-enabled/claw
rm -f /etc/nginx/sites-enabled/default

# --- SSL cert instructions ---
echo ""
echo "=== SSL Certificate ==="
echo "1. Set up DNS: *.claw.gather.is A -> $(curl -s ifconfig.me)"
echo ""
echo "2. Create /etc/letsencrypt/cloudflare.ini with:"
echo "   dns_cloudflare_api_token = <your-cloudflare-token>"
echo "   chmod 600 /etc/letsencrypt/cloudflare.ini"
echo ""
echo "3. Obtain wildcard cert:"
echo "   certbot certonly --dns-cloudflare --dns-cloudflare-credentials /etc/letsencrypt/cloudflare.ini -d $DOMAIN -d *.$DOMAIN"
echo ""

# --- Build Docker image ---
if [ -d "$REPO_DIR" ]; then
    echo "Building Docker image..."
    cd "$REPO_DIR" && docker build -t gather-claw:latest .
fi

# --- Start NGINX ---
echo "Testing and starting NGINX..."
nginx -t && systemctl enable nginx && systemctl restart nginx

echo ""
echo "=== Setup Complete ==="
echo "Next steps:"
echo "  1. Configure DNS and SSL (see above)"
echo "  2. Sync clawpoint-go source into $REPO_DIR/clawpoint-go/"
echo "  3. Build image: cd $REPO_DIR && docker build -t gather-claw:latest ."
echo "  4. Provision a claw: ./provisioning/provision.sh <username> --zai-key <key> --telegram-token <token>"
