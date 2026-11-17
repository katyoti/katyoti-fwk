Patch a previously-mounted UI artifact across turns. Use this tool when you
already called `display-ui-component` (with `persistent: true`) and want to
mutate part of the artifact without remounting — the user sees the change
in place, no flicker, no scroll loss.

## When to use this vs display-ui-component

- **display-ui-component**: first time you show the artifact, OR whole-tree
  reshape (different schema, different layout).
- **patch-ui-component**: targeted mutation of one or a few nodes in an
  artifact that's already mounted (e.g. flip a task status, append a row,
  update a KPI value).

If the artifact wasn't mounted with `persistent: true`, the host rejects
your patches. Set `persistent: true` at display time when you intend to
update.

## Input shape

```json
{
  "artifactId": "art-task-list-001",
  "ops": [
    { "op": "replace", "target": "task-3-status", "value": "done" },
    { "op": "append",  "target": "task-list-items", "value": { "id": "t4", "title": "Buy milk", "status": "todo" } }
  ]
}
```

`artifactId` must match a previously-emitted `display-ui-component` id.

`ops[]` is an ordered list; ops apply in sequence. If `ops[0]` adds a new
node and `ops[1]` mutates it by id, the host rebuilds the id→path lookup
map between ops — so you can address newly-added nodes immediately.

## Addressing: target (preferred) vs path (escape hatch)

Two ways to point at the mutation site:

- **`target`** — a node id you set on the DSL node at mount time
  (`{ kind: "kpi", id: "open-count", ... }`). The host walks the artifact
  tree, finds the unique node with matching id, and resolves to a JSON
  Pointer internally. **Prefer this** — LLMs miscount array indices, but
  ids are stable references you picked yourself.

- **`path`** — a raw JSON Pointer (RFC 6901), e.g. `"/items/3/status"` or
  `"/value"`. Use only when there's no natural id (deep array index, or
  whole-tree replacement at `""`).

Set ONE. If both are set, `path` wins (deliberate escape-hatch override).

Unknown target → tool returns an error listing available ids in that
artifact; correct the id and retry.

## 5 op kinds

### replace — idempotent set at path

Sets the addressed location to `value`. The most common op for flipping a
single field (status, label, count). Cheap; the renderer only updates the
affected sub-tree.

```json
{ "op": "replace", "target": "kpi-open-tickets", "value": 38 }
```

### add — RFC 6902 add

Insert a value at an array index, or set a new object key. Use `path` with
a numeric tail for arrays (`/items/0` inserts at head). Use `target` for
named slots that don't yet exist.

```json
{ "op": "add", "path": "/items/0", "value": { "id": "t0", "title": "Pinned: hotfix" } }
```

### remove — delete at path

Deletes the addressed field or array element. No `value` field.

```json
{ "op": "remove", "target": "stale-banner" }
```

### append — push onto array

Convenience for the common "add to end of list" pattern. Equivalent to
`add` at `/items/-` but easier to read. `target` should point at the array
itself (or its containing node id).

```json
{ "op": "append", "target": "task-list-items", "value": { "id": "t9", "title": "Review PR", "status": "todo" } }
```

### merge — RFC 7396 deep merge

Recursive object merge at path (or the artifact root if no path). Keys
with `null` value are deleted. Use for "patch many fields at once on the
same object" without listing each one.

```json
{
  "op": "merge",
  "target": "user-card",
  "value": { "email": "new@example.com", "phone": null, "address": { "city": "Berlin" } }
}
```

## Patterns

### Live task list (the canonical use case)

Mount once with `display-ui-component` (`persistent: true`). Then across
later turns, patch individual rows:

```json
{
  "artifactId": "art-task-list-001",
  "ops": [
    { "op": "replace", "target": "task-1-status", "value": "done" },
    { "op": "replace", "target": "task-2-status", "value": "in_progress" }
  ]
}
```

### Streaming dashboard KPIs

Mount the dashboard with KPI placeholders. As you compute each metric,
patch its value:

```json
{
  "artifactId": "art-dashboard-001",
  "ops": [
    { "op": "replace", "target": "kpi-open",   "value": 42 },
    { "op": "replace", "target": "kpi-closed", "value": 12 },
    { "op": "replace", "target": "kpi-mttr",   "value": "4.2h" }
  ]
}
```

### Append-then-update (intra-call rebuild)

Ops apply sequentially; the id lookup map rebuilds after each op, so you
can reference a newly-added node in the next op:

```json
{
  "artifactId": "art-task-list-001",
  "ops": [
    { "op": "append",  "target": "task-list-items", "value": { "id": "t4", "title": "Draft email", "status": "todo" } },
    { "op": "replace", "target": "t4-status", "value": "in_progress" }
  ]
}
```

(This assumes the appended row carries `id: "t4"` and its status node is
addressable as `t4-status` — define id conventions when you mount.)

## Slide decks

A `kind: "deck"` artifact is markdown, not a tree: address a slide by its
number in the document, from 1 (vertical slides count), and give the slide's
own markdown — its settings line, slots and notes, no `---`:

```json
{ "artifactId": "deck-q4", "ops": [
  { "op": "replace", "target": "slide-3", "value": "<!-- slide: steps=list -->
## Where we are
- The hub owns the catalog" },
  { "op": "add",     "target": "slide-5", "value": "## A new slide before slide 5" },
  { "op": "remove",  "target": "slide-7" },
  { "op": "append",  "target": "slides",  "value": "## Questions?" }
] }
```

Ops apply in order: after a `remove`, the slides after it move up by one. A
patch is not linted — to see findings, display the whole deck again.

## Tips

- Give every node you might patch later an `id` at mount time. Without
  ids you're stuck with raw JSON Pointers, which break the moment the
  tree shape shifts.
- Batch related ops into one tool call. The host applies all ops
  atomically; one update of the artifact reaches the chat; the user sees a
  single coherent update instead of N flickers.
- For whole-artifact reshape (different schema or layout), call
  `display-ui-component` again with the same `artifactId` — that's a
  cleaner remount than a series of patches.
- When you're done updating a persistent artifact, call
  `complete-ui-component` to mark it final (grays out "streaming" chrome
  in the GUI).
