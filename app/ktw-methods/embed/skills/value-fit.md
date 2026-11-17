---
name: value-fit
description: >-
  Decide whether an incoming asset (a skill, task, document, explanation,
  pattern, snippet) is worth adding to an existing system. Per candidate,
  reach ONE verdict with evidence — add / enhance / included / superseded —
  by scanning what the system already does and testing what the asset would
  genuinely ADD. Load when triaging external material, deciding "is this
  worth importing?", auditing a pile of candidate assets, or comparing an
  asset against what already exists. Cross-domain — code, docs, any sidecar.
iconName: scale
icon: "⚖️"
hashtags: [value-fit, triage, reuse, evaluation, methodology]
category: methodology
---

# Value-fit — is this worth adding?

You are handed material — a skill someone wrote, a task, a doc, an
explanation, a pattern — and asked whether it brings value to an existing
system. The trap is treating "it's nice" as "we should add it." Nice and
**additive** are different: a beautifully-written skill that duplicates one
you already have adds nothing but maintenance.

For each candidate asset, reach exactly **one verdict**, backed by evidence.

## The four verdicts

| Verdict | When | What you do |
|---|---|---|
| **add** | the capability is **absent** from the system; the asset names value you can NAME A PROBE for | create it (route to the system's creation path) |
| **enhance** | it **overlaps** an existing artifact but carries a real delta — more depth, a missing case, a sharper trigger | extend the existing artifact with the delta ONLY (not a second copy) |
| **included** | the system **already fully covers** it — no delta worth keeping | discard; record what already covers it so it is not re-triaged |
| **superseded** | the system's existing version is **as-good-or-better** | discard; harvest only any genuinely-new nuance into the existing one |

One verdict per candidate. No "could be add or enhance" — decide. If you
truly cannot, the overlap scan was too shallow; go back and finish it.

## The method

### 1 — Scan what the system already does (mandatory)

You cannot judge ADD vs INCLUDED without knowing the current surface. List
it: the system's existing capabilities — skills, tasks, references, whatever
the domain's catalog is. For each candidate, find the **nearest existing
artifact** (or confirm there is none). This is the same overlap scan the
creation path does before "extend vs create" — value-fit just adds the
INCLUDED / SUPERSEDED verdicts on top.

### 2 — Apply the value test (what would it ADD?)

ISA's hard-to-vary rule, applied to value: **name the probe**. What can a
user do, or do better, AFTER adding this that they cannot do now — stated as
something testable? If you cannot name that probe, the asset is not additive;
it is INCLUDED (or fluff). "It explains X nicely" is not a probe; "it lets
the agent handle the X-with-Y case that currently fails" is.

### 3 — Compare against the nearest existing artifact

- No nearest artifact + a real probe → **add**.
- A nearest artifact, candidate has a probe IT lacks → **enhance** (the delta
  is exactly that probe's worth of content).
- A nearest artifact that already satisfies the probe, equal or better → the
  candidate is **included** (equal) or **superseded** (existing is better).

### 4 — Require evidence

Every verdict names the existing artifact it compared against (a URI / a
path), or states "no existing artifact covers this." A verdict with no
comparison is a guess — reject it and rescan. ADD verdicts also state the
probe (the testable value); ENHANCE verdicts state the specific delta.

## Anti-patterns (each is a verdict error)

- **Add-the-duplicate** — verdicting ADD when an existing artifact already
  covers it because the candidate is well-written. Quality ≠ additive.
- **Cosmetic enhance** — verdicting ENHANCE when the delta is rewording, not
  new capability. If the probe already passes, it is INCLUDED.
- **Silent supersede** — discarding as SUPERSEDED without harvesting a real
  nuance the candidate had. Take the nuance, then discard.
- **Evidence-free verdict** — any verdict that doesn't name what it compared
  against.

## Routing (one route per verdict)

`add` → the system's creation path. `enhance` → extend the named artifact
with the delta. `included` → discard with the covering reference recorded.
`superseded` → discard after harvesting nuance. One route each — never offer
two.

## Reference

`ktw-methods:/reference/value-fit/taxonomy.md` — the full verdict table with
decision rules, the value-test worked through, the evidence format, and a
worked triage of three candidates (one of each non-discard verdict).

Consumers instantiate this doctrine for their catalog — e.g.
`ktw-compose:/skill/asset-intake` runs it against the live katyoti catalog
and routes the verdicts.
