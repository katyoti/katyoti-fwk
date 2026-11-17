# Lessons from the `tasks` Module Bootstrap

**Date**: 2026-05-06
**Context**: First time the rock-solid method was applied end-to-end on a real module (`tasks`), going from "no charter" to "charter v0.1 + 51/54 DoD-per-UC + all 6 ODs resolved + ready for plan-tier."

This doc captures what we actually did, what we learned, and the heuristics that emerged. The `code-module` skill at [`app/ktw-code/embed/skills/code-module.md`](../../app/ktw-code/embed/skills/code-module.md) is informed by these lessons rather than the original speculative spec. This is the **Phase A.5 retrospective** the workflow doc said would happen — applied at the first opportunity.

## What we did, in actual order

The theoretical workflow says: Bootstrap → Specify → Plan → Execute → Verify → Audit → Promote → Retrospective. The first real run had finer-grained sub-steps:

| # | Step | Output | Time |
|---|---|---|---|
| 1 | Survey source for ground-truth (`presets/modes.go` for the dimension matrix) | Mental model of the actual module | 5 min |
| 2 | Copy audit bundle to `/modules/<module>/audits/<date>/` | Bundle structure validated cheaply | 2 min |
| 3 | Write `audits/<date>/manifest.json` | Audit metadata (date, git SHA, reviews, finding counts) | 5 min |
| 4 | Write `manifest.json` for the module bundle | Aggregator-readable module state | 5 min |
| 5 | Write `charter.md` skeleton (Meta + Concepts + glossary) | Anchor structure | 30 min |
| 6 | Build UC inventory from 4 sources | 47 → 54 UCs after simulator inclusion | 60 min |
| 7 | Function inventory + dimension matrix + spawn policies table | Tier-1 spec content | 30 min |
| 8 | DoR + DoD per module + audit ledger + status board + ODs + glossary | Charter draft v0.1 | 30 min |
| 9 | User remarks → restructure (R#1 API, R#2 simulator, R#3 verify output) | Charter expanded; method docs updated | 60 min |
| 10 | Specify pass: DoD-per-UC for 51/54 UCs in batches | ~330 lines of detailed assertions | 90 min |
| 11 | Resolve 6 ODs (3 deferred → red-deferred UCs; 3 → tier-1 tasks) | Charter ready for plan-tier | 30 min |
| 12 | Update manifest, status board, READMEs after each major change | Bundle stays internally consistent | (continuous) |

**Total**: ~5 hours of focused work for charter v0.1 of one module. The method's "4-8 weeks for tier 1" estimate was about *executing* the milestone after charter is ready — not about the charter bootstrap itself.

## Heuristics that emerged

### UC inventory seeding

UCs come from **four different sources**, each with a different shape:

1. **Source code reads** → `core` UCs. Survey the module's primary types and their methods; one UC per "this method does its job."
2. **Audit findings** → `security` and `core` UCs. Each critical/high finding maps to ≥1 UC. Severity inherited from the finding.
3. **Existing tests** → covered UCs. Each `_test.go` test name implies a UC; if not in the inventory yet, add.
4. **Configuration / preset enumeration** → matrix UCs. Each cell of the dimension matrix should map to ≥1 UC for the supported combinations.

The merge: deduplicate, assign stable IDs with gaps between groups (UC-001..019 engine, UC-020..039 workflow, UC-040..049 scheduling, …) so later additions slot in cleanly.

### Stable IDs with gaps from day 1

Don't number sequentially. Use blocks: UC-040 to UC-045 for scheduling, UC-050 to UC-058 for permissions, etc. When tier-2 work adds UCs to scheduling, they land at UC-046..UC-049 — no renumbering, no test-file rename cascade.

### `(gap)` marker is the planning seed

When writing DoD-per-UC, the L1 evidence often references a test that *doesn't exist yet*. Mark it `(gap)`. Each `(gap)` is a future task: "create the test that proves UC-XXX." This pattern emerged organically and is the cheapest way to convert spec into an actionable task list — one task per `(gap)`.

### Negative DoD for security UCs

Security UCs need **two assertions**: positive ("the right thing happens") AND negative ("the wrong thing must NOT happen"). E.g. UC-057 path-traversal:

- Positive: `/repo/src/foo.go` is allowed
- Negative: `/repo/src/../../etc/passwd` is rejected — under any normalization, any input key, any platform path separator

Without the negative, the test passes when nothing is checked. With the negative, the test catches the bypass.

### L2 scenarios default to simulator-backed

Per remark R#2 — "task is not bound to agents." Every L2 scenario uses `type: simulator` unless the UC intrinsically requires a real LLM (e.g. `ask-user`). This turns L2 from "expensive token-burning" to "cheap deterministic" and makes the verify gate runnable before every promote.

### Resolved-OD shapes diverge

An OD can resolve in three structurally different ways:

- **Tier-1 task**: requires code change. The resolution becomes a task with full Goal + Success Criteria. (OD-001, OD-002, OD-006 went here.)
- **Deferred-red UC**: feature explicitly not implemented; UC stays in inventory with `🟠 fail-deferred` verdict. (OD-004, OD-005 went here.)
- **Pure charter update**: structural; no code change. (OD-003 was just "include this folder in the bundle.")

The skill should ask, when resolving an OD: "does this require code? Does it leave a feature absent? Or is it just scope?"

### Currently-FAILS annotations

When a DoD-per-UC body says "**Currently FAILS** per sec-1," that's both a documentation note and a planning input. The audit found the bug; the DoD says what passing looks like; the gap is a tier-1 task. The annotation is the bridge.

### One-glance status board > per-UC drill-down

The charter status board is the **one table** anyone reads first. UC count, DoD count, passing count, audit findings, deferred reds, ODs blocking. Keep it ≤14 rows; everything else is detail. The factory rollup ([`factory-rollup.md`](factory-rollup.md)) is the same shape across modules.

## What was non-obvious

### Audit relocation surprises

The audit was originally at `/modules/tasks/reviews/tasks_2026-05-06_07-56/`. Moving it to `/modules/tasks/audits/2026-05-06_07-56/` was nominally simple but had two gotchas:

1. **Internal markdown links survived** because they were same-folder (`[architecture.md](architecture.md)`).
2. **Source-file references were text, not links**, so `internal/extensions/task/spawner.go:127` rendered the same way after the move.
3. **But links to OTHER project files** (`../../internal/extensions/...`) would have broken — luckily we didn't have many of those in the audit reports.

Lesson: when relocating an audit bundle, prefer copy + breadcrumb at old location (or ensure links are project-relative-root, not relative-to-folder).

### The charter wants to be ~600-900 lines

I aimed for "comprehensive but not bloated." Actual: 858 lines. Sections that wanted to grow: Use Cases (~150 lines), DoD-per-UC (~330 lines), spawn policies and dimension matrix (combined ~80 lines).

If much under 500 lines, the charter is probably underspecified. If much over 1500 lines, it's probably mixing what should be in `specs/uc-NNN.md` separate files.

### "Stability" alone is too coarse

I used `stable`/`provisional`/`deprecated` per the template. The audit immediately surfaced ~15-20% of `stable` symbols had no callers — meaning they aren't really stable, they're effectively orphaned. Adding a second axis (`truly-public` vs `effectively-internal`) per OD-006 catches this.

The skill should bake in: **two axes**, not one — temporal stability AND consumption breadth.

### The `🟠 fail-deferred` verdict was not in the original spec

It emerged from OD-004 and OD-005 — the user wanted explicitly known-failing UCs to show RED on verify, not be hidden. The 4-verdict model (pass/fail/no-test/skip) didn't capture this. We added 5th: `🟠 fail-deferred` with `deferred_by: OD-NNN`.

This is the most generalizable lesson: any new charter will discover "we know about this gap; track it as a red but don't block promote on it." The pattern needs to be in the skill from day 1.

### Specify-pass length

Filling DoD-per-UC for 51 UCs took ~90 minutes. Per UC: ~5-10 lines. The depth varied:

- Engine primitives (UC-001..019): ~5 lines each (terse — the L1 test name is most of the DoD)
- Permissions (UC-050..058): ~10-15 lines each (security UCs need negative DoDs, finding citations, "currently fails" annotations)
- Simulator (UC-150..156): ~5 lines each (small surface)

Lesson: **batch by category** in the skill — don't try to write all 50 DoDs in one stream. Take a category, write its DoDs, move on. Fatigue produces shallower DoDs; categorical batches keep the depth uniform.

## What to bake into the skill

The skill at [`code-module.md`](../../app/ktw-code/embed/skills/code-module.md) encodes these lessons. Key inclusions:

1. **The 12-step bootstrap order** (Section "Process"). Don't let an LLM go bootstrap → DoD → ODs → manifest in a random order; that produces inconsistencies. Manifest updates must follow content updates.
2. **The 4-source UC inventory** (subsection of Specify). Each source has its own scan pattern.
3. **`(gap)` marker** as a first-class convention.
4. **Negative DoD requirement for `security` severity** — explicit rule.
5. **Simulator-by-default for L2** — explicit rule.
6. **OD resolution decision tree** — task / deferred-red / pure-charter, pick one.
7. **`fail-deferred` verdict** — built into the verify spec from day 1.
8. **Batch DoD authoring by category** — a process hint, not a hard rule.

## What's unchanged from the original spec

- The high-level workflow (Bootstrap → Specify → Plan → Execute → Verify → Audit → Promote → Retrospective) is correct and survives.
- The bundle structure is correct.
- The L1/L2/L3 testing layers are correct (and the simulator framing extends L2).
- The factory layer (rollup, lint, alignment, quality-tooling) is correct.
- The `code-module` verb set (charter, status, lint, plan-tier, verify, audit, promote, factory-status) is correct.

## Open question for the next round

When the charter is bootstrapped *programmatically* by the skill (rather than by hand), how much can be automated?

- **Highly automatable**: manifest scaffolding, audit relocation, status board population from manifest, OD-blocking-tier check, dimension-matrix lift from `presets/modes.go`.
- **Partially automatable** (LLM-assisted): UC inventory from source + tests, DoD-per-UC drafting (one category at a time, human reviews).
- **Human-mandatory**: severity tagging, OD identification, OD resolution, "currently fails" annotations from audit reading.

The skill should be designed so the automatable parts run first (cheap, deterministic), then the human-mandatory parts present clear prompts for the LLM (or human) to fill in.

## Filing this lesson

Per [`workflow.md`](workflow.md) Retrospective phase, this file is the first lesson. When `/modules/docs/lessons/` is created (currently this file lives in `/modules/docs/` for visibility; future lessons go in the subdirectory), this should move to `lessons/2026-05-06_tasks-bootstrap.md`.

---

# Lesson 2 — The E2E Test Authoring Gap (Phase A.5 → B Step 5)

**Surfaced**: 2026-05-06, just after plan-tier 1 generated 30 task files. The user asked: *"do we have a definition of how to create the e2e test cases to validate all UC? where is this analysis and implementation done?"*

**The gap**: we had documented [`test-layers.md`](test-layers.md) (the L1/L2/L3 conceptual model) and DoD-per-UC bodies that referenced test files (e.g. `tests/L2/uc-053-coord-via-workflow.tasktest`). But there was **no concrete authoring procedure**:

- No `.tasktest` schema reference (what fields go in frontmatter, what's in the body)
- No assertion DSL (how to express "must contain event X with payload Y", "must NOT contain Z")
- No worked example (one UC walked end-to-end from DoD to passing scenario)
- No fixture conventions (where they live, naming, immutability)
- No template file an author could copy

A milestone with 18 test-creation tasks pointed into the void. The first author through any of those tasks would have invented their own format, producing inconsistent tests + drift.

## What we did

1. Wrote [`e2e-test-authoring.md`](e2e-test-authoring.md) (537 lines): full schema, DSL, worked example UC-053 end-to-end, common patterns, anti-patterns, harness contract, step-by-step procedure. Loadable: `ktw-code:/reference/modules/e2e-test-authoring.md`.
2. Created [`templates/template.tasktest`](templates/template.tasktest) — the master template authors copy.
3. Created `/modules/<module>/tests/README.md` — test-folder index pointing at the guide.
4. Cross-linked from `test-layers.md` and the `code-module` SKILL.md.

(Original lesson was authored when these files lived under `/modules/docs/`; they migrated into the embed tree in the same Phase A.5 batch — see "Lesson 3" below for the migration retrospective.)

## What this means for the method (and the skill)

### Generalized lesson

**Whenever a charter defines DoD-per-UC that references tests, the author of those tests needs FOUR things:**

1. **A schema / template** — the form of a test
2. **A worked example** — the shape, end-to-end
3. **An assertion DSL** — the contract; what "passing" looks like in machine-checkable terms
4. **A pointer to fixtures** — where inputs live, conventions for them

Without all four, "create a test" is too vague. It produces inconsistent tests across authors, drift over time, and re-litigation of "what does passing look like?" every time a UC is touched.

This applies to **every module charter**, not just `tasks`. Every module's plan-tier creates test-creation tasks; every author needs the four items.

### Skill-level consequence — preconditions for plan-tier

The `code-module plan-tier` verb should **refuse to generate test-creation tasks if `e2e-test-authoring.md` (or its module-specific equivalent) is missing or stale**. Otherwise plan-tier produces tasks that bottom out into "??? figure out the test format." Treat the authoring guide as a **first-class precondition**, like the charter itself.

Concrete check: at `plan-tier <n> <module>` start, lint:
- `e2e-test-authoring.md` is loadable via `ktw-code llm resources read ktw-code:/reference/modules/e2e-test-authoring.md`
- `/modules/<module>/tests/L2/_template.tasktest` exists (or the module declares no L2 work)
- `/modules/<module>/tests/README.md` exists

If any are missing, refuse with a clear "bootstrap the test authoring infrastructure first" message + `--force` override for emergencies (recorded in manifest).

### Skill-level consequence — test-creation task requirements

Every task that says "create a test for UC-X":
- Must cite [`e2e-test-authoring.md`](e2e-test-authoring.md) as the procedure to follow
- Must reference (or copy) a fixture from `fixtures/` rather than inline content
- Must specify the layer (L1/L2/L3) the test targets
- For L2: must use `_template.tasktest` as the starting point
- For security UCs: must include `journal_must_not_contain` (negative DoD)

### Skill-level consequence — test-authoring guide should be portable

`e2e-test-authoring.md` lives at `/modules/docs/` because the procedure is **method-level**, not per-module. Every module that adopts the method shares the same guide. A new module's bootstrap doesn't require rewriting the guide; it requires creating module-specific fixtures and a module-specific `tests/README.md`.

If a future module discovers the procedure needs adjusting (e.g. a different test format for a non-Go module), the guide gets a new section, not a fork.

## Pattern for filing future lessons

Lessons compound. Each lesson should:

1. **Name what was discovered** (the gap)
2. **Show what we did** (the patch)
3. **Generalize to method/skill rules** (what changes for future modules)
4. **List concrete skill changes** (what the skill should now check / refuse / require)

This format keeps lessons actionable. Lessons that don't generalize are still worth recording, but lessons that DO generalize directly drive method/skill evolution — which is the whole point of the Retrospective phase.

## Filing

All three lessons stay in this single file for now. When the lessons subfolder gets used (post-tier-1 promotion), this file splits into one-lesson-per-file under `lessons/`.

---

# Lesson 3 — Method Docs Belong in the Embed Tree (Phase A.5 → progressive discovery)

**Surfaced**: 2026-05-06, immediately after the SKILL.md was rebuilt with all lessons baked in. The user observed: *"have you put all the reference files / temple into app/ktw-code/embed/references/modules and referenced them in the main skill, and have you put each step in a reference file in app/ktw-code/embed/references/modules to avoid to read a huge skill and allow progressive discovery"*

**The gap**: the SKILL.md was bloated (~300 lines) because verb-by-verb procedures, worked examples, and rules were inlined. Method references lived at `/modules/docs/`, NOT in the embed tree, so they didn't ship with the binary and couldn't be loaded via `ktw-code llm resources read`. The structure violated the standard ktw-code skill pattern (a lean SKILL.md + on-demand references in `embed/references/`).

## What we did

1. Migrated 12 method docs from `/modules/docs/` to `app/ktw-code/embed/references/modules/`.
2. Created 8 per-verb references in `app/ktw-code/embed/references/modules/verbs/` (one file per verb, ~80-120 lines each, with consistent anatomy: inputs / when-to-use / preconditions / behavior / output / failure modes / see-also).
3. Created `app/ktw-code/embed/references/modules/templates/` and put `_template.tasktest` master there.
4. **Slimmed the SKILL.md from ~300 lines to ~130** — verb table with one-liners + reference pointers; high-level workflow diagram; 13 always-applicable rules; reference-pointer tables.
5. Updated `/modules/docs/README.md` to redirect to the embed location.
6. Discovered + fixed the `go:embed _`-prefix exclusion (added `all:` prefix to embed directive so `_template.tasktest` ships in the binary).

## What this means for the method (and the skill)

### Generalized lesson

**A skill should be a lean orchestrator, not a manual.** The SKILL.md is what an LLM reads first; references are what it loads on demand based on what it's actually doing. If the SKILL.md exceeds ~200 lines OR if loading it costs more tokens than the work it's about to do, the structure is wrong.

The four operational levels:

| Level | Content | Lines (typical) | Loaded |
|---|---|---|---|
| **SKILL.md** | Verb table, workflow shape, always-applicable rules | 100-200 | Once per skill invocation |
| **Per-verb reference** | One verb's full spec (inputs, behavior, output, failure modes) | 80-150 | When that verb is used |
| **Method reference** | One topic across all verbs (e.g. test-layers, alignment) | 200-500 | When that topic is relevant |
| **Templates** | Reusable artifacts authors copy | (varies) | When authoring something new |

Skipping any level produces friction:
- No per-verb references → SKILL.md bloats; LLMs scan past their relevant verb to find rules
- No method references → the skill assumes prior knowledge it can't possibly have
- No templates → "create a test for UC-X" produces inconsistent output across authors

### Skill-level consequences

- **`code-module` SKILL.md is now 130 lines and stays under 150** as a discipline.
- Per-verb references are the canonical detail location; the SKILL.md just points.
- Method references are the canonical conceptual depth.
- Templates are the canonical starting points for authored artifacts.

### `go:embed` gotcha (Go-specific, but worth recording)

**Files starting with `_` or `.` are excluded from `go:embed` by default.** `_template.tasktest` failed to load until the embed directive used `all:embed/references` instead of `embed/references`.

This bites any Go project using leading-underscore conventions for templates / fixtures / private data. The `all:` prefix is the fix. Check for it whenever an embedded file mysteriously doesn't load.

### Pattern for future skills built from this method

When a future skill is authored using this method:

1. **Start with the SKILL.md** (orient, list verbs, declare rules).
2. **Stub one reference file per verb** in `embed/references/<skill-name>/verbs/` — even if empty initially, the structure is in place.
3. **Move method depth to references** as soon as the SKILL.md exceeds ~200 lines.
4. **Use `all:` in the `go:embed` directive** if any file starts with `_` or `.`.
5. **Verify load via `ktw-code llm resources read`** after rebuilding — references that don't load are worse than missing.

## Cross-cutting observation about the three lessons

Each lesson has the same shape:

1. The user asked a sharp question that surfaced a gap.
2. I patched the gap.
3. Capturing the patch as a lesson + skill update made it permanent.

Lesson 1 (heuristics from the bootstrap): emerged from doing the work.
Lesson 2 (E2E test authoring procedure): emerged from preparing to plan tasks.
Lesson 3 (skill structure + embed): emerged from preparing to ship the skill.

A pattern is forming: **each major phase transition surfaces a gap, and the fix becomes a permanent improvement to the method.** The retrospective phase isn't waiting for tier-promotion — it's happening continuously, and the skill is getting sharper at every cycle.

This is the explicit goal of [`workflow.md`](workflow.md) Retrospective: keep the method itself improving. Three lessons in one day suggest the practice works. Future modules will likely surface fewer per-bootstrap as the method matures.

## Filing

All three lessons stay in this single file for now. When the lessons subfolder gets used (post-tier-1 promotion), this file splits into one-lesson-per-file under `lessons/`. Naming convention: `<date>-<module>-<lesson-slug>.md`, e.g. `2026-05-06-tasks-progressive-discovery.md`.

Each tier promotion should produce **at least one** lesson. The first run already produced two; subsequent runs typically produce fewer (the method is calibrating).
