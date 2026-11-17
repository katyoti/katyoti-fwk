---
name: test-hung-run
description: "Diagnose a hung agent or task run (no output, journals stop mid-loop) by reading the core's goroutine stacks via the KTW_PPROF pprof endpoint. Load when a run stalls and the journals don't say why."
advertised: false
hashtags: [testing, debug, hang, deadlock, pprof, goroutine]
category: guide
---

# Diagnose a hung run (goroutine stacks)

A hang is the one failure the journals CAN'T explain: there's no event for
"blocked here." The journal just stops — typically on an `llm_response`
with no following `tool_call`/`tool_result`, while the GUI shows the run
still running. To see WHERE it's blocked you read the live process's
goroutine stacks.

## When to reach for this

- A run produces no further output and never completes.
- The journal's last entry is mid-loop (e.g. `llm_response` with
  `stopReason: tool_use`, but no `tool_result` after it).
- The GUI/console shows "running" / "waiting for prompt" but no prompt,
  form, or permission dialog is visible to answer.

## Capture the stacks (KTW_PPROF)

`ktw-agents` exposes `net/http/pprof` on `localhost:<port>` ONLY when the
`KTW_PPROF` env var is set (off by default; see `app/ktw-agents/cmd/root.go`).
The GUI spawns its per-universe core with `os.Environ()`
(`app/ktw-gui/internal/orchestrator/universe_paths.go`), so the var
**inherits from the shell you launch the GUI from** — no GUI flag needed.

1. In the launching terminal (bash), set the port:
   `export KTW_PPROF=6060`
2. Launch the GUI from that same terminal: `katyoti gui` (the installation's;
   `./bin/ktw-gui` belongs to no installation and is refused its hub). Or run
   `ktw-agents` directly — same mechanism.
3. Reproduce the hang.
4. Dump the full goroutine stacks:

```bash
curl -s "http://localhost:6060/debug/pprof/goroutine?debug=2" -o grstack.txt
```

`debug=2` is the full per-goroutine stack; `debug=1` is aggregated counts.
Heap/block/mutex profiles are at the same `/debug/pprof/` root.

## Read the stacks

Find YOUR run's goroutine — the one whose frames include the agent loop
(`agent/internal/runtime`, `task/api/engine.(*Executor).loop`) and a tool
dispatch. Goroutines parked in `[chan receive]` / `[select]` are waiting;
the TOP user frame names the blocking call. Grep to jump there:

```bash
grep -nE "runtime\.|executor\.go|PromptPermission|waitForAnswer|awaitAnswer|WithWritePath|Graph\)|semacquire" grstack.txt
```

## Common culprits (top frame → cause → fix)

| Blocked in | Meaning | Fix |
|---|---|---|
| `applyPermissionGate` → `PromptPermission` → `waitForAnswer` | the kernel permission gate is asking to authorize a mutating tool, but the prompt isn't surfaced for this run | add the tool to the active permission profile's `allow_rules` (e.g. `cwm-object-create:**` in `workspace-edit.yaml`), or run with a profile that allows it |
| `ask_user…`/`humangate` `waitForAnswer` / ask-user-data `awaitAnswer` | the agent is waiting on a user prompt/form that never arrives (no channel, or the UI didn't render it) | ensure the run has a HumanGate / the form is surfaced; check `UserCommsInput` |
| `WithWritePath` / `Graph).Lock` / `semacquire` on a CWM mutex | graph-lock contention or a leaked reader | find the goroutine HOLDING the lock; usually another stuck run |
| a cross-process `sender.Send` / bus call | a method-call / NATS round-trip that never returns | check the target sidecar (ktw-hub, …) is up + not itself blocked |

## Rules

- `KTW_PPROF` is OFF by default — it must be set in the env that launches
  the process; one port per core (parallel universes would collide).
- A hang is NOT in the journals — don't try to debug it from `test-journals`
  alone; capture stacks while it's still hung (they're gone once it's killed).
- Once you know the blocking frame, the fix is usually config/wiring
  (permissions, channel, lock ownership), not the tool itself.
