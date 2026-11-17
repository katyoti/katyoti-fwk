---
name: l2-wf
type: workflow
description: "L2 harness: a workflow of two steps — a short one, then one that works 12 s. No LLM."
config:
  steps:
    - name: first
      task: { type: simulator, duration_ms: 300, output: { a: 1 } }
    - name: second
      task: { type: simulator, duration_ms: 12000, output: { b: 2 } }
    - kind: return
      value: "${results.second}"
---

A workflow the harness kills while its second step works.
