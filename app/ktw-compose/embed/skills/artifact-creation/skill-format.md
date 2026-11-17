---
name: skill-format
description: "The ktw skill file contract — strict frontmatter keys, body structure, content rules, and the cross-reference URI syntax. Load before writing or editing any SKILL file."
advertised: false
hashtags: [skill, authoring, format, create-items]
category: guide
---

# The skill file contract

A skill is **on-demand knowhow**: a procedure an LLM loads when it decides the
skill applies. This skill is the *format* — the exact file shape every skill
must take. The *process* of creating one is `ktw-compose:/skill/create-skill`;
the *identity* work is `ktw-compose:/skill/skill-intent-interview`.

## When NOT to write a skill

Write the smaller thing instead:

| You want to… | Author this, not a skill |
|---|---|
| Run deterministic logic, return data | a **tool** (a typed tool / slot method) |
| Expose a fixed value (URL, schema, credential-derived base) | a **tool** that returns it as JSON |
| Hold passive lookup material (a schema, a cheatsheet, a catalog) | a **reference** under `embed/references/` |
| Teach one repeatable *procedure with decisions* | ✅ a skill |

A skill earns its place only when the LLM must follow a *procedure* and make
*choices* along the way. No choices → reference. No procedure → tool.

## Two hard requirements

Every skill MUST work on both surfaces, unchanged:

1. **`<sidecar> llm` surface** — an external LLM drives the binary through `llm`
   verbs (JSON in/out).
2. **The skill plugin** — an in-process agent loop loads the skill as a tool.

The same body is read on both. So the body may **name** things but must never
encode **how to load them** — that is the host's job (see Cross-references).

## Frontmatter — the full contract

The parser is **strict**: any key not in this list fails the parse. Two keys are
required; the rest are optional.

```yaml
---
name: my-skill            # required. kebab-case. = the <name> in <provider>:/skill/<name>
description: ...           # required. see Description rules
requires: [workspace, web]    # optional. skills/tools this skill assumes are present
provides: [task]          # optional. external exposure: task | cli | mcp-prompt | mcp-tool | mcp-resource
icon: "📋"                # optional. emoji for terminal / agent
iconName: clipboard-list  # optional. Tabler icon name for GUI
hashtags: [analysis]      # optional. grouping tags
category: guide           # optional. primary grouping
advertised: true          # optional, default true. false = hidden from listings, still loadable by name
---
```

There is no `scripts:` key — a skill's executable logic is published as a slot
method and called like any other tool.

## Description rules

`description` is the only thing the LLM sees before loading (L0). It decides
selection. Make it:

- **Short** — one sentence where possible.
- **What + When** — what the skill does *and* the trigger ("Load when…").
- **Distinct** — no vocabulary overlap with sibling skills, or the LLM can't pick.

## Multi-level discovery

Content is revealed in levels so context isn't spent until needed:

| Level | What | Loaded when |
|---|---|---|
| **L0** | `name` + `description` | always (if advertised) |
| **L1** | the SKILL body | the skill is loaded |
| **L2** | a **sub-skill** (preferred) or a **reference** | the body points to it and the LLM follows |
| **L3+** | the sub-skill's own L2 | recursive, arbitrary depth |

**Prefer a one-level-deeper sub-skill over a reference file.** A sub-skill has
its own L0 description, so the LLM can judge whether to descend; a reference is
opaque until read. Use a reference only for passive material (a schema, a table).

**Only top skills are advertised.** A dispatcher skill (e.g. `diagrams`) is
`advertised: true`; the shapes it routes to (`c4-diagrams`, `uml-diagrams`) are
`advertised: false` and named from the parent. Each of those may itself route
one level deeper. The listing stays small; depth is reachable but not noisy.

## Cross-references — name the URI, never the loader

Every host (agent loop, `llm` CLI, MCP, HTTP) injects a calling-convention
preamble that maps a URI to the right invocation for that surface. So the body
**names the target by URI and stops**. Never write "use the … tool with {…}" or
"run `<binary> sc skills …`" — that is surface-specific and the host already
said it.

**Canonical verbs (HARD requirement).** There is ONE verb per kind. Write the
verb + the URI and stop — the host binds the verb to its surface. NEVER name a
surface-specific loader — the agent-tool name for fetching a skill's
instructions, a `<binary> skills …` CLI subcommand, the operator read
subcommand, an MCP envelope: each is false on the other surfaces.

| Kind | Verb | Write | URI shape |
|---|---|---|---|
| **skill** | **get-instruction** | "Get-instruction the `ktw-compose:/skill/create-task` skill, then follow it." | `<provider>:/skill/<name>` |
| **resource** (reference / config / data-type) | **read** | "Read the `ktw-docs:/reference/rich-markdown.md` resource." | `<provider>:/<type>/<id>` |
| **tool** (incl. a task `…:tasks/<name>`) | **call** | "Call the `ktw-example:example/search` tool with `{…}`." | `<provider>:<slot>/<method>` |
| **event** | **subscribe** | "Subscribe to `cwm:!workspace/object/*/created`." | `<provider>:!<scope>/<path>` |

The shape after `:` tells the kind: `:/` resource/skill, `:` tool, `:!` event.
Bold the name — LLMs lose names buried in prose.

## Writing the body

- Second-person imperative: "Resolve the id, then call…".
- Show tool inputs as JSON objects — surface-neutral. Never shell flags or curl.
- For a config value the LLM needs, point at a **tool** that returns it — never
  tell the LLM to read a file off disk (the disk may not exist on its host).
- State limits plainly: "If the search is empty, stop — do not invent."
- **One method per outcome.** When two invocations produce the same result with
  no meaningful difference, name exactly ONE — never "you can also…" or a
  parenthetical alternative. Alternatives cost tokens, split LLM behavior
  across paths, and add a decision that buys nothing. Offer a second method
  only when it serves a genuinely different case — and then say which case
  picks which.
- **Say only what is true now.** Describe the capability that exists. Never date
  or version it ("since vX we can…", "after milestone Y…") — a skill is timeless
  knowhow, not a changelog.

## Brevity

Say it once. If two skills need the same passage, one links the other. Do not
inline a sub-skill's content into its parent — name it and let the LLM descend.
Every sentence must earn its tokens; cut anything the LLM already knows.

## Checklist

MUST be present:
- [ ] `name` (kebab-case, matches the intended URI) + `description` (what + when)
- [ ] A procedure with at least one decision point (else it's a reference/tool)
- [ ] Tool inputs as JSON; tools named in bold
- [ ] Cross-references as bare URIs (resource / tool / event)
- [ ] `advertised: false` if this is a sub-skill or an entry-point/`usage` skill

MUST NEVER appear:
- [ ] Any frontmatter key outside the contract above (strict parser rejects it)
- [ ] Alternative invocations for the same outcome ("or you can also…") — one method per outcome
- [ ] Surface-specific loading instructions ("use the X tool with {…}", `<binary> sc …`, curl, MCP envelopes)
- [ ] Shell command lines or CLI flags as the way to call a tool
- [ ] Instructions to read a config/YAML file from disk
- [ ] Time/version framing ("since…", "as of milestone…", "legacy…")
- [ ] Links to files outside this sidecar (a deployed sidecar cannot reach them)
- [ ] Content duplicated from another skill instead of a link
