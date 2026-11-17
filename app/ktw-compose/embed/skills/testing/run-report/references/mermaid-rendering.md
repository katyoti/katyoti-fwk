# Mermaid Delegation Graph Rendering

## Graph Type

Use `graph TD` (top-down) for the delegation tree.

## Node Format

Each node represents one agent in the delegation tree:

```
nodeId["task_name<br/>[mode]<br/>turns: N | tokens: M"]
```

- `nodeId`: use the runId (sanitized — replace `-` with `_`)
- `task_name`: the task name from the session tree
- `mode`: delegation mode (worker, background, fork, coordinator, etc.)
- `turns`: number of LLM turns
- `tokens`: total tokens (input + output)

## Edge Styles

| Delegation mode | Arrow style | Meaning |
|----------------|-------------|---------|
| worker | `-->` (solid) | Synchronous — parent waits for child |
| background | `-.->` (dotted) | Asynchronous — parent continues |
| fork | `==>` (thick) | Clone context — independent execution |
| coordinator | `-->` (solid) | Orchestrator → team members |
| team-leader | `-.->` (dotted) | Team member — parallel work |

## Styling

```mermaid
graph TD
  classDef success fill:#C3E88D,stroke:#333,color:#000
  classDef error fill:#FF5370,stroke:#333,color:#fff
  classDef running fill:#82AAFF,stroke:#333,color:#000

  root_1["goal-planner<br/>[root]<br/>turns: 8 | tokens: 12400"]:::success
  child_1["plan-builder<br/>[worker]<br/>turns: 5 | tokens: 8200"]:::success
  child_2["team-designer<br/>[background]<br/>turns: 3 | tokens: 4100"]:::error

  root_1 --> child_1
  root_1 -.-> child_2
```

## Rules

1. Root node has mode `[root]`
2. Apply `:::success` for completed agents, `:::error` for failed ones
3. Label edges with mode if it differs from default (`worker`)
4. Keep node labels compact — max 3 lines
5. If the tree has >10 agents, group by depth level with subgraphs
