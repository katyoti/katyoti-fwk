---
name: create-capability
description: "Add a capability to the workspace end to end: its declaration, sections, workflow, gate, hooks, policy, aspect, agent block, translations. Load to give objects a new ability without a new type."
advertised: false
hashtags: [cwm, capability, pack, hooks, gates, workflow, authoring, create-items]
category: guide
---

# Adding a capability end to end

A **capability** is a named thing an object *has*: a group of fields and
everything that comes with it — badges, a workflow, hooks, functions, actions,
files, what a chat is told. It is declared ONCE in a pack
(`embed/cwm/capabilities/<name>.yaml`) and put on objects by a type that lists
it, an aspect, the object itself, or its own `attach`. Most new abilities are a
capability, not a new type. The tables (folders, slots, hook points, filters,
which mechanism) are `ktw-compose:/reference/artifact/pack-declarations.md`;
the worked example is ktw-demo's `signoff` (`app/ktw-demo/embed/cwm/`).

## 1. Pick the mechanism first

Answer "what does the person need?" with the which-mechanism table of the
reference, then extend before you create: scan the `capabilities/`,
`aspects/`, `workflows/`, `gates/` folders across sidecars (and
`cwm-object-type-get` / `ObjectAspectList` on a live universe). A capability
named after a kind of thing is a type in disguise
(`ktw-compose:/skill/create-object-type`); a `status` field is a workflow in
disguise.

## 2. Where the pack lives, and what is files-only

- **A content folder** (`ktw-compose create-content`, served by ktw-host): its
  `cwm/` folder is wired for every pack kind — no Go at all.
- **A sidecar in this repo**: `embed/cwm/` covered by its `//go:embed` line —
  no Go wiring: the kit's default pack reads `embed/cwm` as a part of the
  binary's pack, every pack kind included (`sidecarkit.WithoutPack()` drops
  it). Its body schemas sit flat in `embed/cwm/data-types/`. A pack folder
  no kind reads refuses the boot.

Every slot that names an action (hooks, functions, actions, a panel's source,
agent tools) points at a method that must exist. Reusing published ones
(`ktw-methods:methods/file-exists`, a task you author) keeps the work
files-only; a new method with domain logic is the code track
(`ktw-code` / `ktw-gocode`) — say so and hand off.

## 3. Declare the capability

```yaml
# capabilities/<name>.yaml
name: signoff                         # kebab; the key of the group: capabilities.signoff
label: Sign-off
description: Who reviews, who approved, the verdict, the notes.
icon: eye-check
schema: :/data-type/cap-signoff       # the group on each carrier (data-types/…/cap-signoff.dsl.yaml)
type_schema: :/data-type/type-signoff # the section a type writes: `signoff: {required_reviewers: 2}`
badges:
  - {field: verdict, icon: eye-check, colors: {pending: gray, approved: green}}
actions:
  - {uri: :tasks/signoff-request, label: Ask for a sign-off, kind: ai}
agent:
  system: [{id: signoff-how, title: Signing off this object, text: "…"}]
  skills: [{selector: signoff-an-object}]
```

Rules that bite:

- A slot left out is fine; a capability with none is a tag.
- `schema` and `type_schema` are DSL data-types (`ktw-methods:/skill/author-schema`),
  stem == inner `name:` == URI segment. A data-type a Go service decodes is
  generated from its Go type, never hand-written.
- A type puts it in force by listing it in `capabilities:` or by writing its
  section; a capability with only `type_schema` is type-level (no group on
  objects).
- `attach: {types, capabilities}` takes shape only — `when`, `incoming`,
  `outgoing` are refused. An attached capability never attaches another.
- A capability with its own state declares `workflow: {name, state, history}`;
  never a free `state` field.

## 4. The pieces around it (each its own file)

| Need | File | Shape |
|---|---|---|
| several capabilities in one move | `aspects/<name>.yaml` | `name`, `label`, `capabilities: [signoff, lifecycle]` |
| states and moves | `workflows/<name>.yaml` | `initial`, `states: [{name, description, terminal}]`, `transitions: [{name, from, to, gate, force}]` |
| a definition of done | `gates/<name>.yaml` | `requirements: [{name, label, check \| uri \| person: true, resolve}]`; a guard asks `gate('<name>').passes` |
| one more requirement on a gate | `gate-requirements/<name>.yaml` | `gate`, `applies: {types, capabilities}`, `check` |
| a measured rule | `policies/<name>.yaml` | `applies: {capabilities: [...]}`, `hooks: {verify: [...]}`, `agent` |
| behaviour on objects you do not edit | `hooks/<name>.yaml` | `type: <point>`, `uri`, `filter`, `input` |
| a point of your own | `hook-types/<name>.yaml` | `timing: on-demand`, `answer: findings` |
| chat framing for some objects | `agent-contexts/<name>.yaml` | `filter`, `agent: {system, tools, skills}` |
| a tab | `object-panels/<name>.yaml` | `ktw-compose:/skill/create-object-panel` |

A guard (`gate:` of a transition, a gate `check`) may call capability
functions; a function declared `costly` (`verify()`, `lint()`) is decided at
the move, never in a preview. A hook's `when` and `filter` call none.

## 5. Hooks and who they apply to

A hook entry is `{uri, when, filter, input}`. With `input`, the method gets
that map with each `${…}` resolved over the object — an existing method serves
a point with no adapter. A **refusing** entry (`before_*`, `on_transition`)
applies only to objects your provider owns: a type it declares, or your
capability carried by the object's choice (its type, an aspect, the instance —
never your own `attach`). To refuse writes on another pack's types, the
universe grants it in Univers.yaml (`sidecars[].grants.gate: {types,
capabilities}`); without the grant the entry is listed as not applying and
`ktw-compose lint-universe` reports it. A refused write names its refuser.

## 6. Translate

Every text a person reads — `label`, `description`, action and badge labels,
workflow state and transition names, gate requirement labels, agent section
titles, data-type field labels (`<name>.dsl.i18n.yaml`) — gets its other
languages in a `<name>.i18n.yaml` beside the file. Run
`ktw-compose:/skill/translate-contributions`.

## 7. Validate

1. Rebuild (or restart the content folder) and boot: a misspelt key, a section
   that does not match its `type_schema`, a filter with a refused field each
   refuse the declaration, naming it.
2. On a live universe: `ObjectTypeGet` shows the section; an object carrying
   the capability validates its group; `ObjectHooks` lists your entries and
   whether each applies; `TransitionOptions` judges the gated move.
3. `ktw-compose lint-universe --bin-dir <bin>` (scope findings, cross-sidecar
   URIs) and `ktw-compose i18n <pack> --lang fr --missing` (`complete`).
4. In the repo, `go test ./app/<sidecar>/ -run TestCensus` re-reads the
   census; an intended change is re-blessed with `KTW_CENSUS_UPDATE=1` on the
   same command, and the golden's diff reviewed.
5. Prove the whole flow as a scenario case modelled on
   `test_files/scenarios/demo-showcase` (`ktw-compose:/skill/test-scenario-case`):
   create objects, write, transition, read the pane, through the hub.
