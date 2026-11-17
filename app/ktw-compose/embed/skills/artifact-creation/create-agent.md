---
name: create-agent
description: "How to author a ktw agent — a task with type:agent — its system-prompt body, plugins, capability, budgets, and delegation. Load when creating a task that must reason and use tools."
advertised: false
hashtags: [agent, authoring, create-items]
category: guide
---

# Authoring an agent

An agent IS a task with `type: agent` — the body is its system prompt and it
runs an LLM loop with tools until it calls **complete**. This skill covers the
agent-specific authoring beyond the shared task shape. Load
`ktw-compose:/skill/create-task` for the lifecycle and
`ktw-compose:/skill/task-format` for the frontmatter contract.

## When type:agent (vs task / workflow)

| | `type: task` | `type: agent` | `type: workflow` |
|---|---|---|---|
| Engine | verb dispatch | LLM loop + tools | workflow machine |
| Body is… | a description | the **system prompt** | a description |
| Tools | none | the plugin-injected set | per-spawn sub-agents |
| Turns | 1 | up to `MaxTurns` | N steps |

Reach for `agent` when the work needs reasoning + tools + iteration. For fixed
multi-step orchestration with no per-step reasoning, that is a workflow —
`ktw-compose:/skill/create-workflow`.

## The loop

Build system prompt → call the LLM → dispatch the tools it asks for → repeat,
until it calls **complete** or a budget (`MaxTurns` / `MaxBudgetUSD` /
`MaxDurationS`) is hit. So the prompt MUST drive toward a `complete` call.

## Capabilities — plugins / skills / tools

The agent's callable surface comes from three sibling lists under `config:`,
all sharing one wildcard model. The full contract (forms, the
advertised-vs-exact rule) is `ktw-compose:/skill/task-format` — use the most
specific list:

- **`plugins:`** — named bundles (`standard`, `augmented`, `voice-enabled`,
  `full`) or leaf types (`workspace`, `bash`, `task`, `ask-user`, `template`,
  `orchestration`, `web-enabled`, `scripting`, `mcp`). Tool→plugin table:
  `ktw-compose:/reference/artifact/task-tools.md`.
- **`skills:`** — skill selectors (`my-skill`, `cwm-*`, `ktw-methods:/skill/*`,
  `"*"`). Declaring it **auto-attaches the skill plugin scoped to those skills** —
  no need to also list `augmented`.
- **`tools:`** — action/tool URIs (`ktw-weather:weather-service/get-forecast`,
  `ktw-weather:weather-service/*`, `"*"`) exposed as native typed tools.

Prefer scoped `skills:` / `tools:` over a blanket `augmented` so the agent's
catalog stays small and on-task.

## Prompt design

A good agent system prompt:

- opens with role + goal ("You are a… Your job is to…")
- lists concrete numbered steps
- names the tools in **bold**, spelt as registered — kebab-case (`file-read`,
  `ask-user`, `task-status`; a group's methods are `<group>-<method>`) — and
  mandates their use
- ends with a termination line ordering the tool: "When <done>, you MUST use the
  **complete** tool to return <result>." (A `mode: chat` agent is driven by the
  person: `complete` is hidden and its body never orders it.)
- is mode-agnostic — tools by canonical name, never shell command lines
- states goals, not tool mechanics (the tool descriptions handle the how)

Avoid: vague goals, no tool references, a missing `complete` order (the run
then ends on the model's last text, with no structured result), snake_case tool
names (they match no tool and no permission gate), and shell-flavoured
examples.

For choosing the agent's shape (ReAct, plan-execute, reflection, …) the pattern
catalogue + decision tree is `ktw-compose:/skill/agent-design-patterns`. To
sharpen a drafted prompt body, `ktw-compose:/skill/prompt-improvement` applies the
prompt-engineering techniques to it.

## Author-settable run config

These map from the task's frontmatter into the agent run — set them deliberately:

- **`capability`** — `fast | smart | powerful` → the LLM profile.
- **`plugins` / `skills` / `tools`** — the callable surface (above).
- **budgets** — `MaxTurns` (default ~25), `MaxBudgetUSD`, `MaxDurationS`: cap
  runaway loops; set when the task should be bounded.
- **delegation** — an agent that spawns sub-tasks uses the `delegate` tool with
  a mode; the modes + when to use each are `ktw-agents:/skill/delegation-modes`.
  Name it; do not restate the mode table.

## Testing

First runs usually fail — the catalog is
`ktw-compose:/skill/failure-patterns`. Run the agent, then
inspect the session: did it call the right tools, did delegation resolve, did
`ask-user` get answered, did it call **complete**, was a result returned. A
`--trace` NDJSON file captures every tool call for debugging; structured run
analysis is the run-report / run-quality-evaluation surface.
