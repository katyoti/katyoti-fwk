---
name: task-format
description: "The task file contract: where a task lives, its strict frontmatter, the three type values, the body-to-engine mapping, and prompt-writing rules. Load before writing or editing a task file."
advertised: false
hashtags: [task, authoring, format, create-items]
category: guide
---

# The task file contract

A task is a unit of work an engine runs. This skill is the *format* — the exact
shape a task's markdown file takes. The *process* of creating one is
`ktw-compose:/skill/create-task`; the requirement work is
`ktw-compose:/skill/task-intent-interview`.

## Where it lives

A task is a **free-named markdown file**, `<name>.md`, exactly like a skill —
the file name is the task's slug (`run <name>`). It needs a **folder** only when
it ships companion files; then the entry file is named `TASK.md` and the folder
name is the slug.

- **a sidecar owns it** → `app/<sidecar>/embed/tasks/<name>.md` — ships in the
  binary, needs a rebuild.
- **workspace-local** → `<workspace>/.katyoti/tasks/<name>.md` — loaded by the
  host at runtime, no rebuild.

Use the folder form `tasks/<slug>/TASK.md` when the task carries companions: a
`system.md`, schema fixtures, or child task files for a workflow. A plain agent
or single-step task is just one flat `<name>.md`.

## Frontmatter

The root keys are a **closed set** (strict parse rejects anything else) —
this is the exhaustive list, straight from the `Task` struct + the loader:

```yaml
---
name: my-task            # required. the runnable id: run <name> + delegate(task_name:"..."); shown small on cards
title: My Task           # optional. friendly display label for the GUI gallery (falls back to name)
type: agent              # required. agent | task | workflow | simulator
description: ...         # required. one-line, what the task does
icon: "📋"               # optional. emoji for terminal / agent
iconName: clipboard-list # optional. Tabler icon name for GUI
hashtags: [analysis]     # optional. grouping tags
category: analysis       # optional. primary grouping
tags: [quick-action]     # optional. behavioral markers (not the same as hashtags)
advertised: true         # optional, default true. false hides from the gallery but stays callable by name
capability: fast         # optional. root-level shortcut; the loader folds it into config
kind: object               # optional. global (default) | object | collection — see "Binding to an object type"
types: [composition]# required when kind: object — the CWM object types this task targets
filter: object.content?.status != 'done'  # optional. el second pass over the target object
config:                  # type-specific params — the agent/workflow runtime config
  capability: fast       # agent/workflow: fast | smart | powerful (LLM profile)
  plugins:               # named bundles or leaf plugins — see Capabilities below
    - standard
  skills:                # skill selectors (auto-attaches the skill plugin)
    - my-skill
  tools:                 # action/tool URIs exposed as native tools
    - ktw-weather:weather-service/*
---
```

Beside the task, two optional files (`TASK.input.dsl.yaml` /
`TASK.output.dsl.yaml` in a folder task):

- `<name>.input.dsl.yaml` — ktw-schema DSL, the declared input contract
  (forms, tool definitions); not enforced before dispatch;
- `<name>.output.dsl.yaml` — ktw-schema DSL, the typed result shape (used by
  workflows and the agent's `complete` tool).

> **The runtime config lives under `config:`** — `plugins`, `skills`, `tools`,
> `mode`, and any strategy params are all keys of `config:`, never root-level
> frontmatter. Root carries only the meta fields (`name`, `title`, `type`,
> `description`, icons, `hashtags`, `category`, `tags`, `advertised`), the
> targeting fields (`kind` / `types` / `filter`), the `capability`
> shortcut, `permissionProfile`, `group` and `sessionMode`. The contracts are
> files beside the task, never frontmatter.

> **Visibility ≠ callability.** `advertised: false` hides a task from the GUI
> gallery, pickers, and bulk listings — but it stays callable by its exact
> `name`/URI (a direct run, or as a delegated worker). Set it on
> **worker/sub-tasks** that a top-level task runs, so they don't clutter the
> gallery. Give **top-level, user-facing tasks** a friendly `title`. `name`
> stays the runnable id either way.

## Binding a task to an object type

A task can declare that it targets CWM objects of a given type — the "this task
applies HERE" half:

```yaml
kind: object                # global (default) | object | collection
types: [composition]        # the object types it targets (required unless global); * = any
filter: len(children) > 0   # optional el predicate over the target object
```

`kind: object` offers the task on one object of those types; the runtime
threads the object through to the task (`RuntimeContext.CurrentNode`), so the
body can read `<cwm_object_context>`. `kind: collection` offers it at the head
of a type's list, to act on all its objects. `types` is the membership check;
`filter` (el, the one expression language — same as workflow `if.cond`) is an
optional second pass — both must pass for the task to apply. It reads the
OBJECT vocabulary: `object.<capability>.<field>` (e.g. `object.lifecycle.state`),
the type's fields `object.fields.<key>`, `id`, `type`, `parent_id`, `children`.
A filter naming anything else is refused when the task loads, and so are the
retired keys `kind: node | item | item-collection` and `node_types` /
`item_types`.

**Two directions connect a task and an object type; both reach the action bar:**

| Direction | How |
|---|---|
| **type → task** | the OBJECT TYPE lists the task in its `actions:` (object form, with `kind: ai` for an interactive wizard) — use it for a type you own |
| **task → type** | the TASK declares `kind: object` + `types:` — use it to put an action on a type you do not own |

> **The contract files are ktw-schema DSL, NOT JSON Schema.**
> Shape: `name:` + `root: Input` (or `Output`) + `types: { Input: { type:
> object, fields: { <field>: <type> } } }`. Use `fields:` (never
> `properties:`), shorthand types (`string@enum(...)`, `string@date-time`,
> `optional: true`). Full grammar: load `ktw-methods:/reference/schema/dsl.md`.
> Example (a city input, `my-task.input.dsl.yaml` beside `my-task.md`):
> ```yaml
> name: my-task-input
> root: Input
> types:
>   Input:
>     type: object
>     fields:
>       city: { type: string, description: "...", optional: true }
> ```

## The four types — and the body

`type` chooses the engine, and what the markdown body *means*:

| `type` | Engine | The body is… |
|---|---|---|
| `task` | simple machine (verb dispatch) | human-facing description; `config:` carries the call params |
| `agent` | LLM agent loop + tools | the agent's **system prompt** |
| `workflow` | workflow machine | human-facing description; `config.steps` carries the workflow |
| `simulator` | scripted, token-free machine | human-facing description; `config` carries the canned output or a `script:` (`ask` / `say` / `drive` steps) — for tests and dry-runs |

The framework maps the body into the strategy's body field (e.g. the agent
strategy maps body → `config.prompt`). **`agent` and `workflow` have their own
authoring sub-skills** — load `ktw-compose:/skill/create-agent` for an agent's
prompt + plugins, or `ktw-compose:/skill/create-workflow` for `config.steps`. This skill
covers the shared task-file shape.

## Capabilities — three parallel lists

An agent's callable surface is declared by **three sibling lists** under
`config:`, all sharing one wildcard model. Use the most specific one:

| List | Holds | Use it for |
|---|---|---|
| `plugins:` | named bundles + leaf plugin types | the broad toolset (`standard`, `augmented`, …) or a single built-in plugin |
| `skills:` | skill selectors | giving the agent specific skills (methodology, references, scripts) |
| `tools:` | action / tool URIs | exposing a sidecar's callable methods/tasks as native tools |

**`plugins:`** — group shorthands (`standard`, `augmented`, `voice-enabled`,
`full`) or individual leaf types (`bash`, `web-enabled`, …). The full
tool→plugin table is `ktw-compose:/reference/artifact/task-tools.md`. The
`augmented` bundle = `standard` + web research + scripting + **all** advertised
skills; reach for it only when the task truly needs the lot. For a delegating
agent, name `ktw-agents:/skill/delegation-modes` — do not restate the mode table.

**`skills:`** — declaring this **auto-attaches the skill plugin scoped to just
these skills** (no need to also list `augmented` or a skill bundle). Each entry
is a selector:

```yaml
config:
  skills:
    - spec-extractor              # exact name (reaches even an unadvertised skill)
    - cwm-*                       # prefix glob — advertised skills named cwm-*
    - ktw-methods:/skill/*       # all advertised skills from one provider
    - "*"                         # every advertised skill
```

**`tools:`** — every action URI is potentially a callable tool. Each entry
becomes a native typed tool in the agent's catalog (input schema resolved from
the provider, dispatched through the ActionService). Same selector model:

```yaml
config:
  tools:
    - ktw-weather:weather-service/get-forecast   # one method
    - ktw-weather:weather-service/*              # all advertised methods of a slot
    - ktw-acme:tasks/*                            # all advertised tasks of a provider
    - "*"                                         # every advertised tool
```

**Wildcard rule (shared by all three):** a **bulk** selector (`*` or a glob)
matches **advertised** items only — an exact name reaches an unadvertised one
too. So `*` never floods the catalog with every URI in the universe; it means
"everything meant to be offered."

## Prompt-writing rules (for `type: agent` bodies)

- **Bold tool names** — `**complete**`, not `` `complete` ``; agents lose names
  hidden in backticks.
- **List the tools** the agent should use, and **mandate use**: "You MUST use
  your tools — do not just respond with text."
- **Always order the complete tool** — a `**Termination**:` line: "When
  <condition>, you MUST use the **complete** tool to <purpose>." Without it the
  run ends on the model's last text, unstructured. The one exception is
  `config.mode: chat`: a driven chat never completes itself (`complete` is
  hidden), so its body never orders it.
- Refer to tools by their registered kebab-case name (`file-read`, `ask-user`,
  `task-status`; a tool group's methods are `<group>-<method>`) — never a
  snake_case spelling, never a shell command line (the same body may run under
  CLI, MCP, the sidecar method-call transport, or HTTP).

## Checklist

- [ ] `name` (slug, matches the file name) + `type` + `description`
- [ ] `config:` holds `capability` + the capability lists; nothing runtime at root
- [ ] `type: agent` → `capability` set; body is a usable system prompt
- [ ] capabilities scoped: prefer `skills:` / `tools:` over a broad `augmented`
- [ ] tools named in bold, kebab-case as registered; the **complete** tool ordered (agent, unless `mode: chat`)
- [ ] an input contract file (`<name>.input.dsl.yaml`) when callers pass structured input
- [ ] no shell command lines as the way to call a tool; no version/time framing
