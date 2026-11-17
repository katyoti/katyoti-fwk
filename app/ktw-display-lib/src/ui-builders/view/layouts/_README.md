# view-layouts

Pluggable layout strategies for the `<UIView>` composition. Each subfolder under `view-layouts/<name>/` is a layout: it carries a `manifest.yaml` (kind: `view-layout`) and a `renderer.tsx` (default-exporting a `(groupsDef: UIViewGroups) => UIViewRenderer` factory).

Mirrors the `form-layouts/` shape introduced in Subtask 2 of `gui-form`. Both registries share `scanCatalog()` (in `src/content-types/registry.ts`); their `dispatch()` only differ in the per-kind default name (`LAYOUT_DEFAULTS.view = 'detail-pane'` vs `.form = 'groups'`).

## Selection priority

1. **URL `?view=<name>`** — host can force a specific named view variant (per-form-instance override)
2. **DSL `ui-view.layout`** — schema-author preference baked into the schema's `ui-view` extension
3. **`LAYOUT_DEFAULTS.view`** — `'detail-pane'` (when neither URL nor DSL specifies)

## Folder shape

```
view-layouts/
  _README.md              ← this file
  detail-pane/            ← DEFAULT
    manifest.yaml         ← kind: view-layout, name: detail-pane, configSchema.fields.{spacing}
    renderer.tsx          ← default-export `(groupsDef) => UIViewRenderer`
  card/                   ← each group → Card with shadow
    manifest.yaml
    renderer.tsx
  inline/                 ← horizontal field row, no group framing
    manifest.yaml
    renderer.tsx
  two-column/             ← two-column SimpleGrid
    manifest.yaml
    renderer.tsx
  tabs/                   ← each group → Tab
    manifest.yaml
    renderer.tsx
```

## Adding a 6th layout

1. Create `content-types/view-layouts/<name>/manifest.yaml` (kind: `view-layout`, optional `configSchema.fields`)
2. Create `content-types/view-layouts/<name>/renderer.tsx` exporting a default `(groupsDef: UIViewGroups) => UIViewRenderer` factory
3. Add an import + an entry in `VIEW_LAYOUT_FACTORIES` in `src/runtime/registry-wire.ts`
4. Re-bundle (server restart)

## v1 layouts

| Layout | Description | When to use |
|---|---|---|
| `detail-pane` | Vertical Stack with title header + groups stacked | DEFAULT — record-detail expansion |
| `card` | Each group → bordered Card with shadow | Visual emphasis per section |
| `inline` | Flat horizontal Group of all fields | Compact row-summary surfaces |
| `two-column` | Two SimpleGrid columns, balanced | Wider viewports / dense layout |
| `tabs` | Each group → Mantine Tab | Long views with naturally distinct sections |
