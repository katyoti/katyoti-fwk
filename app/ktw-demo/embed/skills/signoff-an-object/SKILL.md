---
name: signoff-an-object
description: "How to sign off an object that carries ktw-demo's sign-off capability: read its sign-off, write notes and a verdict, contest another item, and what the gate signoff-complete asks before approval."
category: demo
hashtags: [demo, signoff, cwm]
iconName: eye-check
icon: "👁️"
---

# Signing off an object

An object carrying the **signoff** capability has a sign-off group
(`capabilities.signoff`): `reviewers`, `approvals`, `verdict` (`pending`,
`approved`, `changes`) and `notes`. Its type says how many approvals it needs
(the type's `signoff.required_reviewers`; one when unset).

## Read where the sign-off stands

- `ktw-demo:signoff/board {id}` draws the Sign-off tab: approvals against the
  requirement, the verdict, the reviewers, what contests the object, the log.
- `ktw-demo:signoff/log {id}` lists every write of the sign-off (its on_update
  hook appends one line per write).
- `signedoff()` in an expression answers whether the approvals are there.

## Write

- Notes and verdict: `ktw-demo:signoff/write {id, verdict, notes}`. An `approved`
  verdict is refused (the capability's before_update hook) until the object has
  its approvals; the refusal says how many are missing.
- Approving is a reviewer's own act (`ktw-demo:signoff/approve`), never an
  agent's.
- To dispute another item: `ktw-demo:signoff/contest {id: <contested>,
  contester: <this object>}` — the contested item gains a Contests tab.

## Move

A sign-off item follows the `signoff` workflow: `draft` → `in-review` →
`approved` (gated by `gate('signoff-complete').passes`: reviewers named, the
approvals there) or `changes-requested`; a person closes it. Ask
`ktw-agents:gates/check {id, gate: signoff-complete}` for the checklist before
proposing a move.

## Check

- Lint (`object-lint`): no reviewer, changes without notes, an approval from
  someone who is not a reviewer.
- Verify (`object-verify`): pass when approved with its approvals, fail when
  changes are asked, unproven otherwise; the policy `signoff-has-notes` also
  fails a sign-off without notes.
