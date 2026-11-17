---
name: cwm-search
description: >-
  How to find objects in CWM: substring search over ids, names, titles and
  tags, and list filters. Use before reading or creating an object.
iconName: search
icon: "🔍"
hashtags: [cwm, search, objects]
category: cwm
advertised: true
---

# Searching CWM

Search is over object METADATA; there is no semantic (vector) search.

| Tool | Searches | When to use |
|------|----------|-------------|
| `cwm-object-search` | Object METADATA — id, name, title, description, tags — across modules, filtered by type, module or capability. Not bodies. | "Which objects are about X?" — words you expect in a title, a description or a tag. |
| `cwm-object-list` | Objects matching filters (type, module, parent, ...) | "Every object of this type / in this module / under this parent." |
| `object-effective` | The objects of one type in force at one object (own, inherited, type-injected) | "Every rule that applies here." |

## Inputs

`cwm-object-search` takes `query` and optional `types`, `modules`, `capabilities`, `limit`.

## Phrasing

The match is on substrings, NOT meaning. Use the words the answer's title or tags would carry:

- ✓ "rate-limit", "embedding"
- ✗ "decision about which embedding model to use for vector search"

Short, distinctive words match best; narrow with `types` / `modules` rather than a longer query. When the answer lives in a body, find its object by metadata, then read it with `cwm-object-get`.

## Combining with a list

```
# Option A: search by words, get matching objects back
results = cwm-object-search(query="rate-limit", types=["decision"])

# Option B: enumerate by filters, pick in-process
objects = cwm-object-list(type="decision", module="api")
```

Option A is cheap when you know a distinctive word. Option B is exhaustive — use when the agent needs ALL of them, not just the matches.

## What to read

- [cwm-graph-navigation](cwm-graph-navigation.md) — how to navigate to the right object to scope the search.
- [cwm-records-not-docs](cwm-records-not-docs.md) — why titles and summaries are short and descriptive (so a search finds them).
