---
name: audit-sidecar
description: "Whole-sidecar quality audit — usage-road trace, per-artifact checks, doc reconciliation, and ranked proposals; load to review, validate, or find drift in an existing ktw-* sidecar."
advertised: true
hashtags: [audit, sidecar, quality, review, usage-road]
category: guide
---

# Sidecar audit

A whole-sidecar quality pass. Four phases, all **read-only** until the last: map
what the sidecar exposes and how it chains, judge each artifact against its
contract, reconcile the live surface against the docs, then propose changes
through the user gate. This skill **orchestrates** the per-kind checks — it does
not re-implement them; it routes to them and assembles the findings.

It sits ABOVE the test/check methods and consumes them:
- surface reachability + contracts → `ktw-compose:/skill/sidecar-testing`
- per-skill static check → `ktw-compose:/skill/check-skill`
- per-task static check → `ktw-compose:/skill/check-task`
- per-object-type static check → `ktw-compose:/skill/check-object-type`
- applying the proposals under a user gate → `ktw-compose:/skill/improve-artifact`

This is a static, evidence-first audit. It never scores an LLM run's quality
(that is `ktw-compose:/skill/skill-evaluation`) and never edits without the
Phase 4 gate.

## Phase 0 — Scope the sidecar

Name the target sidecar and locate its source: `app/<sidecar>/embed/` (the
artifact pack), `app/<sidecar>/embed.go` (the `//go:embed` directive), and
`app/<sidecar>/cmd/bootstrap.go` (what the binary wires).

Enumerate the artifact set by folder: `skills/`, `tasks/`, `references/`,
`data-types/`, every `cwm/<folder>/` (object-types, capabilities, aspects,
workflows, gates, gate-requirements, policies, hooks, hook-types, edge-types,
object-panels, agent-contexts, data-types), `guide/`, `personas/`, `config/`.
Record a count per kind — this is the number every later phase and the docs must
match.

**Ship-coverage check (do it here — it catches ships-invisible dead artifacts).**
For every artifact folder, confirm two things hold together:
1. the folder is covered by a `//go:embed` line in `embed.go`, AND
2. a kind reads it: a folder of `embed/` is published when a kind claims it
   (skills, tasks, references, data-types, personas, guide, display-engines,
   contribution-types — the sidecar's embedded pack), `embed/cwm/` as a part
   of the pack with no option (the folder is embedded and bootstrap.go passes
   no `WithoutPack`), scripts with `WithScripts`. A folder
   no kind claims is not content (`config/`, `docs/`); a pack that reads a
   kind from another folder says so (`WithPackRemap`).

A folder embedded but **not wired** — or wired but **not embedded** — is a dead
artifact that reaches no catalog. Flag each with the exact folder and the missing
side. Also flag any `_to_use/` / staging / `imported-refs/` folder: decide
whether it ships and whether it duplicates an already-promoted artifact.

## Phase 1 — The usage road

Build the entry-point graph: **entry point → action → task → skill → reference**.
The connective tissue is often NOT in the sidecar's own source.

1. **Entry points.** A sidecar's tasks are launched by object-type `actions:`
   (plus `create_task` / `before_create` / `on_create`) and by tasks that target a
   type themselves (`kind: object` + `types:`) — and those object types frequently
   live in ANOTHER pack (the host `app/ktw-agents/embed/cwm/`, a methods pack, a
   sibling sidecar). Grep the WHOLE repo for `<sidecar>:tasks/` and
   `<sidecar>:/skill/`. For every hit inside an `object-types/*.yaml` or
   `capabilities/*.yaml`, record: owner type + file:line, action label, `kind:`,
   target URI. Also count as entry points every method a pack names: a
   panel's `source:`, a hook's `uri`, a capability function's `action`, an
   `agent.tools` entry (capability, type, policy, agent-context).
2. **Task → skill.** For each task under `embed/tasks/`, record name, `type:`,
   one-line description, and the skill URIs + tools its body loads.
3. **Skill → sub-skill.** For each skill, record advertised state, purpose, and
   what it routes to.

Emit three tables — **Entry points**, **Tasks**, **Skills** — then the
**Orphans & dead-ends**:
- tasks reached by NO action (callable only by direct `run`/`delegate`),
- skills loaded by no task AND not advertised (dead weight),
- action URIs pointing at a task/skill that does not exist.

**Distinguish intent from breakage.** An action marked `preview: true` that
points at a not-yet-built task is a deliberate placeholder, not a broken link —
report it as *planned*, never as a defect. An `advertised: false` skill loaded
only by its parent is correct, not an orphan. Only unmarked dangling URIs are
findings.

## Phase 2 — Quality of every artifact

Run the static-quality dispatcher over the whole artifact set —
`ktw-compose:/skill/check-artifact` — which owns the per-kind routing (skill →
`check-skill`, task → `check-task`, object-type → `check-object-type`,
data-type → `check-data-type`, reference → `check-reference`) and aggregates the
verdicts. Point it at the sidecar's `embed/` folder and let it fan out by kind;
this is the content half of the phase.

Then, for a BUILT binary, run the surface + contract battery
(`ktw-compose:/skill/sidecar-testing`) — it proves every contribution is
reachable across CLI / LLM / stdio and that interface↔method coupling holds.
Static content checks and the live contract battery are complementary: one reads
the files, the other exercises the binary. The mechanical lints run on the
binary too: `ktw-compose lint-sidecar --exe <bin>` (every skill and task),
`lint-universe --bin-dir <dir>` (URIs across sidecars, gates out of their
provider's scope) and `i18n <embed folder> --lang fr --missing` (translations).

Collapse the per-artifact verdicts into a findings table and a **cross-cutting
patterns** section — systemic issues that repeat (the same stray frontmatter key
across a family, a whole cluster carrying task-tracker/`US-N` tracking cruft, a
staging folder every consumer still points at). Cross-cutting patterns are worth
more than individual rows because one fix clears many.

## Phase 3 — Handled use cases vs the docs

From the Phase 1 usage road, synthesize the **use-case set the sidecar actually
handles today** — group the reachable entry points into user-facing capability
families, each named for the outcome a user gets.

Then reconcile against `modules/<sidecar>/`:
- Does any doc enumerate the artifacts? Diff its counts against Phase 0's real
  counts — a doc claiming half the live skills is stale.
- Flag drift: dated/phase language (`B1–B6`, `US-N`, `ms-NNNN`, "planned",
  point-in-time counts), links into a task tracker's folder, obsolete path/env names.
- Name the undocumented capabilities (live but absent from the docs) and the
  phantom ones (documented but no longer reachable).

Produce the doc-update proposal: which files to rewrite, what the canonical
entry-point map should say, what to delete. Writing the docs is a Phase-4 action,
gated like any other.

## Phase 4 — Propose, then apply under the gate

Assemble every finding into a **Pareto table** sorted by impact/effort: title +
category (hygiene / correctness / docs / architecture), the observed problem with
its `file:line`, expected impact, effort (low/med/high), regression risk. The
highest-leverage fixes are usually the cheap systemic ones from Phase 2's
cross-cutting section.

Nothing is edited yet. To apply, hand the proposal to the user and run the
measured, user-validated loop in `ktw-compose:/skill/improve-artifact` —
it owns the "user decides every change" gate and the re-measure. For an embedded
sidecar whose source is in this workspace, iterate through the overlay and
`promote` only validated changes (`ktw-compose:/skill/overlay-iteration`), so the
audit's re-run sees each edit with no rebuild.

## Rules

- Read-only through Phases 0–3. No file is edited without the Phase 4 user gate.
- Every finding cites a `file:line`. No verdict without evidence.
- Separate **intent** from **defect**: `preview: true`, `advertised: false`, and
  in-process-accessor services are deliberate, not findings.
- Cross-cutting patterns before individual rows — one systemic fix clears many.
- The sidecar's entry points may live outside its own source; the repo-wide grep
  in Phase 1 is mandatory, not optional.
