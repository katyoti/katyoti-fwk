# `edge_types/` — CWM's generic edge types

Each `*.yaml` file here declares one edge type, published as the
`ktw-agents:/edge-type/<name>` contribution (schema
`system:/data-type/edge-type`). CWM ships only **method-neutral** edges;
a method pack ships its own edge types in its own `cwm/edge_types/`.

## Anatomy of an edge file

```yaml
name: <edge-name>                 # unique identifier
from: <source-kind-or-type>       # documentation when from_capabilities is set
to: <target-kind-or-type>
from_capabilities: [<cap>]        # capabilities the source must carry
to_capabilities: [<cap>]          # capabilities the target must carry
cardinality: M:N                  # 1:1 | 1:N | N:1 | M:N (source:target); empty = M:N
target: object                    # object | path | uri
object_storage: edges             # edges (the source's edges list) | field (a property on one side)
field: left.parent                # for object_storage field only
inverse_name: <name>              # the kind read from the target's side
description: |                    # one paragraph
```

A cardinality outside `1:1`, `1:N`, `N:1`, `M:N` is refused: the edge type
is not registered and the refusal is logged. An `N:1` or `1:1` kind is carried
at most once by a source object.

## Shipped edges

- [parent.yaml](parent.yaml) — the containment tree, stored as the child's `parent` field
- [parent_id.yaml](parent_id.yaml) — the structural tree edge (node side)
- [dependencies.yaml](dependencies.yaml) — DAG dependencies between nodes
- [supersedes.yaml](supersedes.yaml) — replacement chains within a kind
