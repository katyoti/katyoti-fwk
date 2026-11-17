---
name: architect
type: agent-loop
category: persona
augments: architect
advertised: false
applies_to:
  files: ["**/*.go", "**/go.mod"]
description: Go layer for the architect persona - module-kind selection, three-package layout, module.discover.
hashtags: [architect, persona, ktw-gocode]
skills: [onboard-existing-code, gocode-module-decompose]
config:
  plugins: [augmented, cwm, gocode-next-step]
---

## Go specifics (ktw-gocode)

For a Go `module`, the placement work has a concrete shape.

### The kind decision - pick exactly one

- **`framework`** - provides extension points; consumed via plugin registration; rarely imported by name (e.g. the `application` package, `internal/kts/`). STRICT: stdlib + intra-framework imports only.
- **`library`** - exports a typed API consumed via import paths (e.g. `internal/extensions/cwm/api/`). Public surface in `public/` (or top-level `api/`); impl in `internal/`.
- **`sidecar`** - produces an executable under `app/<name>/`; MUST have `cmd/{root,bootstrap}.go` + a buildable main. Public surface aimed at the spawning hub.

Heuristic: has `cmd/main.go`? sidecar. Imported widely without `app/`? library. Hosts a contribution point? framework. Still uncertain -> record an OD.

### Greenfield + retro-engineering

- New module: set `kind` + declare the code `facets` (the package paths).
- Retro: call `module.discover` on existing Go paths and curate its proposals (auto-commit code-symbols, propose-for-review UCs). The `onboard-existing-code` skill walks this; `gocode-module-decompose` covers the kind/layout heuristics.
