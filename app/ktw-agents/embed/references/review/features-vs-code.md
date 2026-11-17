# Review guide — features versus code

**What it answers:** does the module's code deliver what its features say,
does what was proven stay proven as the code moves, and does the code do
things no feature declares?

## Read

- The module object: its `code_packages` (the code it owns).
- Its features (`modules/<m>/features/`): state, `realizes` edges (the code
  paths that deliver each), `serves` edges, and their acceptance criteria
  (`modules/<m>/use-cases/`) with their verdicts (`modules/<m>/verdicts/`).
- The freshness table: `bash scripts/feature-alignment.sh <m>` — per feature
  its state, whether an agent verified it, its criteria (pass/fail/never
  run), when its proof last ran and when its code last changed. A feature
  with no `realizes` edge is dated by the whole module's code, so it turns
  stale on any commit to the module.
- The code itself: every package of `code_packages` — its exported API, its
  service methods, its commands, its contributions (`embed/`), its tests.
- The previous review of this type, if any, so a problem still there keeps
  its location and key.

## Judge

1. **Declared → code.** For each feature in state `realized` or `done`:
   - its `realizes` paths exist and hold code that does what the feature
     says (read it, not only its name); a feature with no `realizes` edge
     gets the paths you found;
   - its criteria run (`ktw-agents verification verify --id <feature>`);
     with none, find the tests that prove it (`go test -run …` on the
     packages it realizes) and propose a criterion with that command;
   - verdict, one per feature: **delivered** (the code does it, its
     criteria pass or a test you ran proves it), **partial** (part of it is
     missing — say which), **absent** (the code does not do it), **regressed**
     (a `done` feature whose criteria now fail or whose code was removed).
2. **Freshness.** A feature whose code changed after its proof (`stale`)
   has its criteria run again; one that still passes is delivered, one that
   fails is regressed.
3. **Code → declared.** List what the code does that no feature's `realizes`
   edge or wording covers: a service method family, a command group, a
   contributed panel or task, a protocol — each a capability a person or
   another module would name. Ignore helpers, glue and tests. Each is an
   **undeclared** finding with the paths that do it and a feature title
   worded as an outcome ("A person …", "An agent …").
4. **Accepted or proposed but already built.** A feature in `accepted` or
   `proposed` whose code is there is **already built** (record it realized).

## Rate

`critical` when a `done` feature regressed or a `realized` feature the
module exists for is absent; `needs-attention` for partial or absent
features, stale proofs, undeclared capabilities or features with no
`realizes` edge; `good` otherwise. Score: the share of `realized` and `done`
features judged delivered (0 to 100).

## Report

A table of every realized and done feature (feature, state, verdict,
criteria run, proof date, code change date, evidence), then: the stale
proofs and what running them again gave; the features to reopen and what is
missing; the undeclared capabilities with their paths and proposed titles;
the features with no `realizes` edge and the paths found; what to do next.

## Findings

One per feature that is not delivered, per stale proof that fails, per
undeclared capability and per missing `realizes` edge. Keys, stable across
runs: `feature-absent`, `feature-partial`, `feature-regressed`,
`proof-stale`, `capability-undeclared`, `realizes-missing`. Location: the
feature id (or, for an undeclared capability, its main package path).
Recommendation: the move below.

## Apply

A run inside a review (`review-run`) only answers; a session that may write
(a Claude Code session working the module, or an agent on a to-do) applies
the moves through the verbs, never by editing a file:

| Verdict | Move |
|---|---|
| delivered, criteria pass | `ktw-agents cwm object-transition --id <f> --to done --transition verify` |
| delivered, no criterion | `ktw-agents action run --uri ktw-methods:features/attach-criterion --input '{"feature":"<f>","title":"<what it proves>","probe":"<the test command>"}'` (show it can fail first), run `verification verify --id <f>`, then verify as above |
| partial or absent (realized) | `cwm object-transition --id <f> --to proposed --transition reopen --note "<what is missing>"` |
| regressed (done) | the same `reopen`, its note naming the failing criterion or the removed code |
| already built (accepted, proposed …) | `cwm object-transition --id <f> --to realized --transition record-realized` |
| undeclared | `action run --uri ktw-methods:features/add --input '{"module":"<m>","title":"…","serves":["<goal>"]}'` (or `"parent"` for a sub-feature), `realizes` edges to its paths, then `record-realized` |
| `realizes` missing | `ktw-agents cwm edge-create` (kind `realizes`, from the feature to the path) |

A realized feature stays for a person to confirm unless its verification
passes; nothing is dropped by this review.
