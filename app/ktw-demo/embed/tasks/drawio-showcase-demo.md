---
name: drawio-showcase-demo
description: Showcase of the drawio engine — pick a type of drawing (ArchiMate, C4, BPMN, AWS, UML), get a real diagram of a topic drawn from its templates, its quality score, then a refinement drawn over it without moving what was already there
type: task
iconName: vector-triangle
icon: "📐"
hashtags: [demo, drawio, diagram, archimate, c4, bpmn, aws, uml]
category: demo
config:
  capability: smart
  plugins:
    - standard
    - diagram-emit
    - diagram-read
  # The notation, kinds and attributes of every viewpoint — preloaded.
  skills:
    - "ktw-agents:/skill/drawio-architecture"
---

# drawio showcase — one type of drawing, drawn by the engine

You demonstrate the drawio engine. You never write draw.io XML: you describe
the diagram as a **logical diagram** (typed nodes, edges, zones) and
`diagram-emit` draws it with the viewpoint's notation, templates and layout.

## Step 1 — Read your input

Read `viewpoint`, `topic` and `refine` from the `FORM_INPUT` block (or from
the message text when run from the CLI). Do NOT use `ask-user`. Defaults:
viewpoint `archimate3-layered`, topic "online grocery ordering", refine `true`.

The viewpoint URI is `ktw-docs:/viewpoint/<viewpoint>`.

## Step 2 — Model the topic in that notation

The skill `drawio-architecture` is loaded: it lists every viewpoint's kinds
and relationships, and the attributes the templates read. Build a logical
diagram that is a genuine model of the topic — real names, no placeholders —
sized to read on one screen:

| viewpoint | build |
|---|---|
| `archimate3-*` | 10–16 elements across the layers the viewpoint shows; the relationships the metamodel allows (actor `assignment` role `assignment` process; process `realization` service; application service `serving` process; component `realization` application service; node `serving` component; data object `realization` business object; requirement `realization` goal). Motivation / strategy / implementation viewpoints: use their own kinds. |
| `c4-context` | a person, the system, 3–4 external systems; `uses` with a label and `attributes.technology`. |
| `c4-container` / `c4-component` | one zone as the boundary (`"label": "<System> [Software System]"` or `"<Container> [Container]"`), 4–6 containers or components inside with `attributes.technology` and `attributes.description`, people and external systems outside. |
| `bpmn2-process` | two pools (zones) — one with two lanes (zones with `"parent": "<pool id>"`) — a start event, 5–8 tasks of the right kinds, an exclusive gateway with labelled `yes` / `no` flows, end events, one message flow between the pools, a data store with an `association`. |
| `aws-architecture` | zones with `kind`: `AWSCloud` › `Region` › `VPC` › `PublicSubnet` / `PrivateSubnet` (nested with `parent`); users outside; 8–12 services (CloudFront, a load balancer in the public subnet, compute and databases in the private subnet, a queue and a function in the region); `connects` and `async` edges. |
| `uml2-class` / `uml2-overview` | 5–8 classes with `attributes.attributes` and `attributes.operations` (lists of strings), an interface, generalization, composition, association, realization. |

Give it a `title` naming the topic and the drawing.

## Step 3 — Draw it and open it

1. Call `diagram-emit` with `{"diagram": <the logical diagram>}`. Write the
   diagram in reading order — `viewpoint` and `title` first, then zones,
   nodes, edges: the GUI opens a draw.io tab as soon as the first node is
   written and draws the diagram as it grows, then lays it out when the call
   is complete. Do not open it again yourself.
2. Call it once. Only if it returns `errors`, fix the model (the error names
   the allowed relationships or kinds) and call it again. Read the `warnings`.
3. Keep the `quality` it returned.

## Step 4 — Refine it (when `refine` is true)

Show that a refinement keeps what the user has seen:

1. Call `diagram-read` with `{"xml": <the xml of step 3>}` — the drawing read
   back into its model.
2. Change the model the way a user would ask: add 2 elements linked to
   existing ones (in the right zone when there are zones) and rename one
   label. Keep every id.
3. Call `diagram-emit` with `{"diagram": <the changed model>, "previous":
   <the xml of step 3>}`.
   Its tab builds up over the first drawing: nothing already drawn moves.

## Step 5 — Complete

Call `complete` with a short report:
- the viewpoint and what the diagram shows (N elements, M relationships, the zones);
- the quality of the first drawing: overlaps (must be 0), crossings, bends;
- when refined: what was added and renamed, and that nothing already drawn moved;
- one line on what to try next: edit the diagram in the tab, save it, and
  `ktw-docs drawio layout <file>` / `ktw-docs drawio score <file>` on it.
