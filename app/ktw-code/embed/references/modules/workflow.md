# Workflow — The Module Quality Lifecycle

The lifecycle the **module-rock-solid method** drives, from "no charter" to "tier 3, battle-tested." Read [`relationship-with-planification.md`](relationship-with-planification.md) and [`charter-template.md`](charter-template.md) first.

## High-level shape

```
┌──────────────┐
│  Bootstrap   │  ← only once per module
└──────┬───────┘
       ▼
┌──────────────┐
│   Specify    │  ← define UCs, DoR, DoD before any test/code work
└──────┬───────┘
       ▼
┌──────────────┐         ┌────────────────┐
│  Plan tier   │ ◀─────  │   Re-plan from │
└──────┬───────┘         │  audit findings│
       ▼                 └────────▲───────┘
┌──────────────┐                  │
│   Execute    │  (module-next)   │
└──────┬───────┘                  │
       ▼                          │
┌──────────────┐                  │
│    Verify    │  (run all 3 layers, check DoD-per-UC)
└──────┬───────┘                  │
       ▼                          │
┌──────────────┐                  │
│    Audit     │  ── new criticals/highs ─┘
└──────┬───────┘
       │  clean
       ▼
┌──────────────┐
│   Promote    │  ← tier+1 (or tier+0 hold; rarely tier-1 demote)
└──────┬───────┘
       ▼
┌──────────────┐
│ Retrospective│  ← method-level lessons; refine template/workflow
└──────┬───────┘
       │
       ▼
   tier reached → stop, or pick next tier
```

The loop-back from `Audit` is the most important arrow. It's why this method exists. The `Retrospective` phase after `Promote` is what keeps the **method itself** from rotting as patterns crystallize.

## Phases — what happens, what changes, what blocks the next phase

### Bootstrap (one-time per module)

**Goal**: the module exists in `/modules/<module>/` with an empty charter and a baseline audit.

Steps:
1. `mkdir /modules/<module>/`
2. Copy [`charter-template.md`](charter-template.md) → `/modules/<module>/charter.md`
3. Fill in `Meta` and `Concepts` only — the rest stays empty
4. Run a **baseline audit** (the parallel-review pattern, e.g. `ktw-code:code-review` against the source folder)
5. Save audit bundle under `/modules/<module>/audits/<date>/`
6. Add the audit row to the charter's audit ledger
7. Set `Meta.Current tier: 0`

Exit criteria: charter file exists, audit ledger has 1 row, tier is 0. Nothing else is required to leave Bootstrap.

### Specify

**Goal**: the charter has UCs, DoR, DoD, and a populated dimension matrix. **No test or code work yet.**

This is the phase the existing project most often skips — and the one most worth not skipping. UCs without DoD are just todos; tests without UC links are just hopes.

Steps:
1. Read the baseline audit; extract the implicit UCs from "what the code does."
2. Read the source's existing tests; extract the implicit UCs from "what tests verify."
3. Read user-facing docs / TASK.md / cmd surfaces; extract the user-visible UCs.
4. Merge into one Use Cases table with stable IDs (UC-001, UC-002, …).
5. For each UC, write the **DoR** (what fixtures/inputs need to exist) and the **DoD-per-UC** (what observable evidence proves it passes).
6. Fill in the dimension matrix (which combinations are supported).
7. List **open decisions** that block UCs from being testable.
8. Write the public API contract (stability levels, breaking-change rules).
9. Charter `Status: draft` → `Status: active`.

Exit criteria: every existing test in the source can be linked to ≥1 UC ID. Every `core` and `security` UC has a DoR and a DoD-per-UC. The dimension matrix has no `?` cells in rows for `core` UCs. Open decisions are all assigned to a tier.

This phase is high-investment **once**. It pays dividends every time a test is written or an audit is interpreted.

### Plan tier

**Goal**: the tier's release exists (a project release, `<module>-rs-tier-<n>`) and one request per gap between current state and target tier DoD targets it.

L3 reads the charter, computes the delta against `Tier <n>` DoD, and files requests in the module via `ktw-agents requests file` (non-interactive). Each request carries:

- The UC IDs it advances
- The audit findings it closes (with file:line citations)
- The open decisions it depends on (or resolves)
- Deliverables (checks) lifted from DoD-per-UC

Requests are included in the milestone `ms-<module>-rs-tier-<n>`.

Exit criteria: every Tier-<n> DoD checkbox has at least one request that, when done, will tick it. Open decisions blocking Tier-<n> carry a `question` request.

### Execute

**Goal**: the milestone burns down via `module-next`.

The internal scheduler (for a module in mode auto), the user or a session takes the module's to-dos (`module-next <module>`, the worker contract of `work-todo`). Each request follows its work template (intake, steps, verification of its deliverables). When its deliverables are verified, its plan finishes `done`.

L3 is **passive** during this phase — it doesn't touch requests. The user can ask L3 for status (`module-rock-solid status <module>`) which re-reads the charter + the milestone state.

Exit criteria: every request the milestone includes is done (its grade is green). Charter is updated by the work as it goes (UCs gain test-plan rows, DoD-per-UC checks flip green, etc.).

### Verify

**Goal**: every UC mapped to the test plan actually passes when its layered tests are run.

Steps:
1. Run L1 suite (`go test ./<module>/... -race`)
2. Run L2 suite (CLI black-box scenarios, see [`test-layers.md`](test-layers.md))
3. Run L3 suite if any UCs are L3-covered
4. Compare results against DoD-per-UC for every UC
5. Update the charter's status board with pass counts
6. Save the test-results JSON under `/modules/<module>/results/<date>/`

Exit criteria: every UC's DoD-per-UC has a green or red verdict. Failed UCs go back to "Plan tier" in the next loop.

This phase is **automatable** — the L3 skill can run it on demand. Not just at tier-promotion time.

### Audit

**Goal**: a fresh audit confirms the verification, and surfaces issues tests didn't catch.

Steps:
1. Run the parallel-review pattern (e.g. `ktw-code:code-review`) against the source folder.
2. Save bundle under `/modules/<module>/audits/<date>/`.
3. Compare new findings against open audit findings:
   - **Closed** (was open in last audit, not in this one) → mark closed in audit ledger
   - **New** (in this one, wasn't in last) → triage by severity
   - **Persisting** (same in both) → flag if older than tier-promotion target
4. Append a row to the charter's audit ledger.

Decision after audit:

| Outcome | Action |
|---|---|
| Zero new criticals/highs, all DoD met | Proceed to **Promote** |
| New criticals/highs OR DoD gaps | Loop back to **Plan tier** with the new findings as new tasks |
| Persisting criticals/highs that the milestone failed to fix | Re-plan but **also** flag the relevant tasks for re-execution (not just new tasks) |

Audits **never** fix anything. They produce evidence; the loop-back is what fixes things.

### Promote

**Goal**: the charter records the new tier; the module's quality bar steps up.

Steps:
1. Run [`code-module lint`](standards-enforcement.md) — must be clean (no errors) or violations explicitly waived.
2. Update `Meta.Current tier` and `Meta.Charter version`.
3. Update the status board (Tier-N targets become "met," next-tier targets become the new gates).
4. Write a `Promotion` journal entry: which audits/results justified the promotion.
5. Release the tier's release: its grade (`ktw-agents releases grade`) is green — every request targeting it done.
6. Snapshot the charter to `charter-archive/v<old_version>.md` before bumping.
7. **Trigger Retrospective** (next phase).

Exit criteria: charter's `Meta.Current tier` matches the audit-confirmed tier. The user can decide whether to start tier+1 planning now or later.

### Retrospective

**Goal**: the **method itself** improves with each promotion. Without this phase, the templates and workflow ossify and stop matching reality.

Triggers:
- Every tier promotion (always).
- Every quarter for active modules even without promotion (cadence — see "Cadence" below).
- Ad-hoc when the team notices a recurring friction point.

Steps:
1. **Gather** — read the milestone's task journals, the audit ledger entries since the last retrospective, and any lessons noted in the charter's Working Memory section.
2. **Distill** — answer four questions:
   - What worked well? (worth keeping in the template/workflow)
   - What didn't? (template/workflow change candidate)
   - What surprised us? (signals an unstated assumption)
   - What's still missing? (gap to add as a follow-up task or method update)
3. **Record** — write `/modules/docs/lessons/<date>-<module>-tier-<n>.md` with the four answers + concrete next actions.
4. **Update** — if a lesson points to a method-level change (template needs a section, workflow needs a phase, lint needs a new rule), make the change OR add a task to a `ms-method-evolution` milestone (for tracking method improvements as their own work).
5. **Communicate** — if multiple people work on the codebase, the lesson file is the artifact they read. The factory-rollup view links to the latest lesson.

Exit criteria: a lesson file exists. The method docs are either updated or carry a TODO referencing the lesson.

**Anti-pattern**: skipping retrospective because "everything went fine." The four questions usually surface something even on smooth runs. Force at least 15 minutes per retrospective.

**Lessons format**:

```markdown
# Retrospective — tasks tier 1 — 2026-07-15

## Worked well
- The L2 scenario format (.tasktest) was easy to author once a fixture pattern emerged
- ...

## Didn't work
- DoR's "fixture exists" check fired late — should be a Specify-time gate, not a test-authoring-time discovery
- ...

## Surprised us
- The number of UCs that needed L2-level coverage was 2× our estimate
- ...

## Still missing
- No way to link a UC to a specific git commit that introduced it
- ...

## Method updates
- Updated workflow.md DoR section: fixture check moves to Specify
- Filed task: `ms-method-evolution/uc-to-commit-link`
```

The Retrospective is **lightweight by design** — 30-60 minutes per pass. The cost of skipping it is much higher than the cost of running it.

## Loop-back paths

The simple `Plan → Execute → Verify → Audit → Promote` is the happy path. The reality is loops. Each loop has a name:

| Loop | Trigger | Action |
|---|---|---|
| **Verify-fail** | A UC's DoD-per-UC test fails | Re-plan that UC's task or open a new one if root cause is elsewhere |
| **Audit-loop** | Audit surfaces a new critical/high | Plan a new task; resume Execute |
| **DoR-fail** | Tester finds the UC isn't actually testable | Loop back to **Specify**: refine the UC's DoR; possibly split into sub-UCs |
| **Spec-drift** | A task during Execute reveals an unstated UC or wrong DoD | Pause the task; fix the charter; resume |
| **Open decision** | A request is blocked on a `decision` still `open` | File a `question` request for it (the blocked one depends on it); decide it (`ktw-methods:decisions/Resolve`); resume the blocked request |

The L3 skill should make these loops mechanical, not heroic. If a developer sees the same loop type fire 3+ times in a row, that's a signal the spec is wrong — escalate to a re-Specify, not a re-Execute.

## Cadence (how often does each phase run?)

- **Bootstrap**: once per module
- **Specify**: heavily once; light revisions on every loop-back
- **Plan tier**: at the start of every tier; re-runs on Audit-loop
- **Execute**: continuous (driven by `module-next` or the planner)
- **Verify**: on request — local (`code-module verify`) or via manual CI trigger. **Not** on every commit. Mandatory before Audit.
- **Audit**: on request; target one per tier (so tier 1 = ≥1 audit; tier 2 = ≥2 audits; tier 3 = ≥3); **plus a re-audit at most every 90 days** for any module at tier ≥1 (staleness gate)
- **Promote**: rare; deliberate; explicit
- **Retrospective**: every promotion; plus quarterly even without promotion for active modules

A typical "tier 1 push" for an existing module: 1 Bootstrap + 1 Specify + 3-5 Plan↔Execute↔Verify↔Audit cycles + 1 Promote + 1 Retrospective. Roughly 4-8 weeks of focused work.

The 90-day staleness gate is enforced by [`code-module factory-status --stale 90`](factory-rollup.md): any module showing up in this list needs a re-audit before next promotion. A light nightly CI job can post the staleness list to a status channel; the heavy gates (lint, race tests, coverage) are on-request only — see [`quality-tooling.md`](quality-tooling.md) "When to run the gates" for the full trigger model.

## Anti-patterns

- **Shipping without DoD-per-UC**: tests pass, but there's no contract for what they prove. Loop-back will find the gap; better to write DoD first.
- **Re-running Audit hoping for different findings**: audits are deterministic-ish. Run `Plan tier` after a bad audit, not another Audit.
- **Skipping Specify on a re-bootstrapped module**: even if the module already exists, the charter doesn't until you write it. Don't write tasks before the charter has UCs.
- **Treating tasks as the spec**: tasks reference UCs; tasks are not UCs. Renumbering UCs because a task changed is a sign the charter's authority hasn't been internalized.
- **Auditing without comparing to last audit**: the `closed/new/persisting` triage is half the value of doing the audit at all.
- **Skipping Retrospective**: "everything went fine" is the most dangerous conclusion. The method only stays sharp if every promotion produces at least one method-level lesson — even if it's "this time the template was complete enough."
- **Letting the charter and code drift silently**: if `code-module lint` reports drift but you keep promoting anyway, you're shipping a charter that lies. Reconcile or roll back; don't ignore.
