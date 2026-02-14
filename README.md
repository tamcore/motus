# Motus - GPS Tracking System

[![Tests](https://github.com/tamcore/motus/actions/workflows/test.yaml/badge.svg)](https://github.com/tamcore/motus/actions/workflows/test.yaml) [![E2E](https://github.com/tamcore/motus/actions/workflows/e2e.yaml/badge.svg)](https://github.com/tamcore/motus/actions/workflows/e2e.yaml) [![Go](https://img.shields.io/github/go-mod/go-version/tamcore/motus)](https://github.com/tamcore/motus/blob/master/go.mod) [![License](https://img.shields.io/github/license/tamcore/motus)](https://github.com/tamcore/motus/blob/master/LICENSE)

A production-ready GPS tracking system with real-time updates, geofencing, notifications, and comprehensive reporting. Compatible with the Traccar API for use with Home Assistant and Traccar mobile apps.

## Features

- **GPS Tracking** — H02 and WATCH protocol support, real-time WebSocket updates, device status monitoring
- **Geofencing** — Draw polygons/rectangles/circles on a map, real-time enter/exit detection via PostGIS
- **Notifications** — Webhook delivery with template variables, event types: geofence, online/offline, overspeed, motion, idle
- **Reports** — Trip detection, route playback with animation, heatmaps, distance charts, CSV/GPX export
- **Security** — Session cookies + Bearer tokens, RBAC (admin/user/readonly), CSRF protection, audit logging
- **UI** — Dark/light themes, mobile responsive, metric/imperial units, timezone preferences

## Quick Start

### Docker Compose

```bash
docker compose -f docker-compose.dev.yml up -d --build --wait
```

Starts the full stack: PostGIS → migrations → admin user seed → Motus server.

Visit: http://localhost:8080
Credentials: `admin@motus.local` / `admin`

### Kubernetes (Helm)

```bash
helm install motus ./charts/motus \
  --namespace motus --create-namespace \
  --set externalDatabase.host=your-pg-host.example.com \
  --set externalDatabase.password=your-secure-password \
  --set ingress.hosts[0].host=motus.example.com \
  --set config.csrf.secret=$(openssl rand -hex 32)
```

See [charts/motus/README.md](charts/motus/README.md) for full Helm chart documentation.

## Configuration

All configuration is via environment variables.

| Variable | Default | Description |
|----------|---------|-------------|
| `MOTUS_DATABASE_HOST` | `localhost` | PostgreSQL host |
| `MOTUS_DATABASE_PORT` | `5432` | PostgreSQL port |
| `MOTUS_DATABASE_NAME` | `motus` | Database name |
| `MOTUS_DATABASE_USER` | `motus` | Database user |
| `MOTUS_DATABASE_PASSWORD` | — | Database password |
| `MOTUS_DATABASE_SSLMODE` | `disable` | SSL mode |
| `POSTGRES_URI` | — | Full connection string (overrides individual params) |
| `MOTUS_SERVER_PORT` | `8080` | HTTP server port |
| `MOTUS_GPS_H02_PORT` | `5013` | H02 GPS protocol port |
| `MOTUS_GPS_WATCH_PORT` | `5093` | WATCH GPS protocol port |
| `MOTUS_DEVICE_TIMEOUT_MINUTES` | `5` | Device offline timeout |
| `MOTUS_DEVICE_CHECK_INTERVAL_MINUTES` | `1` | Timeout check interval |
| `MOTUS_WS_ALLOWED_ORIGINS` | — | Comma-separated WebSocket origins |
| `MOTUS_POSITION_RETENTION_DAYS` | `0` (disabled) | Auto-drop position partitions older than N days |
| `MOTUS_CSRF_SECRET` | — | **Required in production.** 32-byte hex (`openssl rand -hex 32`) |
| `MOTUS_ENV` | `production` | `production` or `development` (affects cookie security) |
| `MOTUS_LOGIN_RATE_LIMIT` | `5` | Login attempts per minute per IP |
| `MOTUS_API_RATE_LIMIT` | `60` | API requests per minute per IP |
| `MOTUS_DEMO_ENABLED` | `false` | Enable demo mode with simulated GPS tracks |
| `MOTUS_DEMO_DEVICE_IMEIS` | — | Comma-separated demo device identifiers |

## CLI

```
motus serve                                     # Start HTTP + GPS servers
motus db-migrate [up|down|status]               # Run database migrations
motus user add --email --name --password --role  # Create user
motus user list                                  # List users
motus device add --uid --name                    # Register device
motus wait-for-db                                # Block until DB is reachable
motus import --dump=... --target-host=...        # Import from Traccar dump
motus replay --input=... --host=... --port=...   # Simulate GPS traffic from logs
motus version                                    # Print version
```

See [docs/import.md](docs/import.md) and [docs/replay.md](docs/replay.md) for tool documentation.

## License

MIT License

## Credits

Built with Go, SvelteKit, PostgreSQL/PostGIS, Leaflet, Playwright, Helm.

---

For development setup, API documentation, testing, and project structure, see **[DEVELOPMENT.md](DEVELOPMENT.md)**.
