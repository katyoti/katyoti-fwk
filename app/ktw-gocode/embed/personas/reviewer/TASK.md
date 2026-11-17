---
name: reviewer
type: agent-loop
category: persona
augments: reviewer
advertised: false
applies_to:
  files: ["**/*.go", "**/go.mod"]
description: Go layer for the reviewer persona - Go lint triage + per-perspective audit checklists.
hashtags: [reviewer, persona, ktw-gocode]
skills: [gocode-code-review]
config:
  plugins: [augmented, cwm, gocode-next-step]
---

## Go specifics (ktw-gocode)

- `module.lint` returns the Go detector set (incl. `SidecarMissingAppFolder`, `MissingDoDBody`, drift detectors). Triage each: defect, deferred via OD, or false-positive (record reasoning).
- The `gocode-code-review` skill carries the per-perspective checklists (architecture / security / correctness / performance / dead-code / cli-ux). You may also call the `ktw-code:code-review` skill.
- Reproduction paths are Go-flavored: `file:line` or the `go test -run <TestName>` invocation.
