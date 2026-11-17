# Content-types catalog

This directory is the **runtime catalog** of every content type the data-DSL knows about — defaults, sidecar contributions, user-authored variants, LLM-generated ones. **One folder = one content type.** Same shape for everything; no special-casing of "framework defaults" vs "third-party".

The folder name is the underscore-prefixed leading char by convention reserved for non-content-type folders (this README is `_README.md`; private helper folders shared between content types may use a `_` prefix and are skipped by the registry loader).

## What lives in a folder

```
<name>/
├── manifest.yaml       required — name, kind, basePrimitive, description, configSchema
├── types.ts            required when a renderer.tsx exists — declares the <Name>Content TS interface
├── renderer.tsx        optional — FieldRendererFactory<SType, CType> default export (Mantine + react-hook-form)
├── i18n/               optional — per-locale message bundles (e.g., en.json, fr.json) loaded as i18next namespaces
└── README.md           optional — human-facing docs for this content type
```

## Implemented vs stub

A folder counts as **implemented** iff it has a `renderer.tsx`. The registry's `hasRenderer` flag exposes this. The harness:
- Implemented → mounts the renderer at the right slot (form / view / column).
- Stub → renders a notice with the manifest description; the field remains visible in the schema/case page.

## Initial v1 catalog (25 entries)

### Implemented (7)

These ship in M7 and are wired in [src/runtime/registry-wire.ts](../src/runtime/registry-wire.ts):

| Folder | Kind | basePrimitive | Notes |
|---|---|---|---|
| [text/](text/) | field-type | string | default for kind=string with no content |
| [email/](email/) | field-type | string | HTML5 email validation; mailto link in view |
| [markdown/](markdown/) | renderer | string | Tabs editor (write/preview); ReactMarkdown view |
| [number/](number/) | field-type | number | default for kind=number; Mantine NumberInput |
| [rating/](rating/) | renderer | number | Mantine Rating widget |
| [slider/](slider/) | renderer | number | Mantine Slider |
| [boolean/](boolean/) | field-type | boolean | default for kind=boolean; Checkbox or Switch via `editAs` |

### Stubs awaiting a renderer (18)

Manifests + `configSchema` only; no `renderer.tsx` yet. The `Authored by` column points at the gui-* todo that will deliver the implementation:

| Folder | basePrimitive | Authored by |
|---|---|---|
| [date/](date/) | string | gui-advanced |
| [html/](html/) | string | gui-rich-text |
| [url/](url/) | string | gui-advanced |
| [uuid/](uuid/) | string | gui-advanced |
| [avatar/](avatar/) | string | gui-media |
| [label/](label/) | string | gui-rich-text |
| [icon/](icon/) | string | gui-rich-text |
| [image/](image/) | string | gui-media |
| [phone/](phone/) | string | gui-advanced |
| [password/](password/) | string | gui-advanced |
| [code/](code/) | string | gui-rich-text |
| [json/](json/) | string | gui-rich-text |
| [color/](color/) | string | gui-advanced |
| [currency/](currency/) | number | gui-advanced |
| [percentage/](percentage/) | number | gui-advanced |
| [progress/](progress/) | number | gui-advanced |
| [file/](file/) | string | gui-media |
| [toggle/](toggle/) | boolean | gui-advanced |

## Adding a new content type

### 1. Create the folder

```
content-types/<name>/
```

`<name>` must be lowercase kebab-case starting with a letter (matches `/^[a-z][a-z0-9-]*$/`).

### 2. Author `manifest.yaml`

```yaml
name: <name>
kind: field-type        # field-type | component | renderer
basePrimitive: string   # string | number | boolean (omit for kind=component)
description: One-line human-facing description.

configSchema:
  fields:
    <param-name>:
      type: string      # string | number | boolean | array
      optional: true
      default: <value>
      description: What this param does.
```

The manifest grammar is described in [schema content types](../../../../../modules/framework/schema/content-types.md#where-a-content-type-lives).

### 3. Author `types.ts` (when shipping a renderer)

```ts
// Each fields/<name>/ folder is two levels under content-types/, so
// reaching back into src/ requires three `../`. Imports for the rest
// of the framework follow the same pattern (../../../src/ui/fields/, etc.).
import type { ContentType } from '../../../schema/schema-contents.ts'

export interface MyContent extends ContentType {
  type: '<name>'
  // mirror the manifest's configSchema.fields here as TS types
  myParam?: string
}
```

The TS interface and the manifest's `configSchema.fields` should stay in sync. Tests in `tests/content-types/parity.test.ts` (S9r2) check this.

### 4. Author `renderer.tsx`

Use the unified factory shape `{ view, form, header, column }`. Each sub-factory takes `{ def, info, texts }`:

- `def`: the UIFieldDef or UIColumnDef the caller declared
- `info`: the renderer-ready CrudFieldInfo (carries `info.schema`, `info.name`, `info.contentType`, `info.label`, …)
- `texts`: i18n facade scoped to the renderer's namespace (`useTexts('<name>')` from inside render fns)

See [text/renderer.tsx](text/renderer.tsx) for the simplest example, [boolean/renderer.tsx](boolean/renderer.tsx) for a per-config branch (Checkbox vs Switch).

### 5. Wire into `registry-wire.ts`

Add the import + entry to [src/runtime/registry-wire.ts](../../src/runtime/registry-wire.ts):

```ts
import MyRenderer from '../../content-types/fields/<name>/renderer.tsx'

const FACTORIES: Record<string, FieldRendererFactory<any, any, any>> = {
  // ... existing
  '<name>': MyRenderer,
}
```

### 6. (Optional) Author translations

```
content-types/<name>/i18n/en.json
{
  "<key>": "Translation"
}
```

Loaded as i18next namespace `<name>` at runtime startup. Access from a render function via `useTexts('<name>').t('<key>')`.

### 7. Restart the bun sidecar

The registry cache reloads at process start. Verify by visiting the harness at `http://localhost:<port>/test` and adding a test case that uses your new content type.

## Folders skipped by the loader

- Names starting with `_` (e.g., `_shared/` for renderer-private utilities)
- Names starting with `.` (e.g., `.cache/`)
- Folders missing `manifest.yaml` (logged as a load diagnostic)

## Authoritative refs

- Manifest grammar: [schema content types](../../../../../modules/framework/schema/content-types.md#where-a-content-type-lives)
- Renderer factory contract: [src/ui/fields/renderer.ts](../src/ui/fields/renderer.ts)
- Field info aggregation: [src/ui/fields/info.ts](../src/ui/fields/info.ts)
- i18n facade: [src/ui/i18n/index.ts](../src/ui/i18n/index.ts)
- Runtime bundle wiring: [src/runtime/registry-wire.ts](../src/runtime/registry-wire.ts)
