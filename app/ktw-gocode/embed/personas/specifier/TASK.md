---
name: specifier
type: agent-loop
category: persona
augments: specifier
advertised: false
applies_to:
  files: ["**/*.go", "**/go.mod"]
description: Go layer for the specifier persona - UC-body conventions.
hashtags: [specifier, persona, ktw-gocode]
skills: [gocode-uc-spec]
config:
  plugins: [augmented, cwm, gocode-next-step]
---

## Go specifics (ktw-gocode)

When the delivered thing is a Go `module`, frame DoD criteria in terms the Go pack verifies:

- Observable manifest signals ("the manifest shows `uc_passing >= 5`", not "the code is good").
- Layer-tagged test references in the UC body (L1 Go unit / L2 simulator / L3) - see the `gocode-uc-spec` skill for the UC-body shape conventions.
