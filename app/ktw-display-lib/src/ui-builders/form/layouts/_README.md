# Form-layouts catalog

Per-form layout strategies — `single` / `groups` / `tabs` / `wizard`. Each folder is a content-type that ships a `UIFormRenderer` factory. The form widget consults `getFormLayoutRegistry()` at render time to pick the layout by name.

## Selection priority

1. URL query param `?layout=<name>` (highest)
2. DSL `ui-form` extension's `layout:` field
3. Default `'groups'` (per `LAYOUT_DEFAULTS` in [src/content-types/manifest.ts](../../src/content-types/manifest.ts))

## What lives in a folder

```
<name>/
├── manifest.yaml       required — name, kind: form-layout, description, configSchema
├── types.ts            optional — per-layout config interface
├── renderer.tsx        required — exports the layout factory function
└── README.md           optional — human-facing docs (when, why, screenshot)
```

The `manifest.yaml` shape mirrors the field manifests but uses `kind: form-layout`. There is no `basePrimitive` (form-layouts aren't bound to a schema base type — they apply to any form built from any object schema).

```yaml
name: groups
kind: form-layout
description: Vertical Stack of collapsible sections
configSchema:
  fields:
    md:        { type: number, default: 12, description: "Default group span (1-12)" }
    grow:      { type: boolean, default: true }
```

## Registry mechanics

[src/content-types/registry.ts](../../src/content-types/registry.ts) exposes `getFormLayoutRegistry()` parallel to `getFieldRegistry()`. The runtime bundle ([src/runtime/registry-wire.ts](../../src/runtime/registry-wire.ts)) statically imports each `form-layouts/<name>/renderer.tsx` and registers them via `wireFormLayouts()`.

## Pluggability

Drop a third-party layout under `content-types/form-layouts/<your-layout>/{manifest.yaml, renderer.tsx}` and it appears in the registry on next process restart. The form widget then accepts `?layout=<your-layout>` (or `ui-form: { layout: <your-layout> }`) and routes through your renderer.

## v1 layouts (4 entries)

| Folder | Status | Notes |
|---|---|---|
| `single/` | NEW (subtask 3 of gui-form) | Single group, no section title, fields in sequence. Trivial — validates the registry pattern works for the simple case. |
| `groups/` | extracted from old-UI `fol-form.tsx` | DEFAULT. Vertical Stack of collapsible sections, each containing a SimpleGrid of fields. |
| `tabs/` | extracted from old-UI `fol-tabs.tsx` | Each group becomes a Mantine Tab. |
| `wizard/` | NEW (subtask 3 of gui-form) | Step-by-step navigation: prev/next buttons + progress indicator + per-step validation gating. Stretches the pluggable pattern to its limits. |
