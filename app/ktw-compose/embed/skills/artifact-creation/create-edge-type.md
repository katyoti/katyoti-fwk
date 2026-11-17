---
name: create-edge-type
description: "Author a CWM edge type: the YAML for a typed relationship between objects (from/to, capabilities, cardinality, storage, inverse). Load to add a new kind of edge to the workspace graph."
advertised: false
hashtags: [cwm, edge-type, authoring, create-items]
category: guide
---

# Creating a CWM edge type

An edge type is ONE YAML file:
`app/<sidecar>/embed/cwm/edge-types/<name>.yaml` (kebab name = file name). It
declares a **typed relationship** that lives ON TOP of the containment tree —
the tree is `parent_id`; edge types are the graph orthogonal to it ("this
feature blocks that one", "this test-case verifies that use-case"). The
kit's default pack reads the sidecar's `embed/cwm` folder and contributes it at
boot, with no option; a sidecar that never shipped CWM assets adds `embed/cwm`
to its `//go:embed` line once. The reference example is ktw-demo's
`edge-types/contests.yaml`.

**Extend before you create.** Scan the existing edge types across sidecars first
— `dependencies`, `derived-from`, `supersedes`, `parent_id` (ktw-agents),
`verifies`, `impacts`, `addresses`, `serves`, `challenges`, `deferred-by` (ktw-methods),
`closes` (ktw-code). A near-duplicate relationship should reuse one of these, not
mint a sibling. Minting a domain edge when the methods common vocabulary already
has one is the anti-explosion smell.

## The fields

```yaml
name: verifies            # kebab, unique across the universe; == file name stem
label: Verifies           # the link as a person reads it from its source; empty = the name
inverse_label: Verified by  # ... and from its target; empty = the inverse name
from: test-case           # the SOURCE endpoint: an object TYPE, or `any`
to: use-case              # the TARGET endpoint (same options)
from_capabilities: []     # capabilities the source object must carry; when set, THE constraint
to_capabilities: []       # capabilities the target object must carry
cardinality: N:1          # 1:1 | 1:N | N:1 | M:N — how many on each side; empty = M:N
target: object            # what the right side is: object (default) | path | uri
object_storage: edges     # where the edge is stored: edges (default) | field
inverse_name: verified-by # the reverse-direction label (from `to`'s perspective)
description: |
  One paragraph: what the edge MEANS, when it is written, and what it
  DRIVES (projections, gates, lint signals). State the invariant if there
  is one (e.g. "cycles forbidden; validated on write").
```

### `from` / `to` and `from_capabilities` / `to_capabilities`
Name a concrete object TYPE (`test-case`, `module`, `feature`) to document the
edge's kinds, or `any` when it spans every type. `from_capabilities` /
`to_capabilities` supersede them as the enforced constraint: an edge whose
source (or target, when it is in the index) lacks one of them is refused on
write. Constrain by capability when the edge belongs to a capability, not a
type (ktw-demo's `contests`: both ends carry `signoff`).

### `cardinality`
`1:1`, `1:N`, `N:1`, `M:N` — read as `from`:`to`. `verifies` is `N:1` (many
test-cases verify one use-case); `dependencies` is `M:N` (a DAG). Any other
value is refused: the edge type is not registered. An `N:1` or `1:1` kind is
carried at most once by a source object.

### `target`, `object_storage` and `field`
`target` says what the right side is: an object id (`object`, the default), a
file of the project (`path`) or a resource or tool URI (`uri`). `object_storage`
says where the edge is stored, once: `edges` (an entry in the source object's
`edges:` list, the default) or `field` (a property on one side, named by
`field`, e.g. `left.parent`) — a kind stored as a field is refused in an
`edges:` list.

### `inverse_name`
The label for traversing the edge backwards (`verifies` → `verified-by`,
`dependencies` → `dependents`). Always name it — the GUI and queries surface both
directions.

### What reads it
The pane's Links list shows every incident edge from this object's side. A
panel on the TARGET of an edge filters on it — `filter: {incoming:
[contests]}` — answered from CWM's index, never by a graph read in a `when`
(`ktw-compose:/skill/create-object-panel`). `label`, `inverse_label` and
`description` are translated in a `<name>.i18n.yaml` beside the file
(`ktw-compose:/skill/translate-contributions`).

## Validate

- Rebuild the sidecar and boot it — type-asset YAML errors fail loudly at load.
  When the sidecar source is in this workspace, iterate on the overlay
  (`.katyoti/overwrites/<sidecar>/embed/cwm/edge-types/<name>.yaml`), served live
  (gated by `allow_overwrites`), and `promote` when validated —
  `ktw-compose:/skill/overlay-iteration`.
- Both `from` and `to` name types that exist; an edge of the type creates between
  two objects of those kinds with `cwm-edge-create`.
