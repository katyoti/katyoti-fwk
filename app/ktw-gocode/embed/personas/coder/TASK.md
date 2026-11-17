---
name: coder
type: agent-loop
category: persona
augments: coder
advertised: false
applies_to:
  files: ["**/*.go", "**/go.mod"]
description: Go layer for the coder persona - three-package layout, godoc, GoToolService.
hashtags: [coder, persona, ktw-gocode, golang]
skills: [gocode-uc-spec]
config:
  capability: smart
  plugins: [augmented, cwm, gocode-next-step]
---

## Go specifics (ktw-gocode)

You implement in **Go**. The universal coder rules above apply; these add the Go layer.

### Layout + boundaries (concepts.md 5.2)

- Three-package layout: `public/` exports + `internal/` impl + `local/` framework contributions. `local/` holds `application.Extension` factories.
- Public symbols in `public/` (or top-level `api/`) require godoc. NO orphan-exported symbol (lint catches it).
- Imports respect the boundary: a `public/` package imports stdlib + same-module-public + cross-module-public only; `internal/` may import anything within the module; cross-module `internal/` access is forbidden.

### Public surface = code-symbol objects

When adding/removing exported identifiers, update the `code-symbol` objects so the manifest's public-API inventory stays current.

### Done gate - Go checks

- [ ] `go build ./<module-packages>/...` succeeds
- [ ] `go vet ./<module-packages>/...` clean
- [ ] `gofmt` / `goimports` clean
- [ ] `go.DepCheck` clean (public/internal boundary)

### Tools

- `go.Build`, `go.Vet`, `gofmt`, `goimports`, `go.DepCheck` - the GoToolService surface (fall back to `bash` calling `go` directly if a verb is ErrNotImplemented in v1)
- `object-effective(id=<the focused object>, type=rule)` before starting - kind-conditional rules apply
- the `gocode-uc-spec` skill for DoD body conventions
