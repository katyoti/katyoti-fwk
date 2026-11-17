# Standards Enforcement — The `lint` Verb

Cross-module rules (sidecar layout, module layout, publish-service conventions, naming, topic constructors, …) drift unless **enforced** rather than only **documented**. The `lint` verb runs the rules as gates: against a module's bundle, against its source code, and against its charter.

This doc defines the rules format, the verb behavior, and how `lint` plugs into the workflow.

## Where rules live

Per OD-A, the rules live at:

```
app/ktw-code/embed/references/modules/rules/
├── sidecar-layout.md
├── module-layout.md
├── publish-service.md
├── naming.md
├── topic-constructors.md
├── permission-enforcement.md
├── config-loading.md
└── ...
```

Each rule is one markdown file. Rules are loaded by the `code-module` skill on demand via `ktw-code llm resources read ktw-code:/reference/<rule>.md`.

Many of these rules already exist as prose in `CLAUDE.md` and the rule objects of `modules/<module>/rules/`. Phase B/C migrates them into the structured format below.

## Rule file format

```markdown
---
id: rule-sidecar-layout-cmd
title: Sidecar cmd/ structure
severity: error | warning | info
scope: bundle | source | charter | manifest | cross-module
applies-to:
  - paths: ["app/<name>/cmd/"]
  - module-kind: sidecar
checks:
  - kind: file-exists
    path: "app/<name>/cmd/root.go"
  - kind: file-exists
    path: "app/<name>/cmd/bootstrap.go"
  - kind: file-list-exact
    path: "app/<name>/cmd/"
    files: ["root.go", "bootstrap.go"]
introduced: 2026-04-01
authoritative: CLAUDE.md ("Sidecar Binary cmd/ Structure")
---

# Rule: Sidecar cmd/ Structure

Every sidecar `app/<name>/cmd/` MUST have exactly:
- `root.go` — only `var version`, `Execute(efs) → appcore.BootstrapCLI(...)`.
- `bootstrap.go` — `buildExtensions(efs embed.FS) []application.Extension` with all wiring.

## Rationale

Uniform structure means readers and tools can find the entry point and the wiring without searching. New sidecars copy the structure from `app/ktw-docs/cmd/` and only edit `bootstrap.go`.

## Common violations

- Files named `bootstrap_*.go` instead of `bootstrap.go` — split the wiring across files; rule fails because it expects exactly two files.
- A `root.go` with extension wiring instead of just `Execute` — cannot be detected mechanically; flagged in audit.
- Missing `bootstrap.go` because wiring is in `root.go` — fails `file-exists`.

## Auto-fix

None. The fix requires human refactoring.

## Test cases

- `app/ktw-docs/cmd/` → pass
- `app/ktw-notify/cmd/` → pass
```

### Frontmatter fields

| Field | Purpose |
|---|---|
| `id` | Stable identifier, kebab-case, never reused |
| `title` | Human-readable name |
| `severity` | `error` (blocks promotion), `warning` (visible but not blocking), `info` (advisory) |
| `scope` | Which artifact the rule checks: bundle (charter + manifest + folder structure), source (Go code), charter (the markdown content), manifest (the JSON), cross-module (relationships between modules) |
| `applies-to` | Which modules / paths the rule targets. Filters by source paths or by module kind (`sidecar`, `extension`, `app`, `library`) |
| `checks[]` | Programmatic checks (see "Check kinds" below). May be empty for human-asserted rules — see "Tier 1 vs Tier 2 rules" |
| `introduced` | Date the rule was added (helps audit-ledger interpretation) |
| `authoritative` | Where the canonical statement of the rule lives (`CLAUDE.md`, an ADR, etc.) |

### Check kinds

| Kind | Description | Example |
|---|---|---|
| `file-exists` | A path must exist | `path: "app/<name>/cmd/root.go"` |
| `file-not-exists` | A path must NOT exist | `path: "internal/extensions/<module>/legacy.go"` |
| `file-list-exact` | A directory contains exactly the listed files | `path: "app/<name>/cmd/", files: [...]` |
| `grep-must-not-match` | A regex must not match in the given files | `path: "**/*.go", regex: "yaml\\.Unmarshal\\(", message: "Use config.DecodeBytes (rule project-config-loading)" |
| `grep-must-match` | A regex MUST match somewhere | `path: "internal/extensions/<module>/local/", regex: "tasklocal\\.Extension"` |
| `go-symbol-exists` | A Go symbol must be exported | `package: "github.com/.../api/engine", symbol: "Executor.Run"` |
| `go-symbol-not-exists` | A Go symbol must not be exported | (used to enforce removed APIs stay removed) |
| `import-must-not-include` | A package must not import another | `package: "api/engine", forbidden: "internal/extensions/task/api"` |
| `manifest-field` | A manifest.json field must equal / contain a value | `path: "manifest.json", field: "current_tier", op: "≥", value: 1` |
| `human-assert` | No mechanical check; rule is a checklist item the auditor confirms | (see Tier 1) |

## Tier 1 (today) vs Tier 2 (later) rules

The `lint` verb supports two operating modes:

- **Tier 1 — checklist mode**: rules with `kind: human-assert` checks. The verb prints the rule body and asks "y/n confirm." Useful for rules whose semantics are too rich to mechanize (e.g. "internal helpers don't leak into public API"). All existing CLAUDE.md rules can be lifted into Tier 1 immediately.
- **Tier 2 — programmatic mode**: rules with executable checks. The verb runs the checks and reports pass/fail without human input. Tier 2 rules are CI-runnable and faster; preferred when feasible.

A rule can mix tiers — programmatic checks AND a final human-assert "anything else look off?"

The migration arc: every rule starts as Tier 1; promote checks to Tier 2 as they prove valuable.

## Verb: `code-module lint`

### Inputs

- `<module>` (optional) — lint a specific module. Without it, lint cross-module rules across the whole portfolio.
- `--rules <list>` — subset of rule IDs to run (default: all applicable)
- `--severity error|warning|info|all` — minimum severity to report (default: `warning`)
- `--format text|md|json`
- `--non-interactive` — skip rules with `human-assert` checks (CI mode)

### Behavior

```
read /modules/<module>/manifest.json (if module specified)
read /modules/<module>/charter.md
determine applicable rules:
  - cross-module rules: always
  - bundle/source/charter/manifest rules: filter by applies-to.paths matching module's source_paths
  - per-module-kind rules: filter by applies-to.module-kind matching manifest.module_kind

for each applicable rule, sorted by severity desc:
  for each check in rule.checks:
    run the check
    record result (pass / fail / skipped)
  if rule has human-assert AND --non-interactive: skip with notice
  else if human-assert: prompt user

aggregate results
print:
  - errors (blocking)
  - warnings (visible)
  - infos (advisory)
  - skipped (with reason)
return non-zero if any errors and --non-interactive, else zero
```

### Output example (text)

```
Linting module: tasks (path: internal/extensions/task/, kind: extension)

Rules applied: 14 / 38 in catalog

ERRORS (must-fix before promote)
  ✗ rule-topic-constructors-only (severity: error)
    Found inline topic strings:
      internal/extensions/task/api/engine/spawner.go:127:  "child."+ name + ".started"
      internal/extensions/task/api/engine/liveness.go:48: HeartbeatTopicPrefix + runID
    Expected: use constructors from api/engine/topics.go.

WARNINGS
  ⚠ rule-package-doc-correct
    api/taskdef.go:1 declares `package api` but doc says "Package taskdef"
    (already fixed by 2026-05-06 cleanup; rule kept for regression detection)

INFOS
  ℹ rule-godoc-coverage
    18 / 47 exported symbols lack a doc comment

Result: 1 error, 1 warning, 1 info. Lint not clean. Promote blocked.
```

### Output example (JSON)

```json
{
  "module": "tasks",
  "rules_applied": 14,
  "rules_total": 38,
  "results": [
    {
      "rule_id": "rule-topic-constructors-only",
      "severity": "error",
      "passed": false,
      "details": [
        {"file": "internal/extensions/task/api/engine/spawner.go", "line": 127, "match": "..."},
        {"file": "internal/extensions/task/api/engine/liveness.go", "line": 48, "match": "..."}
      ]
    }
  ],
  "summary": {"errors": 1, "warnings": 1, "infos": 1, "skipped": 0}
}
```

## Integration with workflow phases

| Phase | When `lint` runs | What happens on failure |
|---|---|---|
| **Bootstrap** | After charter is created, before bundle is committed | Errors block bootstrap; warnings logged |
| **Specify** | Manual; helps the spec author find missing inventory items | Advisory only |
| **Plan-tier** | Auto-runs at start; lint errors become tasks in the milestone | Errors → mandatory tier-N tasks |
| **Verify** | Before tests run — fast feedback on rule violations | Reported alongside test results |
| **Audit** | Lint output is included in the audit bundle (`audits/<date>/lint.json`) | No direct gating; audit summary cites |
| **Promote** | **Required clean (no errors)** before the verb proceeds | Errors block promotion; user can `--force` (recorded in audit) |

## Initial rule catalog (seeded from CLAUDE.md)

These rules already exist as prose in CLAUDE.md and the rule objects of `modules/<module>/rules/`; Phase B/C migrates them into the structured format above.

| Rule ID | Source | Scope | Tier |
|---|---|---|---|
| rule-sidecar-cmd-layout | CLAUDE.md "Sidecar Binary cmd/ Structure" | bundle | T2 |
| rule-module-public-private-split | CLAUDE.md (api/internal/local pattern) | source | T2 |
| rule-publish-service-pattern | CLAUDE.md "Service Publishing" | source | T2 |
| rule-naming-typename-configname | CLAUDE.md "Central Naming" | source | T2 |
| rule-topic-constructors-only | CLAUDE.md "Topic Conventions" | source | T2 (grep) |
| rule-no-yaml-unmarshal | CLAUDE.md "Config Loading" | source | T2 (grep) |
| rule-no-services-em-direct | CLAUDE.md "Application as Single Entry Point" | source | T2 (grep) |
| rule-xdg-directory-usage | CLAUDE.md "Directory Layout (XDG)" | source | T2 |
| rule-godoc-on-exported | (project convention) | source | T2 |
| rule-charter-meta-complete | (this method) | charter | T2 |
| rule-charter-uc-ids-stable | (this method) | charter | T2 |
| rule-test-references-real | (this method) | charter+source | T2 |
| rule-permission-enforcement-opt-out | (post-audit recommendation) | source | T1 |

Each rule gets one markdown file at `app/ktw-code/embed/references/modules/rules/<rule-id>.md`. The catalog grows as patterns crystallize.

## Authoring a new rule

1. Identify a recurring problem (or pattern that should be enforced).
2. Decide scope (bundle / source / charter / manifest / cross-module).
3. Decide severity (error / warning / info) — start at `warning`, promote to `error` once you've validated no false positives.
4. Write the rule file at `app/ktw-code/embed/references/modules/rules/<id>.md` with the frontmatter.
5. Define checks (start at human-assert; promote to programmatic when reliable).
6. Add test cases (paths that should pass; paths that should fail).
7. Run `code-module lint` against an existing module to verify behavior.
8. Add the rule ID to the catalog (this doc's table) and the audit ledger if applicable.

## Why a structured rule format (instead of prose-only)

Rules in CLAUDE.md are prose. Humans read them; LLMs paraphrase them; CI doesn't enforce them. Structured rules:

- Are mechanically checkable (Tier 2)
- Are filterable by scope/severity/module-kind
- Have a stable ID for audit-ledger references
- Can be auto-generated into per-module rule reports
- Survive doc rewrites (the `id` doesn't change even if the prose body does)
- Form a contract that drift-detection ([`alignment.md`](alignment.md)) can verify

The cost is ~10 lines of frontmatter per rule. The benefit is a measurable quality gate.

## Out of scope

- **A separate rule engine**: this doc specifies what the `lint` verb does, not a generic rule engine. The verb implementation (in the future skill) does the dispatch.
- **Auto-fix infrastructure**: each rule may include an `auto-fix` script reference, but the `lint` verb is read-only by design. Use a separate `fix` verb if/when justified.
- **Rule versioning**: rules are evolved in place. If a rule's semantics change materially, retire the old ID and create a new one. The audit ledger preserves which rules were active at each audit date.
