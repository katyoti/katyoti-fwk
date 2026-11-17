# Where-Are-We Review

## Purpose

Answer "where are we?" for a module, subsystem, or project by comparing **intent** (docs, decision records, rules, goals) against **implementation** (the code as it is today) against the **backlog** (open work items). This is a META-review: unlike the other review types, which each examine the code from one angle, this one synthesizes — its inputs include the OUTPUTS of other reviews when they exist. It complements `architecture.md` (structure) and `red-team-architecture.md` (reasoning) by answering the planning question: what did we set out to build, what actually exists, and what is the ordered list of work that closes the gap.

## Scope

Input: a module/subsystem/project that has at least one of: documentation, decision records (ADRs), or prior review reports. Optionally a prior report folder (`modules/tasks/reviews/<scope>_YYYY-MM-DD_HH-MM/`) to synthesize.
Allowed tools: Read, Grep, Glob; parallel sub-agents for the fan-out when available.
Output: one Markdown synthesis document (`where-are-we.md`), five mandatory sections (see Output).

## When to use

- The user asks "where are we", "what's the state of X", "make this module rock solid", or wants to plan a hardening/consolidation milestone
- A prior multi-review run exists and the user wants its findings consolidated against current code
- A module's docs, code, and backlog have drifted and need one consistent statement
- As the LAST report of a combined code-review run, synthesizing the sibling reports

Skip when: the target is a single file or snippet, or the user wants a single-angle review (use the dedicated type instead).

## Modes

| Mode | Trigger | Inputs |
|------|---------|--------|
| **standalone** | No prior review reports for this scope | Fan-out tracks A–C below |
| **synthesis** | A prior `modules/tasks/reviews/<scope>_*/` folder exists, or this review runs last in a combined run | Tracks A–C (lighter) + track D over the prior reports |

In a combined run, this review always runs LAST so it can ingest the sibling reports produced in the same run.

## Process

### Phase 1 — Frame the target

Identify the unit under review and locate its four evidence sources:
1. **Docs** — module documentation folders, READMEs, architecture docs
2. **Code** — the package tree(s)
3. **Intent records** — decision records / ADRs, rules, goals, and the project's task system if present (e.g. the module's requests and goals under `modules/<m>/`, `docs/adr/`, issue lists). Exclude completed/archived items — done is not backlog.
4. **Prior reviews** — `modules/tasks/reviews/<scope>_*/` folders covering this scope (note their dates)

State which sources exist. Missing sources are fine — degrade gracefully and say so in the report (e.g. "no decision records found; intent inferred from docs only").

### Phase 2 — Fan-out

Run the applicable tracks (in parallel when sub-agents are available):

- **Track A — Docs inventory + staleness.** Every doc about the scope: path, what it covers, staleness signals (renamed files/types it still references, removed env vars, dead links, duplicated/overlapping doc modules).
- **Track B — Code map + quality.** Package tree with per-file responsibility; subsystem ownership; dependency edges (and cycles avoided/violated); modularity findings: god files (>500 lines), mixed concerns, stringly-typed cross-package contracts, duplicated logic, scattered wiring. Every finding with `file:line`.
- **Track C — Intent inventory.** Relevant decision records, rules, patterns, goals, and OPEN work items (exclude done). For each: name + what it decided/wants + implementation status if stated.
- **Track D (synthesis mode) — Prior-report ingestion.** From each prior report, extract the health-overview ratings and the findings/priorities lists with their code locations.

### Phase 3 — Re-verify carried-forward findings (MANDATORY in synthesis mode)

Prior findings go stale as code evolves. NEVER carry a prior finding into the synthesis without checking it against current code. Classify each:

| Verdict | Meaning | Action |
|---------|---------|--------|
| `fixed` | The cited code/behavior no longer exhibits the finding | Drop from backlog; count in "progress since last review" |
| `still-open` | Reproducible at the cited (or moved) location | Carry forward with refreshed `file:line` |
| `superseded` | The subsystem was redesigned; the finding no longer applies as stated | Drop, note the redesign |
| `unverifiable` | Cannot confirm either way from the code | Carry forward flagged as unverified — never silently drop |

Report the verdict counts. A synthesis that re-lists fixed findings as open is a failed review.

### Phase 4 — Synthesize

Build the five output sections. The core judgment: for each concern the scope handles, who owns it, is that ownership clean in the code, and what decided-but-undelivered or discovered-but-undecided work exists.

### Phase 5 — Output

Write `where-are-we.md` with exactly these five sections:

```markdown
# Where Are We — <scope> (<date>)

## 1. What we want
Intent, from decision records (cite by NAME, never by path into a transient
task system), docs, goals. The target architecture in a few paragraphs.

## 2. What we have
Current state: package/doc map summary, what is sound, what has drifted.
In synthesis mode: progress since the prior review (N fixed / M still-open /
K superseded) and the surviving findings.

## 3. What we need to do
The consolidated gap: still-open findings + decided-but-unbuilt items +
new findings from this run. Grouped, deduplicated.

## 4. Concerns and ownership
| Concern | Owner | Sub-owner | State |
One row per concern the scope handles; State = clean / blurred / missing.
Name the boundary principle if one emerges (one sentence).

## 5. Proposed work structure
An ordered, dependency-aware backlog ready to become tasks:
N. <slug-like title> — one-line goal. (depends on: <item>; feeds: <item>)
Order by: unblockers first, then structural work, then deferred phases.
```

## Constraints

- Re-verification before carry-forward is non-negotiable (Phase 3) — stale findings are worse than missing ones.
- Cite decision records by name inline; do not link into transient task systems from durable docs.
- Every new code finding needs `file:line`; every backlog item needs a one-line goal and its dependencies.
- Say what was NOT covered (missing sources, skipped tracks) — silent gaps read as "covered".
- Section 5 must be directly usable as a task backlog — no vague "improve quality" items.
