---
name: create-method
description: >-
  The unified method for creating ANYTHING in katyoti — a skill, a task, a
  document, a tool, a sidecar — with quality, increments, and gates. Load
  first when asked to "create / build / produce" something non-trivial, or
  to understand how katyoti's creation methods fit together. It does not do
  the work itself: it routes the want → have → increment → do → verify →
  gate → done loop to the specialist skills that each own one stage.
iconName: route
icon: "🧭"
hashtags: [create, method, lifecycle, quality, increment, gate, methodology]
category: methodology
---

# Create-method — how to create anything, with quality

Creating anything well — a skill, a doc, a tool, a whole sidecar — is the
same loop. This skill is the **map**: it names the loop, the four lenses you
reason in, and **routes each stage to the skill that owns it**. It does NOT
re-do their work — load the routed skill when you reach that stage.

## The loop

```
WANT ──► HAVE ──► INCREMENT ──► DO ──► VERIFY ──► GATE ──► DONE
articulate  reuse-    pull the    build   run the    computed   computed
"done" as   check     next small  it      probes     rollup     convergence
testable    what      slice               (evidence) decides    (never a
claims      exists    (WSJF/JIT)                      done       hand-flip)
```

Run it breadth-first and shallow, depth-first and **late**: only the slice
you commit to NOW gets full acceptance criteria + tasks. Everything else is a
one-line backlog card (value + effort). Defer the expensive detail until it is
committed and therefore stable.

## The four lenses — what you reason in

| Lens | The question | The artifacts |
|---|---|---|
| **WANT** | what does "done" mean? | goal → use-case → acceptance-criterion (one probe each) |
| **HAVE** | what already exists? | the live catalog; reuse before create |
| **INCREMENT** | what's the next small slice? | the WSJF backlog → the committed increment |
| **QUALITY** | what's proven, what's broken? | computed coverage / rollup / gate — never declared |

To **drive these four lenses from the GUI** — surface them as tables, badges,
and an action bar on the method's CWM types — load
`ktw-methods:/skill/drive-method-ui` (it maps each lens to a declarable
type-driven-ui surface, routing to the `author-ui-*` skills).

## The routing table — one skill per stage

Load the named skill when you reach its stage. **One method per outcome** —
do not improvise an alternative to a stage that has an owner.

| Stage | Load | For |
|---|---|---|
| **WANT — articulate done** | `ktw-methods:/skill/isa` | articulating done: the feature a test declares done, its acceptance criteria and anti-criteria; `ktw-methods:/skill/define-goals` for the goals it serves (directions, never done); `ktw-methods:/skill/interview-technique` for the questioning |
| **WANT — shape a raw idea** | `ktw-methods:/skill/new-functionality` | turning "I want X" into features (an epic or a user story is a feature of a given size, never a type) serving goals |
| **HAVE — reuse-check** | `ktw-methods:/skill/value-fit` | triage what exists: add / enhance / included / superseded, with evidence |
| **INCREMENT — plan the slice** | `ktw-methods:/skill/plan-increment` | WSJF-rank the backlog, pull to capacity, trigger just-in-time elaboration |
| **DO — build it** | the **personas** (specifier · architect · coder · reviewer · tester · documenter) + the domain's creation chain | the actual production of the artifact |
| **VERIFY + GATE — prove + decide** | `ktw-methods:/skill/verify-and-gate` | run each criterion's probe, record the evidence, let the deterministic gate decide `done`; `ktw-methods:/skill/completeness-gate` for the structural pre-check; `ktw-methods:/skill/audit-pass` for the perspective audit |

## The invariants that make it work

1. **Elaboration is OPTIONAL until commit.** A backlog item is valid with only
   a title + value + effort. Acceptance criteria + tasks are filled when the
   item is PULLED, never required to file it. (`plan-increment` enforces the
   pull-time elaboration; `lean-increment-flow` is the doctrine.)
2. **Quality is COMPUTED, not declared.** "Done" is the convergence of an
   item's criteria; "goal met" is the rollup of its use-cases + deliverables.
   Never a hand-flipped checkbox or a chat scorecard. (`verify-and-gate` runs
   the probes through `object-verify`; an unverified `done` fails the gate —
   `object-verify` green + `object-lint` with no error finding.)
3. **Traceability to the source idea.** Every derived item — goal, feature,
   use-case, deliverable, decision — traces back to the originating idea it
   serves; nothing appears from nowhere. The idea is a goal or a feature in
   state `idea`; preserve the raw brain-dump in its body (`## Raw Input`) and
   link each derived item to it (a feature `serves` the goal, a sub-feature
   or a use-case sits under the feature). If you
   cannot name the source line a structured element came from, you either
   misread the idea or you are inventing — stop and check with the author. (The
   "WANT" lens captures intent; this invariant keeps the descent faithful to it.)

## Instances — the method, made concrete

This is the domain-neutral method. Its concrete instances:

- **compose** — `ktw-compose:tasks/compose-artifact` (the composition wizard)
  + `ktw-compose:tasks/import-artifact` (value-fit triage of a pile of assets)
  run this exact loop for files-only katyoti artifacts.
- **code** — the rock-solid code method (ktw-code / ktw-gocode) runs it for
  code, with a Go probe-runner behind the same VERIFY/GATE seam.

A new domain plugs into the same loop: implement the DO chain + a probe-runner
(`ktw-methods:/skill/verify-and-gate` §seam), and reuse every other stage.

## Reference

`ktw-methods:/reference/create-method/loop.md` — the loop in depth: each lens,
the JIT-vs-upfront rule, the computed-quality principle, and a worked example
("create a skill" walked end-to-end through the loop).
