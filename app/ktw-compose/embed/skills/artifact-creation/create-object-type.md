---
name: create-object-type
description: "Author a CWM object type: its capabilities, fields, containment, agent surface, actions and checks. Load to create or extend a kind of object in the workspace."
advertised: false
hashtags: [cwm, object-type, capability, authoring, create-items]
category: guide
---

# Creating a CWM object type

Everything in the workspace is an **object**; its `type:` names an object type.
An object type is ONE YAML file, `app/<sidecar>/embed/cwm/object-types/<name>.yaml`
— the file name is the type name. The sidecar ships it by an
`embed/cwm` folder covered by its `//go:embed` line: the kit's default pack reads
it with no option (`sidecarkit.WithoutPack()` drops it). The reference example is ktw-demo's `signoff-item`
(`app/ktw-demo/embed/cwm/object-types/signoff-item.yaml`): a type composing a
capability, writing its sections, moving through a workflow, translated.

**A type is a named bundle of capabilities plus the rules that belong to the
type.** A capability is what an object *has* — a group of fields with its schema,
workflow, panels, actions, checks and agent surface — declared once in
`embed/cwm/capabilities/<name>.yaml` and shared by every type that lists it. The
type adds what constrains the *type*: containment, where it files and shows,
budgets, its sections. Before writing a type, ask which capabilities
already exist (`ktw-agents cwm object-type-list`, the `capabilities/` folders across
sidecars): most new types are an existing bundle with one more capability. A
new capability, aspect, workflow, gate, policy or hook is
`ktw-compose:/skill/create-capability`; a tab on the pane is
`ktw-compose:/skill/create-object-panel`.

## 1. Identity

```yaml
name: my-type          # kebab, unique across the universe; == the file name
label: My Type         # GUI display name
label_plural: My Types # the parent pane's tab listing them; empty = label
description: |
  What an object of this type is, and how it differs from its nearest sibling
  type — the distinction line is what stops mis-filing.
icon: "📦"             # emoji (terminal / agent)
icon_name: package     # Tabler icon (GUI)
abstract: false        # true = no object may have this type
```

## 2. Capabilities and fields

```yaml
capabilities: [lifecycle, file-overview]       # the default bundle, by capability name
fields: :/data-type/<name>                     # the type's OWN fields (object.fields); ":" = this binary
body:                                          # what the markdown body is on this type
  label: Explanation
  format: markdown                             # markdown (default) | text | html | a format a panel owns
```

**Sections.** Any other top-level key must be the name of a capability: it is
that capability's settings on the type, checked against its `type_schema`, and
writing it puts the capability in force. A key naming neither a field nor a
capability refuses the type. The common ones:

```yaml
pane:                       # the object pane: the Information view and the type's header chips
  view: {kind: markdown, data: "**Area**: ${object.fields.area}"}   # empty = a form from fields
  badges: [{field: fields.severity, icon: alert-triangle, colors: {high: red, low: gray}}]
context:                    # object-context: inheritance down the tree (section 9)
  propagation: inherit-from-ancestors
verification:               # the lint bar (section 8)
  gate_blocks_on: error
signoff:                    # a capability's own section (ktw-demo): two approvals
  required_reviewers: 2
```

- **A field that belongs to the kind of thing** (a dossier's project code) goes in
  the type's `fields` data-type. **A field-group several types share, or that
  brings behaviour** (a state with a workflow, a file, a panel) is a capability.
- An object may carry more capabilities than its type lists — the type is the
  default bundle, not the ceiling.
- Author the data-type in DSL under the sidecar's data-types folder and reference
  it by URI; never inline a JSON schema. Load `ktw-methods:/reference/schema/dsl.md`
  for the grammar.

> **Naming rule (load-bearing).** Name the DSL file `<name>.dsl.yaml` where
> `<name>` is the exact segment of `<provider>:/data-type/<name>`. The loader
> derives the id from the **file name**, not the inner `name:` — keep **file stem
> == inner `name:` == URI segment**, or the URI silently does not resolve.

## 3. Where it shows and where it files

```yaml
tree: node          # root (drawn, may sit at the top) | node (drawn under a parent) | none (listed in its parent's pane)
folder: dossiers    # where NEW objects are written, relative to the module; empty = objects
id_prefix: DOS      # readable minted ids (DOS-001); empty = a UUIDv7
runtime: false      # true = produced by runs, filed in the project's .katyoti store, not committed
```

`tree` is a display fact of the type — two objects of one type are never drawn
differently. A `_`-prefixed folder is refused (never scanned).

## 4. Containment — the UNION rule

```yaml
accept_parents: ['*']     # child side: parent types that may host this; '*' = any
allowed_children: []      # parent side: types this may host; empty = no restriction
```

The effective rule is the **UNION** of `parent.allowed_children` and
`child.accept_parents` — a wildcard on either side is enough.

> ⚠️ **A type creatable NOWHERE is a real bug.** If no type lists yours in
> `allowed_children` and yours sets no `accept_parents`, the GUI offers it under
> no parent. A top-level type (a `project`, a `composition`) declares
> `accept_parents: ['*']`. Always answer "from which parent can the user add
> this?" before shipping.

## 5. Lifecycle

```yaml
workflow: decision     # the workflow objects of this type move through; empty = no lifecycle
```

The workflow (`embed/cwm/workflows/`) declares the states and the guarded
transitions; `ObjectTransition` is the only way a state changes, and the state
lives in `capabilities.lifecycle.state`, never in a field. A transition's guard
is `gate:` — an el predicate (`by.kind == 'person'`, or a named definition of
done: `gate('signoff-complete').passes`); `force: person` lets a person force
past it with a note. Authoring a workflow or a gate:
`ktw-compose:/skill/create-capability`.

## 6. Agent surface — what a chat on this object gets

```yaml
agent:
  system:
    - id: my-type-context      # a closer layer with the same id replaces it
      text: |
        One paragraph framing every chat with an object of this type —
        what it is, which methods to prefer, what NOT to hand-edit.
  tools:
    - uri: <provider>:tasks/<name>   # an action URI (a task is a tool)
    - plugin: workspace              # or a plugin ref ("workspace", "bash", "group:standard")
  skills:
    - selector: <provider>:/skill/<name>   # exact name, prefix glob, or <provider>:/skill/*
```

`agent:` is the one way to declare what a chat gets — the same block on a
capability, an `agent-context`, a policy and one object. The `system` text is the
highest-leverage entry: it frames every chat started on the object.

### The chat surface is ENTRY POINTS, not a tool bag

A chat on an object is an **orchestrator**, not a do-everything agent:

1. **Prefer entry-point TASKS over raw tools.** A task IS a tool
   (`<provider>:tasks/<name>`): list a small set of them in `agent.tools` (and
   in `actions` for the user-launched ones). Each one owns its heavy domain tools
   in its own `config`, does one job and returns a result; the chat composes.
2. **Inject a raw tool only when the chat itself must call it inline** — a couple
   of cheap, always-needed calls (a `cwm-*` read). A 50-tool chat is the
   over-provisioning failure.
3. **Capability is additive.** A new ability is a new entry-point task, not 20
   tools added to every chat.

> ⚠️ **Never name a capability the surface can't reach.** A `system` text that
> says "call `leanix-search`" while neither the tool nor a task that wraps it is
> on the surface produces `unknown tool`, and the agent flails.

## 7. Actions and creation

Use the **object form** for every action — a bare URI makes the GUI show the raw
URI as the label. Always give `label` + `description`; set `kind: ai` for an
interactive wizard.

```yaml
actions:
  - uri: <provider>:tasks/<task-name>
    label: Human Label
    description: One line on what it does and the outcome
    kind: ai                       # ai = opens a CHAT TAB; omit for a headless run
    icon: "🎯"
    # confirm: true                # a confirm dialog first
    # when: "object.lifecycle.state != 'done'"   # el predicate over the object; false disables
    # preview: true                # a planned action: shown with "À venir", never runs
create_task: <provider>:tasks/new-my-type   # replaces the create form with a guided wizard that creates the object
hooks:
  before_create:     # run before the create → {accept, reason}; may refuse it
    - uri: <provider>:<slot>/<method>
  on_create:         # run after the create; never undoes it, retried, a failure shown on the object
    - uri: <provider>:tasks/<task-name>
```

A refusing hook (`before_*`, `on_transition`) applies only to objects your
provider owns — a type it declares, or a capability it declares carried by the
object's choice; a universe widens it with a grant
(`sidecars[].grants.gate` in Univers.yaml). A refused write names its refuser.

> ⚠️ **`kind` decides HOW the GUI launches it.** `kind: ai` opens a chat tab
> running the task on this object; the default dispatches a headless run with no
> tab. An interactive wizard left at the default runs fire-and-forget (its
> questions go to the Inbox); a headless action marked `ai` opens an empty chat.

A task can also put itself on your type without editing it: `kind: object` +
`types: [my-type]` in its TASK.md (`ktw-compose:/skill/task-format`).

## 8. Checks — lint, verify, and the gate bar

```yaml
hooks:
  lint:                # findings on demand, never a verdict
    - uri: ktw-methods:methods/file-exists
      input: { path: "${object.fields.skill_file}" }
  verify:              # an acceptance proof
    - uri: <provider>:<slot>/<method>
verification:
  gate_blocks_on: error  # lowest lint severity counted as blocking: error (default) | warn | info | none
```

Each entry is an action URI dispatched through ActionService; with `input`, the
method receives that map, each `${…}` resolved over the object, else
`{point, object, prior}`. A lint entry answers `{status, findings:[{rule,
message, severity}]}`; a verify entry `{status, evidence?, verdict?}` — `pass`
and `fail` count, anything else is UNPROVEN. A guard reads them through the
costly functions `verify()` / `lint()` (`verify().green && lint().blocking == 0`). The methods-pack basics
(`ktw-methods:methods/{file-exists,markdown-frontmatter,frontmatter-keys,non-empty}`)
are generic; a sidecar declares its own domain checks.

## 9. Inheritance down the tree

For a type whose objects other objects should *see* (a rule, a decision), the
type writes a `context:` section:

```yaml
context:
  propagation: inherit-from-ancestors   # inherit-from-ancestors | propagate-to-ancestors | local | local-visible-up | per-criterion
  resolution_strategy: closest-wins     # accumulate | closest-wins | accumulate-with-local-override | local-only | per-criterion-override
  override_key: topic                   # REQUIRED for closest-wins / accumulate-with-local-override
  briefing_fields: [statement]          # fields carried into an object's brief — only what an agent needs
  injected_objects: []                  # objects every object of this type carries
```

| The type is… | propagation | resolution_strategy |
|---|---|---|
| a rule descendants must obey | inherit-from-ancestors | accumulate |
| a decision a closer ancestor may override | inherit-from-ancestors | closest-wins + `override_key` |
| a local note, invisible elsewhere | local | local-only |
| a status a parent wants rolled up | propagate-to-ancestors / local-visible-up | accumulate |

Budgets: `max_size_bytes` (per-write, enforced), `description_max_chars`.

## 10. Translations

The person reads the type's `label`, `label_plural`, `description`, its action
labels and its `pane.badges` labels: give them in every supported language in a
`<name>.i18n.yaml` beside the type (`fr:` then the same keys). The shipped packs
are held at zero missing; `ktw-compose:/skill/translate-contributions` lists,
scaffolds and checks them.

## 11. Validate

- Rebuild the sidecar and boot it — a malformed type fails at load, and the
  census refuses a type that does not match the `object-type` schema. When the
  sidecar source is in this workspace, iterate without rebuilding: write the YAML
  into the overlay (`.katyoti/overwrites/<sidecar>/embed/cwm/object-types/<name>.yaml`),
  served live when the universe allows overwrites; promote when validated
  (`ktw-compose:/skill/overlay-iteration`).
- `cwm-object-type-get {name: my-type}` returns it; an object of the type creates
  under an allowed parent, and a parent's create menu offers it.
- Read every URI the type names — `fields`, actions, checks, `create_task`. A
  dead data-type URI breaks the create form.
- `ktw-compose lint-universe --bin-dir <bin>` lists a declaration out of its
  provider's scope (a refusing hook on a type you do not own).
- Extend before you create: scan the `object-types/` and `capabilities/` folders
  across sidecars before adding a near-duplicate. Check it with
  `ktw-compose:/skill/check-object-type`.
