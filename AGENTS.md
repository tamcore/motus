# AGENTS.md — Agent Guide for motus

## Project Overview

GPS tracking platform: Go backend (chi router) + SvelteKit frontend. Receives GPS device
positions via H02/Watch protocols, stores in PostgreSQL (PostGIS), serves a web UI with
maps, geofences, notifications, reports, and device management.

## Architecture

```
cmd/motus/          — CLI entrypoints (cobra): serve, db-migrate, user, device, import, etc.
internal/
  api/              — HTTP handlers, middleware (auth, CSRF, rate limiting)
  config/           — Config loading from env vars, validation
  protocol/         — GPS protocol servers (H02, Watch)
  model/            — Domain types
  store/            — PostgreSQL data access (pgx)
migrations/         — Goose SQL migrations (embedded via //go:embed)
web/
  src/              — SvelteKit app (routes, lib/components, lib/stores)
  tests/            — Playwright E2E tests
charts/motus/       — Helm chart (deployment, migration job, postgres, redis)
```

## Key Gotchas

### CSRF requires `MOTUS_ENV=development` for HTTP
`MOTUS_ENV` defaults to `"production"`. In production mode, the CSRF cookie gets the
`Secure` flag, meaning it's **only sent over HTTPS**. When running over HTTP (e.g.,
docker-compose E2E tests on `http://localhost:8080`), the CSRF cookie is silently dropped
by the browser, causing **all** state-changing API requests (POST/PUT/DELETE) to fail
with 403. Set `MOTUS_ENV=development` in docker-compose.dev.yml.

Additionally, gorilla/csrf's `Secure` option only controls the cookie flag — it does NOT
tell the Origin header check to use `http://` scheme. The middleware must also call
`gorillacsrf.PlaintextHTTPRequest(r)` when `Secure: false` to prevent "origin invalid"
errors when the browser sends `Origin: http://...`.
`config.LoadFromEnv()` validates ALL env vars (server port, GPS ports, DB, etc.) even
for CLI commands that only need the database. Any command using `connectDB()` (user, device,
keys) triggers full validation. If running in a context where only DB access is needed,
ensure ALL required env vars are set or the command will fail.

Required env vars for ANY CLI command that touches the DB:
- `MOTUS_DATABASE_HOST`, `MOTUS_DATABASE_PORT`, `MOTUS_DATABASE_USER`,
  `MOTUS_DATABASE_PASSWORD`, `MOTUS_DATABASE_NAME`, `MOTUS_DATABASE_SSLMODE`
- `MOTUS_SERVER_PORT`, `MOTUS_GPS_H02_PORT`, `MOTUS_GPS_WATCH_PORT`

### Migrations require PostGIS
Migration `00008_add_geofences.sql` uses `GEOMETRY(GEOMETRY, 4326)`. The database must
use a PostGIS-enabled image (e.g., `postgis/postgis:16-3.5-alpine`), not plain postgres.

### `motus serve` does NOT run migrations
Migrations are a separate step: `motus db-migrate` (uses goose). In docker-compose, this
runs as a oneshot init container before the main service starts.

### Pre-commit hooks run Go tests (short mode)
`.pre-commit-config.yaml` runs `go test -short`, `go vet`, `go fmt`, `golangci-lint`.
The `-short` flag skips integration tests that need Docker (PostGIS via testcontainers,
Redis via testcontainers). The skip is implemented in `testutil.SetupTestDB(t)` and
`setupRedis(t)` via `testing.Short()`. Full integration tests run in CI where Docker is
available.

### Frontend is embedded in the binary
The SvelteKit build output is embedded via `//go:embed all:build` in `web/embed.go`.
The router serves files from the embedded `fs.FS` — no filesystem path dependency.
`http.FileServer` redirects `/index.html` → `/`, so the SPA fallback writes `index.html`
directly to the response to avoid redirect loops. SvelteKit `adapter-static` emits
`route.html` files (e.g. `login.html`), so the router checks `path.html` and
`path/index.html` before falling back to `index.html`.

A `web/build/.gitkeep` placeholder ensures `go build` succeeds without running `npm run build`.
For goreleaser, `before.hooks` runs `npm ci` + `npm run build` to populate the build directory.

### Binary path in container
The motus binary is at `/motus` (not `/app/motus`). ENTRYPOINT is `["/motus"]`, CMD is
`["serve"]`.

### Session & CSRF
- Auth: httpOnly cookie `session_id`
- CSRF: gorilla/csrf double-submit cookie pattern
- Login endpoint is CSRF-exempt and returns CSRF token in response header

### H02 relay requires numeric-only device IMEIs
The H02 protocol server supports an optional relay mode (`MOTUS_GPS_H02_RELAY_TARGET`)
that forwards raw GPS messages to an external system (e.g., Traccar) before decoding.
Traccar's H02 protocol decoder only accepts numeric device identifiers — alphanumeric
IMEIs are silently dropped. Demo device IMEIs must be numeric (default: `9000000000001`,
`9000000000002`). The relay is fail-open: if the relay target is unreachable or drops,
motus continues serving devices normally.

### Demo device cleanup uses configured IMEIs
`demo.Reset()` accepts a `deviceIMEIs []string` parameter and uses `unique_id = ANY($1)`
to identify demo devices for cleanup. The default IMEIs are exported as
`demo.DefaultDeviceIMEIs`. If you change the demo IMEIs via `MOTUS_DEMO_DEVICE_IMEIS`,
the reset logic must receive the matching values.

## Development

### Docker Compose (E2E / local dev)

```bash
docker compose -f docker-compose.dev.yml up -d --build --wait
```

Starts: `db` (PostGIS) → `migrate` (oneshot) → `seed` (oneshot, creates admin user) → `motus`

All services share a YAML anchor (`x-motus-env`) for environment variables.
Rate limits are set high (1000/10000) for testing.

Default credentials: `admin@motus.local` / `admin`

### Kubernetes deployment

```bash
make dev-deploy-k8s
```

Deploys to kube-context `<KUBE_CONTEXT>`, namespace `<KUBE_NAMESPACE>`. Live at `<MOTUS_URL>`.

See `AGENTS.md.local` for actual values (not tracked in git).

### Running tests

```bash
# Go unit tests (with race detector)
go test -race ./...

# E2E tests (requires running stack)
cd web && npx playwright test

# Lint everything
make lint
```

### TDD Methodology (100% Test-Driven)

All changes MUST follow strict TDD: **write tests first, then implement**.

#### Workflow

1. **Red**: Write a failing test that describes the expected behaviour.
2. **Green**: Write the minimum code to make the test pass.
3. **Refactor**: Clean up while keeping tests green.
4. Repeat until the feature/fix is complete.

#### Go Backend

- **Unit tests**: stdlib `testing` + `httptest`, colocated as `*_test.go`.
- **Integration tests**: testcontainers (`postgis/postgis:16-3.4`). Skipped with `-short`.
- **Test utilities**: `internal/storage/repository/testutil/` — `SetupTestDB`, `CleanTables`, `Cleanup`.
- **TestMain pattern**: Every package with integration tests has a `main_test.go` calling `testutil.Cleanup()`.
- **Mock pattern**: Handler tests use real repos against testcontainers DB. Mock only external services.
- **HTTP tests**: Use `httptest.NewRequest` + `httptest.NewRecorder` with `api.ContextWithUser` for auth.

```go
// Example: write test FIRST
func TestMyHandler_NewBehaviour(t *testing.T) {
    pool := testutil.SetupTestDB(t)
    testutil.CleanTables(t, pool)
    // ... setup, call handler, assert response
}
```

#### SvelteKit Frontend

- **Unit tests**: Vitest + jsdom + `@testing-library/svelte` in `web/src/tests/`.
- **E2E tests**: Playwright in `web/tests/e2e/` with page objects in `web/tests/page-objects/`.
- **Mocking**: `vi.mock()` for API client, stores, and SvelteKit modules (`$app/environment`, etc.).
- **Run unit tests**: `cd web && npm run test:unit`
- **Run E2E tests**: `cd web && npx playwright test` (requires running stack)
- **E2E test requirement**: UI changes (new components, layout changes, new pages) MUST
  include Playwright E2E tests. Do NOT run E2E tests locally — they run in CI via the
  `e2e.yaml` GitHub Actions workflow. Write the tests, push, and verify via `gh run watch`.

```typescript
// Example: write test FIRST
describe('NewFeature', () => {
  it('should behave as expected', () => {
    // arrange, act, assert
  });
});
```

#### Test commands

```bash
go test -short ./...          # Local: unit tests only (no Docker)
go test -race ./...           # CI: all tests including integration
cd web && npm run test:unit   # Frontend unit tests
cd web && npx playwright test # E2E tests (needs running stack)
make lint                     # All linters
```

### Makefile targets

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

## E2E Test Patterns

### Auth setup
Playwright uses a **storageState** pattern. `tests/auth.setup.ts` logs in once and saves
cookies to `web/.auth/user.json`. The `chromium` project depends on this setup project.
The `auth` project runs with empty storageState for login page tests.

**Important**: storageState file must be outside `test-results/` (Playwright clears it).

### Strict mode violations
The UI has responsive layouts with **desktop tables + mobile cards** showing the same data.
Locators like `text=email` will match both views → strict mode violation.
Always scope to the desktop table: `page.locator('table.users-table').locator(...)`.

### Rate limiting
Rate limits are configurable via `MOTUS_LOGIN_RATE_LIMIT` and `MOTUS_API_RATE_LIMIT` env
vars (requests per minute, token bucket per IP). Set high in docker-compose for E2E to
avoid cascade failures from the auth setup's login request.

### Fetch mocking in E2E tests
**`page.route()` does NOT reliably intercept SvelteKit client-side fetches.** Use
`page.addInitScript()` to monkeypatch `window.fetch` instead. This injects the mock
before SvelteKit hydration, guaranteeing interception. Pattern:
```typescript
await page.addInitScript(() => {
  const origFetch = window.fetch;
  window.fetch = async function (input, init) {
    const url = typeof input === 'string' ? input : input instanceof URL ? input.href : input.url;
    if (url.includes('/api/target-endpoint')) {
      return new Response('[]', { status: 200, headers: { 'Content-Type': 'application/json' } });
    }
    return origFetch.call(this, input, init);
  } as typeof fetch;
});
```

### PostGIS healthcheck timing
The PostGIS container has a two-phase startup: init scripts run, then PostgreSQL restarts.
`pg_isready` passes during the init phase, but the DB is briefly unavailable during restart.
Use an actual SQL query in the healthcheck:
```yaml
healthcheck:
  test: ["CMD-SHELL", "pg_isready -U motus -d motus && psql -U motus -d motus -c 'SELECT 1' -q"]
```

### Common selector patterns
- Device table: `.device-table` (not `.table`)
- Device UID badge: `.uid-badge` (not `.uid`)
- Nav links: use `a[href="/map"]` for exact match (`:has-text("Map")` also matches "Heatmap")
- Device cards: `a.device-card` to skip loading skeletons
- Dashboard heading: "All Devices" (not "Recent Devices")
- Leaflet draw: `.leaflet-draw-edit-edit` exists but `.leaflet-draw-edit-remove` does not
  (remove is configured as `false`; deletion is via sidebar)
- Notifications: `eventTypeSelect` is required — must `selectOption()` before submit

### Page Object files
Located in `web/tests/page-objects/`. Use these rather than inline selectors.

## CLI Commands

```
motus serve                          # Start HTTP + GPS servers
motus db-migrate [up|down|status]    # Run database migrations (goose)
motus user add --email --name --password --role  # Create user
motus user list                      # List users
motus device add --uid --name        # Register device
motus wait-for-db                    # Block until DB is reachable
motus import                         # Import from Traccar dump
motus version                        # Print version
```

## Database

- PostgreSQL with PostGIS extension
- 35 goose migrations (embedded via `//go:embed`)
- Positions table is partitioned (`00022_partition_positions.sql`)
- Key tables: users, devices, positions, geofences, notification_rules, api_keys, sessions
- `users` table has NO `updated_at` column (don't add one in INSERT queries)
