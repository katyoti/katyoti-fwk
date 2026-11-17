# Alignment — Keeping Charter and Code in Sync

The charter is only valuable if it tells the truth about the code. The moment they drift, the charter becomes a beautiful lie — readers stop trusting it, and the rock-solid program loses its anchor.

This doc defines **how the charter stays aligned with the code** at three levels (the user's framing: goals → concepts → specs) and how alignment is verified by the `lint` verb.

## Three levels of alignment

```
┌─────────────────────────────────────────────────────────────────┐
│ Level 1 — Goals alignment                                       │
│   Does the module actually serve the goals it claims?           │
│   Verified: charter Meta.Linked goals reference real goal files;│
│             goal files mention the module if it's a primary     │
│             contributor.                                        │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│ Level 2 — Concepts alignment                                    │
│   Do the named entities in the charter match real types/files?  │
│   Verified: every Concept's "Source" reference resolves;        │
│             every public type with godoc is considered for      │
│             inclusion.                                          │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│ Level 3 — Specifications alignment                              │
│   Function inventory ↔ exported symbols                         │
│   Use Cases ↔ source paths they exercise                        │
│   Test plan ↔ real test functions                               │
│   Dimension matrix ↔ actual mode preset definitions             │
└─────────────────────────────────────────────────────────────────┘
```

Each level has automatable checks; together they form the **alignment contract** that `lint` enforces.

## Level 1 — Goals alignment

### What's checked

- **Forward**: charter `Meta.Linked goals` lists files. Each must exist at a goal object, `modules/<module>/goals/G-NNN-<slug>.cwm.md` (named `<module>.G-NNN`).
- **Reverse (advisory)**: when a module is the primary contributor to a goal, the goal's `Contributing modules` field should list it. (Not a hard rule; goals can outpace modules.)
- **Consistency**: a charter that claims to support a goal but has no UC mentioning that goal's concerns is suspect.

### What's NOT checked (yet)

- Whether the module *actually advances* a goal — that's a human judgment, lint can't help.
- Whether goals are themselves well-formed — that's the goal-management layer's concern.

### Lint check kinds

```yaml
- kind: file-exists                              # forward
  path: "modules/<module>/goals/<linked-goal>.cwm.md"
  for-each: charter.Meta.Linked goals
- kind: human-assert                             # consistency
  text: "Each Linked goal has at least one UC that advances it."
```

## Level 2 — Concepts alignment

### What's checked

The charter's `Concepts` section has a glossary table. Each entry has a `Source` column citing a Go file. Lint verifies:

- **Source path exists** (mechanized via file-exists check).
- **Symbol exists in source** (mechanized via go-symbol-exists check).

Reverse drift is harder but valuable:

- **Undocumented public type**: if `internal/extensions/<module>/api/` exports a type with godoc that is NOT in the charter's Concepts glossary, lint emits an info-level reminder. The auditor decides: add to charter, or hide from public API.

### Lint check kinds

```yaml
- kind: file-exists
  path: <source-cite>
  for-each: charter.Concepts.glossary[].Source
- kind: go-symbol-exists
  package: <inferred-from-path>
  symbol: <Concept.Name>
  for-each: charter.Concepts.glossary
- kind: undocumented-public-types     # reverse
  package: charter.Meta.Module-paths
  exclude: charter.Concepts.glossary[].Name
```

The reverse check is one of the most valuable signals: it catches new exports that should be in the charter but aren't.

## Level 3 — Specifications alignment

The richest level. Three sub-areas:

### 3a. Function inventory ↔ exported symbols

The charter's Function inventory table lists symbols by name + stability. Lint:

- **Forward**: every entry must resolve to a real exported symbol via `go list -json`.
- **Reverse**: every exported symbol in the module's `api/` (or designated public surface) must appear in the inventory, OR be flagged as `provisional`/`internal-pending`.

```yaml
- kind: go-symbol-exists
  symbol: <inventory.Symbol>
  for-each: charter.Specifications.Function-inventory
- kind: undocumented-exports
  package: charter.Meta.Public-surface-paths
  exclude: charter.Specifications.Function-inventory[].Symbol
  severity: warning
```

### 3b. Use Cases ↔ source paths

Each UC may reference source paths (e.g. "exercises `api/engine/spawner_impl.go`"). Lint:

- **Forward**: every cited path exists.
- **Reverse**: source files implementing core machinery should be cited by ≥1 UC. (Soft check; large files OK to skip.)

```yaml
- kind: file-exists
  path: <uc.source-path>
  for-each: charter.Specifications.Use-cases[].source-paths
```

### 3c. Test plan ↔ real test functions

Every `Test plan` row says: "UC-XXX is covered at L1 by `package.TestName`." Lint:

- **Forward**: `go test -list "TestName" ./<package>/...` must list the test.
- **Reverse**: `go test -list ".*" ./<module>/...` returns all tests; tests not mapped to UCs are flagged (info — they may be implementation tests, not UC tests).

```yaml
- kind: go-test-exists
  package: <test-plan.package>
  test-name: <test-plan.test-name>
  for-each: charter.Test-plan[].L1
- kind: tests-without-uc-mapping
  package: charter.Meta.Module-paths
  mapped-tests: charter.Test-plan[].L1.test-name
  severity: info
```

### 3d. Dimension matrix ↔ mode preset code

For modules with a dimension matrix (the `tasks` module especially), lint verifies the matrix matches the source:

- Each mode in the matrix exists as a preset in `presets/modes.go` (or equivalent).
- Each dimension has a config struct in `presets/config.go`.
- Empty cells (unsupported combinations) are not contradicted by code (e.g., if matrix says "no remote+restricted," but resolver allows it, flag).

```yaml
- kind: go-symbol-exists
  symbol: <mode-name>
  package: presets
  for-each: charter.Specifications.Dimension-matrix.modes
```

## Drift kinds

Two directions, four severities:

| Direction | Symptom | Severity | Action |
|---|---|---|---|
| **Forward drift** (charter ahead of code) | Charter cites `engine.Foo`; code doesn't have it | **Error** | Block promote; fix charter or restore symbol |
| **Forward drift, soft** | Charter cites a path that no longer exists | **Warning** | Update charter |
| **Reverse drift** (code ahead of charter) | New exported symbol in `api/` not in inventory | **Info → Warning** if at promote-time | Author should add to charter or mark internal |
| **Cross-drift** (rules + spec disagree) | Rule says X; charter says NOT-X | **Error** | Reconcile |

Errors block promote. Warnings are visible. Infos are advisory.

## When alignment is checked

| Phase | Alignment check |
|---|---|
| Bootstrap | Light — verifies the source paths exist; doesn't expect populated inventory yet |
| Specify | After UC inventory is filled; full alignment check expected |
| Plan-tier | Misalignments become tasks (e.g., "add the 6 missing exports to inventory") |
| Verify | Test plan ↔ tests subset of the alignment check |
| Audit | Full alignment report appears in audit bundle |
| Promote | Must be clean (or violations explicitly waived) |

The skill verb [`code-module lint`](standards-enforcement.md) bundles standard rules + alignment checks; from a user perspective, alignment is one category among the rules.

## Tooling per check kind

| Check kind | Tool |
|---|---|
| `file-exists` | shell `test -e` or Go `os.Stat` |
| `go-symbol-exists` | `go doc <package>.<symbol>` returns success, OR `go list -json` parses the package |
| `go-test-exists` | `go test -list <regex> ./<pkg>/` |
| `grep-must-not-match` | ripgrep or Go regex over file glob |
| `go-import-allowed` | `go list -deps` against an allowlist |

The skill implementation can shell out to `go` directly — no parser needed for most checks.

## A worked example

Charter says (in Specifications):

```markdown
| Symbol | Kind | Stability | UC coverage |
|---|---|---|---|
| engine.Executor.Run | method | stable | UC-001, UC-002, UC-005 |
```

Lint runs:

1. `go doc github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine.Executor.Run` → exit 0 → ✓
2. UC-001, UC-002, UC-005 all exist in the charter's Use Cases table → ✓

Now suppose someone renames `Run` to `Start` in source. On the next `code-module lint`:

1. `go doc ...Executor.Run` → exit 1 → **error**: "Function inventory entry `engine.Executor.Run` not found"
2. Suggestion: "Did you mean `engine.Executor.Start`? Update the inventory."

Conversely, suppose the source adds a new exported method `engine.Executor.Pause`. Lint runs the reverse check:

1. List exported symbols of `engine.Executor` → `Run, Resume, Pause`
2. Compare to inventory → `Pause` is missing
3. **Warning**: "Undocumented public method `engine.Executor.Pause` (added since charter v0.1)"
4. Suggestion: add to inventory, or mark as `internal-pending`.

## Charter author's checklist (for alignment)

When updating any charter section that references code, the author asks:

- [ ] Did I cite a real file path? (Concepts, UCs)
- [ ] Did I cite a real exported symbol? (Function inventory)
- [ ] Did I link a real test name? (Test plan)
- [ ] Did I name modes that exist in `presets/modes.go`? (Dimension matrix)
- [ ] Did I link a goal file that exists? (Meta)

Run `code-module lint <module>` after edits to catch any I forgot.

## What alignment can't catch

Honest about the limits:

- **Semantic drift**: charter says "Executor.Run starts from initial state." Code's behavior changes to start from saved state. The symbol still exists; lint passes; charter lies. Mitigation: UC-level tests catch it (the L1/L2 layer); audit catches it (read the code).
- **Goal alignment in spirit**: lint can verify the goal *file* exists. It can't verify the module advances the goal in any meaningful way.
- **Cross-module narrative**: if module A's charter says it depends on module B's charter, lint can verify the cross-reference exists, but not whether A's UCs are still satisfiable given B's current state.

These gaps are why the **audit method** complements lint. Lint is the cheap, frequent gate. Audit is the deep, periodic review. Together they cover most drift.

## Out of scope

- **AST analysis** — for now, alignment uses `go list`, `go doc`, `go test -list`, and grep. Full AST analysis (e.g. tracing data flow to verify a UC's semantics) is overkill for this layer.
- **Cross-language alignment** — the Go-only modules are the scope. TS sidecar bundles (ktw-display) may need their own alignment story; defer.
- **Auto-fix** — lint reports drift; the fix is a human edit. A future enhancement could auto-update the inventory from `go list`, but that risks rubber-stamping renames that should be reviewed.
