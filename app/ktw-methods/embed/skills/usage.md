---
name: usage
description: >-
  What ktw-methods is for and how to drive it. Load first when encountering this sidecar.
hashtags: [usage]
category: guide
advertised: true
---

# ktw-methods — usage

`ktw-methods` is the centralized home for **cross-domain articulation
methods** — reusable thinking procedures that are not specific to any one
sidecar or domain. Other capabilities (composition wizards, dossier
refinement, code methodology) load these skills instead of re-inventing
their own.

## What it ships

| Skill | What it is |
|---|---|
| `ktw-methods:/skill/create-method` | **START HERE for creating anything.** The unified method — want → have → increment → do → verify → gate → done — that ROUTES each stage to the skill below. Load first when asked to create / build / produce something non-trivial. |
| `ktw-methods:/skill/isa` | **ISA — Ideal State Artifact**: articulate "done" for any thing as one document of testable claims. Routes to scaffold / interview / check-completeness workflows. (the WANT lens) |
| `ktw-methods:/skill/value-fit` | Triage existing material — add / enhance / included / superseded, with evidence. (the HAVE lens) |
| `ktw-methods:/skill/plan-increment` · `…/new-functionality` | WSJF backlog pull + just-in-time elaboration; shape a raw idea into features serving goals. (the INCREMENT lens) |
| `ktw-methods:/skill/verify-and-gate` · `…/completeness-gate` · `…/audit-pass` · `…/decision-capture` | The QUALITY lens: run probes + **computed** done/goal-met gate; structural pre-check; perspective audit; decision capture. |
| `ktw-methods:/skill/drive-method-ui` | **The UI lens** — make a creation method DRIVABLE from the GUI: declare the four-lens surface (want / have / increment / quality + action bar) on the method's types. Routes to `author-ui-{form,view,table}` · `compose-ui-live` · `render-data-as-ui`. |
| `ktw-methods:/skill/interview-technique` | Structured interview methodology — open-before-closed, one question at a time, summarize-then-confirm. |

## How to drive it

- Agent: load a skill with **skills-get-instruction**, then follow it.
- CLI: `ktw-methods sc skills list` for the catalog, `ktw-methods llm resources read <uri>` for any skill or reference content.

This sidecar is files-only: skills + references, no Go business logic.
