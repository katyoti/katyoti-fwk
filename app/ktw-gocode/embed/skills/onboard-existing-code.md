---
name: onboard-existing-code
description: >-
  Help a user (or persona) decide where existing Go code belongs in
  the CWM model, then drive the discovery + classification + seeding
  flow that turns code into a tracked Module with charter / UCs /
  test-cases. Use when there is real code at a path but no charter,
  or when a chunk of code should be promoted into the workspace's
  managed set. Picks between "update existing module" / "new module" /
  "split existing module" before any seeding runs.
icon: "⚓"
iconName: anchor
hashtags: [retro-engineer, discovery, module, bootstrap, onboarding]
category: lifecycle
provides: [task]
---

# Onboard Existing Code — Retro-Engineering Bootstrap

This skill drives a user from "I have Go code at path X (but no
charter / no UCs / no module object)" to a properly-placed `module`
object with seeded child objects.

It is **conversational + discovery-driven**. It calls the
`module.discover` verb under the hood, but the skill's job is to
DECIDE WHERE THE CODE BELONGS before that verb fires. Bad placement
is the most expensive mistake to undo, so the skill front-loads that
decision.

> **The hard rule**: this skill REFUSES to call `module.discover`
> until the placement question is answered and confirmed. No "let's
> just scan and see what comes out."

## Procedure

### Phase 1 — Locate the code

Open with:

> "Where is the code? Give me a path (a Go package, a folder, or a
> list of folders). I'll look at what's there before we decide where
> it fits."

Capture: the path(s), and a one-line user description of what the
code does ("the budget enforcement bits", "the LLM-trace exporter").
Do NOT proceed if the user can't describe what the code does in one
line — that's a sign the code itself is unclear, not just unplaced.

### Phase 2 — Scout (read-only, NO discovery yet)

Use file tools to **shallowly** inspect the path:

1. `glob` to list `*.go` files
2. `file_read` the package's top-level `doc.go` (if any) and the
   first 50 lines of the main files
3. Check for: `cmd/main.go`, `app/` parent directory, presence of
   `public/internal/local/` layout
4. Read `go.mod` of the module root to identify dependencies

Report back to the user in 5-10 lines:

> "Here's what I found at <path>:
>   - <N> .go files across <M> packages
>   - Top-level type: <type or interface>
>   - Detected layout: <layout signal>
>   - Detected kind hint: <framework | library | sidecar | unclear>
>   - Existing tests: <test file count>
>   - Imports beyond stdlib: <count + notable ones>
> "

### Phase 3 — Decide placement (the critical step)

Walk the user through **three placement options** in order. Always
ask, never assume:

#### Option A — Update an existing module object

Query CWM:

```
cwm-object-list(parent=<project-root object id>, type=module)
```

For each existing module, check if its `code_packages` overlaps with
the user's path, OR if the user's described purpose overlaps the
existing module's charter (if any).

If overlap exists, ask:

> "This code looks like it might already belong to <existing-module>.
> Should we EXTEND that module's `code_packages` to include this
> path, rather than create a new module?"

If yes → skip to Phase 5 with `mode: update-existing`.

#### Option B — Create a new module

If no overlap, propose a new module:

> "I'd create a new `module` object named <slug>. Based on what I saw:
>   - kind: <framework | library | sidecar>
>   - code_packages: [<path>]
>   - doc_folder: <proposed path or null>
>   - charter_path: modules/<slug>/charter.md
>
> Does the kind look right? (See `references/module-kind-decision.md`
> if unsure.)"

If yes → Phase 4 with `mode: new-module`.

#### Option C — Split an existing module

If overlap exists BUT the code is large or covers a distinct domain:

> "This code might better be its own module, even though it currently
> sits inside <existing-module>. Splitting it would create a new
> module sibling. Trade-off: separate quality arcs (independent
> tier promotion) vs. more objects to maintain. Do you want to split?"

If yes → coordinate with the architect persona; this skill exits
with a recommendation rather than executing the split itself.

### Phase 4 — Decide what features the code realizes

**Before** running discovery, ask:

> "Does this code realize one or more existing Features, or is the
> feature side not yet captured? If it realizes features, name them
> (or describe what user-visible capability they correspond to)."

Query CWM:

```
cwm-object-search(query=<the described capability>, types=[feature])
```

Match by user-described capability. If matches exist → note them as
`realized-by` edges to be created. If no matches → ask the user if
features should be created NOW or LATER:

- **NOW**: invoke the `new-functionality` skill (separate skill) to
  bootstrap the feature side, THEN come back here for the module
  side. This is the cleanest order if the feature side will exist
  anyway.
- **LATER**: proceed with module creation; leave `realized-by` empty.
  Architect persona can backfill the feature side once placement is
  clear.

### Phase 5 — Run discovery (now safe)

Placement is confirmed; features are noted. Call the verb:

```
module.discover(
    path=<user-path>,
    target_module_id=<new-or-existing>,
    auto_commit_code_symbols=true,    // per §15.12: AST output is safe
    propose_use_cases=true,             // proposed, not auto-written
    propose_test_cases_from_tests=true  // propose seed UCs from Test*
)
```

Report the discovery output to the user:

> "Discovery proposed:
>   - <N> code-symbol objects (already committed: deterministic from AST)
>   - <M> use-case objects (PROPOSED, awaiting your review)
>   - <P> test-case objects (PROPOSED, one per existing Test* function)
>   - <Q> dependency edges to other modules
>
> The committed objects are visible now. The proposed objects are queued
> for the specifier persona to refine. Approve all / approve some /
> reject all?"

### Phase 6 — Hand off

After approval:

- Print the module's object id + path
- Print the count of objects now living under the module
- Recommend the next persona to spawn:
  - `architect` (if classification / decomposition still needed)
  - `specifier` (if proposed UCs need DoD bodies refined)
  - `tester` (if there's a quick win: existing tests already pass, can
    cache UC status as `pass`)

## Rules this skill follows

- **Placement before discovery** — never run `module.discover`
  before the placement question is answered and confirmed.
- **Update wins over create** — if existing modules overlap, extend
  them; don't create siblings unless the user explicitly chooses to.
- **AST is safe, judgment is reviewable** — auto-commit
  AST-derived objects (`code-symbol`); ALWAYS propose
  judgment-derived objects (`use-case`) for review (per §15.12).
- **Feature side first if it'll exist anyway** — if the code clearly
  realizes a feature that should be tracked, bootstrap that feature
  before running discovery, so `realized-by` edges land in one
  pass.
- **No tier-claims** — the skill's job ends at "module exists with
  seed objects." Tier promotion is a separate workflow.

## When NOT to use this skill

- The code is greenfield (nothing exists yet) → use the
  `new-functionality` skill or directly invoke `module.create`.
- The code is already a tracked module and just needs a tier bump →
  use the `module.plan-tier` verb.
- The user is reorganizing existing modules → use the `architect`
  persona; this skill handles placement-into-the-model, not
  refactoring-the-model.

## References (load on demand)

| Topic | Reference |
|---|---|
| Module kind decision tree (framework / library / sidecar) | `references/module-kind-decision.md` |
| Layout signals that imply kind | `references/layout-signals.md` |
| Splitting vs extending — heuristics | `references/split-or-extend.md` |
| Seed-object review checklist | `references/seed-review-checklist.md` |
| `module.discover` verb spec | `references/verb-module-discover.md` |
| CWM tool reference | `references/cwm-plugin-tools.md` |

## Output format

When discovery completes, the skill prints:

```
Module onboarded:
  id:           budget-enforcer
  name:         budget-enforcer
  kind:         library
  code_packages: [internal/extensions/budget/]
  doc_folder:   modules/budget-enforcer/  (proposed; not yet created)
  charter_path: modules/budget-enforcer/charter.md (skeleton stub)
  tier_state:   bootstrapping
  tier_target:  1

Objects committed:
  code-symbol  × 14   (AST-derived; safe)
  test-case    × 6    (one per existing Test* function in budget_test.go)

Objects proposed for review:
  use-case     × 7    (titled by inference from public symbols + test names)

Edges proposed:
  realized-by  × 1    (→ existing feature f-budget-control)

Next step: spawn `specifier` on this module to refine the proposed
use-case objects. Then `architect` to confirm placement and
dependencies.
```
