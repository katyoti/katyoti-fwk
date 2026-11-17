---
name: drawio-architecture
description: >-
  Author editable architecture diagrams (ArchiMate, C4, BPMN, AWS, UML) as drawio
  through the diagram-emit agent tool: pick a viewpoint, write the logical
  diagram, fix what validation reports, iterate over the drawing the user
  kept. Use when the user wants an EDITABLE
  diagram to open in drawio.app or Confluence, not a static image.
category: writing
hashtags: [diagram, drawio, architecture, uml, archimate, viewpoint, editable]
icon: 🏛️
iconName: sitemap
---

# Editable Architecture Diagram (drawio)

**When this skill applies**: the user wants a structured diagram they can keep editing (open in drawio.app, save to Confluence, tweak by hand). Output is editable `.drawio` XML, NOT a static image.

**When it doesn't apply**: the user wants a static image (PNG / SVG / PDF) from existing Mermaid / PlantUML / Graphviz source. Use `doc-create` + the `ktw-docs:docs/diagram` tool instead.

## The tool

Call **`diagram-emit`** with a logical diagram. The diagram names its **viewpoint** — the type of drawing, which brings a notation (the kinds and their rules), a template library (the looks) and a layout profile. The tool validates the diagram against the notation, lints it, lays it out with ELK (unless `no_layout: true`) and draws every element with its kind's template.

```json
{
  "diagram": {
    "viewpoint": "ktw-docs:/viewpoint/<id>",
    "title": "...",
    "nodes": [{"id":"n1","label":"...","kind":"<ElementKind>","attributes":{...},"zone":"z1"}],
    "edges": [{"id":"e1","source":"n1","target":"n2","kind":"<RelationshipKind>","label":"...","attributes":{...}}],
    "zones": [{"id":"z1","label":"...","order":1}],
    "layoutHints": {"preset":"archi-layered"}
  },
  "no_layout": false
}
```

## Step 1 — pick the viewpoint

| Viewpoint | Draws | Element kinds | Relationship kinds |
|---|---|---|---|
| **`ktw-docs:/viewpoint/archimate3-layered`** | ArchiMate, every layer, top to bottom | all 61 ArchiMate 3.2 kinds: strategy (`Capability`, `Resource`, `ValueStream`, `CourseOfAction`), business (`BusinessActor`, `BusinessRole`, `BusinessProcess`, `BusinessFunction`, `BusinessEvent`, `BusinessService`, `BusinessObject`, `Contract`, `Product`, …), application (`ApplicationComponent`, `ApplicationInterface`, `ApplicationService`, `ApplicationProcess`, `DataObject`, …), technology (`TechnologyNode`, `Device`, `SystemSoftware`, `CommunicationNetwork`, `TechnologyService`, `Artifact`, …), physical (`Equipment`, `Facility`, `Material`, …), motivation (`Stakeholder`, `Driver`, `Goal`, `Outcome`, `Principle`, `Requirement`, `Constraint`, …), implementation (`WorkPackage`, `Deliverable`, `Plateau`, `Gap`, …), `Grouping`, `Location`, `Junction` | `composition`, `aggregation`, `assignment`, `realization`, `serving`, `access`, `influence`, `triggering`, `flow`, `specialization`, `association` |
| **`ktw-docs:/viewpoint/archimate3-application-cooperation`** | applications, their interfaces, services and data, left to right | the application kinds | all |
| `ktw-docs:/viewpoint/archimate3-motivation` | stakeholders, drivers, goals, requirements | the motivation kinds | all |
| `ktw-docs:/viewpoint/archimate3-strategy` | capabilities, resources, value streams | the strategy kinds + `Outcome`, `Goal` | all |
| `ktw-docs:/viewpoint/archimate3-technology` | infrastructure and physical | technology + physical kinds | all |
| `ktw-docs:/viewpoint/archimate3-implementation-migration` | work packages, deliverables, plateaus, gaps | implementation kinds | all |
| **`ktw-docs:/viewpoint/c4-context`** | C4 system context | `Person`, `ExternalPerson`, `SoftwareSystem`, `ExternalSoftwareSystem` | `uses` |
| **`ktw-docs:/viewpoint/c4-container`** | C4 containers — a zone is the system boundary | + `Container`, `Database`, `Queue` | `uses` |
| **`ktw-docs:/viewpoint/c4-component`** | C4 components — a zone is the container boundary | + `Component` | `uses` |
| **`ktw-docs:/viewpoint/bpmn2-process`** | BPMN process in pools and lanes, left to right | `Task`, `UserTask`, `ServiceTask`, `SendTask`, `ReceiveTask`, `ScriptTask`, `ManualTask`, `BusinessRuleTask`, `SubProcess`, `CallActivity`; `StartEvent`, `MessageStartEvent`, `TimerStartEvent`, `SignalStartEvent`, `IntermediateMessageCatchEvent`, `IntermediateMessageThrowEvent`, `IntermediateTimerEvent`, `IntermediateSignalEvent`, `ErrorEvent`, `EndEvent`, `MessageEndEvent`, `ErrorEndEvent`, `TerminateEndEvent`; `ExclusiveGateway`, `ParallelGateway`, `InclusiveGateway`, `EventBasedGateway`; `DataObject`, `DataStore` | `sequenceFlow`, `messageFlow`, `association` |
| **`ktw-docs:/viewpoint/aws-architecture`** | AWS services in their groups, left to right | about 50 services (`EC2`, `Lambda`, `ECS`, `EKS`, `S3`, `RDS`, `Aurora`, `DynamoDB`, `ElastiCache`, `CloudFront`, `Route53`, `APIGateway`, `ElasticLoadBalancing`, `SQS`, `SNS`, `EventBridge`, `Kinesis`, `Glue`, `SageMaker`, `CloudWatch`, `IAM`, `Cognito`, `WAF`, …) + `Users`, `Client`, `Mobile`, `Internet`, `Datacenter` | `connects`, `async` |
| **`ktw-docs:/viewpoint/uml2-class`** | UML class diagram | `Class`, `Interface`, `Package` | `generalization`, `realization`, `dependency`, `association`, `aggregation`, `composition`, `usage` |
| **`ktw-docs:/viewpoint/uml2-overview`** | every UML 2.5 kind | + `Component`, `Node`, `Actor`, `UseCase`, `Activity`, `State`, `Lifeline` | same |

Other sidecars can publish their own viewpoints: `ktw-docs drawio viewpoints` lists every one in the universe; `ktw-docs drawio notations <notation URI>` shows a notation's element and relationship kinds.

## Step 2 — choose the modelling pattern

### Multi-kind (ArchiMate, UML)

Each element gets the **kind** that says what it is; the kind's template gives its shape and colour.

```json
{
  "viewpoint": "ktw-docs:/viewpoint/archimate3-layered",
  "nodes": [
    {"id":"customer", "label":"Customer",    "kind":"BusinessActor"},
    {"id":"order",    "label":"Place Order", "kind":"BusinessProcess"},
    {"id":"svc",      "label":"OrderService","kind":"ApplicationService"}
  ],
  "edges": [
    {"id":"e1", "source":"customer", "target":"order", "kind":"triggering"},
    {"id":"e2", "source":"svc",      "target":"order", "kind":"serving"}
  ]
}
```

ArchiMate's relationships are checked against the metamodel: `validate` refuses one the standard never allows and lists the allowed ones. The usual chains: actor `assignment` → role `assignment` → process; process `realization` → service; application service `serving` → business process; component `realization` → application service; artifact `realization` → component; node `serving` → component; data object `realization` → business object; process `access` → object; requirement `realization` → goal. `association` joins anything, and a `Junction` or `Grouping` takes any relationship.

### One kind, many variants (a sidecar's own notation)

A notation a sidecar publishes may have few kinds, its look driven by `attributes` through the template library's bindings. Don't invent kinds — the colour difference IS an attribute. For an application-flux notation of that pattern (its viewpoint named here `<sidecar>:/viewpoint/app-flux`):

```json
{
  "viewpoint": "<sidecar>:/viewpoint/app-flux",
  "nodes": [
    {"id":"billing", "label":"Billing", "kind":"Application",
     "attributes":{"domain":"reference", "markers":["xls"], "description":"Invoices and prices",
                   "external_url":"https://catalog.example.com/apps/billing"}},
    {"id":"ledger",  "label":"Ledger",  "kind":"Application",
     "attributes":{"domain":"invoicing", "description":"General ledger"}}
  ],
  "edges": [
    {"id":"f1", "source":"billing", "target":"ledger", "kind":"Flow",
     "attributes":{"flow_type":"catalog", "interface_codes":[{"code":"IF-7349", "url":"https://catalog.example.com/interfaces/7349"}], "data_types":["Item","Price"]}}
  ]
}
```

The library's bindings say which attribute drives what (a domain → a palette's fill and border, a marker → a badge, a flow type → a colour): `ktw-docs drawio notations <notation URI>` shows the kinds, the library's YAML its bindings.

## Step 3 — the attributes a template reads

- **A one-kind notation**: the attributes its library's bindings name (above) — read them in the library before writing the diagram.
- **UML `Class`**: `attributes: []string`, `operations: []string` → one per line between separators; the box grows to fit.
- **ArchiMate**: usually no attributes — the kind is the discriminator.
- **C4**: `technology` and `description` on every element (the box reads name, `[Container: technology]`, description); `technology` on a `uses` edge (read under its label). A boundary is a zone: in `c4-container` the software system around its containers, in `c4-component` the container around its components.

A template reads any scalar attribute as `%name%`; how a library's templates and bindings are written: `ktw-docs:/reference/drawio-authoring.md`.

## Step 4 — pick a layout (optional)

The viewpoint brings a layout profile; override it field by field with `layoutHints`. Presets built into ktw-docs:

| Preset | Description |
|---|---|
| **`archi-layered`** | layered, left to right, with zone partitioning — application landscapes, flows, "which systems talk to which" |
| `layered-down` / `layered-right` | hierarchical top-down / left-right |
| `force`, `stress` | force-directed, for graphs with no main direction |
| `radial`, `mrtree` | trees (no cycles) |
| `rectpacking` | boxes packed, no edges to honour |

**Swimlanes**: declare `zones[]` (stable `id`, `label`, `order`) and set each node's `zone` (or list a zone's `members`). A zone can sit inside another (`parent`): in BPMN a pool is a zone and each lane a zone whose `parent` is the pool; the bpmn2-process viewpoint draws zones as lanes (`layoutHints.lanes`), bands every flow crosses. A zone's `kind` picks its look: in aws-architecture a zone of kind `AWSCloud`, `Region`, `AvailabilityZone`, `VPC`, `PublicSubnet`, `PrivateSubnet`, `SecurityGroup` or `AutoScalingGroup` is that AWS group, nested with `parent` (Region › VPC › subnet). Fine-tune with `layoutHints.elkOptions: {"elk.spacing.nodeNode": "120"}`.

## Step 5 — call, fix, present

```
diagram-emit({"diagram": {"viewpoint": "ktw-docs:/viewpoint/archimate3-layered", …}})
```

Output: `{xml, errors[], warnings[], quality}`.

- **`errors`** — validation. An unknown viewpoint or a malformed diagram draws nothing. An unknown kind, a relationship the notation forbids (the message lists the allowed ones) or a dangling edge still draws, but is wrong: fix and re-emit.
- **`warnings`** — lint and templates: orphan node, empty zone, long label, a kind the viewpoint does not show, a binding that failed.

In ktw-gui the call opens a draw.io tab as soon as the first node is written and draws the diagram while you write it, then lays it out when the call is complete — write `viewpoint` and `title` first, then zones, nodes, edges, and do not open the drawing again with another tool. The XML also opens in drawio.app and in Confluence. `quality` says how it reads: overlaps must be 0; mention a jump in crossings.

## Step 6 — iterate

The current version is **the drawing the user has**, not your last JSON: every element carries its model identity, and the user may have edited it.

1. **Read it**: `diagram-read({"xml": <the .drawio the user kept>})` → the diagram with their edits (Edit Data values, boxes added by hand as kind `unknown`, locked nodes in `pinned`).
2. **Change only what was asked** on that diagram — **keep every id**:
   - *Refine* ("move X to zone Y", "drop Z", "add W"): the named nodes, edges, zones.
   - *Restyle* ("colour by criticality", "draw it in ArchiMate"): attributes or viewpoint; structure unchanged.
   - *Regenerate* ("redo it for …"): a new model from the source; skip step 3's `previous` and warn that the user's hand edits of the old drawing are not carried over.
3. **Emit over it**: `diagram-emit({"diagram": …, "previous": <same XML>})` — nothing the user has seen moves, locks hold, new nodes go into their zone or beside a neighbour. If the drawing gets untidy after many changes, suggest `ktw-docs drawio layout <file>`.
4. **Say what changed** in one or two sentences ("Moved SAP CO to PILOTAGE; nothing else moved"). Never paste the XML or the JSON.

A change names a node the diagram lacks → say so and offer the closest label. A drawing not made by `diagram-emit` reads with kind `unknown` everywhere → propose kinds from the viewpoint's notation first. Full rules: `modules/drawio/docs/iteration-protocol.md`.

## Common pitfalls

- **Don't pass raw mxGraph XML** — `diagram-emit` only takes the typed diagram.
- **Don't invent kinds** — use the closest kind of the notation; if the gap is structural, a sidecar publishes its own notation and viewpoint.
- **Don't skip layout** for more than a handful of nodes.
- **Don't mix patterns** within one diagram.

## See also

- `ktw-docs:/reference/drawio-authoring.md` — notations, viewpoints, template libraries, bindings
