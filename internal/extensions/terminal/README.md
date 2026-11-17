# Terminal extension — the `Terminal` slot

This package publishes the terminal protocol (Room + Translator + HumanGate +
ServeTask) as a bus slot named `Terminal` on ktw-agents. The ktw-agents core
of an active universe hosts it, so a client (typically ktw-gui) runs many
tasks in that universe's core concurrently, each exactly as `ServeTask` runs
it in any other mode.

```
[ktw-gui] ──bus request──► [ktw-agents core of the universe] ──ServeTask──► task
    ▲                                    │
    └──── bus publish on the run's ──────┘
          session subjects
```

## Wire contract

Defined in [api/types.go](api/types.go):

| Direction | Address | Purpose |
|---|---|---|
| request | `ktw-agents:Terminal/start-task` | start a task; returns the `runID` and the journal session id |
| request | `ktw-agents:Terminal/cmd` | forward a raw terminal-protocol command (answer, send_message, cancel_child, ui_event) into the running task |
| request | `ktw-agents:Terminal/cancel` | cancel a run (idempotent) |
| request | `ktw-agents:Terminal/list-sessions` | every session the core's Room holds, running or finished |
| publish | `…session.<sid>.task.<runID>.event.event` | each terminal-protocol event the Translator emits, unchanged |
| publish | `…session.<sid>.task.<runID>.event.done` | `DoneNotification` (`{runID, error, msg}`), exactly once per run |

The two publish subjects are built by `subjects.TaskEventSubject`
(`internal/kts/subjects/subjects.go`); `<sid>` is the bus session
(`StartTaskInput.SessionID`, default `default`). The slot has no generated CLI
(`cli=false`).

## Implementation shape

```
Server side (local/service.go):

  StartTask handler:
    runID = in.RunID, else newRunID()
    journalSessionID = engine.GenerateSessionID(taskName)
    serverCh, clientCh = adapter.NewChannelPair(64)
    runs[runID] = {clientCh, cancel, tenant, universe, busSession}
    go forwardEvents(clientCh → bus.Publish(TaskEventSubject(..., runID, "event")))
    go runTask(ServeTask(runCtx, serverCh, opts))   ← canonical task runner
    return {runID, journalSessionID}

  Cmd handler:
    runs[runID].clientCh.Send(command)              ← forwards into ServeTask

  Cancel handler:
    runs[runID].cancel()

  When ServeTask returns:
    bus.Publish(TaskEventSubject(..., runID, "done"), {runID, error, msg})
    delete runs[runID]
```

## How clients consume it

The pattern ktw-gui uses
([app/ktw-gui/internal/supervisor/terminal_ktsp.go](../../../app/ktw-gui/internal/supervisor/terminal_ktsp.go)):

1. One plain `bus.Subscribe` per universe core on `…session.<sid>.>` (not a
   queue subscription: every window gets every event).
2. Generate the `runID` client-side and register the run BEFORE calling
   `start-task`, so no event arrives unrouted.
3. Demultiplex by the `runID` and kind tokens of the subject to per-run
   channels.
4. Wrap each per-run channel as a `terminal.TerminalTransport`, so the rest of
   the client code is unchanged.

## Tests

```bash
go test ./internal/extensions/terminal/...
```

## See also

- Decision: [how the GUI runs a task in a universe](../../../modules/display/decisions/OD-008-how-does-the-gui-run-a-task-in-a-universe-and.cwm.md)
- Protocol and slot: [modules/display/terminal/concepts.md](../../../modules/display/terminal/concepts.md)
- Canonical task runner: [internal/kts/protocol/terminal/server/serve.go](../../kts/protocol/terminal/server/serve.go)
- Channel transport (used internally): [internal/kts/protocol/terminal/adapter/channel.go](../../kts/protocol/terminal/adapter/channel.go)
- Client adapter: [app/ktw-gui/internal/supervisor/terminal_ktsp.go](../../../app/ktw-gui/internal/supervisor/terminal_ktsp.go)
