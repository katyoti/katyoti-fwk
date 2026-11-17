---
name: cwm-knowledge-capture
description: >-
  Decision tree for where an agent writes what it produces: typed
  objects and module pages (durable), an object's working memory, or
  messages to other agents. Use before recording anything.
iconName: file-pen
icon: "📝"
hashtags: [cwm, objects, memory, capture]
category: cwm
advertised: true
---

# Where Does This Knowledge Go?

Every piece of information an agent produces fits one of a few shapes. Picking the wrong shape compounds: records get oversized, the `working` memory file turns into an ad-hoc log, agents can't find each other's writes.

## Decision tree

```
Is this an OUTCOME that future agents will refer to?
├── Yes → TYPED OBJECT (a child object, budget-enforced)
│   ├── A reusable rule? → type=rule (kind constraint)
│   ├── A reusable how-to? → type=rule (kind recipe)
│   ├── A choice, open or made? → type=decision (through the decisions service)
│   ├── A doubt about an object? → type=decision with a `challenges` edge to it
│   ├── A direction never finished? → type=goal (state idea until a person puts it forward)
│   ├── Something a test declares done? → type=feature (state idea until put forward)
│   ├── A success criterion? → type=acceptance-criterion (under the feature or its use-case)
│   ├── A fact learned while working? → a review `finding`, or the module's page
│   ├── A piece of work? → a request in its module (`ktw-agents requests file`)
│   └── An external doc reference? → type=source-document
│
└── No, this is LIVE process state
    ├── Multi-agent coordination? → MESSAGES (send-message / check-messages)
    └── Where the work stands, what is next? → the `working` memory file (replaced whole)

What was LEARNED is an outcome, not process state: a review `finding` or the module's page.
```

## TYPED OBJECTS — durable, inheritable

Create a child object of the record's type under the object it is about:

```json
cwm-object-create {
  "module": "<module>",
  "object": {
    "type": "rule",
    "capabilities": {
      "container": { "parent": "<module>.<parent-local>" },
      "content": { "title": "…", "body": { … } }
    }
  }
}
```

Omit `id` to have one minted. The body must validate against the type's schema. Budget is enforced — see [cwm-records-not-docs](cwm-records-not-docs.md).

Typed objects participate in the inheritance algebra: a `rule` under the project's root object is in force at every object below it. Records are versioned via `supersedes`: a new rule that replaces an old one carries a `supersedes` edge to it (a decision is superseded through the decisions service); the old one is kept (audit trail) but the new one wins in `object-effective`.

## MEMORY — the `working` file

Every object carries the `memory` capability and its ONE named file, `working`: where the work on the object stands — what is underway, what is next — so the next session resumes where this one left off. A chat started on an object already receives it in its `<cwm_object_context>` block as *Working memory* (`(empty)` when nothing is written: there is nothing to look up). Read another object's with `cwm-object-named-file-read(id, capability="memory", name="working")`; write with `cwm-object-named-file-write(id, capability="memory", name="working", content)` — every write REPLACES the file.

**Anti-pattern**: using memory as a journal of learnings. What was learned goes to a `finding` or the module's page, where the next agent finds it by walking the tree. Nor is memory a history. What HAPPENED to an object is its journal's, not memory. The reasons for a choice belong in a `decision`.

## COORDINATION — messages

Agents working together coordinate through the orchestration tools: `send-message` to a teammate or the team, `check-messages` to read what arrived; progress is tracked in the task ledger.

## Typed objects vs memory: the rule of thumb

If a future agent should be able to FIND this knowledge by walking the tree → TYPED OBJECT. If it's the state of the work in progress, meaningless after the task → the `working` memory file. When in doubt, prefer a typed object — the budget enforces compactness; if your record feels like a log, it is a journal entry, not memory.

## What to read

- [cwm-records-not-docs](cwm-records-not-docs.md) — budget arithmetic + how to compact.
- [cwm-onboarding](cwm-onboarding.md) — the three typed concepts.
