# Claw Provisioning

Scripts for managing ClawPoint-Go agent containers on a server.

## Host Setup (one-time)

```bash
sudo ./setup-host.sh /opt/gather-infra/gather-claw
```

This installs NGINX, certbot, creates the directory structure, and builds the Docker image.

### DNS Setup

Point these records to your server IP:
```
*.claw.gather.is   A   <server-ip>
claw.gather.is     A   <server-ip>
```

### SSL Certificate

Using Let's Encrypt wildcard cert via Cloudflare DNS challenge:
```bash
certbot certonly --dns-cloudflare \
  --dns-cloudflare-credentials /etc/letsencrypt/cloudflare.ini \
  -d claw.gather.is -d *.claw.gather.is
```

## Provisioning a Claw

```bash
./provision.sh <username> [options]

Options:
  --zai-key <key>            z.ai API key (Anthropic-compatible)
  --telegram-token <token>   Telegram bot token
  --telegram-chat-id <id>    Telegram chat ID for matterbridge
  --pb-admin-token <token>   PocketBase admin token (optional)
```

Example:
```bash
./provision.sh newbo --zai-key "abc123" --telegram-token "123:ABC" --telegram-chat-id "-100123"
```

This creates the user directory, Docker container, and NGINX config. The ADK API is accessible at `https://newbo.claw.gather.is`.

## Deprovisioning

```bash
# Stop container, remove NGINX config (keeps data)
./deprovision.sh username

# Stop container, remove NGINX config AND delete all data
./deprovision.sh username --delete-data
```

## Automated Provisioner

The `claw-provisioner.sh` script polls gather-auth for pending deployments:
```bash
CLAW_PROVISIONER_KEY=secret ./claw-provisioner.sh --loop
```

## Directory Layout (on host)

```
/srv/claw/users/<username>/
  docker-compose.yml       # Per-user compose (generated from template)
  data/                    # Mounted to /app/data (persistent memory, wallet)
  soul/                    # Mounted to /app/soul (SOUL.md, IDENTITY.md)

/etc/nginx/claw-users/
  <username>.conf          # Per-user NGINX reverse proxy
```

## Resource Limits

Each container gets:
- **512 MB RAM** (`mem_limit`)
- **1 CPU core** (`cpus`)

A CX41 (16 GB RAM, 4 vCPU) supports ~30 concurrent claws.
