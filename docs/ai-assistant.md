# AI Assistant

The AI assistant lets users control Motus in natural language from the `/chat`
page. It can read device positions, list geofences and calendars, report trip
distances, query events, and create or update geofences, calendars, and
notification rules — all by calling backend tools during a single
conversational turn.

The assistant is **opt-in** and requires a working OpenAI-compatible API key.
When disabled, the Chat nav link is hidden and the chat routes are not
registered. When enabled, a "Chat" link appears in the sidebar and a
Redis-backed 24-hour conversation history keeps context across browser
reloads.

---

## Architecture

```
Browser /chat ──POST {message}──► /api/chat ──► Service.Stream ──► OpenAI-compatible API
                                      │              │
                                      │              ├──► MCP tool dispatch (16 tools)
                                      │              │        │
                                      │              │        └──► repositories, geocoder, services
                                      │              │
                                      │              └──► append to Redis history
                                      │
                                      └──◄ SSE: token / tool_call / tool_result / done / error
```

**Request walk-through:**

1. The browser sends only the new user message — no history.
   (`web/src/lib/api/chat.ts`)

2. The handler appends it to the Redis conversation list at
   `motus:chat:history:<userID>`, then hands off to `Service.Stream`
   (`internal/api/handlers/chat.go`, `internal/ai/chathistory/handle.go`).

3. `Service.Stream` recovers the full history from Redis, prepends the
   system prompt, and converts all registered MCP tools to OpenAI function
   schemas (`internal/ai/chat/service.go`).

4. The LLM reply streams back. If the model requests tool calls, the
   service dispatches each one to the matching MCP handler
   (`service.go` → `internal/ai/mcp/tools.go`). Tool handlers call
   application repositories and services (devices, positions, geofences,
   calendars, notifications).

5. Each tool result is persisted to Redis and emitted to the browser as a
   `tool_result` SSE event before the next iteration begins.

6. Once the model produces a final text reply (or hits `MOTUS_AI_MAX_TOOL_LOOPS`),
   a `done` event closes the stream.

**Key packages:**

| Package | Role |
|---|---|
| `internal/ai/chat/` | Service, agentic loop, SSE sink, message types |
| `internal/ai/mcp/` | MCP server, tool registrations, tool handlers |
| `internal/ai/chathistory/` | Redis-backed conversation store |
| `web/src/routes/chat/` | SvelteKit page, frontend gating |
| `web/src/lib/{api,stores}/chat.ts` | API client, Svelte store |

---

## Configuration

All configuration is via environment variables (or the equivalent Helm values).
The feature is **disabled by default** — set `MOTUS_AI_ENABLED=true` to
activate it.

| Env var | Default | Purpose |
|---|---|---|
| `MOTUS_AI_ENABLED` | `false` | Master gate. When false, `/api/chat*` routes are not registered, no MCP server is built, and the frontend hides the Chat nav link. |
| `MOTUS_AI_BASE_URL` | `https://api.openai.com/v1` | Base URL of any OpenAI-compatible chat completions API. |
| `MOTUS_AI_API_KEY` | — | **Required when enabled.** Bearer token sent to the LLM provider. |
| `MOTUS_AI_MODEL` | `gpt-4o-mini` | Model identifier forwarded to the provider. |
| `MOTUS_AI_MAX_TOKENS` | `4096` | Per-completion token cap. |
| `MOTUS_AI_TEMPERATURE` | `0.2` | Sampling temperature (0 = deterministic). |
| `MOTUS_AI_TIMEOUT` | `90s` | Wall-clock budget for one chat request, including all tool iterations. |
| `MOTUS_AI_MAX_TOOL_LOOPS` | `8` | Maximum number of tool-call iterations per turn. |
| `MOTUS_AI_SYSTEM_PROMPT` | _(built-in)_ | When non-empty, replaces the built-in system prompt entirely. The built-in prompt describes available tool categories, instructs the model to call `get_server_time` before resolving relative dates, restricts the assistant to motus topics (with refusal and prompt-injection-resistance instructions), and appends today's date. |
| `MOTUS_AI_GUARDRAIL_ENABLED` | `true` | Pre-flight topic classifier. Before the main model runs, a cheap non-streaming completion (temperature 0, 4 max tokens) classifies the latest user message as on/off-topic using the last 6 text turns for context. Off-topic messages receive a canned refusal without invoking the main model or tools; each refusal is logged (INFO, first 120 chars of the message). Classifier errors fail open (the request proceeds). |
| `MOTUS_AI_GUARDRAIL_MODEL` | _(same as `MOTUS_AI_MODEL`)_ | Model used for the guardrail classification call. Point it at a smaller/cheaper model to cut guardrail cost. |

Sources: `internal/config/config.go`, `internal/config/validate.go`.

### Helm values

The `ai:` block in `charts/motus/values.yaml` maps directly to the env vars
above. For production deployments, prefer the secret indirection instead of
embedding the key in values:

```yaml
ai:
  enabled: true
  model: gpt-4o-mini
  apiKeyRef:
    secretName: motus-ai-secret
    secretKey: api-key
```

`apiKeyRef.secretName` / `apiKeyRef.secretKey` mount the key from a
Kubernetes Secret rather than embedding it in the release values.

---

## Authentication & Authorization

- **All chat routes** require an authenticated session or API key (`Auth`
  middleware, `internal/api/router.go`).

- **Inside each tool handler**, `requireUser` reads the user from request
  context via `api.UserFromContext` (`internal/api/respond.go`). An
  unauthenticated request gets a 401 from the route middleware before the
  handler runs.

- **Write tools** (`create_*`, `update_*`, `delete_*`) additionally call
  `requireWriteAccess`, which rejects readonly API keys. Regular session
  logins and full-access API keys can use all tools.

- **Admin scope:** `list_devices` expands to all users' devices when
  `user.IsAdmin()` is true; other listing tools show only the calling
  user's own resources.

- **Route-level note:** `/api/chat` does *not* have a `WriteAccess`
  middleware wrapper — readonly enforcement is intentionally per-tool
  so that a readonly key can still use the chat page for queries.

---

## Chat History

Conversation context is stored in Redis as a LIST of JSON-encoded
`chat.Message` values under the key `motus:chat:history:<userID>`.

| Property | Value |
|---|---|
| Key pattern | `motus:chat:history:<userID>` |
| TTL | 24 hours, sliding — refreshed on every append |
| Max turns | 30 user/assistant turns |
| Max size | 64 KB (serialized) |
| Fallback | If Redis is down, chat works as single-turn (no 500s) |

When the trim caps are reached, the oldest entries are dropped from the
head. The trim logic ensures the new head is never a `tool` message —
orphaned tool messages without their preceding assistant call would break
the LLM's context window.

Sources: `internal/ai/chathistory/store.go`, `internal/ai/chathistory/trim.go`,
`internal/ai/chathistory/handle.go`. Wired in `cmd/server/serve/serve.go`.

---

## HTTP Endpoints

All three endpoints require an authenticated user.

### `POST /api/chat`

Start or continue a conversation.

**Request body:**
```json
{ "message": { "role": "user", "content": "Show me my devices." } }
```

**Response:** `text/event-stream` — a sequence of `ChatEvent` objects (see
[SSE Event Protocol](#sse-event-protocol) below).

**Validation:** `role` must be `"user"` and `content` must be non-empty.
Anything else returns `400`.

### `GET /api/chat/history`

Retrieve the full conversation history for the calling user.

**Response:**
```json
{ "messages": [ { "role": "user", "content": "..." }, ... ] }
```

Returns `{ "messages": [] }` when no history exists. Includes `tool` role
messages with `toolCallId` and `content` (the raw JSON result), which the
frontend uses to re-populate tool-call cards on reload.

### `DELETE /api/chat/history`

Clear the calling user's conversation history.

**Response:** `204 No Content`. Idempotent — deleting an empty key succeeds.
The "New conversation" button in the UI calls this endpoint.

---

## SSE Event Protocol

The `POST /api/chat` response is a `text/event-stream` where each event is:

```
data: <json>\n\n
```

Five event shapes (`internal/ai/chat/messages.go`):

| `type` | Additional fields | When emitted |
|---|---|---|
| `token` | `delta: string` | One chunk of the assistant's text reply |
| `tool_call` | `id: string`, `name: string` | The model has decided to call a tool |
| `tool_result` | `id: string`, `name: string`, `result?: unknown`, `error?: string` | Tool finished; `result` is the parsed JSON response or `error` is the error message |
| `done` | — | Stream complete; no more events will follow |
| `error` | `message: string` | A fatal error occurred; the stream will close |

**Client behavior:**
- Accumulate `token.delta` into the assistant message content.
- On `tool_call`, add a pending tool card (shown as "Running…" until the
  matching `tool_result` arrives, matched by `id`).
- On `tool_result`, populate the card with the result or error.
- On `done` or `error`, stop the stream and update UI state.

---

## Frontend Gating

The chat UI is hidden unless `ServerInfo.aiEnabled` is `true` (from
`GET /api/server`). Two independent gates enforce this:

1. **Route guard** — `web/src/routes/chat/+page.ts` redirects to `/` when
   `serverInfo.aiEnabled` is falsy. This runs server-side on first load.

2. **Nav link** — `web/src/routes/+layout.svelte` wraps the Chat link in
   `{#if $serverInfo?.aiEnabled}`, so the link never appears for disabled
   deployments.

The "New conversation" button calls `DELETE /api/chat/history` then resets
the local Svelte store to `[]`. No page reload is required.

---

## Adding a New MCP Tool

1. **Register** the tool in `internal/ai/mcp/tools.go` inside the relevant
   `// ---- domain ----` section using `mcp.NewTool(...)`.

2. **Write the handler** in the same file:
   ```go
   func handleMyTool(ctx context.Context, req mcp.CallToolRequest, deps Deps) (*mcp.CallToolResult, error) {
       user, err := requireUser(ctx)
       if err != nil {
           return mcp.NewToolResultError(err.Error()), nil
       }
       // For write tools only:
       if err := requireWriteAccess(ctx); err != nil {
           return mcp.NewToolResultError(err.Error()), nil
       }
       // ... business logic ...
       return jsonResult(map[string]any{"id": result.ID}), nil
   }
   ```

3. **Wire dependencies** — if the handler needs a new repository or service,
   add it to the `Deps` struct in `internal/ai/mcp/server.go` and pass it
   from `cmd/server/serve/serve.go`.

4. **Update the tool reference** — add the new tool to
   [`docs/ai-mcp-tools.md`](ai-mcp-tools.md) in the same domain group.
   There is no auto-generation; the doc must be kept in sync by hand.

5. The MCP server calls `ListTools()` at startup to build OpenAI function
   schemas (`internal/ai/chat/service.go`), so no additional registration
   is needed in the chat service.
