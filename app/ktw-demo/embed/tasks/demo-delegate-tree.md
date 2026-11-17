---
name: demo-delegate-tree
type: workflow
description: "Multi-level, multi-mode delegation tree — pure simulators, zero LLM cost. Coordinator root fans out to worker/flaky branches and a nested sub-team workflow, then runs a background delegate and joins it. THE fixture for the Team tab / session view and for asserting spawn dimensions end-to-end."
iconName: sitemap
icon: "🌳"
hashtags: [demo, delegate]
category: demo
config:
  # The workflow's OWN mode: every spawn below sees ParentMode=coordinator,
  # so the sticky coordinator-restricts-children policy applies to children
  # AND (via spawnPolicies serialization through the nested level) to
  # grandchildren. Leaves record __spawn_dimensions to make that assertable.
  _mode: coordinator
  steps:
    - name: fan-out
      kind: parallel
      on_branch_error: collect
      branches:
        - name: worker-a
          mode: worker
          task:
            type: simulator
            duration_ms: 200
            record_context: true
            output:
              role: "worker-a"
              report: "fast analysis done"
        - name: flaky-b
          mode: worker
          task:
            type: simulator
            duration_ms: 100
            fail_probability: 1.0
            fail_message: "flaky-b always fails (injected)"
          on_error:
            fallback:
              name: flaky-b-recovery
              task:
                type: simulator
                duration_ms: 50
                record_context: true
                output:
                  role: "flaky-b-recovery"
                  report: "recovered via fallback"
        - name: sub-team
          mode: worker
          task:
            type: workflow
            _mode: worker
            steps:
              - name: grandchild-a
                task:
                  type: simulator
                  duration_ms: 150
                  record_context: true
                  output:
                    role: "grandchild-a"
                    report: "level-2 work done"
              - name: grandchild-b
                task:
                  type: simulator
                  duration_ms: 100
                  record_context: true
                  output:
                    role: "grandchild-b"
                    report: "level-2 follow-up done"
              - kind: return
                value: "${results}"
    - name: bg-report
      mode: background
      task:
        type: simulator
        duration_ms: 300
        record_context: true
        output:
          role: "bg-report"
          report: "background digest ready"
    - name: while-bg-runs
      task:
        type: simulator
        duration_ms: 50
        output:
          role: "while-bg-runs"
          report: "foreground work overlapping the background child"
    - kind: wait
      for_event: "child.bg-report.done"
      timeout_ms: 10000
    - kind: return
      value: "${results}"
---

# Demo: delegate-tree

**Pattern**: a full multi-level, multi-mode delegation tree with zero LLM cost —
the canonical simulation of "a long-running task that delegates with multiple
modes to sub-tasks".

```
coordinator root (workflow, _mode: coordinator)
├── worker-a        (parallel branch, mode: worker)
├── flaky-b         (parallel branch — always fails, recovers via fallback)
├── sub-team        (parallel branch — NESTED workflow, _mode: worker)
│   ├── grandchild-a
│   └── grandchild-b
├── bg-report       (sequence-level delegate, mode: background — immediate handle)
├── while-bg-runs   (foreground work overlapping the background child)
└── wait child.bg-report.done  (the background JOIN pattern)
```

**What to watch in the journal / GUI Team tab**:
- `child.<name>.started/done/failed` at BOTH levels (sub-team's grandchildren too)
- `child.flaky-b.failed` then the fallback recovery
- `results.bg-report` is `{run_id, background: true}` — the workflow did NOT
  suspend on it; `while-bg-runs` overlaps it; the `wait` joins it
- Every `record_context: true` leaf's output carries `__spawn_dimensions` —
  under the coordinator root, children AND grandchildren show the restricted
  tool set (sticky-policy propagation, UC-053/054)

**Cost**: $0 — pure simulators. Use it to exercise the Team tab, the session
view, dimension assertions, and the engine's concurrency/join machinery.
