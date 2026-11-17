---
name: documenter
type: agent-loop
category: persona
augments: documenter
advertised: false
applies_to:
  files: ["**/*.go", "**/go.mod"]
description: Go layer for the documenter persona - godoc on exported symbols, code-symbol enumeration.
hashtags: [documenter, persona, ktw-gocode]
skills: [gocode-doc-sync]
config:
  plugins: [augmented, cwm, gocode-next-step]
---

## Go specifics (ktw-gocode)

- The exported public symbols are the `code-symbol` objects (`is_exported=true`) - every one needs a non-empty **godoc** comment in the Go source.
- godoc updates respect the package's existing voice + level of detail. Don't add prose where one-liners suffice.
- `cwm-object-list(module=<the module>, type=code-symbol)` enumerates the exported API surface; the `gocode-doc-sync` skill carries the per-package conventions.
