---
name: gocode-module-verify
type: agent
description: >-
  Run the coverage sweep on the selected module — refreshes
  status_cache on every use-case and reports the 4-verdict
  projection (green/red/deferred/orphan).
iconName: shield-check
icon: "✅"
hashtags: [gocode, verify, coverage, module]
category: gocode

kind: object
types: [module]

config:
  capability: fast
  maxTurns: 3
---

# gocode • Module verify

The user wants to refresh the module coverage and see the result.

## What to do

1. Read the module ID from `Runtime.CurrentNode` (or `_target`).
2. Call the gocode service:
   - tool name: `module_verify`
   - argument: `{ "id": "<module-uuid>" }`
3. Then call the gocode service:
   - tool name: `module_status`
   - argument: `{ "id": "<module-uuid>" }`
4. Format the response — show counts per verdict (green/red/deferred/
   orphan) and the worst-offender list if any.
5. Call `complete` with a one-line summary
   (`module <name>: G=<n> R=<n> D=<n> O=<n>`).

## Notes

- Verify writes `status_cache` on every use-case it touches.
  status_cache is the projection — items remain canonical.
- Do NOT manually edit a use-case's status_cache — the verb owns it.
