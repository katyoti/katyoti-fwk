# @ktw/display-lib

Schema-driven UI rendering library for the ktw ecosystem.

## What's here

- `src/schema/` — Schema AST types + query helpers (the canonical shape every consumer of a DSL schema reads).
- `src/ui-core/` — Field renderers, action surface, filter surface, i18n adapters, presentational components.
- `src/ui-builders/` — Form / view / table / wizard / page / drawio builders. Each takes a schema AST + a named `ui-*` extension and returns a React component.
- `src/content-type-registry/` — Loader + registry for content-types (e.g. `email`, `markdown`, `rating`).
- `src/types/`, `src/utils/` — Cross-cutting infrastructure (JSON types, json-path, override merging).
- `content-types/` — Per-content-type manifests + renderers (`fields/`, `form-layouts/`, `view-layouts/`).

## What's NOT here

- HTTP server / iframe runtime → lives in `@ktw/display` (the bun sidecar)
- Widget URL composition → `@ktw/display`'s `widget-url.ts`
- KTSP membership → `@ktw/ktsp-sdk`

## Consumers

- **`@ktw/display`** — wraps the renderers in an iframe runtime served over HTTP. Used by LLM-streamed UI artifacts + remote browser scenarios.
- **`ktw-gui` frontend** — imports the renderers directly into its Wails-hosted React tree. Used for trusted built-in surfaces (task input forms, item editors, node editors).

The same renderer code drives both delivery modes. One source of truth.

## Status

v0.1 — extracted from `@ktw/display` on 2026-06-02 as part of the ms-schema-driven-ui US-S1 work (universal task-run form surface).
