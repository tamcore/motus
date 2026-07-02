# AGENTS.md — Agent Guide for motus

## Project Overview

GPS tracking platform: Go backend (chi router) + SvelteKit frontend. Receives GPS device
positions via H02/Watch protocols, stores in PostgreSQL (PostGIS), serves a web UI with
maps, geofences, notifications, reports, and device management.

## Architecture

```
cmd/motus/          — CLI entrypoints (cobra): serve, db-migrate, user, device, import, etc.
internal/
  api/              — HTTP router, middleware (auth, CSRF, rate limiting), context helpers
    handlers/       — Unified ogen Handler + SecurityHandler implementations
    middleware/     — chi middleware (auth, CSRF, rate limit, write-access, headers)
    oas/            — Generated ogen code (DO NOT edit manually — see make generate)
  config/           — Config loading from env vars, validation
  protocol/         — GPS protocol servers (H02, Watch)
  model/            — Domain types
  storage/
    repository/     — PostgreSQL data access (pgx); testutil/ for integration test helpers
migrations/         — Goose SQL migrations (embedded via //go:embed)
docs/
  openapi.yaml      — OpenAPI 3.0 spec (source of truth for the HTTP API)
web/
  src/              — SvelteKit app (routes, lib/components, lib/stores)
  tests/            — Playwright E2E tests
charts/motus/       — Helm chart (deployment, migration job, postgres, redis)
```

### API Layer (ogen)

The HTTP API is spec-first using [ogen](https://github.com/ogen-go/ogen). `docs/openapi.yaml`
is the source of truth; `internal/api/oas/` contains the generated Go server code.

**Never edit `internal/api/oas/` by hand.** Regenerate with:

```bash
make generate
```

This runs `go generate ./internal/api/...` which invokes ogen. Always import the package as:

```go
oas "github.com/tamcore/motus/internal/api/oas"
```

The generated `oas.Handler` interface is implemented by `handlers.Handler`
(`internal/api/handlers/handler.go`). `handlers.NewHandler(HandlerConfig{...})` is the
single constructor — pass all repos via `HandlerConfig`. The `SecurityHandler`
(`handlers.NewSecurityHandler`) validates credentials per-operation using the same repos.

**`RouterConfig.Auth` must use `middleware.LoadAuthContext`, not `middleware.Auth`.**
`LoadAuthContext` populates user/API-key context from credentials but passes through
unauthenticated requests unchanged — allowing public endpoints like `GET /api/health` to
respond without a 401. The ogen `SecurityHandler` enforces per-operation auth requirements.
`middleware.Auth` (returns 401 for all unauthenticated requests) is only for non-ogen routes.

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

### The go.mod `go` directive pins the CI toolchain
All workflows use `setup-go` with `go-version-file: go.mod`, so the `go` directive
selects the **exact** toolchain CI builds with. Keep it at the latest patch release
(e.g. `go 1.26.4`, not `go 1.26.0`) — otherwise the Security workflow's govulncheck
fails on already-patched Go standard library CVEs, and release binaries ship the
vulnerable stdlib.

### Pre-commit hooks run Go tests (short mode)
`.pre-commit-config.yaml` runs `go test -short`, `go vet`, `go fmt`, `golangci-lint`, and
a `generate-check` hook. The `-short` flag skips integration tests that need Docker
(PostGIS via testcontainers, Redis via testcontainers). The skip is implemented in
`testutil.SetupTestDB(t)` and `setupRedis(t)` via `testing.Short()`. Full integration
tests run in CI where Docker is available.

The `generate-check` hook triggers only when `docs/openapi.yaml` is staged. It runs
`make generate` then `git diff --exit-code internal/api/oas/` — the commit is blocked if
the generated code is out of sync with the spec. The same check runs in CI as
`generate-check` job in `.github/workflows/test.yaml`.

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
- `MOTUS_CSRF_SECRET` is **required** in non-development environments. The server
  refuses to start without it. Generate with `openssl rand -hex 32`. All pods in a
  multi-pod deployment must share the same secret. Development mode (`MOTUS_ENV=development`)
  allows an empty secret and falls back to a per-restart random key.
- Session-expiry redirects carry `?returnTo=<path>`; the login page navigates there
  after auth. **Any** redirect-target logic in the frontend must go through
  `sanitizeReturnTo` in `web/src/lib/utils/returnTo.ts` — it is the open-redirect
  guard (rejects `//host`, backslashes, absolute URLs, schemes). The OIDC callback
  does not honor `returnTo` (would need threading through the OIDC state record).

### H02 relay requires numeric-only device IMEIs
The H02 protocol server supports an optional relay mode (`MOTUS_GPS_H02_RELAY_TARGET`)
that forwards raw GPS messages to an external system (e.g., Traccar) before decoding.
Traccar's H02 protocol decoder only accepts numeric device identifiers — alphanumeric
IMEIs are silently dropped. Demo device IMEIs must be numeric (default: `9000000000001`,
`9000000000002`). The relay is fail-open: if the relay target is unreachable or drops,
motus continues serving devices normally.

### GPS protocols have no device authentication (by design)
H02 and Watch are plaintext TCP protocols; device identity is the **self-reported
IMEI** in each message. There is no handshake, shared secret, or signature — this is
inherent to the protocols and cannot be fixed in motus. Consequences:

- Anyone who can reach the GPS ports can inject positions for any known IMEI
  (spoofing) or, when device auto-creation is enabled, register new devices that
  land in the default user's account (`resolveOrCreateDevice` in
  `internal/protocol/server.go`).
- **Mitigation is network isolation**: firewall the GPS ports (`MOTUS_GPS_H02_PORT`,
  `MOTUS_GPS_WATCH_PORT`) to carrier/APN source ranges, or expose them only on a
  VPN/private interface. Do not expose them to the open internet unless spoofed
  positions are an accepted risk.
- Read deadlines, a connection limit, and bounded line buffers protect against
  resource exhaustion, not against spoofing.

### OIDC email linking requires a verified email
On first OIDC login the handler links the OIDC subject to an existing local account
by email **only when the IdP asserts `email_verified`** (boolean `true` or string
`"true"`). IdPs that omit the claim entirely will not link — set
`MOTUS_OIDC_TRUST_UNVERIFIED_EMAIL=true` only if the IdP is known to verify email
addresses; otherwise an attacker-controlled IdP account with a victim's address
could take over the local account.

### Demo instance credentials
Login is by **email**, not username (`POST /api/session` rejects strings without `@`).
The demo accounts (seeded in `internal/demo/service.go`) are:

| Email | Password | Role |
|---|---|---|
| `demo@motus.local` | `demo` | user |
| `admin@motus.local` | `admin` | admin |

Legacy `?token=` login uses the username part of the email (`demo`, `admin`).

### Demo accounts are write-protected
When demo mode is enabled, `demo.IsDemoAccount(email)` blocks profile and user
modification (`"demo accounts cannot be modified"`). On the dev K8s instance even
`admin@motus.local` counts as a demo account — to exercise flows like password
change, create a throwaway user first (`POST /api/users` as admin) and delete it
afterwards.

### Demo device cleanup uses configured IMEIs
`demo.Reset()` accepts a `deviceIMEIs []string` parameter and uses `unique_id = ANY($1)`
to identify demo devices for cleanup. The default IMEIs are exported as
`demo.DefaultDeviceIMEIs`. If you change the demo IMEIs via `MOTUS_DEMO_DEVICE_IMEIS`,
the reset logic must receive the matching values.

### AI Chat Feature (`MOTUS_AI_*`)

The AI chat feature exposes an in-process MCP tool registry (via `mark3labs/mcp-go`) invoked
by a streaming chat orchestrator that speaks to any OpenAI-compatible Chat Completions endpoint.

**Env vars** (all optional; required fields marked):

| Variable | Default | Notes |
|---|---|---|
| `MOTUS_AI_ENABLED` | `false` | Set to `true` to enable `/api/chat` and the Chat nav link |
| `MOTUS_AI_BASE_URL` | `https://api.openai.com/v1` | Any OpenAI-compatible endpoint |
| `MOTUS_AI_API_KEY` | *(required when enabled)* | Prefer a Kubernetes Secret / `apiKeyRef` in production |
| `MOTUS_AI_MODEL` | `gpt-4o-mini` | Model name understood by the target endpoint |
| `MOTUS_AI_MAX_TOKENS` | `4096` | Max completion tokens per request |
| `MOTUS_AI_TEMPERATURE` | `0.2` | Sampling temperature |
| `MOTUS_AI_TIMEOUT` | `90s` | Total wall-clock timeout per `POST /api/chat` request |
| `MOTUS_AI_MAX_TOOL_LOOPS` | `8` | Max tool-call iterations before the loop is cut off |
| `MOTUS_AI_SYSTEM_PROMPT` | *(built-in)* | Override the default system prompt |

**Architecture:**
- `POST /api/chat` → `internal/ai/chat.Service.Stream` → `internal/ai/mcp` (MCP tool dispatch)
- MCP tools read `api.UserFromContext(ctx)` and enforce `UserHasAccess` on every resource.
- Readonly API keys can call `/api/chat` for read-only queries; write tools (`create_geofence`)
  return an error for readonly keys.

**Helm:** `ai.enabled: false` by default in `charts/motus/values.yaml`. Override in
`charts/motus/values-dev.yaml` (gitignored). When `ai.enabled` is `true` and `ai.apiKey`
is non-empty (and `ai.apiKeyRef.secretName` is empty), an `<fullname>-ai` Secret is created
automatically by `templates/ai-secret.yaml`.

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
- **Component tests**: rendering Svelte components in Vitest works via the
  `svelteTesting()` plugin in `web/vite.config.ts` (without it, the Svelte server
  build is resolved and `render()` fails with `lifecycle_function_unavailable`).
  See `web/src/tests/modal-focus.test.ts` + `helpers/ModalFocusHost.svelte` for the
  pattern (host component for slot-based testing).
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
| `generate` | Regenerate `internal/api/oas/` from `docs/openapi.yaml` (ogen) |
| `lint` | Run all linters (go vet, golangci-lint, frontend check) |
| `test` | Run all tests (includes lint) |
| `dev-deploy-k8s` | Build dev image, push, deploy to K8s |
| `dev-reset-database` | Reset database (delete all data) |
| `dev-full-deploy` | Full dev deployment with data import |
| `clean` | Clean build artifacts |

## CI Workflows

| Workflow | Trigger | What it does |
|----------|---------|--------------|
| `test.yaml` (Unit Tests) | push to master, PRs | `go test -race ./...` + Codecov |
| `e2e.yaml` (E2E Tests) | push to master, PRs | Docker Compose stack → Playwright |
| `commit-lint.yaml` | PRs only | Commitlint on PR commits |
| `security.yaml` (Security) | push to master, PRs | gosec, govulncheck, semgrep |
| CodeQL (GitHub default setup) | PRs | `Analyze (go/javascript-typescript/actions)` checks |

CodeQL default-setup checks occasionally fail with GitHub-side
"Requires authentication" errors during SARIF upload/init. These runs cannot be
re-run via `gh run rerun` — push an empty commit to re-trigger.

### Security scanning conventions

- gosec and govulncheck are **go.mod `tool` directives** (like ogen) and are invoked
  as `go tool gosec` / `go tool govulncheck` — manage versions in go.mod, not in the
  workflow. semgrep is pinned in `security.yaml` (`pipx install semgrep==<version>`).
- Standalone gosec honors `#nosec G<rule>` comments, **not** `//nolint:gosec`
  (golangci-lint is the reverse). Per-line false positives get inline `#nosec` /
  `nosemgrep: <full-rule-id>` comments with a `--` justification.
- Excluded wholesale, with reasons in `security.yaml`: gosec `G124` and semgrep
  `cookie-missing-secure` (the cookie `Secure` flag is intentionally conditional on
  `MOTUS_ENV` via `isSecureEnvironment()`; HttpOnly/SameSite are always set).
- Path excludes live in `.semgrepignore` (generated `internal/api/oas/`, demo
  tooling, `*_test.go`, `web/build/`). Note: when `.semgrepignore` exists, semgrep's
  built-in default ignores are replaced — keep `.git/`, `node_modules/` listed.

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
