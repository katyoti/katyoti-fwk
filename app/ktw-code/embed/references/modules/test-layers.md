# Test Layers — L1 / L2 / L3

The module-rock-solid method exercises every Use Case at one or more of three layers. This document defines the three layers, when to use each, the existing infrastructure they ride on, and the recursion-gotcha that makes L3 dangerous if used alone.

> **For the concrete authoring procedure** — `.tasktest` schema, fixture conventions, assertion DSL, a worked example, the harness contract — see [`e2e-test-authoring.md`](e2e-test-authoring.md). This doc covers WHAT and WHY each layer is for; the authoring doc covers HOW to write a test that satisfies a DoD.

## At a glance

| Layer | Drives | Speed | Existing infra | When to use |
|---|---|---|---|---|
| **L1** | A single function / state-machine / package | <1s/test | `_test.go` + `go test -race` | Pure logic UCs; permission-resolver math; YAML loader; node parser |
| **L2** | The compiled binary, end-to-end | seconds | `ktw-test` virtual terminal, journal NDJSON, `ktw-utils session` | Black-box behavior; dimension matrix; CLI surfaces; orchestration |
| **L3** | A "stable enough" build orchestrating a candidate build (or vice versa) | seconds-to-minutes | KTSP + delegate + journal | Self-hosted scenarios; agent-driven test scenarios; cross-binary UC |

Every UC must be coverable at **at least one** layer; security UCs are recommended at ≥2 layers.

---

## L1 — Go unit / integration tests

The familiar layer. `go test ./internal/extensions/<module>/... -race`.

### Scope

- Single-package or single-function UCs
- State-machine transitions in isolation
- Pure parsing / loading / validation
- Mode-resolver math (e.g. preset merge, parent-mode coercion)
- Permission-hook decisions given a fixed input
- Mock-friendly UCs (where injecting a fake EventBus or StateStore is cheap)

### What L1 cannot do well

- Cross-binary behavior (the agent runtime, the sidecars, the CLI all in one)
- Real LLM tool-call streams (mocking the LLM is fine for permission paths, dishonest for orchestration)
- Concurrent end-to-end scenarios (you can simulate them, but reproducing the race timing of a real run is fragile)
- Anything that requires the full DI graph wired (the bootstrap is heavy; better to use L2)

### Conventions

- One test file per source file: `foo.go` ↔ `foo_test.go`.
- Test names embed the UC ID: `func TestUC002_CoordinatorEscalation(t *testing.T)`.
- Table-driven tests are encouraged. Each row's name embeds the UC and case: `"UC-002/worker-mode-from-coordinator"`.
- Race detector is non-optional — `-race` runs in CI for any change to this module.
- Time-based tests use a clock interface; `time.Sleep` is forbidden in production code (CLAUDE.md project rule) and discouraged in tests.

### Example

```go
// File: api/engine/presets/resolver_test.go
func TestUC002_CoordinatorForcesSafeSubset(t *testing.T) {
    cases := []struct {
        name        string
        parentMode  string
        childRequest string
        wantSubset   bool
    }{
        {"UC-002/worker-from-coord", "coordinator", "worker", true},
        {"UC-002/teammate-from-coord", "coordinator", "teammate", true},
        {"UC-002/worker-from-worker", "worker", "worker", false},
    }
    for _, c := range cases {
        t.Run(c.name, func(t *testing.T) {
            cfg := configOverrideFromModeWithParent(c.childRequest, c.parentMode)
            gotSubset := equalToolSet(cfg.AllowedTools, presets.SafeToolSubset)
            if gotSubset != c.wantSubset { t.Fatalf(...) }
        })
    }
}
```

### When to NOT use L1

If the UC's DoD says "observed in journal" or "child agent saw permission-denied," you're in L2 territory. L1 can verify the resolver math, but it can't verify what an agent actually receives at runtime. Both layers are valid; pick the one DoD-per-UC requires.

---

## L2 — CLI black-box

The module-rock-solid method's main workhorse. Drives the real binary as a real user would, then asserts on observable evidence (journal events, trace lines, exit codes, output files).

### Scope

- Anything observable from outside the binary
- Dimension × mode combinations (parametrize the same scenario over mode values)
- Workflow YAML execution
- Orchestration (delegate, spawn-teammate, send-message, check-messages)
- Permission enforcement (does a forbidden tool actually fail at runtime?)
- Per-spawn restrictions (worktree paths, time budgets, token limits)
- Background task lifecycle (spawn, poll, kill, notification)
- CLI surface UCs (every `ktw-agents <verb>` should have a UC, even if it's just "exit code is 0")

### Existing infrastructure

The project already has the harness this layer needs:

- **`ktw-test`** — virtual terminal harness (per CLAUDE.md / `ktw-testing` skill). Drives `ktw-agents run <task>` against a controlled `home` directory; intercepts stdin/stdout/stderr; can simulate `ask-user` interactions.
- **`internal/extensions/simulator/`** — **the canonical L2 test driver** (see "Simulator-backed L2 testing" below). Token-free `Machine` strategy that lets you test every dimension property of the engine without invoking an LLM. Part of the `tasks` module (per its charter).
- **NDJSON journal** — every run writes a journal under the session dir. Per-event records with stable schema. The session journal is the assertion target for most L2 tests.
- **`ktw-utils session show / aggregate / tree`** — query journals after the fact. Returns structured data the test can assert on.
- **`--trace` flag** — writes enriched per-tool-call NDJSON when extra detail is needed (LLM text, streaming deltas, tool I/O).

### Simulator-backed L2 testing — the cost-aware default

**A key principle for the rock-solid program**: a task is not bound to an agent. Most of what the engine does (spawn, schedule, route events, enforce permissions, handle workflow control flow) is testable **without ever calling an LLM**. The `simulator` Machine strategy is what makes this possible.

A simulator-backed L2 scenario uses `type: simulator` instead of `type: agent-loop` in the TASK.md:

```yaml
# fixtures/uc-040-worker-mode/coord-task.md
---
type: workflow
description: UC-040 — delegate(mode: worker) is sync
---
config:
  steps:
    - spawn:
        name: child-1
        task:
          type: simulator
          config:
            duration_ms: 100
            output: { ok: true }
        mode: worker          # the dimension under test
    - return:
        result: child-1.result
```

The scenario tests the worker-mode UC end-to-end:

- Real `Spawner.Spawn` runs.
- Real `Channel.Subscribe` blocks on `child.<name>.done`.
- Real journal events emit (`child.started`, `child.done`).
- The simulator child waits 100ms and returns `{ok: true}`.
- The parent resumes; UC-040 passes if the journal shows the expected sequence.

**No LLM tokens consumed. Deterministic. Fast (<1s). Reproducible.**

Use simulator for:

- Every Scheduling UC (UC-040..UC-045)
- Every Permissions UC where the question is "did the hook fire?" (UC-050..UC-058) — the simulator child tries the forbidden tool inside its `output` block, or the assertion is on absence
- Every AgentComms UC (UC-060..UC-067) — orchestration tools work the same regardless of who's behind the Machine
- Every workflow-control UC (UC-020..UC-035) — sequence/parallel/race etc.
- Every Background-task UC (UC-100..UC-104)
- Every Reliability UC once wired (UC-110..UC-113)

Use agent-loop (real LLM) only when the UC **intrinsically** requires LLM behavior — e.g. UC-080 (`ask-user`), or any test that depends on actual tool-call semantics from a real model. These should be a small minority and live in L3 (where token cost is justified by the broader scenario).

### Test format

L2 test cases are **declarative scenarios**. A `.tasktest` file (or a directory of files) defines:

```yaml
# /modules/tasks/tests/L2/uc-002-coord-escalation.tasktest
uc: UC-002
title: Coordinator forces SafeToolSubset on workers

setup:
  home: fixtures/home-with-coord-task/      # contains a coord-task.md TASK.md
  llm: fixtures/llm-script-coord-tries-write.json
                                            # scripted LLM responses (deterministic)

invoke:
  cmd: ./bin/ktw-agents run coord-task --pipe --home <SETUP_HOME>
  timeout: 60s

assertions:
  exit_code: 0
  journal:
    - event: spawn
      child_run_id: ANY
      spawn_permissions.allowed_tools: $SAFE_TOOL_SUBSET
    - event: tool_denied
      tool: file-write             # the LLM tried; permission hook denied
      reason: not_in_allowed_tools
    - event: complete
  journal_must_not_contain:
    - event: tool_call
      tool: file-write             # absolute negative — never executed
```

The harness reads the scenario, runs `invoke`, parses the journal, and checks every assertion. Failures point at the specific assertion + the journal location.

### Conventions

- One scenario file per UC in the trivial case; a directory of scenarios when a UC parametrizes (e.g. UC over the dimension matrix).
- Assertions cite UC IDs in their failure messages: `"UC-002 violation: child has unrestricted tools"`.
- The deterministic-LLM fixture is essential. A real LLM call is non-reproducible; use the simulator (already in `internal/extensions/simulator/`) or scripted JSON.
- Fixtures live under `/modules/<module>/fixtures/`; never inline TASK.md content in the scenario file.

### What L2 cannot do well

- Inter-binary behavior at scale — if a UC requires `ktw-agents` AND a sidecar AND a third binary all running, L2 grows fragile.
- Long-running scenarios where wallclock matters (most L2 timeouts should be <60s).
- Anything that needs a fresh LLM each run (use the simulator).

### Example UC mapping

| UC | Why L2 fits |
|---|---|
| UC-002 (coordinator escalation) | DoD-per-UC checks journal events; L1 alone cannot prove the runtime hook actually fired |
| UC-004 (parallel branch fail_fast cancels siblings) | DoD requires observed `to.<sibling>.cancel` events — only visible in journal |
| UC-010 (delegate background returns task ID immediately) | DoD asserts the parent's `delegate` tool returns a task ID without blocking; only visible at runtime |

---

## L3 — Self-hosted scenario

The most powerful and most dangerous layer. A "stable enough" build of `ktw-agents` runs a scenario that drives a candidate build (or vice versa). The validation evidence is the candidate build's journal, the stable build's report, or both.

### Why it matters

The runtime is recursive: agents can spawn agents, workflows can call other workflows, sidecars expose tools that other agents call. L1 and L2 each see one slice. L3 sees the whole stack interacting with itself.

L3 is the only layer that can verify:

- "An agent can correctly debug another agent's failure"
- "A workflow that uses delegate end-to-end produces a coherent journal"
- "The trace plugin's output is faithful enough that another agent can reconstruct the run"
- "A team of teammates coordinates correctly via the mailbox"

### Existing infrastructure

L3 doesn't need new infrastructure — it composes what's already there:

- **Two binaries on PATH**: a frozen `bin/ktw-agents.stable` (the reference) and the candidate `bin/ktw-agents`.
- **KTSP** for inter-binary communication (already supported as the canonical transport).
- **Journal sharing**: both binaries write to the same session dir; assertions can read either side's journal.
- **The `delegate` tool**: lets one agent kick off work in another binary, returning a task ID that can be polled.

### Two patterns

**Pattern A — Stable orchestrates candidate**:

```
stable build runs:
  TASK.md type: agent-loop
    prompt: "Use delegate to run scenario X against the candidate build,
             then read its journal and verify <DoD-per-UC>."
  →  delegate(target: candidate, task: scenario-X.md)
  →  candidate executes scenario-X
  →  stable reads candidate's journal via ktw-utils session show
  →  stable's agent reports pass/fail with citations
```

The stable agent is the test harness. The candidate is the unit under test. The DoD-per-UC is asserted by the stable agent's prompt.

**Pattern B — Candidate orchestrates stable**:

The inverse: the candidate's orchestrating agent is being tested for *its* ability to drive other agents correctly. The stable build is the work-doer.

Use Pattern A when the candidate is the target of the test (most common). Use Pattern B when "can the candidate orchestrate?" *is* the UC.

### The recursion gotcha

If a bug in the candidate breaks the candidate's journal-emission code, an L3 test that depends on the candidate's journal **cannot** detect that bug — the evidence is missing. Worse: the test might pass because the missing journal events look like "no violation."

**Mitigation**: every UC covered at L3 must be **also** covered at L1 or L2. L3 is augmentation, never substitution. Tier 2 promotion gates this explicitly: L3 alone is never sufficient.

### Conventions

- L3 scenarios live under `/modules/<module>/tests/L3/` as TASK.md files (the candidate or stable agent runs them).
- Pin the stable build to a specific git SHA; record it in the test's frontmatter.
- Assertions are made by the orchestrating agent's prompt, NOT by an external harness — the agent reads the journal and reports.
- Failures must include a reproduction shell command using both binaries.

### Example

```markdown
# /modules/tasks/tests/L3/uc-014-cross-team-mailbox.task.md

---
type: agent-loop
uc: UC-014
title: Cross-team mailbox messages do not leak across teams
stable_build_sha: <pinned>
---

Run the candidate build with two teams of two teammates each. The team-A
coordinator sends a message addressed to a team-B teammate. Read the journal:

- DoD-per-UC: no team-B teammate's mailbox contains the team-A message.
- DoD-per-UC: a `mailbox_rejected` event appears for the cross-team attempt.

Use delegate(mode: background) to launch the candidate; poll task-status
until done; then call ktw-utils session show <run_id> --format json to read
the journal. Verify both assertions and report.
```

### When to NOT use L3

- For a UC fully observable from L2 — L3 is overkill.
- During the bootstrap of a new module — L3 needs a stable build to anchor against; if the module is too young to have a stable, defer L3 until tier 1.
- For UCs that involve real LLM calls in both binaries — non-deterministic, unaffordable, and hard to debug.

---

## UC ↔ layer guidance

| UC kind | Recommended layers |
|---|---|
| Pure-logic / parsers / resolvers | L1 |
| State machines in isolation | L1 + L2 (L2 verifies the integration) |
| Permissions / security / safety guarantees | L1 + L2 (mandatory at ≥2 layers for tier-1) |
| CLI surface / verbs / flags | L2 |
| Dimension × mode matrix | L2 (parametric) |
| Cross-binary orchestration / agent-of-agents | L3 + L1/L2 backup |
| Performance / budget enforcement | L2 (with timing assertions) |
| Failure modes / cancellation / kill | L2 (race-detector under controlled concurrency) |
| Threat-model attack paths | L2 + L3 (red-team agent in L3) |

## Test pass criteria (per layer)

| Layer | Pass = |
|---|---|
| L1 | `go test -race -count=1` exits 0; every UC's table case passes |
| L2 | Every scenario's assertions match the journal; no leaked goroutines/files; harness exit code 0 |
| L3 | Orchestrating agent's report says "DoD met for UC-XXX" with citations; both binaries return clean status |

## Aggregate test results

A single JSON file at `/modules/<module>/results/<date>/results.json`:

```json
{
  "date": "2026-05-06",
  "module": "tasks",
  "git_sha": "abc123…",
  "by_uc": {
    "UC-001": {"L1": "pass", "L2": "pass", "L3": null},
    "UC-002": {"L1": "pass", "L2": "fail", "L3": "pass"},
    "UC-003": {"L1": "pass", "L2": null,   "L3": null}
  },
  "totals": {"pass": 38, "fail": 2, "skip": 7, "uc_total": 47},
  "tier_1_dod_met": false,
  "tier_1_blockers": ["UC-002 L2 failing"]
}
```

The L3 skill reads this to update the charter's status board on every `verify` invocation.
