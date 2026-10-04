# Development

## Prerequisites

- Go (version in `go.mod`)
- Node.js 24+
- PostgreSQL 17 with PostGIS

## Local Setup

The fastest way to get a running stack is Docker Compose:

```bash
docker compose -f docker-compose.yaml -f docker-compose.dev.yaml up -d --build --wait
```

This starts: PostGIS → migrations → admin seed → Motus server on http://localhost:8080.

For active development with hot-reload, run the backend and frontend separately:

```bash
# Terminal 1: Start only the database
docker compose -f docker-compose.yaml -f docker-compose.dev.yaml up db -d --wait

# Terminal 2: Run migrations and start backend
make build
./bin/motus db-migrate up
./bin/motus serve

# Terminal 3: Start frontend dev server
cd web
npm install
npm run dev
```

Frontend dev server: http://localhost:5173 (proxies API to :8080).

### Creating an admin user

```bash
./bin/motus user add --email admin@motus.local --name "Admin" --password admin --role admin
```

## Project Structure

```
motus/
├── cmd/motus/              Main binary (serve, import, replay, user, device, db-migrate)
├── internal/
│   ├── api/                HTTP handlers, middleware, router
│   ├── audit/              Audit logging
│   ├── config/             Configuration (all env var loading)
│   ├── demo/               Demo mode GPS simulation
│   ├── model/              Data models
│   ├── notification/       Webhook sender, template engine
│   ├── protocol/           GPS protocol decoders (H02, WATCH)
│   ├── services/           Business logic (events, timeouts)
│   ├── storage/            Database repositories
│   ├── version/            Build version info
│   └── websocket/          WebSocket hub
├── web/
│   ├── src/
│   │   ├── lib/            Components, API client, stores, utilities
│   │   └── routes/         SvelteKit pages
│   └── tests/
│       ├── e2e/            Playwright E2E tests
│       ├── fixtures/       Test fixtures (auth, test data)
│       └── page-objects/   Page object models
├── migrations/             Database migrations (goose, embedded via go:embed)
├── charts/motus/           Helm chart for Kubernetes
└── docs/                   Additional documentation
```

## Testing

### Backend (Go)

```bash
# All tests with race detector
go test -race ./...

# With coverage
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out

# Specific package
go test ./internal/storage/repository -v
```

### Frontend (Playwright E2E)

```bash
cd web

# Run all E2E tests (requires running stack)
npx playwright test

# Specific test file
npx playwright test tests/e2e/devices.spec.ts

# With UI mode for debugging
npx playwright test --ui
```

### Makefile Targets

Run `make help` to list all targets.

## API Reference

[`docs/openapi.yaml`](docs/openapi.yaml) is the source of truth. A running server serves
interactive docs at `/api/docs`. Live updates stream over the WebSocket at `/api/socket`.

## Data Model

### Tables

| Table | Description |
|-------|-------------|
| `users` | User accounts with roles |
| `devices` | GPS devices with status and speed limits |
| `positions` | GPS positions (partitioned by month) |
| `sessions` | Authentication sessions |
| `geofences` | Geofence polygons (PostGIS GEOMETRY) |
| `events` | Geofence/device/overspeed/motion/idle events |
| `notification_rules` | Notification configurations |
| `notification_log` | Delivery tracking |
| `audit_log` | Admin and user action audit trail |
| `api_keys` | API key management |

### Relationships

- `user_devices` — many-to-many user ↔ device assignments
- `user_geofences` — many-to-many user ↔ geofence associations

### Notes

- Positions are partitioned by month (`00022_partition_positions.sql`) for efficient retention
- The `users` table has **no** `updated_at` column
- Migrations use [goose](https://github.com/pressly/goose) and are embedded via `//go:embed`
- PostGIS is required for geofence geometry columns
