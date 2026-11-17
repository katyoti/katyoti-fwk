---
name: gocode-test-author-l1
description: >-
  How the tester persona authors L1 (Go unit) tests for a UC.
  Naming convention, assertion shape, negative DoD assertions for
  security UCs, and the TCID convention that ties the test back to
  its UC.
icon: "🧪"
iconName: test-tube
hashtags: [tester, l1, go-test, ktw-gocode]
category: runbook
provides: [skill]
---

# L1 Test Authoring

This skill walks the tester persona through writing an L1 (Go
unit) test that the verify aggregator can attribute to a UC.

## What an L1 test verifies

- Single-package behavior with deterministic inputs
- Pure functions, state transitions, decode/encode invariants
- Error paths (wrong inputs, edge cases)
- Negative DoD assertions for security UCs (must-not-happen)

**Not in scope for L1**: cross-package integration (that's L2 via
simulator), real-LLM behavior (that's L3), filesystem-heavy I/O
unless contained in `t.TempDir()`.

## Naming convention — the TCID match

The test-case item's `tc_id` body field follows:

```
L1-uc<NNN>-<slug>
```

The Go test function name itself follows Go conventions
(`TestXxx`), but the **test-case item id** uses the TCID format
so the coverage aggregator can join it to UC-NNN.

Example mapping:

```
UC-053 "Spawner.Cancel emits to.<name>.cancel"
  ↓
test-case item id: L1-uc053-spawner-cancel-event
test_ref:          internal/extensions/task/api/engine/spawner_test.go::TestCancel_EmitsCancelEvent
Go test name:      TestCancel_EmitsCancelEvent
```

The Go test name is descriptive (visible in `go test` output);
the TCID is structural (parseable by the matcher).

## Test shape — the canonical structure

```go
func TestCancel_EmitsCancelEvent(t *testing.T) {
    t.Parallel()

    // Arrange — set up the minimum state needed
    bus := engine.NewMemoryEventBus()
    ch := engine.NewChannel(bus)
    spawner := engine.NewLocalSpawner(ch)

    // Act — invoke the unit under test
    h, err := spawner.Spawn(ctx, engine.ChildSpec{...})
    if err != nil { t.Fatalf("Spawn: %v", err) }

    cancelEvent := waitForEvent(t, bus, "to.<name>.cancel", 100*time.Millisecond)
    err = spawner.Cancel(h.RunID, "test reason")
    if err != nil { t.Errorf("Cancel: %v", err) }

    // Assert — verify the observable outcome
    if cancelEvent == nil {
        t.Errorf("Cancel did not emit to.<name>.cancel event within 100ms")
    }
    if cancelEvent.Topic != "to.spawner-test.cancel" {
        t.Errorf("event topic = %q, want to.spawner-test.cancel", cancelEvent.Topic)
    }
}
```

## Hard rules

- `t.Parallel()` unless the test mutates global state or uses
  `t.Setenv` (those serialize per Go's testing rules)
- `t.Helper()` on test-helpers so failures point at the call site
  not the helper
- `t.TempDir()` for any filesystem fixture; never write outside
- `context.Background()` for the root context; `WithTimeout` when
  the unit-under-test has long-running ops
- `t.Cleanup` for resource teardown (preferred over `defer` —
  runs even on test panic)

## Negative DoD — required for security UCs

When the UC is severity `security`, the L1 test MUST include a
**must-not-happen** assertion. Pattern:

```go
// Negative DoD: the spawn-permission-hook MUST deny `bash` when
// DisallowedTools contains it. Verify the journal does not contain
// a successful bash tool-call.

ran := runAgentLoopWith(t, plugins, /* DisallowedTools: ["bash"] */)
if foundBashCall(ran.Journal) {
    t.Error("bash tool-call appeared in journal despite being disallowed")
}
```

Mark the test-case item with `negative: true` in its body. The
lint detector flags security UCs whose test-cases don't carry a
negative DoD.

## What NOT to do

- ❌ Skip `t.Parallel()` "just in case" — tests share no state
  unless declared; default to parallel
- ❌ Use `time.Sleep` for synchronization — use `bus.Subscribe`
  with timeout or `sync.WaitGroup` instead
- ❌ Hard-code absolute paths — `t.TempDir()` + relative
- ❌ Assert on log strings — assert on the actual state change /
  event / return value
- ❌ Write `last_run_status` on the test-case object from inside
  the test — that's `verify`'s job (the test just runs)

## After authoring

1. Create the `test-case` object via `cwm-object-create`, as a child of
   the use-case it verifies, in that use-case's module:

   ```yaml
   module: <the UC's module>
   object:
     id: <the UC's module>.L1-uc053-spawner-cancel-event
     type: test-case
     name: L1-uc053-spawner-cancel-event
     parent: <the UC-053 object id>
     body:
       id: L1-uc053-spawner-cancel-event
       layer: L1
       test_ref: internal/extensions/task/api/engine/spawner_test.go::TestCancel_EmitsCancelEvent
       last_run_status: not-run        # verify will update
           negative: false                  # true if this is a negative DoD test
   ```

2. Run `go test -run '^TestCancel_EmitsCancelEvent$' ./internal/extensions/task/api/engine/...`
   to confirm it passes locally.

3. Run `module.verify --uc UC-053 --layer L1` — verify will
   record the result in a test-run object + update UC-053's
   `status_cache` via UCSetStatus.

## References

- Concepts.md §9 — coverage state machine + 4-verdict projection
- Companion skills: `gocode-test-author-l2`, `gocode-test-author-l3`
