---
name: define-goals
description: >-
  Clarify a project's or initiative's GOALS into `goal` objects, each with an
  observable success metric — goal before solution. Use when starting a
  project/initiative, or when goals are vague, missing, or solution-shaped
  ("we should rewrite X"). NOT for shaping a single feature into sub-features
  (use new-functionality); NOT the interview mechanics themselves (that's
  the interview-technique reference); NOT the granular per-UC ISA Goal.
icon: "🎯"
iconName: target
hashtags: [goals, objectives, metrics, project, initiative, ideation]
category: lifecycle
---

# Define project goals

Turn "what is this project/initiative FOR?" into a small set of `goal` objects,
each carrying an **observable success metric** — the hard-to-vary signal of
how close the work is. A goal is a direction the module strives toward and
never finishes (always `want.reachability: asymptotic`; worded as an outcome
or a quality): no test ever says it is done. Something a test can declare
done is a **feature** that serves the goal. This is the project/initiative altitude; for a
single capability use `new-functionality`, and for the granular per-use-case
done-condition use the ISA Goal (see the `isa` skill).

> **The rule (ISA §2):** a goal with no nameable success metric is fluff —
> anything satisfies it, so it constrains nothing. Every goal gets a metric or
> it is not recorded.

## Procedure

1. **Load the interview technique.** Follow `ktw-methods:/skill/interview-technique`
   for the conduct (open before closed, one question per turn, summarize then
   confirm). This skill supplies the *what*; that one supplies the *how*.
2. **Surface the real goal — not the solution.** If the input is
   solution-shaped ("migrate to NATS", "rewrite the parser"), ask "what would
   be true / different if that were done?" and record the underlying goal. Keep
   the solution as a candidate, not the goal.
3. **Scan existing goals first.** `cwm-object-list(parent=<the target object>, type=goal)`
   at the target scope (in a chat on a goal or a module, the goals service
   tools do this: `ktw-methods:goals/list`). If one already covers it, **extend/refine it** rather than add a
   near-duplicate.
4. **Pin a success metric per goal.** For each goal, get one observable signal:
   a number + threshold, a user-visible outcome, or a binary condition
   ("p99 < 200ms on the 1000-node tree", "an architect can produce a dossier
   end-to-end with no per-step help"). If the user can't name one, the goal
   isn't sharp yet — keep asking, don't record fluff.
5. **Propose, then write.** Present the draft goal set (goal + metric + scope)
   as one block; confirm; create the `goal` objects under the project /
   initiative object — `cwm-object-create` with `type: goal` and
   `parent` set to it, or `ktw-methods:goals/add` when
   the chat carries the goals tools. Print the created ids. If the user isn't ready, output the
   draft for them to save and exit unwritten.

## Done when
- Each recorded `goal` has a non-empty observable success metric.
- No goal is solution-shaped without its underlying why recorded.
- No near-duplicate of an existing goal was created (extend wins).

## Don't
- Don't record a goal with no success metric — ask until one exists.
- Don't record something a test can declare done as a goal — it is a feature.
- Don't decompose into features here — hand off to `new-functionality`
  / the `specifier` persona once goals are set.
- Don't accept a solution as the goal — surface the why first.
