---
name: create-persona
description: "Author a persona: an agent-loop role bound to CWM objects (bind mode, types, augments, applies_to, skills). Load to create a reusable agent role spawned on workspace objects."
advertised: false
hashtags: [cwm, persona, agent, authoring, create-items]
category: guide
---

# Creating a persona

A persona is a **reusable agent role spawned on objects** — an agent-loop task
PLUS the CWM binding metadata that says which objects it targets and how. It is
ONE folder: `app/<sidecar>/embed/personas/<name>/TASK.md` (kebab name = folder
name). The agent-loop half is authored like any agent —
`ktw-compose:/skill/create-agent` owns the loop/prompt/plugins craft; this skill
adds the persona-specific frontmatter and the library-binding rules.

## The frontmatter

```yaml
---
name: architect            # kebab, unique; == folder name
type: agent-loop           # personas are agent-loop tasks
category: persona          # marks it for the CWM persona library
description: One line — the role and its job.
# iconName: code           # optional — Tabler icon (GUI)
# icon: "⌨️"               # optional — emoji (terminal)
# hashtags: [persona]      # optional
augments: architect        # optional — layer this onto a baseline persona (see below)
advertised: false          # with augments — an augmentation is never listed beside its baseline
applies_to:                # optional, on an augment — merge it only for this work
  files: ["**/*.go"]
  modules: [planification]
bind_mode: object          # object | global | custom (see below)
types: [module, project]   # the object types it may target; [] = any
default_gate: code-implemented   # optional — the DoD criterion this persona toggles
skills: [onboard-existing-code, gocode-module-decompose]   # skills merged into its plugin set
config:
  plugins: [augmented, cwm]   # the plugin bundle the loop runs with
---

<body — the persona's behavioral guidance: its role, how it works on a bound
object, which methods/tasks to prefer. This is the persona's system-prompt half.>
```

## The persona-specific fields

### `augments` — the layered library (highest-leverage)
The persona library is **tiered**: a domain layer can AUGMENT a baseline persona
of the same `name`. `augments: architect` means "I am the ktw-gocode layer of the
baseline `architect`" — at load, the baseline and the augment are **merged by
name**: bodies append, `skills`/`plugins` union. Prefer augmenting an existing
baseline over authoring a fresh persona; **never ship two same-named baselines**
(the merge would be ambiguous). Omit `augments` only for a genuinely new baseline
role. An augment declaring `applies_to` (`files` globs, `modules` ids) is merged
only when the work the persona is spawned for matches it — a Go layer joins
only for Go files.

### `bind_mode`
How the persona is spawned: `object` (the spawn names a target object — the
usual case), `global` (no target, e.g. a system-health auditor) or `custom`
(the body defines its binding; routed to `object` for now).

### `types`
Which object types this persona is meant for (`[module, project]`); `[]` = any.
The library lists personas by it. Bind only where the role makes
sense — an `architect` on a `module`, not on a leaf object. The retired key
`allowed_node_types` refuses the persona.

### `default_gate`
Optional: the DoD criterion this persona toggles when its work is accepted
(e.g. `code-implemented`). Wire it only when the persona OWNS that criterion —
otherwise leave it unset.

## Body

The body is the persona's behavioral half — same craft as an agent system prompt
(`ktw-compose:/skill/create-agent`), but framed as a standing ROLE on a bound
object: what it is responsible for, which slot methods / entry-point tasks to
prefer, what it must NOT hand-edit. Keep it to the role's decisions; the generic
object context comes from the `agent:` blocks of its type and capabilities.

## Validate

- Rebuild the sidecar and boot it; the persona appears in the CWM persona library
  and binds to an object of an allowed type. When the source is in this workspace,
  iterate on the overlay
  (`.katyoti/overwrites/<sidecar>/embed/personas/<name>/TASK.md`), served live,
  and `promote` when validated — `ktw-compose:/skill/overlay-iteration`.
- If `augments` is set, confirm the baseline of that name exists and the merged
  role (unioned skills/plugins, appended body) is what you intended.
- Every skill in `skills:` and every plugin in `config.plugins` resolves — a
  named-but-absent skill sends the bound agent flailing.
