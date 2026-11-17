---
name: discover-wants
description: >-
  Find what existing software does and why, and propose it as the module's
  wants: goals, features, use cases — each saying where it was found, linked,
  never accepted on a person's behalf. Use for a discover request's propose step.
icon: "🔎"
iconName: search
hashtags: [discover, reverse-engineering, goals, features, use-cases]
category: lifecycle
---

# Discover wants — existing software, as its goals, features and use cases

Software that exists already does things for someone, for a reason. This skill turns
what a survey found into the module's **wants**: the **goals** it serves (why), the
**features** that serve them (what it does), the **use cases** each feature wants (how a
person uses it). A person reviews every proposal; nothing is accepted for them.

## 1 — Start from what exists

Before proposing anything:
- `cwm-object-list(module=<m>, type=goal)`, `type=feature`, `type=use-case` — the module's
  wants already there; `cwm-object-search` for names close to what you found.
- A want already there is **linked**, never proposed again. One that is close but not the
  same is proposed, its body naming the one it is close to.

## 2 — Propose what the software shows

From the survey (the to-do before yours: its result is in its packet's request trail, or
`todos-list` the request):

- **Features** — one per thing a person can do with the software, named by the outcome
  ("a task is marked done"), not the code ("TaskService.update").
- **Use cases** — for each feature, the ways it is used; where a test or a command shows
  it works, say so: it becomes an acceptance criterion at specification.
- **Goals** — why the features exist: the lasting outcome for the people using it. Few;
  a goal no feature serves is not one this software shows.

Create each with `cwm-object-create` in the module (`parent`: the module, or the feature a
use case belongs to). Its **body** says what it is in one or two sentences, then:

```
Found in: <files, symbols, docs, tests — with paths>
```

It enters in its first state (an idea): only a person puts it forward.

## 3 — Link them

- `cwm-edge-create({kind: "serves", from: <feature>, to: <goal>})` — each feature, the goal(s) it serves;
- `cwm-edge-create({kind: "wants", from: <feature>, to: <use case>})` — each feature, its use cases.

## 4 — Hand the result back

The result lists what you proposed (ids, by type), what you linked to instead, and what
you saw but could not place (dead code, a goal with no feature) — the person's review
starts from it.

## Rules

- Propose only what the code, its tests or its documents show; never what it should do.
- Nothing is accepted, specified or dropped by you: the person reviews every proposal.
- Change no code and no document while discovering.
