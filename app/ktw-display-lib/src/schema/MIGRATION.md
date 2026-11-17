# Schema lib — DSL alignment migration log

> **Purpose**: track every change to the lib's types/fields made during the **DSL alignment sweep** (started 2026-05-01) so consumers can be migrated lock-step. The lib at `_ktw_workspace/old_ui_main/src/modules/core/schema/` (the source of this lib) had several callers in `cpm-core/` and `cpm-ui/` (and other places) — when those libs are imported into the active codebase, they must be updated to the new shape.

## Why this file exists

The lib was promoted on 2026-05-01 from `_ktw_workspace/old_ui_main/...` as the canonical TS schema authority for refined Architecture C. Then we discovered the lib's field names didn't match the DSL grammar in [modules/framework/schema/dsl.md](../../../../modules/framework/schema/dsl.md). The user-led mismatch resolution sweep aligns the lib to the DSL.

This means **every external consumer of the lib's types** (now and future-imported) needs the same renames applied. This file is the migration recipe.

## Status legend

- ✅ **applied** — change has landed in the lib
- 🟡 **pending** — change is decided but not yet applied
- 🔴 **breaking** — consumer code referencing the old shape will fail to compile

## Migration table

> **All changes are 🔴 breaking** for consumers. Apply lock-step.

### M1 — Schema-level fields (decided 2026-05-01)

| Old (lib) | New (lib) | Rationale |
|---|---|---|
| `Schema { types, typeName: string, version?: string, description? }` | `Schema { name: string, version?: number, description?, imports?: string[], root: string, types }` | Align with DSL `name` / `imports` / `root`; `version` becomes integer (rule [a config format version is a monotonic integer](../../../../modules/framework/rules/R-007-a-config-format-version-is-a-monotonic-integer.cwm.md)). DSL keeps `$schema` for YAML-file marker; lib doesn't carry it. `root` stays required (lib semantics). |

**Field renames**:
- `typeName` → `root`

**New fields**:
- `name: string` (required)
- `imports?: string[]`

**Type changes**:
- `version?: string` → `version?: number`

**Status**: ✅ **applied 2026-05-01** — `tsc` clean, smoke tests pass. Files touched: `schema-types.ts`, `schema-builder.ts`, `schema-query.ts`, `schema-json.ts`, `schema-utils.ts`, `tests/smoke.test.ts`.

### M2 — Array bounds (decided 2026-05-01)

| Old (lib) | New (lib) | Rationale |
|---|---|---|
| `ArrayType.minItems`, `ArrayType.maxItems` | `ArrayType.min`, `ArrayType.max` | Align with DSL `min` / `max` for array length. |

**Field renames**:
- `minItems` → `min`
- `maxItems` → `max`

**Status**: ✅ **applied 2026-05-01** — `tsc` clean, smoke tests pass.

### M3 — Default value (decided 2026-05-01)

| Old (lib) | New (lib) | Rationale |
|---|---|---|
| `BaseTypeWithContent.defaultValue`, `LinkType.defaultValue` | `BaseTypeWithContent.default`, `LinkType.default` | Align with DSL `default`. |

**Field renames**:
- `defaultValue` → `default`

**Status**: ✅ **applied 2026-05-01** — `tsc` clean, smoke tests pass.

### M4 — Detail level (decided 2026-05-01)

| Old (lib) | New (lib) | Rationale |
|---|---|---|
| `SchemaFieldOptions.detailLevel: 1 \| 2 \| 3` | `SchemaFieldOptions.detail: 1 \| 2 \| 3` | Align with DSL `detail`. |

**Field renames**:
- `detailLevel` → `detail`

**Status**: ✅ **applied 2026-05-01** — `tsc` clean, smoke tests pass.

### M5 — Naming convention + Link/Tree field names (decided 2026-05-01)

**Convention change**: lib's TS field names switch from camelCase to **snake_case** for JSON-bound schema fields. Aligns with DSL convention.

| Old (lib, camelCase) | New (lib, snake_case + DSL field names) |
|---|---|
| `LinkType.linkMode` | `LinkType.mode` |
| `LinkType.targetIdField` | `LinkType.target_id` |
| `LinkType.linkType` | `LinkType.link_type` |
| `LinkType.reverseField` | `LinkType.reverse` |
| `LinkType.displayField` | `LinkType.display` |
| `LinkType.allowCreate` | `LinkType.allow_create` |
| `LinkType.searchable` | `LinkType.searchable` ✓ (no change) |
| `TreeType.maxDepth` | `TreeType.max_depth` |
| `TreeType.rootValue` | `TreeType.root_value` |
| `TreeType.allowCircular` | `TreeType.allow_circular` |
| `TreeType.showPath` | `TreeType.show_path` |
| `TreeType.enableDragDrop` | `TreeType.drag_drop` |

**Field renames** (combined case + name):
- `linkMode` → `mode`
- `targetIdField` → `target_id`
- `linkType` → `link_type`
- `reverseField` → `reverse`
- `displayField` → `display`
- `allowCreate` → `allow_create`
- `maxDepth` → `max_depth`
- `rootValue` → `root_value`
- `allowCircular` → `allow_circular`
- `showPath` → `show_path`
- `enableDragDrop` → `drag_drop`

**Status**: ✅ **applied 2026-05-01** — `tsc` clean, smoke tests pass.

**Note**: this convention change implies the OTHER lib types should also be reviewed for camelCase → snake_case. Will sweep as part of the lib-side application; track here.

---

### M6 — Content type shape (decided 2026-05-01)

**Decision**: nested content structure (lib wins on shape), but rename the field and the discriminator.

**Final shape**:

```yaml
field:
  type: string                       # base type
  content:                            # nested object OR string ref
    type: label                       # content type discriminator (renamed from `content`)
    options: {...}                    # content-specific params
    colorMap: {...}
```

**Variants accepted**:
- `content: 'label'` — string ref, equivalent to `{ type: 'label' }` with no params
- `content: { type: 'label', options: {...} }` — inline object with params
- `type: 'string@label'` — shorthand on the type, expands to `{ type: 'string', content: { type: 'label' } }`

| Old (lib) | New (lib + DSL) |
|---|---|
| `BaseTypeWithContent.contentType: TContent` (where TContent extends `StringContentType \| StringContentTypeRef`) | `BaseTypeWithContent.content: ContentRef` where `ContentRef = string \| { type: string; ...params }` |
| Inside the discriminated content union: `{ content: 'date', pattern? }` | `{ type: 'date', pattern? }` (the **discriminator field is `type`**, not `content`) |

**Field renames**:
- `contentType` → `content` (on the BaseTypeWithContent / type itself)
- Inside the content object: `content` (the discriminator) → `type`

**Type changes**:
- Content can now be a string ref (`'label'`) OR an object — extending the previous union which already allowed both via `StringContentType | StringContentTypeRef`
- All concrete content variants in `schema-contents.ts` (e.g. `DateContentType`, `MarkdownContentType`) need their discriminator field renamed `content` → `type`

**DSL changes**:
- Long-form: params now nest under `content:` instead of being top-level keys on the field. So:
  ```yaml
  # OLD DSL (top-level params on type):
  status:
    type: string@label
    options: {...}
    colorMap: {...}
  
  # NEW DSL (params nested under content):
  status:
    type: string@label    # OR content: { type: label, options: {...}, colorMap: {...} }
    content:
      options: {...}      # OR keep as shorthand and inline params via (...)
      colorMap: {...}
  ```
- Shorthand still works: `string@label` expands to `{ type: 'string', content: { type: 'label' } }`
- Shorthand with params: `string@slider(min:0,max:100)` expands to `{ type: 'string', content: { type: 'slider', min: 0, max: 100 } }`

**Status**: ✅ **applied 2026-05-01** — `tsc` clean, smoke tests pass.

**Cross-cutting impact**: `schema-contents.ts` (377 LOC of content-type definitions) needs every content variant updated — change `{ content: 'X', ... }` → `{ type: 'X', ... }` in each typed union case. This is a sweep across the whole content catalog.

---

### M7 — Content-type catalog separation + open-shape (decided 2026-05-01)

**Decision**: separate the system primitives (10 BaseType kinds) from the predefined content-type catalog. Content types are NOT part of the base DSL library types — they're a default catalog shipped alongside the lib, structurally identical to user/sidecar/LLM-authored types. **Everything is open-shape**; typed discriminated unions for content types are dropped.

**Reframing**:
- **System primitives** (10 closed kinds: `string`, `number`, `boolean`, `object`, `array`, `choice`, `link`, `tree`, `any`, `null`) stay in the **core schema lib** at `src/schema/`
- **Content envelope** (`content?: ContentRef` where `ContentRef = string | { type: string, [key]: unknown }`) stays in the core schema lib — the SHAPE
- **Default content-type catalog** (the 25 typed variants currently in `schema-contents.ts`) moves to a **separate location**, ideally as **folders** matching the per-content-type manifest format from [schema content types](../../../../modules/framework/schema/content-types.md#where-a-content-type-lives) — each folder has `manifest.yaml + components/*.tsx`. This unifies default + user + sidecar + LLM-authored content types around ONE format.

**Final shape (TS-side)**:

```ts
// src/schema/schema-types.ts (core lib)
type ContentRef = string | { type: string; [key: string]: unknown }

interface BaseTypeWithContent<T> extends BaseType {
  default?: T
  content?: ContentRef          // open-shape — accepts ANY content type name + params
}
```

| Concern | Where | Format |
|---|---|---|
| BaseType (10 system primitives) | `src/schema/` core lib | TS discriminated union (closed) |
| Content envelope (the shape) | `src/schema/` core lib | Open `ContentRef = string \| { type, [key]: unknown }` |
| Default content-type catalog | `content-types/<name>/` folders (per § 3 manifest format) | YAML manifest + TSX components — same format as sidecar/user/LLM contributions |
| Sidecar / user / LLM-authored content types | `<homedir>/.ktw/content-types/`, `<sidecar>/embed/content-types/`, runtime registry | Same folder format |

**Cross-cutting impact** (the big one): `schema-contents.ts` (377 LOC) becomes essentially obsolete:
- The typed discriminated unions (`DateContent`, `MarkdownContent`, `LabelContent`, etc.) — drop or move to legacy
- Plain type aliases (`ContentColor`, `LabelVariant`, `StringPattern`, etc.) — likely keep as TS conveniences but they're optional now
- `DefinedStringContentRefMap`, `StringContentRefMap`, etc. — replace with a runtime registry built from the manifest folders

**Status**: ✅ **applied 2026-05-02** — landed via `unify-content-types-as-folders` (S1r2-S9r2). `tsc` clean, 34 tests pass, runtime bundle 1.2 MB JS / 334 KB CSS, end-to-end browser-verified.

**What landed**:
- `schema-contents.ts` reduced from 377 LOC to ~140 LOC. Dropped: every `*Content` typed variant (DateContent, MarkdownContent, AvatarContent, …), `StringContentType` / `NumberContentType` / `BooleanContentType` / `SpecializedContentType` unions, all `*Name` / `*Ref` aliases, `ContentTypeItem`, `BooleanIconTypes`, `BooleanEditType`. Kept: open `ContentType = { type: string }`, new `ContentRef = string | ContentType`, color/label/transformation/options aliases that `src/ui/` consumes.
- `schema-types.ts` `BaseTypeWithContent.content` is now `ContentRef`. Per-base-type generics dropped from `StringType`/`NumberType`/`BooleanType`. Renderer narrowing happens at the renderer's TSX level via `FieldRendererFactory<SType, CType>`.
- `schema-query.ts` `getTypeContent()` rewritten to use `KIND_DEFAULT_CONTENT` (string→text, number→number, boolean→boolean) + the open envelope. Legacy `*RefMap` exports + `TypeWithContentRefMap` removed.
- 25 default content types now live as folders at `app/ktw-display/content-types/<name>/` (7 implemented Mantine TSX renderers + 18 stub manifests awaiting gui-rich-text/gui-media/gui-advanced).
- React+Mantine iframe runtime built via Bun.build, served at `/widget/runtime.{js,css}`.
- Tests: `tests/content-types/registry.test.ts`, `parity.test.ts`, `tests/runtime/bundle.test.ts`, `tests/ui/factory.test.ts`.

**Reference docs added**: [content-types/_README.md](../../content-types/_README.md) (contributor guide), [src/ui/](../ui/) (UI module skeleton), [src/runtime/](../runtime/) (bundle pipeline).

---

## All 7 mismatches resolved (M1-M7 applied)

- M1-M6 applied 2026-05-01 (initial DSL alignment sweep)
- M7 applied 2026-05-02 (open-shape envelope + folder-based content-type catalog + Mantine TSX renderers, via `unify-content-types-as-folders`)

When applied to consumers in `_ktw_workspace/old_ui_main/`, every match listed below breaks. Migration is mechanical (rename per the table) and lock-step.

---

## Application checklist

When applying the migration to the lib source files (`schema-types.ts`, `schema-builder.ts`, `schema-contents.ts`, `schema-query.ts`, `schema-utils.ts`, `schema-json.ts`, `index.ts`):

1. Apply each renaming in `schema-types.ts` first (the type definitions)
2. Update `schema-builder.ts` to use new field names in its `addType` / `addField` / `addExtension` flows
3. Update `schema-query.ts` to read new field names in walk/find/resolve helpers
4. Update `schema-utils.ts` and `schema-json.ts` if they reference the renamed fields
5. Run `bun run tsc` after each file to catch breakage early
6. Run `bun test` to validate the smoke test still works
7. Move each row in this file from 🟡 pending → ✅ applied

## Consumer migration checklist (for future imports)

When importing `cpm-core/`, `cpm-ui/`, `contribution-points/`, or any other code that referenced the original schema lib types:

1. Search for the old field names listed above
2. Apply the renames mechanically
3. Verify TS compiles
4. Verify behavior with a smoke test

Affected files identified 2026-05-01 (in `_ktw_workspace/old_ui_main/`):

- `modules/core/contribution-points/menu-entry-contribution-point.ts`
- `modules/core/cpm-core/common/field-types/base.ts`
- `modules/core/cpm-ui/test/product.schema.tsx`
- `modules/core/cpm-ui/test/site-info.schema.tsx`
- `modules/core/cpm-ui/test/types.ts`
- `modules/core/cpm-ui/ui-crud/hooks/useCrudFieldHelpers.ts`
- `modules/core/cpm-ui/ui-fields/renderers/renderer-factory.ts`
- `modules/core/cpm-ui/ui-fields/renderers/renderer-types.ts`
- `modules/core/cpm-ui/ui-fields/renderers/_backup/*` (backup files; may be deleted instead of migrated)
- `modules/core/cpm-ui/ui-text/utils.ts`

## Sources of truth

- DSL grammar: [modules/framework/schema/dsl.md](../../../../modules/framework/schema/dsl.md)
- Architecture: [framework/schema/architecture.md](../../../../modules/framework/schema/architecture.md)
- Decisions: [where the universe's data types live](../../../../modules/framework/decisions/OD-004-where-do-the-universe-s-data-types-live-and.cwm.md) (schema side), [where the UI agents and sidecars use lives](../../../../modules/display/decisions/OD-006-where-is-the-ui-that-agents-and-sidecars.cwm.md) (UI side)
- Source ADR for the alignment sweep: this conversation 2026-05-01 (will be linked when the implementing todo lands)
