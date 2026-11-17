# Display Bridge Extension

`displaybridge` is the Go-side stub that exposes the `ktw-display` TypeScript sidecar's API surface to Go consumers as a normal Go-side KTSP service. Phase 1 of refined Architecture C (locked 2026-05-01); see [where the universe's data types live](../../../modules/framework/decisions/OD-004-where-do-the-universe-s-data-types-live-and.cwm.md), [where the UI agents and sidecars use lives](../../../modules/display/decisions/OD-006-where-is-the-ui-that-agents-and-sidecars.cwm.md) and [modules/framework/schema/architecture.md](../../../modules/framework/schema/architecture.md).

## The stub pattern

Go consumers (agent loop, MCP clients, KTSP API consumers) don't know whether the schema service is implemented in Go or via HTTP forwarding to bun. They call:

```go
b := application.UseService[displaybridge.SchemaBridge](ctx)
out, err := b.Parse(ctx, displaybridge.ParseInput{Body: yaml})
```

Internally, **Phase 1** translates each call to an HTTP request against the bun sidecar. **Phase 2** (ms07+, `ts-ktsp-sidecar-tooling` todo) replaces the HTTP transport with native KTSP from the bun side — same Go interface, different transport. **Consumer code does not change.**

The bridge is the migration boundary: as long as the `SchemaBridge` interface in [api/service.go](api/service.go) stays stable, the Phase 2 swap is purely an implementation change inside this package.

## Package layout

```
displaybridge/
  api/
    service.go         SchemaBridge interface + I/O types + ServiceDef
    types.go           Schema/Type/TypeRef/ContentRef AST (mirrors bun's)
    diagnostic.go      Diagnostic shape (severity/code/message/path)
  local/
    extension.go       Extension() — published-service wiring + WithExternalProcess
  internal/
    spawn.go           BunSpec — auto-detects compiled binary; falls back to `bun run`
    auth.go            EndpointProvider interface + StaticEndpoint + EnvEndpoint
    client.go          httpClient with Get/Post/PostJSON + retry on transient 5xx
    errors.go          BridgeError (preserves bun's wire shape) + status mapping
    slots.go           schemaBridge: per-slot HTTP forwarders + Close (io.Closer)
    cache.go           bounded LRU for compiled JSON Schemas (cap 256)
    validator.go       native ValidateValue using santhosh-tekuri/jsonschema/v5
    sse.go             /sse/schema-changed subscriber + reconnect loop
```

## How calls flow (Phase 1)

```
Go consumer
   │
   ▼
application.UseService[SchemaBridge](ctx).Parse(...)
   │
   ▼ (slot dispatch)
schemaBridge.Parse(ctx, in)
   │
   ▼ (httpClient.PostJSON with idempotent retry)
HTTP POST http://127.0.0.1:<allocPort>/api/parse
Authorization: Bearer <sessionToken>
   │
   ▼
ktw-display (bun sidecar, supervised by publish.ExternalProcess)
   │
   ▼ JSON response
schemaBridge.Parse returns api.ParseOutput
```

Forward slots (12 total) translate Go I/O ↔ JSON over HTTP. The single non-forwarded slot — `ValidateValue` — runs **in-process** via `santhosh-tekuri/jsonschema/v5` against an LRU cache of compiled JSON Schemas (capacity 256), populated on miss by calling `GenerateJSONSchema` via the same bridge. That keeps the validation hot path 100 % Go.

## Bun lifecycle

The bridge is registered with `publish.WithExternalProcess(BunSpec())`. The supervisor in [internal/core/publish/supervisor.go](../../core/publish/supervisor.go) handles:

- **Lazy spawn**: constructing the bridge spawns nothing; the first call that needs bun (`GetWidgetURL`) starts it, once even under concurrent callers. A failed start is returned to that caller and retried, with a fresh process, on the next use. `Close` stops the running (or still-starting) process and refuses later starts. A one-shot CLI command therefore runs no bun.
- **Healthz probe**: polls `/healthz` every 100 ms until first 2xx response.
- **Restart-on-crash**: exponential backoff (1, 2, 4, 8 s, capped at 60 s); max 5 restarts in any 5-minute window.
- **Port allocation**: kernel-assigned via `netutil.AllocFreePort`; substituted into `${ALLOC_PORT}` placeholders in `BunSpec`.
- **Token rotation**: a fresh 32-byte hex token per spawn, in `${SESSION_TOKEN}`. Both sides see the same value — bun reads `KTW_DISPLAY_TOKEN` from env, the bridge reads it via `EndpointProvider.Token()`.

## Dev vs. prod

`BunSpec` auto-detects which bun process to launch:

1. `KTW_DISPLAY_BIN` env var — explicit override (used for tests / custom builds).
2. `app/ktw-display/dist/ktw-display{.exe,.bin}` — compiled binary (production).
3. Fallback: `bun run app/ktw-display/src/index.ts` (development, no build step).

So `bun run` works during dev, the compiled binary takes over once `bun run build` produces `dist/ktw-display`, and tests can point at any custom binary.

## SSE cache invalidation (Subtask 5)

After bun's first successful start (never before — it would otherwise spawn bun at boot), the bridge subscribes to `/sse/schema-changed` from bun ([app/ktw-display/src/sse.ts](../../../app/ktw-display/src/sse.ts)). Behavior:

- **`schema-changed` event** → `cache.Invalidate(name)` — next `ValidateValue` re-fetches.
- **Disconnect** → `cache.Clear()` then reconnect with exponential backoff (1 s → 30 s, reset to 1 s after a stream survives ≥ 30 s).
- **Heartbeats / unknown events / malformed JSON** → silently ignored.

Phase 1 bun emits a `: heartbeat` every 30 s and nothing else (no `InstallSchema` handler exists yet to mutate schemas). When future codegen-target todos ship `InstallSchema`, they call `emitSchemaChanged(name, version)` from `sse.ts` and the cache invalidation flows automatically.

## Endpoint resolution

`EndpointProvider` is consulted on **every** call so token/port rotation across bun restarts is picked up without the client holding stale state.

- `StaticEndpoint{URL, Token}` — pin fixed values (tests, manual setups).
- `EnvEndpoint{}` — read `KTW_DISPLAY_PORT` / `KTW_DISPLAY_TOKEN` from process env.
- Future (ms06d): a supervisor-backed provider that reads from the live `*ExternalProcess` directly.

The factory in [local/extension.go](local/extension.go) currently uses `EnvEndpoint{}` — useful for tests that `t.Setenv` and for manual setups where bun is started out-of-band. The supervisor-backed provider is the migration target.

## Error model

Non-2xx responses from bun map to `*BridgeError` preserving the JSON wire shape:

```go
type BridgeError struct {
    Status      int            // HTTP status
    Message     string         // bun's "error" field
    Detail      string         // bun's "detail" field
    Diagnostics []Diagnostic   // bun's structured diagnostics
}
```

Predicates: `IsNotImplemented()` (501), `IsUnauthorized()` (401), `IsServerError()` (5xx), `IsTransient()` (5xx except 501).

**Phase-1 convention**: bun's stub handlers return 501 with `{error: "not implemented", detail: "see <todo-name>"}`. The bridge surfaces these as `BridgeError.IsNotImplemented() == true`, with the slot name in the wrapped error chain (`"Parse: displaybridge: bun returned 501..."`). Consumers can branch cleanly on "this slot's bun handler isn't ready yet" without re-parsing strings.

## Coexistence with the legacy Go parser

`internal/extensions/schema/` ships the legacy Go-side DSL parser. During the coexistence period, both extensions are registered:

```go
// app/ktw-agents/cmd/bootstrap.go
schemalocal.Extension(),         // legacy parser + `schema` CLI
displaybridgelocal.Extension(),  // Phase-1 stub bridge
```

The bridge ships **service surface only** (`WithKTSPSlotTags({"cli": "false"})`); the legacy `ktw-agents schema parse` CLI stays in place. The CLI rewire happens in the `deprecate-go-schema-extension` cleanup task — at that point the legacy package is removed and the CLI delegates to the bridge.

## Phase 2 swap path

When `ts-ktsp-sidecar-tooling` (ms07+) lands:

1. The bun sidecar adopts KTSP natively (no more HTTP-only).
2. `internal/client.go`, `internal/sse.go`, and `internal/auth.go` are replaced with KTSP-transport equivalents.
3. The forward-slot impls in `internal/slots.go` swap their `b.client.Post(...)` calls for `b.ktspSender.Call(...)` — same shape, different transport.
4. `api/` and the published `SchemaBridge` interface are **unchanged**. Consumers don't notice.

## Tests

```bash
# Unit tests (fast, no bun needed)
go test ./internal/extensions/displaybridge/...

# Integration tests (require bun on PATH)
KTW_DISPLAYBRIDGE_INTEGRATION=1 go test -count=1 -run TestIntegration_ ./internal/extensions/displaybridge/...

# Coverage
go test -cover ./internal/extensions/displaybridge/...
```

The integration suite (gated on `KTW_DISPLAYBRIDGE_INTEGRATION=1`) spawns a real ktw-display via `publish.NewExternalProcess` and exercises the full lifecycle: spawn → healthz → bridge call → SSE subscribe → close. Crash/restart coverage is partial — full external-kill testing waits on a follow-up that adds a test-only Kill hook to `publish.ExternalProcess`.
