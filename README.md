# Motus - GPS Tracking System

[![Tests](https://github.com/tamcore/motus/actions/workflows/test.yaml/badge.svg)](https://github.com/tamcore/motus/actions/workflows/test.yaml) [![E2E](https://github.com/tamcore/motus/actions/workflows/e2e.yaml/badge.svg)](https://github.com/tamcore/motus/actions/workflows/e2e.yaml) [![Go](https://img.shields.io/github/go-mod/go-version/tamcore/motus)](https://github.com/tamcore/motus/blob/master/go.mod) [![License](https://img.shields.io/github/license/tamcore/motus)](https://github.com/tamcore/motus/blob/master/LICENSE)

A production-ready GPS tracking system with real-time updates, geofencing, notifications, and comprehensive reporting. Compatible with the Traccar API for use with Home Assistant and Traccar mobile apps.

## Features

- **GPS Tracking** — H02, WATCH and OsmAnd / Traccar Client (Android/iOS) protocol support, real-time WebSocket updates, device status monitoring
- **Geofencing** — Draw polygons/rectangles/circles on a map, real-time enter/exit detection via PostGIS
- **Notifications** — Webhook delivery with template variables, event types: geofence, online/offline, overspeed, motion, idle
- **Reports** — Trip detection, route playback with animation, heatmaps, distance charts, CSV/GPX export
- **Security** — Session cookies + Bearer tokens, RBAC (admin/user/readonly), CSRF protection, audit logging
- **UI** — Dark/light themes, mobile responsive, metric/imperial units, timezone preferences
- **AI Assistant** — Natural-language control of geofences, calendars, notifications, and device queries via any OpenAI-compatible API (opt-in, requires API key). See [docs/ai-assistant.md](docs/ai-assistant.md).

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
| `MOTUS_GPS_OSMAND_PORT` | `5055` | OsmAnd / Traccar Client HTTP protocol port |
| `MOTUS_DEVICE_TIMEOUT_MINUTES` | `5` | Device offline timeout |
| `MOTUS_DEVICE_CHECK_INTERVAL_MINUTES` | `1` | Timeout check interval |
| `MOTUS_WS_ALLOWED_ORIGINS` | — | Comma-separated WebSocket origins |
| `MOTUS_POSITION_RETENTION_DAYS` | `0` (disabled) | Auto-drop position partitions older than N days |
| `MOTUS_CSRF_SECRET` | — | **Required in production.** 32-byte hex (`openssl rand -hex 32`) |
| `MOTUS_ENV` | `production` | `production` or `development` (affects cookie security) |
| `MOTUS_TRUSTED_PROXIES` | loopback + private ranges | Comma-separated IPs/CIDRs of reverse proxies whose `X-Forwarded-For` / `X-Real-Ip` set the client IP (rate limits, audit log). Set it to your proxy's address when motus is reachable from a private network without one |
| `MOTUS_LOGIN_RATE_LIMIT` | `5` | Login attempts per minute per IP |
| `MOTUS_API_RATE_LIMIT` | `60` | API requests per minute per IP |
| `MOTUS_DEMO_ENABLED` | `false` | Enable demo mode with simulated GPS tracks |
| `MOTUS_DEMO_DEVICE_IMEIS` | — | Comma-separated demo device identifiers |
| `MOTUS_AI_ENABLED` | `false` | Enable the AI assistant chat feature. See [docs/ai-assistant.md](docs/ai-assistant.md) for the full `MOTUS_AI_*` configuration reference. |

## Smartphone Tracking (Traccar Client)

The [Traccar Client](https://www.traccar.org/client/) apps for Android and iOS report
positions over the OsmAnd HTTP protocol on `MOTUS_GPS_OSMAND_PORT` (default `5055`).
Both the query-string format and the JSON format of current app versions are supported.

1. Create a device whose unique ID matches the app's **Device identifier**, or enable
   `MOTUS_DEVICE_AUTO_CREATE` to create devices on their first report.
2. In the app, set **Server URL** to `http://<gps-host>:5055` — the same host that serves
   the H02 and WATCH ports (with Helm, the `serviceGPS` LoadBalancer).
3. Set the app's reporting **frequency below `MOTUS_DEVICE_TIMEOUT_MINUTES`**. The app
   has no persistent connection, so devices only go offline via this timeout; with the
   app default of 300 s and the motus default of 5 minutes, devices flap between online
   and offline. Use e.g. 60–240 s, or raise the timeout.

Like H02 and WATCH, the port speaks plain HTTP without authentication; see the
network isolation notes for GPS ports in [AGENTS.md](AGENTS.md).

## CLI

```
motus serve                                     # Start HTTP + GPS servers
motus db-migrate [up|down|status]               # Run database migrations
motus user add --email --name --password --role  # Create user
motus user list                                  # List users
motus device add --unique-id --name [--user]     # Register device (assigned to --user)
motus wait-for-db                                # Block until DB is reachable
motus import --dump=... --target-host=...        # Import from Traccar dump
motus replay --input=... --host=... --port=...   # Simulate GPS traffic from logs
motus version                                    # Print version
```

See [docs/import.md](docs/import.md) and [docs/replay.md](docs/replay.md) for tool documentation.

See [docs/ai-assistant.md](docs/ai-assistant.md) and [docs/ai-mcp-tools.md](docs/ai-mcp-tools.md) for the AI assistant feature and its 16 MCP tools.

## License

MIT License

## Credits

Built with Go, SvelteKit, PostgreSQL/PostGIS, Leaflet, Playwright, Helm.

---

For development setup, API documentation, testing, and project structure, see **[DEVELOPMENT.md](DEVELOPMENT.md)**.
