# archdiagram

The drawio engine's internal pipeline. Callers outside the extension go
through [`../../render/`](../../render/) (`EmitDiagramV2WithLogical`,
`Validate`, `Lint`, `LayoutELKGraph`, `Export`).

1. **Pivot** — [`schema.go`](schema.go): `ArchDiagram` / `Node` / `Edge` /
   `Zone`, built from a `LogicalDiagram` by the translator in `render/`.
2. **Layout** — [`elk.go`](elk.go) runs elkjs through bun on the pivot;
   [`layouts.go`](layouts.go) reads the host's named presets
   (`embed/elk-layouts/`). Node boxes come from the stencil set's
   `defaultSize`, set before layout so layout and emit agree.
3. **Style resolver** — [`style_resolver.go`](style_resolver.go) evaluates a
   `*StencilSetBody` against a node's `Scope` and produces the drawio style
   string, the decoration cells, the HTML label and its tooltip.
4. **Emit** — [`render_v2.go`](render_v2.go) (`RenderV2`: layout hints →
   defaults → sizing → ELK → emit), [`emit_drawio_v2.go`](emit_drawio_v2.go)
   (`DrawIOEmitterV2`) and [`legend.go`](legend.go) (one row per kind used).

Authoring reference for stencil sets:
[`app/ktw-docs/embed/references/stencil-authoring.md`](../../../../../app/ktw-docs/embed/references/stencil-authoring.md).

## Tests

- `style_resolver_test.go` — the 6 resolver primitives, the entry points, helpers.
- `style_resolver_golden_test.go` — a `<name>.input.json` + `<name>.expected.json`
  pair under `testdata/style_resolver/` is a new case; no code change.
- `render_v2_golden_test.go` — whole diagrams under `testdata/render_v2/`,
  rendered without layout; `KTW_REGEN_GOLDEN=1` rewrites the expected files.
- `legend_test.go`, `render_v2_test.go`, `truncate_tooltip_test.go`.
