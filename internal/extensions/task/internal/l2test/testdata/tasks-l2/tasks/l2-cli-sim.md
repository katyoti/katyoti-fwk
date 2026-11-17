---
name: l2-cli-sim
type: simulator
description: "L2 harness: a run a CLI starts and quits; it would work 90 s. No LLM."
config:
  duration_ms: 90000
  output:
    status: l2-cli-sim-done
---

The run `ktw-agents run` starts and a Ctrl+C quits.
