---
name: l2-sim
type: simulator
description: "L2 harness: a run that works 12 s (one step) then completes. No LLM."
config:
  duration_ms: 12000
  output:
    status: l2-sim-done
---

A working run the harness kills or stops in the middle of its step.
