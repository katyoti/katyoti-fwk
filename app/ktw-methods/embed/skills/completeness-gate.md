---
name: completeness-gate
description: >-
  Decide "is it done?" by a COMPUTED worst-of rollup over children — never an
  LLM judgment. Each leaf carries a computed status from a probe; a parent is
  only as done as its weakest leaf. Use when checking coverage / a tier gate /
  whether a node, increment, or dossier section is complete. NOT for writing
  criteria (that's the ISA skills) — this only rolls up their computed verdicts.
icon: "🚦"
iconName: traffic-cone
hashtags: [completeness, gate, rollup, coverage, verdict, computed]
category: lifecycle
---

# Completeness gate

A completeness verdict is **computed, not judged**. You roll up the computed
status of the leaves; the parent inherits the WORST one. This is the single
method-neutral rule behind code's UC-coverage / liaison verdict and da's
section-status — do not re-describe it per pack.

## The cardinal rule — computed, never "looks done"

Every leaf's status comes from a **probe / computed signal** — a test run, an
acceptance-criterion check, a "required section non-empty" test, a lint result.
You NEVER set a status by LLM judgment ("this looks complete"). If a leaf has no
probe, its status is `not-run` (a gap), not `pass`. The gate's job is to read
those computed signals and reduce them — nothing more.

## The worst-of ladder

Reduce a set of child statuses to one by taking the worst. The canonical order
(strongest-bad first) is:

```
regressed  >  fail  >  blocked/harness-error  >  not-run  >  pass
```

A parent is `pass` **only when every leaf is `pass`**. One `fail` anywhere makes
the parent `fail`; one `not-run` keeps it `not-run` (incomplete, not failed).
`regressed` (was pass, now fails) outranks a plain `fail` so a regression is
never masked. Map a pack's own status vocabulary onto this ladder, then reduce.

## Procedure

1. **Enumerate leaves + their computed status.** Pull each leaf's status from
   its probe/verdict source (e.g. a CodeService coverage aggregate, an AC
   verify result, a section-filled check). Do not infer a status you can't read.
2. **Reduce worst-of** up each level of the tree, level by level, to the node
   you're gating.
3. **Report the rollup + the blockers.** State the computed verdict AND the
   specific leaves that hold it back ("`fail` — ISC-7 probe red; ISC-12 not-run").
   A bare colour with no blocker list is not actionable.
4. **Gate = the computed verdict.** "Done" iff the rollup is `pass`. If a leaf is
   intentionally deferred, it must carry an explicit deferral record (see
   `ktw-methods:/skill/decision-capture`) — a deferral is a recorded decision,
   never a silent `pass`.

## Anti-criteria

- **Anti:** the gate must never flip to `pass` on an LLM "looks complete" — only
  a computed all-leaves-pass does.
- **Anti:** a leaf with no probe is reported `pass`. (It is `not-run` — a gap.)
- **Anti:** a `regressed` leaf is reduced to a plain `fail` and the regression
  hidden.
