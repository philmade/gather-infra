# Agency - Tinode-Native Architecture

A fresh rewrite using Tinode as the source of truth for chat entities.

## Philosophy

**Tinode IS the database for chat entities.** Stop fighting it.

- **Old approach**: PocketBase stores workspaces, members, agents → Bridge syncs to Tinode
- **New approach**: Tinode stores everything chat-related → PocketBase only does OAuth

## Architecture

```
┌─────────────┐    ┌─────────────┐    ┌────────────────────┐
│   Frontend  │───▶│ PocketNode  │───▶│      Tinode        │
│  (Web App)  │    │ (Go + gRPC) │    │ (Source of Truth)  │
└─────────────┘    └─────────────┘    └────────────────────┘
                         │
                         │ WebSocket (/agency/connect)
                         ▼
                   ┌───────────┐
                   │Agency SDK │ ← Runs on developer's machine
                   │  (Python) │
                   └───────────┘
```

## Quick Start

1. Copy `.env.example` to `.env` and customize:
   ```bash
   cp .env.example .env
   ```

2. Start the services:
   ```bash
   docker compose up -d
   ```

3. On first run, initialize the database:
   ```bash
   # Set RESET_DB=true in .env for first run only
   docker compose up -d
   ```

4. Bootstrap PocketBase:
   ```bash
   curl -X POST http://localhost:8090/api/bootstrap
   ```

5. Access the services:
   - PocketBase Admin: http://localhost:8090/_/
   - Tinode WebSocket: ws://localhost:6060/v0/channels

## Components

| Component | Port | Description |
|-----------|------|-------------|
| **db** | 3306 | MySQL for Tinode storage |
| **tinode** | 6060, 16060 | Chat server (WebSocket + gRPC) |
| **pocketnode** | 8090 | PocketBase + Tinode integration |

## API Endpoints

### PocketNode

- `GET /api/health` - Health check
- `POST /api/bootstrap` - First-time setup
- `GET /api/tinode/credentials` - Get Tinode credentials (auth required)

## Tinode-Native Data Model

### User Types

```
usr (Tinode user account)
├── metadata.bot = false      → Human User (linked via PocketBase OAuth)
└── metadata.bot = true       → Agent Bot (deployed by workspace owner)
```

### Topic Types

```
grp (Tinode group topic)
├── metadata.type = "workspace"   → Workspace (the hub)
└── metadata.type = "channel"     → Channel (spoke within workspace)
```

## Development

### Build PocketNode locally

```bash
cd pocketnode
go build .
./pocketnode serve --http=0.0.0.0:8090
```

### Run tests

```bash
cd pocketnode
go test ./...
```

## Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `MYSQL_ROOT_PASSWORD` | MySQL root password | root |
| `POCKETBASE_ADMIN_EMAIL` | Initial admin email | - |
| `POCKETBASE_ADMIN_PASSWORD` | Initial admin password | - |
| `TINODE_API_KEY` | Tinode API key for account creation | (built-in) |
| `TINODE_PASSWORD_SECRET` | Secret for password derivation | agency_tinode_sync_v1 |
| `RESET_DB` | Reset Tinode DB on startup | false |
