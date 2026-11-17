# ktw-display

TypeScript sidecar for the data-DSL **codegen execution + widget server**. Peer of the Go sidecars (`ktw-docs`, `ktw-notify`, ...) — same lifecycle, same discovery via `extensions.yaml` — but the implementation is TS, run by [Bun](https://bun.sh/).

> Spawned by `ktw-agents` on launch as a managed subprocess. Stopped on shutdown.

## Architectural split

The data-DSL stack splits between Go and TS by what each language is best at:

- **Go** (in `internal/extensions/schema/`): schema parser, AST normalization, content-type registry (with provider system), discovery (filesystem + sidecar contributors + in-memory index), manifest validation, TSX file delivery, KTSP authority for the entire surface.
- **TS** (this sidecar): codegen execution (template evaluation + AST walk + output text), widget HTTP server, bun-build cache for TSX bundles, postMessage handler for iframe ↔ host.

ktw-display **does not parse YAML or read manifest files directly** — it asks Go via KTSP for pre-parsed ASTs and pre-resolved manifests. Go is the single source of truth for "what schemas/types exist." TS is the source of truth for "what to produce" (code or rendered widgets).

## Two surfaces

| Surface | Purpose | Consumed by |
|---|---|---|
| **HTTP `/widget/<kind>?…`** | iframe-mounted schema-driven UI (forms, tables, views, dashboards, decks, fields) | **ktw-gui** via `<iframe>` (URL returned from Go-side schema service) |
| **HTTP `/api/gen/<target>`** | Codegen output (json-schema, ts-types, go-structs, mantine-v9, vue, sql, ...) | **Go-side schema service** at `internal/extensions/schema/` (proxies KTSP slot calls here) |
| **HTTP `/api/parse`** | TS-side schema parser (peer of the Go parser, same canonical AST) | Go-side parity tests; future TS callers |
| **HTTP `/api/types`** | Content-type registry queries (list, lookup, manifest) | Both Go and ktw-gui |
| **HTTP `/healthz`** | Liveness probe | ktw-agents lifecycle manager |

All requests carry a per-session token (`?token=…` on widget URLs, `Authorization: Bearer …` on API calls); see the design todo for the auth model.

## Test harness (developer surface)

Activated only via the `--test` CLI flag (no env var, no production leak):

```powershell
$env:KTW_DISPLAY_PORT="9990"
bun run src/index.ts --test
```

Then `http://localhost:9990/test` opens an interactive showcase + smoke testing surface:

- **Left pane** — tree of test cases discovered automatically from `src/test-cases/<category>/<case>.ts` (drop a file, restart server, the case appears)
- **Right pane** — selected case rendered live via the shared schema-walker
- **Outcome strip** — every postMessage event from the case shows here (change/submit/etc.)
- **Stable URLs** — each case is addressable at `/test/<category>/<case>` for sharing and browser-automation navigation
- **API** — `/test/api/cases` returns the catalog; `/test/api/cases/<id>` returns the full case incl. optional `script` for browser automation

See [`src/test-cases/README.md`](src/test-cases/README.md) for the case authoring guide.

When the `--test` flag is absent, all `/test*` routes return 404. The harness CANNOT leak through env-var inheritance.

## How it integrates

ktw-display is **just another sidecar** from the framework's perspective — KTSP discovery, port allocation, lifecycle hooks all match the existing Go sidecars. The implementation language is the only thing that's different.

```
┌──────────────────────────────────────────────────────────────────────┐
│                    ktw-agents (Go binary)                             │
│                                                                       │
│  ┌──────────────────────────────────────────────────────────────┐   │
│  │  internal/extensions/schema/  (the schema authority)         │   │
│  │   • parser (DSL → canonical AST)                              │   │
│  │   • content-type registry (filesystem + sidecar providers)    │   │
│  │   • manifest validation, kind discriminator, i18n             │   │
│  │   • TSX file delivery (read from disk → serve over KTSP)      │   │
│  │   • KTSP slots: Parse, Validate, ListTypes, GetTypeManifest, │   │
│  │     GetTSXFile, GetSchemaAST, Generate*, GetWidgetURL         │   │
│  └─────────────────┬─────────────────────────────────────────────┘   │
│                    │ KTSP (in-process)                               │
│                    │                                                  │
│  ┌─────────────────┴─────────────────────────────────────────┐       │
│  │ Go sidecar lifecycle manager                              │       │
│  │  • spawns ktw-display at startup, stops at shutdown     │       │
│  │  • passes KTW_DISPLAY_PORT, KTW_DISPLAY_TOKEN, KTSP URL    │       │
│  └─────────────────┬─────────────────────────────────────────┘       │
└────────────────────┼─────────────────────────────────────────────────┘
                     │ spawn / KTSP
                     ▼
       ┌──────────────────────────────────────────────┐
       │  ktw-display (this sidecar)                │
       │   • receives AST + manifests + TSX from Go    │
       │     via KTSP (no local parsing or registry)   │
       │   • runs codegen targets in TS:               │
       │     /api/gen/json-schema                      │
       │     /api/gen/ts-types                         │
       │     /api/gen/go-structs                       │
       │     /api/gen/<future-target>                   │
       │   • serves widget iframes:                    │
       │     /widget/form, /widget/view, /widget/table │
       │     /widget/wizard, /widget/deck, /widget/…   │
       │   • bun-build cache for TSX → JS bundles      │
       └────────────────┬─────────────────────────────┘
                        │ HTTP (iframe)
                        │
       ┌────────────────┴─────────────────┐
       │       ktw-gui (Wails)             │
       │   • mounts <iframe src="..."/>    │
       │   • handles postMessage events    │
       │   • talks to Go via KTSP for      │
       │     non-widget operations         │
       └───────────────────────────────────┘
```

- **ktw-agents** spawns `bun run app/ktw-display/src/index.ts` (dev) or the compiled `dist/ktw-display` binary (prod) at startup; stops it on shutdown. Passes `KTW_DISPLAY_PORT`, `KTW_DISPLAY_TOKEN`, and the parent KTSP URL via env vars.
- **The Go-side schema service** is the canonical authority for everything schema-related. When ktw-gui (or another caller) invokes a KTSP slot like `GenerateJSONSchema`, the schema service:
  1. Resolves the AST locally (Go parser, in-process)
  2. Calls ktw-display's `/api/gen/json-schema` HTTP route with the AST as JSON
  3. Returns the generated text to the original caller
- For **widget rendering**, the schema service's `GetWidgetURL` slot returns an iframe URL pointing at `http://127.0.0.1:<ktw-display-port>/widget/...?token=…`; ktw-gui mounts the iframe.
- **Iframe content** runs in the webview but is served by ktw-display — sandboxed by browser origin policy + iframe boundary.
- ktw-display **never** reads YAML or scans the filesystem for manifests; it requests parsed AST + already-validated manifests + TSX bytes from Go on demand.

## Folder layout

```
app/ktw-display/
├── package.json
├── tsconfig.json
├── README.md                       (this file)
├── src/
│   ├── index.ts                    HTTP server entry point
│   ├── ast.ts                      TS-side AST type defs (mirrors Go-side; for type-checking received ASTs)
│   ├── ktsp_client.ts              Talks to Go via KTSP (fetches AST, manifests, TSX)
│   ├── targets/                    Codegen targets (one per output language)
│   │   ├── json-schema/
│   │   ├── ts-types/
│   │   ├── go-structs/
│   │   ├── mantine-v9/
│   │   └── …
│   ├── widgets/                    Widget HTTP handlers (one per composition kind)
│   │   ├── form.ts
│   │   ├── view.ts
│   │   ├── table.ts
│   │   ├── wizard.ts
│   │   ├── deck.ts
│   │   └── dashboard.ts
│   └── helpers/                    Shared codegen helpers (pascalCase, jsonEncode, …)
└── tests/                          bun test
```

**Note**: there is no `parser/` directory. ktw-display does not parse YAML or read manifest files. It receives pre-parsed ASTs from the Go-side schema service.

## Content-type discovery

Content-type discovery is **owned by the Go side** — `internal/extensions/schema/internal/registry/`. The Go registry has a provider system mirroring how skills/scripts/references are discovered:

- `FilesystemContentTypeProvider` — scans `<homedir>/.ktw/content-types/`, `<projectroot>/.ktw/content-types/`, and the framework's built-in directory
- `SidecarContentTypeProvider` — registers content-types contributed by sidecars at startup (each sidecar's `embed/content-types/` is scanned)
- Filesystem watchers invalidate cache on change
- Indexed in memory keyed by `(provider, name)`

ktw-display queries the Go registry via KTSP slots (`ListTypes`, `GetTypeManifest`, `GetTSXFile`) when it needs them. It never scans the filesystem itself.

Each content-type is a folder with `manifest.yaml` + per-target codegen templates + per-framework TSX renderer components. The manifest carries a `kind: field-type | component | renderer` discriminator (see the engine design doc).

## Dev

```bash
cd app/ktw-display
bun install
bun run start          # standalone server (port from env var KTW_DISPLAY_PORT, default 0=auto)
bun test
bun run tsc            # type-check
```

When run via ktw-agents, the port + auth token are passed via env vars:
- `KTW_DISPLAY_PORT` — the port to bind (chosen by ktw-agents to avoid conflicts)
- `KTW_DISPLAY_TOKEN` — the per-session auth token

In production: `bun build --compile --outfile dist/ktw-display src/index.ts` produces a self-contained executable. ktw-agents spawns the executable directly — Bun runtime is not required on the user's machine.

## Design

The ownership split between the Go schema spine and this sidecar is described in [modules/framework/schema/architecture.md](../../modules/framework/schema/architecture.md); the host ↔ iframe wire in [modules/display/host-iframe-wire.md](../../modules/display/host-iframe-wire.md).
