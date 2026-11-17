---
name: demo-delegate-parallel-fallback
type: workflow
description: "Parallel branches with on_error.fallback (zero LLM cost — pure simulator)"
iconName: git-branch
icon: "🔀"
hashtags: [demo, delegate]
category: demo
config:
  steps:
    - name: fanout
      kind: parallel
      on_branch_error: collect
      branches:
        - name: branch-a-success
          task:
            type: simulator
            duration_ms: 100
            output:
              source: "A"
              data: "branch A finished"
        - name: branch-b-success
          task:
            type: simulator
            duration_ms: 150
            output:
              source: "B"
              data: "branch B finished"
        - name: branch-c-with-fallback
          task:
            type: simulator
            duration_ms: 100
            fail_probability: 1.0
            fail_message: "branch C primary path always fails"
          on_error:
            fallback:
              name: branch-c-fallback
              task:
                type: simulator
                duration_ms: 50
                output:
                  source: "C-backup"
                  data: "branch C recovered via fallback"
    - kind: return
      value: "${results}"
---

# Demo: delegate-parallel-fallback

**Pattern**: workflow `kind: parallel` with three concurrent branches, where branch C's
primary task always fails and recovers via `on_error.fallback`.

**What to watch in the journal / GUI Team tab**:
- Three `child.<branch>.started` events fire roughly simultaneously
- `child.branch-c-with-fallback.failed` fires (primary failure)
- `child.branch-c-fallback.started` then `child.branch-c-fallback.done` (recovery)
- `workflow.<id>.done` with results map containing all three branches

**Cost**: $0 — pure simulator, no LLM calls. Useful as a smoke test for the
workflow engine, the parallel primitive, and the fallback strategy.
