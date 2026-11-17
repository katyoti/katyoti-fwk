---
name: cwm-graph-navigation
description: >-
  Concrete tool patterns for navigating the CWM object tree. Covers
  cwm-object-tree / get / list / search / resolve, object-effective, and
  the difference between local and inherited views. Read after cwm-onboarding.
iconName: route
icon: "🗺️"
hashtags: [cwm, navigation, tools, graph]
category: cwm
advertised: true
---

# Navigating the CWM Graph

Three families of tools, each answering a different question.

## "Where am I?" — browsing objects

| Tool | Use case |
|------|----------|
| `cwm-object-tree` | The tree under a root (a module, the project, any object), as nested summaries. Use `depth` for a budget. |
| `cwm-object-list` | Objects filtered by `module`, `type`, `parent` and/or an effective `capability`. |
| `cwm-object-get` | One object by `module` + `id`: its type, capabilities (field groups) and edges. |
| `cwm-object-search` | Substring match on id, name, title, description and tags — metadata only, never bodies. |
| `cwm-object-resolve` | A full name (project / module / object) to the object it names. |

**Pattern**: start with `cwm-object-tree` for orientation, then drill down with `cwm-object-get` once a target is known.

## "What applies here?" — objects in force

This is the most common query shape. Given a focused object, which rules are in force at it?

`object-effective(id, type)` returns the resolved set for ONE object type: the object's own children of that type, what the type's propagation reaches down the tree, and what its type injects, reduced by the type's resolution strategy:

```
object-effective(id="proj-x.root", type="rule")
→ [rule under proj-x.root, rule from an ancestor, …]
```

Set `include_by: true` to get the diagnostic grouping (local / tree / type) — useful when an agent needs to know WHERE a rule comes from to decide whether to override it locally.

A rule with `scope: repo` reaches every module wherever it sits, so it is not in this tree walk; the briefing below tells it, with every other constraint in force.

## "Give me the full briefing" — object context

A chat started on an object already receives its agent context in the `<cwm_object_context>` block: the identity card, the sections its project, the rules in force, its extension, its capabilities and its type contribute, and its working memory (`(empty)` while nothing is written). No call is needed to warm up on it.

For another object: `cwm-object-get(id)` returns the object, `cwm-object-tree` what is under it, and `cwm-object-named-file-read(id, capability="memory", name="working")` its working memory.

## Local vs inherited

| You want | Call |
|----------|------|
| Just the rules written AT this object | `cwm-object-list(parent=<id>, type="rule")`, or `object-effective` with `include_by: true` and read the `local` bucket |
| Everything that applies INCLUDING inherited | `object-effective` |
| Search across the whole workspace | `cwm-object-search` (ids, names, titles, descriptions, tags; see [cwm-search](cwm-search.md)) |

## Name-vs-id resolution

`cwm-object-get` takes `module` + `id`, where the id is module-qualified (`<module>.<local>`). When all you have is a full name (`<project-uuid>.<module>.<local>`) or a module-qualified id without its module, `cwm-object-resolve(ref)` returns the object's id, module and file.

## Write paths

This skill covers READS. Write tools (`cwm-object-create` / `-update` / `-move` / `-delete`) follow the same shape but enforce the parent type's allowed children (`cwm-object-allowed-children` lists them). See [cwm-knowledge-capture](cwm-knowledge-capture.md) for choosing where to write.
