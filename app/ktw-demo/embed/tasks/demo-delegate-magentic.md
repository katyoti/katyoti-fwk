---
name: demo-delegate-magentic
type: workflow
description: "Magentic: manager + 2 specialists coordinate via a shared ledger machine"
iconName: book-2
icon: "📒"
hashtags: [demo, delegate]
category: demo
config:
  steps:
    - name: magentic-team
      kind: parallel
      on_branch_error: collect
      branches:
        - name: shared-ledger
          task:
            type: ledger
            id: "magentic-demo"
            initial:
              status: "open"
              plan:
                step1: { task: "name a color", assigned_to: null, result: null }
                step2: { task: "name a shape", assigned_to: null, result: null }

        - name: manager
          task:
            type: task
            capability: fast
            plugins: [standard, ledger-manager]
            prompt: |
              You are the **manager** in the Magentic pattern. The shared ledger ID is `magentic-demo`.

              Begin by saying "starting manager".

              **Phase 1 — read + assign tasks**:
              1. Call `ledger-snapshot` with `ledger_id: "magentic-demo"` to read the current plan. Note the `version` returned.
              2. Update the ledger to assign tasks. Use `ledger-update` with patch:
                 `{ "plan": { "step1": { "assigned_to": "alice" }, "step2": { "assigned_to": "bob" } } }`

              **Phase 2 — wait for completion (event-driven, NOT polling)**:
              3. Loop using `ledger-wait-update` (NOT `ledger-snapshot`):
                 - Call `ledger-wait-update` with `ledger_id: "magentic-demo"` and `since_version` set to the latest version you've seen. This BLOCKS until a newer snapshot arrives (or 30s timeout).
                 - Inspect the returned state: if both `plan.step1.result` and `plan.step2.result` are non-null, you're done — exit the loop.
                 - Otherwise, update `since_version` to the returned `version` and call `ledger-wait-update` again.
                 **Never** loop `ledger-snapshot` with the same args — the agent loop will abort you for repeat tool calls. Each `ledger-wait-update` call has a higher `since_version` and so is unique.
              4. Once both results appear, call `ledger-update` with patch `{ "status": "done" }`.

              **Phase 3 — terminate the ledger**:
              5. Call `ledger-complete` with `ledger_id: "magentic-demo"` so the workflow can finalize. (If you forget, the safety net fires it on your behalf when you return.)

              End by saying "finished manager", then call `complete` with a summary like:
              `"Plan complete. step1 (alice): <result>. step2 (bob): <result>."`

        - name: alice
          task:
            type: task
            capability: fast
            plugins: [standard, ledger-worker]
            prompt: |
              You are **alice**, a specialist in the Magentic pattern. The shared ledger ID is `magentic-demo`.

              Begin by saying "starting alice".

              **Wait for your assignment (event-driven, NOT polling)**:
              1. Call `ledger-wait-update` with `ledger_id: "magentic-demo"` and `since_version: 0`.
                 This BLOCKS until the manager assigns tasks (or 30s timeout).
              2. Look at the returned state. Find the step in `plan` whose `assigned_to` is `"alice"`. Read its `task` description.
                 - If your assignment isn't there yet, call `ledger-wait-update` again with `since_version` set to the version you just received.
                 - **Never** loop `ledger-snapshot` with the same args — the agent loop will abort you for repeat tool calls.
              3. Once you find your assignment, do the trivial task (e.g., name one color) and post the result back to the ledger via:
                 `ledger-update` with patch `{ "plan": { "step1": { "result": "<your answer>" } } }`
                 (replace `step1` with whichever step the manager assigned to you).

              End by saying "finished alice", then call `complete` with the result you posted.

        - name: bob
          task:
            type: task
            capability: fast
            plugins: [standard, ledger-worker]
            prompt: |
              You are **bob**, a specialist in the Magentic pattern. The shared ledger ID is `magentic-demo`.

              Begin by saying "starting bob".

              **Wait for your assignment (event-driven, NOT polling)**:
              1. Call `ledger-wait-update` with `ledger_id: "magentic-demo"` and `since_version: 0`.
                 This BLOCKS until the manager assigns tasks (or 30s timeout).
              2. Look at the returned state. Find the step in `plan` whose `assigned_to` is `"bob"`. Read its `task` description.
                 - If your assignment isn't there yet, call `ledger-wait-update` again with `since_version` set to the version you just received.
                 - **Never** loop `ledger-snapshot` with the same args — the agent loop will abort you for repeat tool calls.
              3. Once you find your assignment, do the trivial task (e.g., name one shape) and post the result back to the ledger via:
                 `ledger-update` with patch `{ "plan": { "step2": { "result": "<your answer>" } } }`
                 (replace `step2` with whichever step the manager assigned to you).

              End by saying "finished bob", then call `complete` with the result you posted.

    - kind: return
      value: "${results['magentic-team'].manager}"
---

# Demo: delegate-magentic

**Pattern**: workflow `kind: parallel` running 4 children — one `ledger` machine
holding the shared plan + status, plus a manager agent and two specialist agents
(`alice`, `bob`) that read and update the ledger.

**Plugin roles** (split for lifecycle clarity):
- Manager has `ledger-manager` → snapshot + update + **`ledger-complete`** (terminates the ledger when coordination is done). Plus an OnResult/OnAbort safety hook that auto-fires `ledger-complete` if the manager returned without calling it explicitly — so the workflow can never hang on a forgotten teardown.
- Specialists have `ledger-worker` → snapshot + update only. They cannot tear down the shared state.

The ledger is the **first-class shared state**. Coordination happens via
patches and snapshots, not via direct messages between agents.

**Topology**:
```
workflow
├── shared-ledger  (ledger machine, topic id = "magentic-demo")
├── manager        (assigns work, polls for completion, marks status: done)
├── alice          (waits for assignment, posts result)
└── bob            (waits for assignment, posts result)
```

**What to watch in the journal / GUI Team tab**:
- 4 child sessions appear concurrently
- `ledger.magentic-demo.snapshot` events fire after each `ledger-update`
  - version 0 = initial; bumps each time anyone publishes
- `ledger.magentic-demo.update` events from manager, alice, bob
- Final `workflow.<id>.done` once the manager calls `complete`

**Cost**: ~25K tokens (~$0.03): supervising-best manager + two fast-best
specialists. Each agent loops a few turns polling the ledger.

**Note**: agents coordinate via `ledger-wait-update` (event-driven blocking
wait), NOT by polling `ledger-snapshot` in a loop. Looped `ledger-snapshot`
calls with the same arguments trigger the agent loop's repeat-tool-call
guard and abort the agent. The wait-update tool monotonically increases
its `since_version` argument across calls, sidestepping the guard while
also being faster, cheaper, and simpler. Use `ledger-snapshot` only for
"give me the current state right now" — never for "wait until something
changes".
