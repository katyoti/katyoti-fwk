---
name: cwm-onboarding
description: >-
  Mental model for the Contexia Workspace Manager (CWM). Read this first
  before using any cwm-* tool. Covers objects, capabilities and edges, the
  object tree, the inheritance algebra, and the records-not-docs invariant
  that constrains every write.
iconName: brain
icon: "🧠"
hashtags: [cwm, model, onboarding, knowledge-graph]
category: cwm
advertised: true
---

# CWM Mental Model

CWM is a typed, multi-process knowledge graph for one workspace. Every agent that operates on the workspace reads and writes through the `cwm-*` tool family. This skill is the conceptual base; the per-tool patterns live in [cwm-graph-navigation](cwm-graph-navigation.md) and [cwm-search](cwm-search.md).

## Three typed concepts

CWM has exactly three typed-data concepts. Know the difference — picking the wrong one is the #1 source of confusion.

### 1. Objects — one model for everything

Everything CWM holds — a project, a module, a goal, a feature, a rule, a decision, a document — is an **object**. Each object has:

- a stable, module-qualified `id` — `<module>.<local>` (e.g. `m.F-001`); a module object's id is the bare module name,
- a `type` (e.g. `rule`, `decision`, `feature`) — the named default bundle of capabilities, which also decides allowed children and the folder the object is filed in,
- `capabilities` — one field group per capability (see below),
- optional `edges` to other objects and named `files`.

Object types are declared by the packs that bring them (ktw-safe, future ktw-book) — no Go code needed unless the type has a custom behavioural invariant. `cwm-object-type-get` reads one; `cwm-object-allowed-children` lists what may be created under an object.

### 2. Capabilities — named field groups

What an object is, and where it appears, comes from the capabilities it carries:

- `container` — its place in the tree: `name`, `description`, `parent` (the parent object's id — a strict tree),
- `content` — a record belonging to another object: `title`, `body` (validated against the type's schema), `status`,
- `document` — a wrapped prose file with thin metadata,
- `memory` — the `working` and `notes` named files; `file-overview` — the `overview` (or `page`) named file.

A record such as a rule or a decision is a **child object of type `rule`/`decision`** whose `parent` is the object it belongs to. `cwm-object-capabilities` lists the capabilities in force on an object and where each comes from.

### 3. Edges — typed relationships

Every relation between two objects other than the tree's parent is an **edge of a declared kind** (`serves`, `realizes`, `depends-on`, `supersedes`, …), stored in the source object's `edges:` list. Read and write them with `cwm-edge-list` / `cwm-edge-create` / `cwm-edge-delete`.

## Inheritance: the "objects in force" algebra

When an agent asks "what rules apply at this project?", CWM walks the tree and gathers all rules whose propagation reaches the focused object. The propagation is declared by the collected type:

- `local-only` — in force only at the owning object.
- `inherit-from-ancestors` — rolls down the tree (e.g. `rule`).
- `local-visible-up` — rolls UP to ancestors.

Use `object-effective(id, type)` to get the resolved set for an object + type. A chat started on an object already receives its full briefing in `<cwm_object_context>` (identity card, sections from its project, rules, capabilities and type, its memory files); `cwm-object-get` and `cwm-object-tree` read another object and what is under it.

## Records-not-docs: the invariant

Every record write is budget-enforced. Default budgets (per storage shape):

- `yaml-inline` records: ~4 KB.
- `ring-buffer` compacts: ~8 KB.
- `yaml-with-body` documents: ~64 KB.

If a write exceeds the budget, the substrate refuses with `cwm: budget exceeded`. **The fix is never to raise the budget** — it is to:

1. Split the content into multiple objects (one question per `decision`, not a long list).
2. Move prose to a `document` type (yaml-with-body) instead of stuffing it into a `rule`.
3. Use a `source-document` type (snapshot + anchor) to reference external prose.

See [cwm-records-not-docs](cwm-records-not-docs.md) for the budget arithmetic.

## Multi-process safety

CWM holds a process-level lockfile at `<root>/.lock`. Multiple binaries can read concurrently; writes serialize through the lock + a strict-order per-object mutex. Agents do not need to think about locking — the substrate handles it. The only visible effect: a concurrent write that loses the CAS race returns `cwm: concurrency conflict`; the agent should re-read + retry.

## Where to go next

- [cwm-graph-navigation](cwm-graph-navigation.md) — concrete tool patterns for browsing the object tree.
- [cwm-knowledge-capture](cwm-knowledge-capture.md) — decision tree for "where does this knowledge go?".
- [cwm-search](cwm-search.md) — phrasing semantic queries.
- [cwm-source-ingestion](cwm-source-ingestion.md) — when you need to anchor external prose.
- The CWM concepts document (`modules/cwm/concepts.md` in the repository) — the long-form design document.
