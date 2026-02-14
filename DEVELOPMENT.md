# Development

## Prerequisites

- Go 1.26+
- Node.js 24+
- PostgreSQL 16 with PostGIS

## Local Setup

The fastest way to get a running stack is Docker Compose:

```bash
docker compose -f docker-compose.dev.yml up -d --build --wait
```

This starts: PostGIS → migrations → admin seed → Motus server on http://localhost:8080.

For active development with hot-reload, run the backend and frontend separately:

```bash
# Terminal 1: Start only the database
docker compose -f docker-compose.dev.yml up db -d --wait

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

| Target | Description |
|--------|-------------|
| `build` | Build motus binary |
| `lint` | Run all linters (go vet, golangci-lint, frontend check) |
| `test` | Run all tests (includes lint) |
| `dev-deploy-k8s` | Build dev image, push, deploy to K8s |
| `dev-reset-database` | Reset database (delete all data) |
| `dev-full-deploy` | Full dev deployment with data import |
| `clean` | Clean build artifacts |

## CI Workflows

| Workflow | Trigger | What it does |
|----------|---------|--------------|
| `test.yml` (Unit Tests) | push to master, PRs | `go test -race ./...` + Codecov |
| `e2e.yml` (E2E Tests) | push to master, PRs | Docker Compose stack → Playwright |
| `commit-lint.yml` | PRs only | Commitlint on PR commits |

## API Reference

### Authentication

**Login:**
```
POST /api/session
Content-Type: application/json

{"email": "user@example.com", "password": "password"}
→ User object + session cookie + X-CSRF-Token header
```

**Generate API Token:**
```
POST /api/session/token
Cookie: session_id=...
→ {"token": "abc123..."}
```

**Use Token:**
```
GET /api/devices
Authorization: Bearer abc123...
```

### Devices

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/devices` | List all devices |
| POST | `/api/devices` | Create device |
| GET | `/api/devices/{id}` | Get device |
| PUT | `/api/devices/{id}` | Update device |
| DELETE | `/api/devices/{id}` | Delete device |

### Positions

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/positions` | Latest positions for all devices |
| GET | `/api/positions?deviceId={id}&from={iso}&to={iso}&limit={n}` | Time-range query |

### Geofences

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/geofences` | List user's geofences |
| POST | `/api/geofences` | Create geofence (GeoJSON) |
| PUT | `/api/geofences/{id}` | Update geofence |
| DELETE | `/api/geofences/{id}` | Delete geofence |

### Notifications

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/notifications` | List notification rules |
| POST | `/api/notifications` | Create rule |
| PUT | `/api/notifications/{id}` | Update rule |
| DELETE | `/api/notifications/{id}` | Delete rule |
| POST | `/api/notifications/{id}/test` | Send test notification |

### Events

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/events` | List recent events |
| GET | `/api/events?deviceId={id}` | Filter by device |

### Users (Admin Only)

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/users` | List all users |
| POST | `/api/users` | Create user |
| PUT | `/api/users/{id}` | Update user |
| DELETE | `/api/users/{id}` | Delete user |
| GET | `/api/users/{id}/devices` | List user's devices |
| POST | `/api/users/{id}/devices/{deviceId}` | Assign device to user |
| DELETE | `/api/users/{id}/devices/{deviceId}` | Unassign device |

### WebSocket

```javascript
const ws = new WebSocket('wss://motus.example.com/api/socket');
ws.onmessage = (event) => {
  const data = JSON.parse(event.data);
  // {devices: [...], positions: [...], events: [...]}
};
```

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
