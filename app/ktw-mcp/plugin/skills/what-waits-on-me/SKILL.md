---
name: what-waits-on-me
description: "List what waits on the person - their decisions, approvals, tests by hand, questions and blocked work - with each item's context and the answers it accepts, then discuss one. Use when the person asks what waits on them, what you expect from them, which decisions are theirs, or what they have to do."
---

# What waits on me

Everything waiting on the person is one queue, « À faire »: read it with the
`ktw-agents` tool of the KaTyoti MCP server.

```
ktw-agents(args: ["views", "person-queue", "--out", "json"])
```

Add `"--target", "<module or object id>"` to keep one module's items, or one
request's or decision's. The answer is `items` (in the order to work them),
`by_action` and `count`.

## Each item

| Field | What it is |
|---|---|
| `key` | the item's stable id: name it when the person picks one |
| `action` | the one thing it asks (below) |
| `object`, `request`, `module` | what it is about: a request, a decision or an Inbox entry |
| `title`, `asks` | what it is, and what is expected in one line |
| `proven`, `done` | what is already proven; what the work before a validation did |
| `options`, `proposal`, `proposal_why`, `challenged` | a decision's options, the one proposed and why, the points raised against it |
| `by_hand` | a hand test's exact steps, and why no agent can do it |
| `waits_on` | why it cannot be done yet (a validation waits on its hand tests); empty: it can |
| `held` | the person put it on hold: shown last, not counted |

## What each action accepts

| `action` | The person answers |
|---|---|
| `decide` | one of `options`, with why (`proposal` is the one proposed) |
| `test` | *passes* (with what they saw), or *fails* (with how) |
| `validate` | *approve*, or *send back* with what to change - once `waits_on` is empty |
| `answer` | the answer, as text |
| `unblock` | *try again* once the cause is fixed, *resume* it, or *drop* it |
| `notice` | nothing: for information only |

## How to present it

1. Give the count by action, then the items in the queue's order: one line each
   - its `key`, its title, what it asks; `waits_on` when it is set.
2. When the person picks one, show its context: the options and the proposal
   with why and the challenges (a decision), the steps (a hand test), what was
   done and what is proven (a validation), and the answers it accepts.
3. Discuss it as long as the person wants: read the objects it names with the
   same tool (`requests get --id <request>`, `cwm object-get --id <object>`).

The answer is the person's own act. Once they said what they answer, give it
through its act tool - `decide`, `answer-inbox`, `dismiss-inbox`, `report-todo`,
`test-by-hand`, `verify-by-hand`, `hold-item`, `resume-item`,
`waive-module-update` - with exactly what they said: Claude Code asks them to
allow it, and the act is recorded as relayed. Refused because relaying is off,
tell them they turn it on with `katyoti profile relay on`, or answer in the Inbox
or the request view. Never choose an answer for them, and never pass one of
these acts through the plain `ktw-agents` tool.
