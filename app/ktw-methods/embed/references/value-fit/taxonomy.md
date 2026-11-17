# Value-fit — verdict taxonomy, value test, evidence, worked triage

The detailed contract behind `ktw-methods:/skill/value-fit`. The skill body
is the procedure; this is the reference you consult when a verdict is close.

## Verdict decision table

| Nearest existing artifact? | Candidate has a probe it lacks? | Existing is better on that probe? | Verdict |
|---|---|---|---|
| none | yes (real, testable) | — | **add** |
| none | no (no nameable probe) | — | discard as fluff (record: not additive) |
| yes | yes | — | **enhance** (delta = that probe's content) |
| yes | no — existing already satisfies it, equal | — | **included** |
| yes | no — existing satisfies it, better | yes | **superseded** |

"Probe" = a thing a user/agent can do, or do better, that is observable.

## The value test, worked

> Candidate: a doc "How to write good git commits for this repo."
> Nearest existing: `<provider>:/skill/commit-message-style` (suppose it
> exists and covers subject/body/task-refs).
>
> Probe attempt: "After adding this, the agent can …?" — it can write a good
> commit. But the existing skill already does that. Is there a probe the doc
> adds? It explains the *reasoning* nicely — but reasoning prose is not a
> probe; the observable outcome (a good commit) already passes.
> → **included**. Evidence: `<provider>:/skill/commit-message-style` already
> covers subject/body/task-refs; the doc adds no failing-case it fixes.
>
> Now suppose the doc ALSO specifies how to reference a CWM node id in the
> trailer, which the existing skill omits. Probe: "the agent can write a
> commit that links to a CWM node" — currently fails. → **enhance**.
> Evidence: existing skill lacks the node-trailer case; delta = that one
> section.

## Evidence format (required per verdict)

```
candidate: <name / source>
verdict: add | enhance | included | superseded
compared_against: <existing URI/path>   # or "none — no existing artifact covers this"
probe: <the testable value>             # add: what becomes possible; enhance: the missing case
delta: <what to add>                    # enhance only: the specific content to merge
nuance_harvested: <…>                   # superseded only: the one thing worth keeping
```

A verdict missing `compared_against` is not a verdict — rescan.

## Worked triage — three candidates

System surface (pretend catalog): `doc-create`, `commit-message-style`,
`mermaid-syntax`.

1. **Candidate: "PlantUML diagram cheatsheet."**
   compared_against: `mermaid-syntax` (nearest — both are diagram syntax) →
   but it covers a DIFFERENT engine the system has no skill for. Probe: "the
   agent can author a PlantUML sequence diagram" — currently fails (only
   mermaid exists). → **add**. probe: PlantUML authoring becomes possible.

2. **Candidate: "Markdown tips for docs."**
   compared_against: `doc-create` (covers markdown authoring). Probe: any new
   observable? It restates what doc-create already does. → **included**.
   compared_against recorded; no failing case.

3. **Candidate: "Mermaid sequence-diagram guide, with the `autonumber` and
   `par` blocks."**
   compared_against: `mermaid-syntax`. Existing covers basic mermaid but NOT
   `autonumber`/`par`. Probe: "author a mermaid sequence with autonumbered
   parallel blocks" — currently underspecified. → **enhance**. delta: the
   autonumber + par sections appended to `mermaid-syntax`.

Note: zero ADD-the-duplicate, zero cosmetic-enhance. Each verdict names what
it compared against.

## Why this is just ISA + extend-vs-create, composed

- The **value test** is ISA's hard-to-vary rule pointed at value: a claim of
  worth that names no probe constrains nothing.
- The **add/enhance** split IS the creation path's "extend vs create."
- value-fit's contribution is the two DISCARD verdicts (included, superseded)
  + the evidence discipline that makes a pile of "nice things" triageable
  instead of hoarded.
