# `ktw-display` schema lib — canonical TS schema authority

This directory holds the framework's **canonical TS schema lib**: DSL parser entry points (to be added), AST types, builders, walkers, queries, content-type catalog, validation utilities.

Under refined Architecture C (locked 2026-05-01), this is the single source of truth for the schema-semantic core. Go consumes its outputs via the bridge service (`go-bridge-for-display-server`); GUI compositions import its types directly into in-page React.

## Lineage

This lib was promoted from `_ktw_workspace/old_ui_main/src/modules/core/schema/` on **2026-05-01**. The original was 1265 LOC across 7 files, validated as ~100% portable during the 2026-04-30 architectural review. The promotion was mechanical:

- **Path move**: `_ktw_workspace/old_ui_main/src/modules/core/schema/<file>.ts` → `app/ktw-display/src/schema/<file>.ts`
- **Internal imports**: relative imports preserved as-is (no cross-module references to update)
- **External alias removed**: the original imported `JsonObject` / `JsonValue` / `Jsonify` from `@core/types` (a path alias from the old workspace). Inlined locally in `schema-query.ts` and `schema-types.ts` to keep the lib self-contained.
- **`tsconfig.json` adjustment**: `verbatimModuleSyntax` set to `false` in `app/ktw-display/tsconfig.json` to accept the source's import style (mixed value/type imports without `import type`). Re-enabling would require sweeping the source to add `type` keywords; deferred as a follow-up cleanup if desired.
- **Strict-mode reconciliation**: a small number of strict-null-check assertions added to `schema-query.ts:getType` to satisfy the stricter tsconfig used here. No behavior change.

## Files

| File | Purpose |
|---|---|
| `index.ts` | Public re-exports |
| `schema-builder.ts` | Abstract `SchemaBuilder` — fluent API for constructing schemas in TS |
| `schema-contents.ts` | Open `ContentType` envelope + DSL/UI value aliases (color names, label variants, options helpers, …). The default content-type catalog moved out to [content-types/](../../content-types/) folders in M7. |
| `schema-json.ts` | JSON serialization helpers |
| `schema-query.ts` | AST query / walk / find utilities; carries the `KIND_DEFAULT_CONTENT` map (string→text, number→number, boolean→boolean) |
| `schema-types.ts` | Canonical AST type definitions; `BaseTypeWithContent.content: ContentRef` open envelope |
| `schema-utils.ts` | Misc utilities |

## Content-type catalog

The schema lib **does not own** the content-type catalog. It only carries the open envelope:

```ts
type ContentRef = string | { type: string; [key: string]: unknown }
```

The actual content types — `text`, `email`, `markdown`, `number`, `rating`, `slider`, `boolean`, plus 18 stubs awaiting renderers — live as folders under [`app/ktw-display/content-types/<name>/`](../../content-types/). Each folder ships `manifest.yaml` + (optionally) `types.ts` + `renderer.tsx`. See [content-types/_README.md](../../content-types/_README.md) for the contributor guide.

The runtime registry (in [src/content-types/](../content-types/)) loads the catalog at boot. The schema lib's only responsibility is to normalize `field.content` into `{ type: <name>, ...params }`; the registry resolves names to renderers.

## Migration log

See [MIGRATION.md](MIGRATION.md) for the complete history of the lib's API evolution. M1-M6 (DSL alignment) and M7 (open envelope + folder catalog) all applied.

## What's coming next

1. **YAML/JSON DSL parser** — wraps the existing builder API with a parser that accepts YAML shorthand syntax (`string@email?`, `Item[]`, `~Category`, etc.). See [modules/framework/schema/dsl.md](../../../../modules/framework/schema/dsl.md). Likely a follow-up todo.
2. **Canonical-AST JSON wire format** — the AST already serializes to JSON via `schema-json.ts`. Future work: lock the wire format spec for KTSP transport (between bun ↔ Go bridge).
3. **Codegen target host** — per-target plugin shape consuming AST + opts and emitting text. Lives at [src/targets/](../targets/).

## Authoritative references

- Architecture: [framework/schema/architecture.md](../../../../modules/framework/schema/architecture.md)
- Decisions: [where the universe's data types live](../../../../modules/framework/decisions/OD-004-where-do-the-universe-s-data-types-live-and.cwm.md) (schema side), [where the UI agents and sidecars use lives](../../../../modules/display/decisions/OD-006-where-is-the-ui-that-agents-and-sidecars.cwm.md) (UI side)
