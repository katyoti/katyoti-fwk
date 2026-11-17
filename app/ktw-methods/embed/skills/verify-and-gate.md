---
name: verify-and-gate
description: >-
  The verify/gate seam — how a deliverable is proven and how "done" / "goal
  met" are COMPUTED, not declared. Load when wiring verification for a
  created artifact: run each acceptance criterion's probe, record the
  evidence, and let the gate decide done. Domain-neutral (compose / da /
  code); each domain plugs in its own probe-runner. Never an LLM judgment.
iconName: shield-check
icon: "🛡️"
hashtags: [verify, gate, acceptance, provenance, quality, methodology]
category: lifecycle
---

# Verify and gate — proof, not assertion

The last lens of the creation method (`ktw-methods:/skill/create-method`):
once an artifact is produced, you do not *assert* it is done — you **verify**
it and let a **deterministic gate** decide. "Done" and "goal met" are
**computed convergence** (lean-increment-flow §7.7), never a hand-flipped
checkbox or a chat scorecard.

## If it is provable, prove it — no assumption

A claim is either *provable* or *judged*. When a deterministic check exists — a
lint, a probe, a test, a URI that resolves — **run it; never assume it would
pass**. A confident reading of the rules is not evidence that the artifact obeys
them; the check is. This binds your OWN reasoning hardest: "the format guard
would catch that", "the test surely passes", "it looks done" are assumptions
until a probe says so. When something can be proven, the only honest verdict is
the one the probe returns — so reach for the probe first, before you conclude.
Reserve *judgment* for what genuinely cannot be probed, and then say so
explicitly (`manual:`), never letting it auto-pass. Everything below is this rule
made mechanical.

## The model — what carries the proof

| Item | Field | Meaning |
|---|---|---|
| `deliverable` | `addresses: [<uc-id>]` | the use-cases this artifact must satisfy (its acceptance) |
| `deliverable` | `produced_by` / `produced_at` / `evidence` | provenance — set ONLY on the verified transition to `done` |
| `use-case` | inline `acceptance_criteria[]` (`test_ref`, `status`) | the atomic claims + their probe + result |
| `use-case` | `status_cache` | the rolled-up verdict (worst-of its ACs) |
| `test-case` | `last_run_status`, `verifies: [<uc-id>]` | one probe run + the use-case it covers |

## The loop — verify a deliverable

For each produced deliverable, for each use-case it `addresses`:

1. **Run each AC's probe** (its `test_ref`) through the domain's
   **probe-runner** (below). The result is `pass` / `fail` / `harness-error`.
2. **Record a `test-case`** per AC — `last_run_status` + a `verifies` ref to
   the use-case. The evidence is durable, not prose.
3. **Write each AC `status`** (`pass` / `fail` / `no-test`) and the use-case
   `status_cache` (pass iff every AC passes).
4. **Only when every addressed use-case passes**, set the deliverable
   `status: done` AND its provenance (`produced_by` = the run id,
   `produced_at` = now, `evidence` = the test-case id / probe / a checkable
   path).

This loop is the methods-pack `verify.Harness` (Go) — a domain wires a
`ProbeRunner` and a CWM `Store`; the harness does the rest deterministically.

## The probe-runner seam (the per-domain plug)

How a `test_ref` actually runs is the ONE domain-specific piece — the
LanguagePack `RunUnitTest` analogue. Everything else is shared.

- **Files-only** (compose, da): the `verify.FilesProbeRunner` — token-free,
  offline. `test_ref` grammar:
  - `exists:<path>` — the artifact exists at its home
  - `resolves:<uri>` — a skill/task/reference URI resolves in the catalog
  - `manual:<note>` — a human-only check; ALWAYS `harness-error` (no
    automatable probe → can never auto-pass; forces a real probe or an
    explicit human sign-off)

Paths are **workspace-relative** — the runner's `Root` is the workspace, not a
source tree. A method pack runs autonomously (no binary, no source access): the
artifact lives where the run can write it, and the probe reads it there.
- **Code**: a `go test -run <pattern>` runner (the code pack's job).
- An unknown `test_ref` is a `harness-error`, never a silent pass — a probe
  that cannot run is not evidence.

So author every acceptance criterion's `test_ref` as a runnable probe, not a
sentence. "the skill loads" → `resolves:<provider>:/skill/<name>`; "the file is at its
home" → `exists:app/<sidecar>/embed/skills/<name>.md`.

## The gate — `object-verify` green + `object-lint` clean

Two calls, on ANY node that holds deliverables (composition, code module, da
dossier, generic effort):

- **object-verify** walks the node AND everything under it and runs every `verify`
  hook in force on each. Every acceptance criterion (an object carrying the
  `checkable` capability) has its check run and ONE `verdict` recorded beside
  it: an `expression` probe evaluated over the object, a `command` probe run in
  the project the universe serves (exit 0 passes — `bun test`, `go test ./pkg`).
  A policy's command probe is left to the method harness (`needs-harness`). It returns `{objects, runs, passed, failed[], unproven[]}`. Only `pass`
  is a pass; `needs-harness` / `no-probe` / `harness-error` / `hook-error` are
  **unproven** — neither pass nor fail. **Green** = something ran, nothing
  failed, nothing unproven.
- **object-lint** runs every `lint` hook in force on the node (`id`) or a whole
  `module` and returns FINDINGS (`rule`, `message`, `severity`) — never a
  verdict. The gate needs no error-severity finding.

`object-verify` green AND `object-lint` with no blocking finding == the proof holds.
Only then does a deliverable earn `done` + provenance. For a feature the
workflow enforces it: the `feature` workflow's `finish` (and `verify`) passes
only through the `ready-to-test` gate, whose requirements are `verify().runs > 0`,
`verify().green` and `lint().blocking == 0` (blocking = at or above the type's
`gate_blocks_on`); only a person may force it, and the forced pass is recorded.
The gate makes NO LLM judgment of its own. (Structural completeness — the
decomposition is well-formed — is checked before, by the composition's
structural checklist and the `completeness-gate` skill; this skill is about
VERIFICATION, after structure.)

## The one rule

**Provenance + a passing addressed use-case is the price of `done`.** Set
status on faith and the gate rejects it. Verify, record, then gate — the
evidence is the record, the verdict is computed.
