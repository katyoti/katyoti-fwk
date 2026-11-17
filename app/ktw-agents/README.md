# ktw-agents

The agent runtime + task engine + agent tools. The heavyweight binary —
spawned by `ktw-hub` on first call, hosts every agent run, ships the
default permission profiles + the `delegate` tool + the `complete` tool.

## Composes modules

This binary is a composition of many capability modules:

| Layer | Modules |
|---|---|
| Framework | [framework](../../modules/framework/) (incl. [kts](../../modules/framework/kts/)) |
| Substrate | [bus](../../modules/bus/) (client side), [catalog](../../modules/contributions/) |
| Agent | [agent](../../modules/agent/) + [humangate](../../modules/agent/humangate/) + [ledger](../../modules/agent/ledger/) + [mcp](../../modules/agent/mcp/) + [orchestration](../../modules/agent/orchestration/) |
| Task | [task](../../modules/tasks/) |
| Skills | [skills](../../modules/skills/) |
| Tools | [tools](../../modules/tools/) (file + shell + web) |
| Workspace | [workspace](../../modules/workspace/), [worktree](../../modules/worktree/) |
| Scripts | [scripts](../../modules/scripts/) |
| LLM | [llm-profiles](../../modules/llm-profiles/) |
| Schema | [schema](../../modules/framework/schema/) |
| Display | [display/agent-tools](../../modules/display/agent-tools/), [display/displaybridge](../../modules/display/displaybridge/), [display/terminal](../../modules/display/terminal/), [display/tui](../../modules/display/tui/) |

## Bootstrap

See [cmd/bootstrap.go](cmd/bootstrap.go) for the actual extension wiring.

## Operational notes

- Spawned lazily by the hub's `ConnMgrService.EnsureSpawned` (cold-start
  ~5-15 s; warm calls are bus hops).
- One process per active universe (the hub manages its lifecycle).
- Heaviest startup of any sidecar — sub-second is the goal but ~10 s on
  cold Windows + AV is normal.
