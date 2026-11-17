---
name: decision-capture
description: >-
  Capture a choice, an open question or a challenge as a `decision` object.
  Use when a choice is made, a question must be parked, or an object is questioned.
icon: "🧭"
iconName: signpost
hashtags: [decision, capture, rationale, tradeoff, challenge]
category: lifecycle
---

# Decision capture

A decision worth making is worth recording — with its **reasoning**, so a later
reader (or agent) doesn't re-litigate it. Every choice is one object type,
`decision` (ids `OD-NNN`, filed in the module's `decisions/`), on the `decision`
workflow: `open` → `applies` (decided, in force) → `superseded` (replaced by a
later decision) or `retired` (no longer in force); `open` → `dropped` (closed
without a choice). Its state is the `decision` field-group (`options`,
`selected`, `raised`, `resolved`, `proposal`, `challenge`); its reasoning is
the prose body (`## Question`, `## Options`, `## Decision`). **Options are
short labels** (a few words, at most 80 characters, in your language): a label
is what a button and `selected` name; its explanation is its `### <label>`
section under `## Options`. Act through the decisions service
(`ktw-methods:decisions/<Verb>`), never by writing the file. The person sees
a decision in its **Decision** tab: the proposal, its challenge and one card
per option with a button choosing it.

## 1. A decision already taken — what + why

A settled choice: `ktw-methods:decisions/Record` with its question, options,
the selected one, its `raised` (and `resolved`) dates and the `## Decision`
prose; it lands directly in `applies`. Always carry the **why** — a decision
without rationale is a note, and gets reversed by the next person who hits the
same fork.

```
## Decision
<what was chosen> — <why; the alternative + the reason it lost>
❌ DEAD END: tried <X>, failed because <Y> (don't retry)
```

Dead ends are decisions too — recording "don't retry X because Y" saves the
exact rework it describes.

## 2. An open decision — a parked question that blocks

A choice you can't settle yet but must not lose: `ktw-methods:decisions/Open`
(the question is its title, context under `## Question`), then
`decisions/ProposeOption` per candidate answer (+ the tradeoff of each, in
prose). Its id `OD-NNN` is stable — never renumbered (references would break).

A use-case or deliverable blocked on it is **deferred against it** (a
`deferred-by` edge to the decision), not silently dropped — when the decision
is taken, the gate (`ktw-methods:/skill/completeness-gate`) can flip the
deferred leaf.

## 2b. Prepare an open decision — options a person can choose from

To make an open decision ready for the person, `ktw-methods:decisions/Prepare`
with `{id, options: [{label, means, pros[], cons[], cost, consequence}],
proposal: {option, why}, actor}`:

- **label**: short (*Not listed*, *Listed, made useful*), never a sentence;
- **means**: what choosing it means, in one or two lines;
- **pros / cons**: one line each;
- **cost**: what doing it costs; **consequence**: what follows if chosen;
- **proposal**: the option you recommend (one of the labels) and why, in a
  few lines.

Prepare replaces the options and their sections whole: prepare again to
change them. The long analysis (a table, the evidence) stays prose under
`## Question`, never in a label.

## 2c. Challenge a proposal

A reviewer who disputes a prepared proposal records its points with
`ktw-methods:decisions/Challenge` `{id, actor, points[]}` (one line each); the
Decision tab shows them under *Proposition contestée*. A later challenge
replaces the earlier one. The preparer answers by preparing again; the person
judges whether the points are answered when choosing. (To dispute a decision
that already applies, open a new decision against it: shape 3.)

## 3. A challenge — a decision that questions an object

To dispute an object (a rule, a feature, another decision), open a decision
whose question is the doubt and give it a `challenges` edge to the object it
questions. The target shows it in its Challenged by tab
(`ktw-methods:challenges/Challengers`).

## Procedure

1. **Settled?** → `Record` (shape 1). **Unsettled but consequential?** →
   `Open` (shape 2). **Questioning an object?** → `Open` + a `challenges` edge
   (shape 3). **Neither (just a note)?** → the node journal, not here.
2. **Write the why.** For a decision taken: the alternative and why it lost.
   For an open one: the options and each option's tradeoff. No why → it's not
   captured, it's just stated.
3. **On deciding an open one:** `decisions/Resolve` with the selected option
   (it sets `selected` and `resolved`, appends `## Decision`, and moves it
   `open → applies`); then re-check any leaf deferred against it. A decision
   a person must take is theirs: from an agent's session, a Resolve naming a
   person is refused — prepare it and let them choose in the Decision tab. A
   decision decides once: a second Resolve answers who decided and when.
4. **When a later decision replaces it:** `decisions/Supersede` from the newer
   one (a `supersedes` edge; the older moves `applies → superseded`). No longer
   in force without a replacement → transition `retire`; an open question
   closed without a choice → transition `drop` (`ObjectTransition`).

## Anti-criteria

- **Anti:** a decision recorded without its rationale (a bare "we chose X").
- **Anti:** an option label that is a sentence: the explanation goes in its
  section (Prepare's `means`), the label stays a few words.
- **Anti:** a decision renumbered on edit, breaking a deferral reference.
- **Anti:** a blocked item dropped instead of deferred against an `OD-NNN`.
- **Anti:** a decision edited in place to reverse it — a new decision supersedes it.
