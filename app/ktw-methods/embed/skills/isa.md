---
name: isa
description: >-
  ISA — Ideal State Artifact: articulate "done" for any thing (project,
  artifact, document, decision) as ONE document of testable claims. Load when
  asked to articulate an ideal state, define done, write acceptance criteria,
  scaffold or deepen or audit an ISA, or drive an interview toward a clear,
  hard-to-vary intention. Routes by verb to the scaffold / interview /
  check-completeness workflows.
iconName: target-arrow
icon: "🎯"
hashtags: [isa, articulation, criteria, done, methodology]
category: methodology
---

# ISA — Ideal State Artifact

The ISA is **one document** that articulates "done" for any thing whose ideal
state you are pursuing. It is read through five lenses at once:

1. **Ideal-state articulation** — the written, hard-to-vary explanation of done.
2. **Test harness** — its criteria (ISCs) ARE the tests; each names one binary probe.
3. **Build verification** — passing the ISCs verifies what was built.
4. **Done condition** — the work is complete when every ISC passes.
5. **System of record** — for the thing being articulated, across its life.

Because it is all of these, you never keep a spec AND a test plan AND an
acceptance checklist that drift apart. **Never invent parallel artifacts**
(`acceptance.yaml`, separate test specs) — the ISA already covers that surface.

## The core rule — hard-to-vary ≡ testable

A claim is **load-bearing** if and only if you can name a test that would
falsify it. If you cannot say what failure looks like, the claim is **fluff**
— it constrains nothing.

```
Fluff:        Email is delivered to the user.
Load-bearing: Email arrives in the primary inbox (not Spam) within 60s.
```

Every other rule in this method is a consequence of this one.

## Three guardrails + the derived fourth

| Guardrail | Binds | Example |
|---|---|---|
| **Principles** | the *thinking* | "User-facing systems prioritize responsiveness." |
| **Constraints** | the *solution space* | "OAuth via standard libraries — never roll our own crypto." |
| **Out of Scope** | the *vision* | "Mobile native apps are not part of v1." |
| **Anti-criteria** | the *test surface* | "Anti: `/admin` returns 200 in the v1 build." |

The first three are author-stated prose. Anti-criteria are **derived** — they
are how the prose guardrails become probe-able. ≥1 `Anti:` ISC is always
required.

## Workflow routing

Match the verb in the request to a workflow and load that skill:

| Verb / intent | Skill |
|---|---|
| "scaffold", "create", "generate", "new ISA" | `ktw-methods:/skill/isa-scaffold` |
| "interview me", "deepen", "fill in", "ask me questions", "clarify the intention" | `ktw-methods:/skill/isa-interview` |
| "check", "audit", "score", "is it complete?" | `ktw-methods:/skill/isa-check-completeness` |
| "seed", "bootstrap from this repo / existing code" | `ktw-methods:/skill/isa-seed` |
| "reconcile", "merge back", "fold the working copy into master" | `ktw-methods:/skill/isa-reconcile` |
| "append decision / changelog / verification", "record that…" | `ktw-methods:/skill/isa-append` |

A fresh articulation normally chains: (seed an existing repo →) scaffold →
interview (until the intention is hard-to-vary) → check-completeness. Across a
long-lived ISA, **append** is the canonical writer for the Decisions / Changelog
/ Verification sections, and **reconcile** folds a working copy back into master
keyed on stable ISC IDs. For most short-lived uses only scaffold + interview +
check-completeness are needed.

## The target — file or CWM items

Every workflow takes a **target** deciding how the ISA materializes:

- `isa-file` (default) — a single `ISA.md` at the caller-given path,
  shaped per the format spec.
- `cwm-items` — the same content as CWM items on a caller-given node
  (Goal → the `goal` it serves, never done; the thing a test declares
  done → a `feature`; ISCs → `acceptance-criterion` objects under its
  `use-case`s or the feature; Features → sub-`feature`s serving the goal;
  Test Strategy → `test-case` items; remaining prose sections go to the
  node body). One
  method, two materializations — the mapping table is in the format spec.

## References

- `ktw-methods:/reference/isa/format-spec.md` — the file-shape contract:
  twelve-section body, tier gate (E1–E5), ISC grammar, ID-stability,
  splitting test, the CWM mapping. **The spec wins on contradiction** with
  any skill prose.
- `ktw-methods:/reference/isa/examples/e1-minimal.md` — smallest valid ISA
  (Goal + Criteria only).
- `ktw-methods:/reference/isa/examples/e3-project.md` — mid-size project
  ISA, eight sections populated.

Read the example closest to your scale before scaffolding — copy its section
headers, then populate.
