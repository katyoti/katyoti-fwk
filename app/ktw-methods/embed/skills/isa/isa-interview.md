---
name: isa-interview
description: >-
  Adaptive question-and-answer that fills in or deepens an ISA's sections —
  one question at a time via ask-user, write-back after every answer, until
  the intention is hard-to-vary. Workflow of the isa umbrella skill.
iconName: messages
icon: "🎤"
hashtags: [isa, interview, clarification]
category: methodology
advertised: false
---

# ISA — Interview

Deepen an ISA by interviewing the user. Use when the prompt alone does not
carry enough signal to scaffold a strong ISA, when a completeness check
flags thin sections, or when the caller needs a **clear intention** before
work may start.

Conduct rules come from `ktw-methods:/skill/interview-technique` — load it
first and apply it: open before closed, ONE question per **ask-user** call,
summarize before confirming, respect the user's time.

## Inputs

| Input | Required | Description |
|---|---|---|
| isa | yes | The ISA to deepen — a file path or a CWM node ID |
| section | no | Interview only that section; otherwise walk all thin sections |
| max_questions | no | Default 8 — keep the interview tight |

## Procedure

### 1 — Read the ISA and classify sections

Load the ISA (file, or the node body + items for `cwm-items`). Classify
every section: populated / thin / missing (definitions in
`ktw-methods:/reference/isa/format-spec.md`).

### 2 — Build the question queue

Walk sections in the body order, questions only where thin or missing:

| Section | Probe questions |
|---|---|
| **Problem** | "What's broken right now?" / "Who feels it most?" / "What does the broken state cost?" |
| **Vision** | "When this is done, what does the user feel?" / "What would make someone tell a friend?" |
| **Out of Scope** | "What would be tempting to add but distract from the core?" / "Which scope decisions do you want locked so they don't drift?" |
| **Principles** | "What truths must this respect regardless of implementation?" |
| **Constraints** | "What are the immovable mandates?" / "What must nobody ever try?" |
| **Goal** | "In one or two sentences, what is the verifiable done state?" |
| **Criteria** | "Walk me through the test that proves ISC-N." / "Is ISC-N one binary probe, or should it split?" |
| **Test Strategy** | "How would you actually verify ISC-N — what command, tool, or probe?" |
| **Features** | "What units would you split this into? What can run in parallel?" |

Lead with the most foundational unanswered question.

### 3 — Ask, then write back — every time

One **ask-user** call per question. After EVERY answer, immediately write it
into the ISA (Edit the file, or update the node/items for `cwm-items`) —
the user should feel the document filling as they answer. If an answer
contradicts existing content, the new answer wins; log the change in
`## Decisions` with a `refined:` prefix.

If an answer is aspirational rather than concrete, push once: "what would a
test that proves that look like?" If the user cannot articulate it, the
claim is not yet hard-to-vary — flag it in `## Decisions` and move on.

### 4 — Stop conditions

End when ANY of:
- all sections required by the depth are populated to non-thin detail;
- `max_questions` reached;
- the user says that's enough / skip the rest / done;
- two consecutive answers add no signal ("I don't know", "skip").

### 5 — The clear-intention bar

When the caller needs a go/no-go (e.g. a wizard's exit gate), the intention
is clear when ALL of:
- the Goal cannot be reworded without changing the Criteria set;
- every ISC names a probe;
- ≥1 `Anti:` ISC exists.

If any fails after the stop conditions hit, report **intention not yet
clear** with the specific gaps — do not soften the verdict.

### 6 — Final completeness pass

Run `ktw-methods:/skill/isa-check-completeness`. Report remaining gaps to
the caller without blocking — the user can iterate later. If the user
abandoned mid-interview, save what you have and add a `## Decisions` entry:
"interview paused at <section> — N answered, M skipped."
