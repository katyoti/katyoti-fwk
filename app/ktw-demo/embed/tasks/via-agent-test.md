---
name: via-agent-test
description: Tests the agent-as-human channel — delegates via-agent-child in agent_as_human mode and answers its questions as the user.
type: agent
iconName: flask
icon: "🧪"
hashtags: [demo, test, agent-as-human]
category: demo
config:
  capability: smart
  mode: worker
  plugins:
    - standard       # delegate + task-status + complete
    - orchestration  # answer-child + the agent-as-human guidance
---

# Via-Agent Test

Delegate the **via-agent-child** task in **`agent_as_human`** mode (one
`delegate` call with `mode: agent_as_human`). Then act as its user: when it
asks questions, answer with favorite color **blue** and favorite number **7**.
When the child completes, **complete** with its result.

(How to answer its questions is described in the orchestration guidance — you
do not need to manage the mechanics here.)
