# E2E Test Authoring — UC → DoD → `.tasktest` → Passing Verdict

How to actually write the tests that the charter's DoD-per-UC bodies reference. This doc fills the gap between [`test-layers.md`](test-layers.md) (the conceptual L1/L2/L3 layers) and the real test files. Read this before authoring any L2 scenario or L3 self-hosted scenario.

L1 (Go unit tests) is well-trodden territory and uses standard `_test.go` conventions; this doc gives it brief coverage but focuses on L2 (the new layer where `.tasktest` files and the simulator live) and L3 (cross-binary scenarios).

## The lifecycle of a test

```
┌───────────────────────────────────────────────────────────────────┐
│  Specify phase (charter authoring)                                │
│  Output: DoD-per-UC body says exactly what evidence proves the UC │
│          passes — at L1, L2, L3.                                  │
│          Marks "(gap)" for tests that don't exist yet.            │
└────────────────────────┬──────────────────────────────────────────┘
                         │
                         ▼
┌───────────────────────────────────────────────────────────────────┐
│  Plan-tier phase                                                  │
│  Each "(gap)" marker becomes a task in the milestone.             │
│  Tasks are batched by category (engine / workflow / dimensions /  │
│  L2-scenarios / L3-scenarios).                                    │
└────────────────────────┬──────────────────────────────────────────┘
                         │
                         ▼
┌───────────────────────────────────────────────────────────────────┐
│  Execute phase (per-task)                                         │
│  Author writes:                                                   │
│    - Fixtures (TASK.md files, scripted-LLM responses, sample dirs)│
│    - The test itself (Go _test.go file or .tasktest scenario)     │
│    - Negative DoD assertions for security UCs                     │
│  Commits the test alongside any code changes the UC required.    │
└────────────────────────┬──────────────────────────────────────────┘
                         │
                         ▼
┌───────────────────────────────────────────────────────────────────┐
│  Verify phase                                                     │
│  `code-module verify <module>` runs all layers; produces per-UC   │
│  green/red board. Failed UCs go back to Plan-tier.                │
└───────────────────────────────────────────────────────────────────┘
```

The DoD-per-UC body **is the test design**. The Execute phase implements it. There is no separate "test design document" per UC.

## Layer selection: which layer for which UC

| UC kind | Recommended layer | Why |
|---|---|---|
| Pure logic (parsers, resolvers, validators) | **L1** | Fast, deterministic, no setup |
| State machine in isolation | **L1** | Mock the surrounding plumbing; assert transitions |
| Engine primitives (Channel, Spawner, Inbox, EventBus, StateStore) | **L1** | Test interfaces directly; race detector mandatory |
| Permission / security gates given a fixed input | **L1** | Hook math; table-driven |
| Workflow YAML loading | **L1** | YAML in → Node tree out |
| End-to-end behavior the LLM-driven user sees | **L2** | Runtime + journal + assertion |
| Dimension × mode combination working at runtime | **L2** | Parametrize over the matrix |
| Cross-binary orchestration (delegate spawning a sidecar) | **L3** | The only layer that exercises real KTSP |
| "An agent can debug another agent's failure" | **L3** | Self-hosting scenario |

Most UCs need **at least one layer**. Security UCs need **at least two** (per charter Tier-1 DoD).

## Fixture conventions

Inputs (TASK.md content, scripted-LLM responses, sample home dirs) live under:

```
/modules/<module>/fixtures/
├── home-with-coord-task/                    sample home dir
│   ├── tasks/
│   │   └── coord.md                         a TASK.md
│   └── config/
│       └── ...
├── llm-script-coord-tries-write.json        deterministic LLM response sequence
├── workflow-parallel-fail-fast.md           an inline workflow TASK.md
└── ...
```

Naming rules:

- **Descriptive** (`home-with-coord-task/`, not `home1/`)
- **Immutable**: once a fixture is referenced by a test, **don't edit it in place**. Copy + version: `llm-script-coord-tries-write-v2.json`.
- **No secrets**: fixtures are checked in; never put real credentials.
- **One fixture, many tests** is OK (and encouraged) — composable.

A fixture is **deleted** only after `code-module lint` confirms zero references.

## L1 — Go `_test.go` conventions

Standard Go testing, with three rock-solid-method-specific rules:

1. **Test name embeds UC ID**: `func TestUC053_CoordinatorRestrictsViaWorkflow(t *testing.T)` or `t.Run("UC-053/coord-via-workflow", ...)` for table cases.
2. **Race detector is mandatory** for any test of concurrent code: `go test -race`.
3. **Time-based tests use a fake clock**, never `time.Sleep` in production code.

Example:

```go
// File: api/engine/presets/resolver_test.go

func TestUC053_CoordinatorForcesSafeSubset(t *testing.T) {
    cases := []struct {
        name         string
        parentMode   string
        childRequest string
        wantSubset   bool
    }{
        {"UC-053/worker-from-coord", "coordinator", "worker", true},
        {"UC-053/teammate-from-coord", "coordinator", "teammate", true},
        {"UC-053/worker-from-worker", "worker", "worker", false},
    }
    for _, c := range cases {
        t.Run(c.name, func(t *testing.T) {
            cfg := configOverrideFromModeWithParent(c.childRequest, c.parentMode)
            gotSubset := equalToolSet(cfg.AllowedTools, presets.SafeToolSubset)
            if gotSubset != c.wantSubset {
                t.Fatalf("UC-053: parent=%s child=%s: got subset=%v, want %v",
                    c.parentMode, c.childRequest, gotSubset, c.wantSubset)
            }
        })
    }
}
```

The DoD-per-UC body in the charter cites this test by name. `code-module verify` runs it via `go test -run ^TestUC053_CoordinatorForcesSafeSubset$`.

## L2 — `.tasktest` scenarios

The new layer. Each `.tasktest` file is a declarative scenario the harness reads, runs, and asserts on. **Most use `type: simulator` for token-free determinism** (per the lessons doc's L2-simulator-default rule).

### File layout

```
/modules/<module>/tests/L2/
├── _template.tasktest                  reference template
├── scheduling/
│   ├── uc-040-worker-sync.tasktest
│   ├── uc-041-background-async.tasktest
│   └── ...
├── permissions/
│   └── ...
└── workflow/
    └── ...
```

Subfolders by UC category. One `.tasktest` per UC (or per UC parametrization).

### `.tasktest` schema

```yaml
# Frontmatter (all tests)
uc: UC-053                              required — stable UC ID
title: Coordinator restricts via workflow path
description: |                          optional human-readable
  Verifies that a workflow-spawned child of a coordinator parent
  receives SafeToolSubset, closing sec-1 / threat-T1.
severity: security                      copies from charter
audit_finding: sec-1                    optional — links back

# Setup — what the harness creates before invoking
setup:
  home: fixtures/home-with-coord-task/  copied to a temp dir
  llm: fixtures/llm-script-coord-tries-write.json
                                        scripted LLM responses (or "simulator")
  env:
    KTW_HOME: $TMP_HOME

# Invoke — the command the harness runs
invoke:
  cmd: ./bin/ktw-agents run coord-task --pipe --home $TMP_HOME
  timeout: 60s
  expect_exit: 0                        or 1 / "non-zero" / etc.

# Assertions — evaluated against the run's journal + outputs
assertions:
  # Positive: must contain
  journal:
    - event: spawn
      child_run_id: "any"
      spawn_permissions.allowed_tools: $SAFE_TOOL_SUBSET   # variable resolved by harness
    - event: tool_denied
      tool: file-write
      reason: not_in_allowed_tools
    - event: complete

  # Negative: must NOT contain (security UCs must include this)
  journal_must_not_contain:
    - event: tool_call
      tool: file-write                   # absolute negative — never executed

  # File system / output
  output:
    stdout_contains: "completed"
    stderr_not_contains: "panic:"
    files_created: []                    optional list of paths the test expects

# Per-UC verdict mapping
verdict_on_pass: pass
verdict_on_fail: fail                   or "fail-deferred" if this UC is OD-deferred
deferred_by: null                       set to OD-NNN if verdict is fail-deferred
```

### Frontmatter fields — reference

| Field | Type | Required | Purpose |
|---|---|---|---|
| `uc` | string | yes | UC ID; must exist in charter |
| `title` | string | yes | Human-readable name |
| `description` | string (multi-line) | no | What and why |
| `severity` | enum | no | `core` / `security` / `qol`; copies from charter |
| `audit_finding` | string | no | Links to a finding ID (e.g. `sec-1`, `threat-T2`) |
| `setup.home` | path | yes | Fixture home dir to copy |
| `setup.llm` | `simulator` \| path | yes | Either the literal string `simulator` (test uses `type: simulator` tasks) OR a path to a scripted-LLM response file |
| `setup.env` | map | no | Extra env vars |
| `invoke.cmd` | string | yes | Shell command; supports `$TMP_HOME` and other harness-provided variables |
| `invoke.timeout` | duration | yes | Hard wall-clock cap |
| `invoke.expect_exit` | int \| string | yes | Expected exit code; `non-zero` means any error code |
| `assertions.journal` | list | no | Events that must appear (in order if `ordered: true`) |
| `assertions.journal_must_not_contain` | list | no | Events that must NOT appear |
| `assertions.output.stdout_contains` | string | no | Substring match on stdout |
| `assertions.output.stderr_not_contains` | string | no | Negative substring match on stderr |
| `assertions.output.files_created` | list of paths | no | Files expected to exist post-run |
| `verdict_on_pass` | `pass` | no | Default: `pass` |
| `verdict_on_fail` | `fail` \| `fail-deferred` | no | Default: `fail`. Use `fail-deferred` for known-deferred UCs (UC-073, UC-094) |
| `deferred_by` | OD-ID | no | Required when `verdict_on_fail: fail-deferred` |

### Assertion DSL — what each kind asserts

#### `assertions.journal`

A list of event matchers. Each matcher specifies which event payload fields must match:

```yaml
journal:
  - event: spawn                                # event type
    child_run_id: any                           # accept any value
    spawn_permissions.allowed_tools: $SAFE_TOOL_SUBSET
                                                # exact match against variable
  - event: tool_denied
    tool: file-write
    reason: not_in_allowed_tools
```

Match semantics:
- **`any`** — wildcard for that field
- **`$VARIABLE`** — resolved from the harness's variable scope (e.g. `$SAFE_TOOL_SUBSET` is the well-known list)
- **Literal value** — exact match (string, number, bool, list)
- **`{contains: X}`** — for strings, substring match; for lists, list contains the item
- **`{regex: ^foo}`** — regex match (string fields only)
- **`{exists: true|false}`** — field presence check

By default, journal events can appear **in any order**. Add `ordered: true` at the top of the list to require strict ordering.

#### `assertions.journal_must_not_contain`

Same matcher format. Hard negative. **Required for every security UC** per the negative-DoD rule.

#### `assertions.output.*`

Plain substring matches on stdout/stderr; plain path checks on filesystem.

### Variables the harness provides

| Variable | Meaning |
|---|---|
| `$TMP_HOME` | Temp dir for this run; cleaned up at end |
| `$SAFE_TOOL_SUBSET` | The list `[file-read, file-write, file-edit, glob, complete]` (from `presets.SafeToolSubset`) |
| `$RUN_ID` | The run's `runID` if needed for assertions |
| `$NOW` | Current time at scenario start (for time-based assertions) |
| `$SCENARIO_DIR` | The directory containing the `.tasktest` file (for relative paths) |

Harness-provided variables make scenarios portable across environments.

## A worked example — UC-053 end-to-end

The DoD-per-UC body in the charter says:

> **UC-053: Coordinator parent applies SafeToolSubset spawn policy to children**
>
> - L1: `taskplugin/mode_overrides_test.go::TestCoordinator_RestrictsWorker` (exists). Verify `configOverrideFromModeWithParent("worker", "coordinator")` returns config with `AllowedTools = SafeToolSubset` and `PermissionMode = restricted`.
> - L2: `tests/L2/uc-053-coord-restricts-via-delegate.tasktest` — coord parent simulator delegates(mode: worker) to a simulator child; assert child's effective config has SafeSubset.
> - L2 (workflow path): `tests/L2/uc-053-coord-restricts-via-workflow.tasktest` — coord parent, workflow child with type: simulator. **Currently FAILS** (per sec-1 / threat-T1) because `LocalSpawner.Spawn` bypasses mode-coercion. UC-053 close requires this to pass.

### Step 1 — Author the L1 test (already exists)

The L1 test lives at `internal/extensions/task/internal/taskplugin/mode_overrides_test.go::TestCoordinator_RestrictsWorker`. The DoD says what it asserts; the test already does it.

### Step 2 — Author L2 fixture: a coordinator TASK.md that uses workflow

```markdown
<!-- /modules/tasks/fixtures/coord-with-workflow-child/tasks/coord.md -->
---
type: workflow
description: UC-053 — coordinator with workflow-spawned child
mode: coordinator
---
config:
  steps:
    - spawn:
        name: child-1
        task:
          type: simulator
          config:
            duration_ms: 50
            output: { ok: true }
        # Note: workflow path; mode_overrides historically skipped here (sec-1)
    - return:
        result: child-1.result
```

### Step 3 — Author L2 scenario

```yaml
# /modules/tasks/tests/L2/permissions/uc-053-coord-via-workflow.tasktest
uc: UC-053
title: Coordinator restricts workflow-spawned child to SafeToolSubset
description: |
  The known-broken case from sec-1 / threat-T1. LocalSpawner.Spawn currently
  bypasses configOverrideFromModeWithParent on the workflow path. Fix lands
  in tier-1-spawn-policies-refactor.
severity: security
audit_finding: sec-1

setup:
  home: fixtures/coord-with-workflow-child/
  llm: simulator                              # no LLM needed; tasks are simulator type
  env:
    KTW_HOME: $TMP_HOME

invoke:
  cmd: ./bin/ktw-agents run coord --pipe --home $TMP_HOME
  timeout: 30s
  expect_exit: 0

assertions:
  journal:
    # Must observe: child spawned with SafeSubset applied
    - event: child_started
      name: child-1
      spawn_permissions.allowed_tools: $SAFE_TOOL_SUBSET
      spawn_permissions.permission_mode: restricted
    - event: child_done
      name: child-1
    - event: workflow_done

  journal_must_not_contain:
    # Negative: child MUST NOT receive unrestricted tools
    - event: child_started
      name: child-1
      spawn_permissions.allowed_tools: ["*"]   # the unrestricted form
    - event: child_started
      name: child-1
      spawn_permissions.permission_mode: default
```

### Step 4 — Run the scenario

The runner (today: hand-invoked; tomorrow: `code-module verify`):

```
$ ktw-test run /modules/tasks/tests/L2/permissions/uc-053-coord-via-workflow.tasktest
```

### Step 5 — Until tier-1-spawn-policies-refactor lands, this UC verifies as 🔴

That's the point. The test is the gate. When the fix lands, the same `.tasktest` flips to 🟢 with no edits.

## L3 — Self-hosted scenarios

L3 scenarios live as TASK.md files an orchestrating agent runs. They look like:

```markdown
<!-- /modules/tasks/tests/L3/uc-014-cross-binary-spawn.task.md -->
---
type: agent-loop
uc: UC-014
title: Cross-binary spawn end-to-end
stable_build_sha: <pinned>
---

Run the candidate `ktw-agents` with a workflow that spawns a sidecar via
`delegate(target: ktw-docs, ...)`. After completion:

1. Read the candidate's journal via `ktw-utils session show <run_id> --format json`.
2. Verify the journal contains:
   - `child_started` event with `target: ktw-docs`
   - The `ktw-docs` invocation logged in its own journal
3. Report `DoD met for UC-014` with citations, OR list violations.
```

The orchestrating agent IS the assertion. It reads journals and reports. L3 results feed back into `code-module verify` via the agent's report.

**Recursion gotcha** (per [test-layers.md](test-layers.md)): a UC covered at L3 must also be covered at L1 or L2. L3 is augmentation, never substitution.

## The harness contract

`ktw-test` (the existing virtual-terminal harness) is the L2 runner. The contract:

| Input | Source |
|---|---|
| The `.tasktest` file | path argument |
| The candidate binary | `./bin/ktw-agents` (or path argument) |
| Fixtures | `setup.home` resolved relative to `.tasktest` location |
| LLM script | `setup.llm` (literal `simulator` or path to JSON) |

| Output | Form |
|---|---|
| Per-assertion result | `{matcher, status: pass/fail, journal_event_index, reason}` |
| Overall verdict | `pass | fail | fail-deferred | error` (error = harness itself broke) |
| Run journal | preserved at `$TMP_HOME/sessions/<runID>/journal.ndjson` for debugging |
| Stdout/stderr | captured |

The runner is **stateless** — every run is fresh `$TMP_HOME`.

`code-module verify` aggregates per-`.tasktest` results into the per-UC table per the `verify` verb reference ([`verbs/verify.md`](verbs/verify.md)).

### Today's state (Phase A)

The harness exists for stdin/stdout interception (per CLAUDE.md memory: `ktw-test` virtual terminal harness). It does **not yet** natively understand `.tasktest` files — that's Phase B/C work. For now:

1. A test author writes a `.tasktest` per the schema above.
2. The author manually:
   - Copies fixtures to a temp dir
   - Runs `./bin/ktw-agents run <task> --home <tmp>`
   - Reads the journal at `<tmp>/sessions/<runID>/journal.ndjson`
   - Checks assertions by eye / grep / `jq`
3. When `code-module verify` Go-side ships, it parses `.tasktest`, drives the harness, evaluates assertions, produces the per-UC verdict.

The schema documented here is forward-compatible: scenarios written today will run unchanged when the runner ships.

## How to add a new UC's test from scratch

Step-by-step procedure. Use this as a checklist when working a `(gap)` task:

1. **Read the UC's DoD-per-UC body** in the charter. It tells you the layer(s), assertion shape, and any negative DoD.
2. **Pick a layer** if the DoD didn't specify one (most do; some leave it to the test author).
3. **Identify or create fixtures**:
   - L1: usually inline structs / values
   - L2: TASK.md(s), scripted-LLM JSON, sample home dir
4. **Write the test**:
   - L1: Go `_test.go` with `TestUC<N>_<Description>` naming
   - L2: `.tasktest` per the schema above (uc / setup / invoke / assertions)
   - L3: TASK.md for the orchestrating agent
5. **Add a row** to the charter's Test plan referencing the new test (or update the existing row from `(gap)` to `<test name>`).
6. **Run it manually first** to confirm it produces the expected verdict (pass for "feature works" UCs; deferred-fail for OD-deferred UCs).
7. **For security UCs**: confirm the negative DoD assertion fires when an attempt is made.
8. **Commit** the test alongside any code change the UC required.
9. **Update charter status board** (UCs passing count). Tier-1 DoD progresses.

## Common patterns

### Scenario over a parametric matrix

Some UCs (e.g. UC-040..UC-045 scheduling) parametrize over many config values. Don't write 6 separate `.tasktest` files — write one with a parametric block:

```yaml
uc: UC-040
title: delegate(mode: worker) is sync — over all worker variants
parametric:
  - {duration_ms: 0,    expected_done_in: <100ms}
  - {duration_ms: 100,  expected_done_in: <200ms}
  - {duration_ms: 500,  expected_done_in: <600ms}

setup:
  home: fixtures/worker-task/
  llm: simulator

invoke:
  cmd: ./bin/ktw-agents run worker --pipe --home $TMP_HOME
  timeout: 5s

assertions:
  # ... assertions reference {duration_ms} and {expected_done_in} from the param row
```

Each row produces one verdict; the UC verdict is the AND of all rows.

### Asserting absence (negative DoD)

For security UCs, **always include `journal_must_not_contain`** with the abuse case:

```yaml
journal_must_not_contain:
  - event: tool_call
    tool: bash                          # restricted child must never invoke bash
  - event: child_started
    spawn_permissions.allowed_tools: ["*"]    # never an unrestricted child
```

Without this, the test might pass when nothing happens at all.

### Asserting timing

For UCs about scheduling / cancellation, time matters:

```yaml
assertions:
  journal:
    - event: child_done
      ts_within: 200ms                  # asserts wallclock from invoke start
```

Use sparingly; timing assertions can be flaky on slow CI.

### Linking back to audit findings

Every security/correctness UC's `.tasktest` should cite `audit_finding`. This makes the green/red view link straight back to the original audit report:

```yaml
audit_finding: threat-T2-second
```

When the verdict is 🔴, the verify output includes the finding's link.

## Anti-patterns

- **Inline TASK.md in the `.tasktest`**: don't. Put it in `fixtures/`. Keeps `.tasktest` focused on assertions.
- **Editing fixtures**: don't. Copy + version. Tests pin to specific fixture versions.
- **Skipping the negative DoD on security UCs**: don't. Without it, security tests can pass for the wrong reason.
- **Real LLM in L2**: don't unless the UC is intrinsically about LLM behavior (rare). Use `simulator` or scripted JSON.
- **One giant `.tasktest` covering 5 UCs**: don't. One file per UC (or per parametrization). Keeps verdict mapping clean.
- **Test names that don't embed the UC ID**: don't. The UC ↔ test mapping must be greppable.
- **Time.Sleep in production code under test**: don't (CLAUDE.md project rule). Use a fake clock injected via interface.

## Where this doc fits

| Doc | Covers |
|---|---|
| [`charter-template.md`](charter-template.md) | What a charter contains (incl. DoD-per-UC structure) |
| [`test-layers.md`](test-layers.md) | The L1/L2/L3 conceptual model + when to use each |
| **This doc** | **How to actually write a test from a DoD** |
| [`verbs/verify.md`](verbs/verify.md) | The `verify` verb that runs the suite and reports per-UC verdicts |
| [`workflow.md`](workflow.md) | Where in the workflow each phase happens (Specify writes DoD; Execute writes tests; Verify runs them) |

## Status

This doc is part of Phase A.5 — written after the `tasks` module bootstrap surfaced the gap. The `.tasktest` schema is **forward-compatible**: scenarios written against it today will run unchanged when `code-module verify` Go-side ships. Until then, hand-running scenarios is acceptable; the schema gives every author the same template.

Reference for additions to this doc: when patterns crystallize during real test authoring (e.g. ms-tasks-rs-tier-1.3's L2 scenario tasks), capture them in the "Common patterns" section. Don't let it grow without merging similar entries.
