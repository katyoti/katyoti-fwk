# Charter Template

A **charter** is the single source of truth for one module's quality program. It lives at `/modules/<module>/charter.md` and is the durable artifact that outlives milestones, audits, and contributors.

This document is the structural spec. To bootstrap a charter for a new module, copy this structure into `/modules/<module>/charter.md` and fill it in.

## Charter sections (mandatory order)

```markdown
# <Module> Module Charter

## Meta
## Concepts
## Specifications
  ### Use Cases
  ### Function inventory
  ### Dimension matrix
  ### Public API contract
## Definition of Ready (DoR)
## Definition of Done (DoD)
  ### Per UC
  ### Per module — Tier 1 / Tier 2 / Tier 3
## Test plan
## Audit ledger
## Status board
## Open decisions
## Glossary
```

Below: each section's purpose, format, and a worked example referencing what `tasks` would look like.

---

## Meta

Frontmatter-style, single block. Tracks the charter itself, not the module.

```markdown
## Meta
- Module path: internal/extensions/task/
- Charter version: 0.1
- Charter status: draft | active | frozen
- Current tier: 0 | 1 | 2 | 3
- Last reviewed: YYYY-MM-DD
- Owners: <names>
- Linked goals: [<module>.G-NNN](../<module>/goals/G-NNN-<goal-slug>.cwm.md)
```

`Module path` is canonical. If the module spans multiple directories, list them all — but a module SHOULD be one source-tree subroot.

---

## Concepts

The mental model a reader needs **before** they can understand the spec. Not a tutorial, not a reference — a glossary of *named ideas* and the relationships between them.

Format: 1-3 paragraphs of prose **then** a glossary table. Diagrams (Mermaid) where they earn their place.

```markdown
## Concepts

The tasks module is the agent / task / workflow execution engine. Three primitives compose:

- **Machine** — a state machine the engine drives via Step/Resume.
- **Channel** — event-routing layer between executors and the EventBus.
- **Spawner** — child-executor lifecycle manager.

Tasks compose into workflows (a workflow IS a Machine). Agents are Machines too. The engine is domain-agnostic; agent-specific behavior lives in plugin hooks.

| Concept | Definition | Source |
|---|---|---|
| Machine | ... | api/engine/machine.go |
| Channel | ... | api/engine/channel.go |
| ... | ... | ... |
```

Concepts must be **named** — every term used elsewhere in the charter must appear here. If you can't define it crisply here, the spec downstream will be muddy.

---

## Specifications

The contract the module fulfills, in four parts.

### Use Cases

Numbered list of UCs with **stable IDs**. Tests reference these IDs forever. Once published in a tier-1 charter, an ID is never reused or renumbered.

```markdown
### Use Cases

| UC ID | Title | Trigger | Expected outcome | Severity |
|---|---|---|---|---|
| UC-001 | Spawn child agent in worker mode | LLM calls delegate(mode: worker) | Child runs, returns output, parent resumes | core |
| UC-002 | Spawn child in coordinator mode forces SafeToolSubset | LLM in coordinator mode calls delegate(mode: any) | Child receives restricted permission set regardless of requested mode | security |
| UC-003 | Workflow YAML parses Sequence node | Loader reads TASK.md type: workflow | Tree of Node{Sequence: [...]} matches YAML | core |
| ... | ... | ... | ... | ... |
```

Severity = `core` (must work; module is broken without it) / `security` (safety guarantee) / `quality-of-life` (nice but not blocking) / `cosmetic`.

For non-trivial UCs, link to a UC detail file: `/modules/<module>/specs/uc-001.md`.

### Function inventory

Public API (top-level package) only. Internal helpers do not appear here unless they cross a module boundary.

```markdown
### Function inventory

| Symbol | Kind | Stability | UC coverage |
|---|---|---|---|
| `engine.Executor.Run` | method | stable | UC-001, UC-002, UC-005 |
| `engine.Channel.Subscribe` | method | stable | UC-010 |
| `engine.NewLocalSpawner` | constructor | stable | UC-001 |
| ... | ... | ... | ... |
```

Stability = `stable` / `provisional` / `deprecated`. Anything `provisional` is fair game to break between tiers.

### Dimension matrix

For modules with composable behaviors (the `tasks` module has 6 intrinsic dimensions × 9 modes), the matrix lives here. Empty cells = combination not supported.

```markdown
### Dimension matrix

|             | worker | background | fork | isolated | coordinator | teammate | remote |
|-------------|--------|------------|------|----------|-------------|----------|--------|
| Scheduling  | sync   | async      | sync | sync     | sync        | async    | varies |
| Permissions | parent | parent     | full | restrict | parent      | parent   | parent |
| Isolation   | inproc | inproc     | wt   | ctx-iso  | inproc      | ctx-iso  | proc   |
| ...         | ...    | ...        | ...  | ...      | ...         | ...      | ...    |
```

### Spawn policies (recommended companion table to dimensions)

Per OD-001 option (d): some "modes" carry rules that fire at *spawn time* and transform child state. These are **spawn policies**, not dimensions. Track them in a separate table:

```markdown
### Spawn policies

| Mode | Spawn policies on this agent's children |
|------|------------------------------------------|
| coordinator | Permissions ← SafeToolSubset; AgentComms ← team-membership-injection |
| team-leader | AgentComms ← team-membership-injection; UserComms ← surface-children-progress |
| teammate    | (none — teammate is intrinsic; role is "I am a member of team X") |
| worker      | (none) |
| ...         | ... |
```

Together, the dimension matrix + spawn policies table fully describe a mode. The mode preset is the name; the two tables are the contents.

The matrix is **the source of truth** for which combinations the module supports. Tests parametrize over both. Empty cells in the dimension matrix = explicit "not supported" — needs a UC if it should be. Spawn policies that propagate transitively (grandchildren) need their own UCs.

### Public API contract

Stability levels and breaking-change policy for the module's public surface. Not exhaustive code; just rules.

```markdown
### Public API contract

- `api/` is the public surface; consumers may import it. `internal/` is private.
- All `stable` symbols obey semver-like discipline within a charter version.
- Adding a field to an exported struct is a non-breaking change IF the zero value preserves prior semantics.
- Removing or renaming a `stable` symbol requires a tier rollback or an explicit major-version bump in the charter.
- Provisional symbols can break between tier promotions with a journal entry.
```

---

## Definition of Ready (DoR)

What must be true **before** a UC can be tested. Not a workflow gate — a sanity gate. If a UC fails its DoR, work on it doesn't start.

```markdown
## Definition of Ready

For any UC to be testable:
- [ ] UC has a stable ID and appears in the Use Cases table
- [ ] The trigger and expected outcome are observable (journal event, exit code, file content, exit message)
- [ ] All input fixtures exist under /modules/<module>/fixtures/
- [ ] The harness layer is chosen (L1 / L2 / L3) and the harness exists
- [ ] No upstream UC blocks this one (or the blocker is acknowledged in this UC's notes)
```

DoR is checked by the workflow before allowing test authoring. It is not a per-tier gate.

---

## Definition of Done (DoD)

Two parts: per-UC (does this UC pass?) and per-module (is this module rock-solid?).

### Per UC

For each UC, what evidence proves it passes? Be concrete — these are the assertions the automated tests check.

```markdown
### Per UC — UC-002 example

UC-002: Spawn child in coordinator mode forces SafeToolSubset.

Pass criteria:
- Test scenario: TASK.md sets mode: coordinator. The agent calls delegate(mode: worker) inside.
- Observed in journal: spawnPermissions.AllowedTools matches presets.SafeToolSubset exactly.
- Observed: any tool outside SafeToolSubset that the LLM tries triggers a permission-denied event.
- Negative: no path from coordinator-parent reaches a child with AllowedTools = nil.
- Test runs at L2 (CLI black-box) and L1 (unit-level mode-resolver test).
```

DoD-per-UC is the contract tests against. It must be written **before** the test, not derived from a passing test.

### Per module — Tier 1 / Tier 2 / Tier 3

Tiers are coarse quality bars. A module sits at one tier; promotion is explicit and irrevocable until the next audit demotes it.

```markdown
### Per module — Tier 1 ("rock solid for our use")

- [ ] All `core` and `security` UCs have DoD-per-UC and pass
- [ ] ≥80% of all UCs have DoD-per-UC and pass
- [ ] Latest audit has zero critical findings, zero unmitigated high findings
- [ ] Race detector passes the full L1 suite
- [ ] L2 black-box harness covers every dimension × mode combination that the matrix lists as supported
- [ ] Public API contract is documented and respected for the past 30 days
- [ ] No abandoned-in-place code in the module's public surface
```

```markdown
### Per module — Tier 2 ("production for paying users")

- [ ] All Tier 1 criteria
- [ ] 100% of UCs have DoD-per-UC and pass
- [ ] Performance budget met: p99 step latency under <BUDGET>; memory bounded under <SCENARIO>
- [ ] L3 self-hosted scenario passes (stable build orchestrating candidate)
- [ ] Threat model is current (last update < 90 days)
- [ ] Documentation (concepts + UCs + matrix) reviewed by someone other than the author
```

```markdown
### Per module — Tier 3 ("battle-tested")

- [ ] All Tier 2 criteria
- [ ] Two consecutive monthly audits with zero new high findings
- [ ] Mutation testing applied to security-critical code; ≥80% mutation kill rate
- [ ] Public API frozen for 90 days (no breaking changes)
- [ ] External consumer (sidecar / 3rd party) imports the module and reports satisfaction
```

Tier numbers are rough proxies; adjust per module if needed. The shape is what matters: **explicit, observable, stop-the-line gates**.

---

## Test plan

A table mapping UC ID ↔ test layer ↔ test file. Empty cells = uncovered gap.

```markdown
## Test plan

| UC ID | L1 (Go unit) | L2 (CLI black-box) | L3 (self-hosted) |
|---|---|---|---|
| UC-001 | engine/spawner_test.go::TestSpawn | tests/L2/uc-001-worker.tasktest | — |
| UC-002 | taskplugin/mode_overrides_test.go::TestCoordinator | tests/L2/uc-002-coord-escalation.tasktest | tests/L3/uc-002-self.scenario |
| UC-003 | wkflow/loader_test.go::TestSequence | — | — |
| UC-004 | — | tests/L2/uc-004-parallel.tasktest | — |
| UC-005 | — | — | — | ← gap |
| ... |
```

A UC counts as covered if **any** layer covers it; layered coverage is recommended but not required for tier 1. Tier 2 expects security UCs at ≥2 layers.

See [`test-layers.md`](test-layers.md) for what each layer means and when to use it.

---

## Audit ledger

Append-only log of audits run against the module. Each entry links to a dated audit bundle under `/modules/<module>/audits/`.

```markdown
## Audit ledger

| Date | Bundle | Reviewers | Critical | High | Medium | Quick wins | Promotion outcome |
|---|---|---|---|---|---|---|---|
| 2026-05-06 | [audits/2026-05-06_07-56/](../tasks/audits/2026-05-06_07-56/) | ktw-code:code-review (13 agents) | 3 | 6 | 8 | 6 | held at tier 0 — see ms07d |
| ... |
```

`Promotion outcome` records the L3 decision after the audit: `held at tier <n>` / `promoted to tier <n+1>` / `demoted to tier <n-1>`.

The audit bundle itself contains the full reports. The charter only carries the index entry and the *outcome*.

---

## Status board

Single-glance table. Updated by the L3 skill on every `status`/`audit`/`promote` invocation.

```markdown
## Status board

| Metric | Value | Tier-1 target | Status |
|---|---|---|---|
| Tier | 0 | 1 | ⚠ work in progress |
| UCs total | 47 | — | — |
| UCs with DoD-per-UC | 12 | 47 | ✗ |
| UCs passing | 8 | ≥38 (80%) | ✗ |
| Critical audit findings open | 3 | 0 | ✗ |
| High audit findings open | 6 | 0 | ✗ |
| Race detector | not run | green | ✗ |
| L2 dimension coverage | partial | full | ✗ |
| Public API days stable | 0 | 30 | ✗ |
| Last audit | 2026-05-06 | <90 days | ✓ |
```

The status board is the answer to "where are we?" — both for the user and for the L3 `status` verb.

---

## Open decisions

Decisions the module-owner has surfaced but not resolved. Charter-level decisions (not implementation choices). Each gets a stable ID; tasks may depend on a decision being made.

```markdown
## Open decisions

| OD ID | Question | Options | Default | Owner | Due |
|---|---|---|---|---|---|
| OD-001 | Is "role" (coordinator/teammate/worker) a 7th dimension? | (a) yes, formalize as 7th; (b) no, fold into Permissions+AgentComms; (c) keep as parentMode magic string; (d) **introduce "spawn policies" as a separate first-class concept (recommended)** | (d) | tasks-charter | tier-1 plan |
| OD-002 | Reliability NFR wiring: wire / re-spec under MS09 / delete? | wire / re-spec / delete | wire | tasks-charter | tier-1 plan |
```

**Note on OD-001 option (d)**: the audit's "hidden 7th dimension" finding actually points at a **missing concept**, not a missing dimension. Dimensions are intrinsic to an agent (what it IS); "role" is relational (how parent state transforms child state at spawn time). The cleaner abstraction is to keep 6 intrinsic dimensions AND introduce **spawn policies** — typed, composable rules that fire at spawn time. Modes (`worker`, `coordinator`, `teammate`) become named compositions of (a) a dimension preset + (b) zero or more spawn policies. This preserves dimension orthogonality, makes coordinator escalation discoverable instead of a magic string in `mode_overrides.go`, and lets future spawn behaviors grow without inflating the dimension list.

Open decisions block the tasks that depend on them. The status board can include "N open decisions blocking tier-N promotion."

---

## Glossary

Optional but recommended. Defines abbreviations, internal jargon, and references that aren't obvious.

```markdown
## Glossary

- **DoR** — Definition of Ready (this charter, see above)
- **DoD** — Definition of Done (this charter, see above)
- **UC** — Use Case (numbered, stable ID)
- **OD** — Open Decision
- **WaitSpec** — engine type, see api/engine/wait.go
```

---

## How charters relate to other artifacts

| Artifact | Lives in | Relationship to charter |
|---|---|---|
| Requests and to-dos | the module's requests (`ktw-agents requests list --module <module>`) | Carried on UC IDs from this charter (`--on`); never define new UCs themselves |
| Decisions | `modules/<module>/decisions/` — `decision` objects | Charter's "Open decisions" are the module's decisions in state `open`; decided, they apply |
| Rules and patterns | `modules/<module>/rules/` and the module pages (`modules/<module>/*.md`) | Patterns describe the *how*; charter describes the *what* |
| Goals and features | `modules/<module>/goals/`, `modules/<module>/features/` | Charter `Meta.Linked goals` lists which goals this module serves; its UCs prove the features |
| Source code | `internal/extensions/<module>/` | Charter Function inventory cites these symbols |
| Audit bundles | `/modules/<module>/audits/<date>/` | Indexed by charter's Audit ledger |
| Test fixtures | `/modules/<module>/fixtures/` | Referenced by Test plan rows |

The charter never duplicates content from other artifacts — it indexes them.

---

## Versioning

Charters version monotonically: `0.1` (draft) → `1.0` (first tier-1 promotion) → `1.1` (any change after promotion) → `2.0` (tier-2 promotion or breaking spec change). The `Meta.Charter version` reflects this.

Older versions live at `/modules/<module>/charter-archive/v<n>.md` so the history is reviewable.

---

## What this template is NOT

- Not a test report
- Not a replacement for ADRs (those record decisions; the charter records the spec)
- Not a substitute for source code documentation (godoc still owns symbol-level docs)
- Not negotiable in shape — fields can be empty for a draft charter, but the **section ordering and presence is fixed** so any reader knows where to look.
