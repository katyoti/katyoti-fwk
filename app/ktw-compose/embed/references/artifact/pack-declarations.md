# Pack declarations — the lookup tables

What a CWM pack (`embed/cwm/`, one folder per kind, one file per declaration —
the file name is the name when the body gives none) can carry, condensed. A URI
starting with `:` names the binary hosting the pack (`:/data-type/x`,
`:signoff/board`). The full guide, with every field tied to its Go type, is
`modules/cwm/objects/what-a-capability-and-a-sidecar-give.cwm.md` in the
ktw-agents repository.

## The folders

| Folder | Kind | Gives the person |
|---|---|---|
| `object-types/` | object-type | a new kind of thing to create, with its form, its place in the tree, its tabs |
| `capabilities/` | capability | a field-group and everything that comes with it, on any type that lists it |
| `aspects/` | aspect | several capabilities in one move ("+ Aspect"), no new type |
| `workflows/` | workflow | states, the moves between them, who may make them |
| `policies/` | policy | a rule over objects of some types, measured by its hooks, told to the chat |
| `hook-types/` | hook-type | a point of the pack's own, run on demand (`ObjectRunHooks`) |
| `hooks/` | hook | behaviour at a point for every object its filter admits — no type edited |
| `edge-types/` | edge-type | a named relation, its inverse, who may take part |
| `data-types/` | data-type | the schema of a group, a section, fields, a form, a row (sub-folders allowed) |
| `object-panels/` | object-panel | a tab on the pane of every object the filter admits |
| `agent-contexts/` | agent-context | what a chat on the admitted objects is told, given and offered |
| `gates/` | gate | a definition of done named once, made of requirements |
| `gate-requirements/` | gate-requirement | one more requirement on another pack's gate, for the objects it `applies` to |

The last four are read by extensions: a binary whose pack holds them imports
their `Kinds()` (`panecontrib`, `occontrib`, `gatescontrib`) or it refuses to
boot. A ktw-host content folder wires them all for its `cwm/` folder.

## A capability's slots (all optional; none = a tag)

| Slot | What it does |
|---|---|
| `name`, `label`, `description`, `icon` | the id (and the key `capabilities.<name>` on an object); the section header and badge |
| `schema` | data-type of the group on each carrier; CWM validates it, the pane draws it |
| `type_schema` | data-type of the section a type writes under the capability's name; none on objects = type-level |
| `files` | named files every carrier has: `{path, media, description, owned}`, `${…}` over the object |
| `actions` | what every carrier can run: `{uri, label, description, kind: ai\|sync\|async, icon, confirm, preview, when, input}` |
| `hooks` | entries by point: `{uri, when, filter, input}` |
| `agent` | `{system: [{id, title, order, text}], tools: [{uri}\|{plugin}], skills: [{selector}], files: [...]}` |
| `attach` | `{types, capabilities}` — put on every object it selects (`"*"` = every type) |
| `badges` | header chips over its own fields: `{field, label, icon, tooltip, colors}` |
| `uses` | capabilities whose fields this one reads |
| `workflow` | `{name, state, history}` — its own state, beside the object's lifecycle |
| `functions` | `{name, action, args, returns, timeout, costly}` — read-only questions for expressions |

## Hook points

| Point | Answer | Fires |
|---|---|---|
| `before_create` / `before_update` / `before_delete` | `{accept, reason}` — may refuse | before the write |
| `on_create` / `on_update` / `on_delete` | none; retried, a failure shown on the object | after the write |
| `on_transition` | `{accept, reason}` — may refuse | on a lifecycle move |
| `lint` | `{status, findings: [{rule, message, severity}]}` | on demand |
| `verify` | `{status, evidence?, verdict?}` — pass, fail, else UNPROVEN | on demand |
| `bootstrap` | none | when a project opens |

The method receives `{point, object, prior}`, or the entry's `input` map with
each `${…}` resolved over the object. **Scope**: a refusing entry
(`before_*`, `on_transition`) from provider P applies only to an object whose
type P declares or that carries, by its own choice (its type, an aspect, the
instance), a capability P declares — never through P's own `attach`. A universe
widens it in Univers.yaml: `sidecars: [{name: P, grants: {gate: {types: [...],
capabilities: [...]}}}]`. Out of scope, the entry is listed not applying with
the grant to add (`ktw-compose lint-universe`); a refused write names its
refuser. Every other point applies universe-wide.

## Filters

Panels, `hooks/` and agent-contexts select objects with one filter:
`types` (`*` = any), `capabilities` (effective set), `incoming` / `outgoing`
(edge kinds, from CWM's index), `when` (an el predicate over `object`,
`current`, `id`, `type`, `parent_id`, `children`, `links`). Fields AND, a list
is any-of, a misspelt key refuses the declaration. A capability's `attach`, a
policy's and a gate requirement's `applies` take only `types` and
`capabilities`: an object's shape never depends on its content.

## Which mechanism

| I need… | Use |
|---|---|
| a new kind of thing people create | an object-type |
| fields reusable across types | a capability with a `schema` |
| settings a type sets for my extension | a capability `type_schema` + the type's section |
| several capabilities, no new identity | an aspect |
| a capability on objects I do not edit | `attach` |
| a measured rule over objects | a policy with a `verify` hook |
| states and who may move | a workflow (never a `status` field) |
| refuse a write / react after one | a `before_*` / `on_*` hook |
| findings on demand / a pass-fail criterion | a `lint` / `verify` hook |
| a definition of done named once | a gate (+ gate-requirements); a guard asks `gate('<name>').passes` |
| a question expressions can ask | a capability function |
| a tab on some objects | an object-panel with a filter |
| how a type's Information tab looks / header chips | the type's `pane.view` / `badges` |
| something to run on an object | an `actions` entry, or a task with `kind: object` + `types` |
| what a chat on an object knows and can do | an `agent` block, or an agent-context |
| a relation between objects | an edge-type, never an id in a field |

## The worked example — ktw-demo's sign-off

`app/ktw-demo/embed/cwm/` uses every row above: `object-types/signoff-item.yaml`
(capabilities, fields, workflow, its `signoff:` and `pane:` sections),
`capabilities/signoff.yaml` (schema, type_schema, badges, the function
`signedoff()`, hooks, actions, agent), `capabilities/signoff-log.yaml`
(`attach` + an owned file), `aspects/signable.yaml`, `workflows/signoff.yaml`
(guard `gate('signoff-complete').passes`), `gates/` + `gate-requirements/`,
`policies/signoff-has-notes.yaml`, `edge-types/contests.yaml`,
`hook-types/signoff_digest.yaml`, `hooks/signoff-bootstrap.yaml`,
`object-panels/signoff-board.yaml` and `contests.yaml`,
`agent-contexts/signoff-item.yaml`, each translatable one with its
`.i18n.yaml`. The scenario case `test_files/scenarios/demo-showcase` drives it
all through the hub with no LLM.
