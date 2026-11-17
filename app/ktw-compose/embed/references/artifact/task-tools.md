# Task tools reference

The tools an agent task can call, grouped by the plugin that provides them.
Named by `ktw-compose:/skill/task-format`. Every tool name is kebab-case, as
registered — a prompt or a gate spelling it otherwise matches nothing. Passive lookup — pick the plugins
whose tools the task needs. (Skills are declared via `skills:` and sidecar
action URIs via `tools:` — both are separate lists, not entries here; see
`ktw-compose:/skill/task-format`.)

| Tool | Plugin | Description |
|------|--------|-------------|
| `complete` | task | Signal task completion with a summary |
| `delegate` | task | Delegate to a sub-task |
| `task-status` | task | Poll a background task's progress |
| `file-read` | workspace | Read a file |
| `file-write` | workspace | Write a file (creates / overwrites) |
| `file-edit` | workspace | Edit a file (find/replace) |
| `glob` | workspace | Find files by glob pattern |
| `grep` | workspace | Search file contents (ripgrep) |
| `bash` | workspace | Run a shell command |
| `ask-user` | ask-user | Ask the user a question; blocks until answered |
| `render-template` | template | Render a Jinja2-like template |
| `create-team` | orchestration | Create a team mailbox |
| `spawn-teammate` | orchestration | Spawn a teammate agent into the team |
| `send-message` | orchestration | Send a message to a teammate or team |
| `check-messages` | orchestration | Poll the agent's mailbox |
| `list-team` | orchestration | List active teammates |
| `await-child` / `answer-child` / `stop-child` | orchestration | Wait for a child's question or result, answer it, stop it |
| `skills-get-instruction` | skill | Load a skill's instructions by URI `<provider>:/skill/<name>` (attached via `skills:`) |
| `read-resource` | skill | Read a reference (or any non-skill resource) by URI |
| `list-resources` | skill | List the resources of one kind matching some words |
| `web-fetch` | web-fetch | Fetch a URL |
| `web-search` | web-search | Search the web |
| `wikipedia` | wikipedia | Look up a Wikipedia article |
| `scripts-run` | scripts | Run a named script |
| `program` | program | Run a configured external program |

## Plugin groups

- `standard` — workspace (file-read/write/edit, glob, grep, bash) + task
  (complete, delegate, task-status) + ask-user + template +
  create-document + display/complete-ui-component + orchestration
- `web-enabled` — web-fetch + web-search + wikipedia
- `scripting` — scripts + program
- `augmented` — `standard` + `web-enabled` + `scripting` + `skill` (all
  advertised skills). The broad default; prefer a scoped `skills:` list instead
  when the task needs only specific skills.
- `voice-enabled` — `augmented` + notify (TTS)
- `full` — `voice-enabled` + mcp + context
- a curated subset — list individual plugin names, e.g. `[workspace, ask-user, web-enabled]`

For a sidecar's own callable methods/tasks (e.g. `ktw-weather:weather-service/*`),
declare them in the task's `tools:` list — they become native typed tools. For
sidecar skills, use `skills:`. Neither is in this table because they ship with
the sidecar, not the agent runtime.
