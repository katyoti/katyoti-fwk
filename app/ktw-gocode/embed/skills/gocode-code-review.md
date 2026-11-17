---
name: gocode-code-review
description: >-
  Go-specific checklists for the code review types — what to look for in
  Go code under each one (architecture, security, threat model,
  correctness, performance, quality, dead code, library, package layout,
  CLI UX, complexity, data model, test coverage, red team), as read-only
  checks an LLM can run during a review of a Go module.
icon: "🛡"
iconName: shield-check
hashtags: [review, audit, perspectives, reviewer, ktw-gocode]
category: runbook
provides: [skill]
---

# Code Review — by Perspective

The perspectives are review types of the reviews module: the
automated ones (correctness, quality, package layout, dead code, lint)
are the `go-checks` review, run by the toolchain; the judgment ones are
ktw-code's code review types, each run by an agent following
`ktw-code:/skill/code-review` from a module's Reviews panel. This skill
is the Go-specific checklist an agent adds to such a review of a Go
module.

## How to use

In a review of one type, **read the relevant code + items** under the
matching perspective below, look for violations of its bullet list, and
report each as a finding of the review result (severity, statement,
location, recommendation, a stable key) — the reviews service files it
and follows it from one review to the next.

**Hard rules (reiterated from the reviewer persona):**

- Read-only on code. Never `file_write` / `file_edit`.
- Read-only on `status_cache`. Defects, not direct fixes.
- Findings carry severity (critical / high / medium / low / info)
  and a reproduction path (file:line or test name).
- Use the canonical perspective-prefix for finding IDs:
  `arch-N` / `sec-N` / `threat-T<n>` / `corr-N` / `perf-N` /
  `quality-N` / `dead-N` / `lib-N` / `pkg-N` / `cli-N` /
  `complex-N` / `data-N` / `test-N` / `redteam-N`.

## Architecture (arch-*)

Look for:

- Cross-module imports that violate the module-kind dependency
  rules (sidecar imports another sidecar's internal/, library
  reaches into framework internals, etc.)
- Modules with no clear single responsibility — the kind doesn't
  match what the code does
- Public surface that's larger than the documented contract

**Severity guidance**: critical when a violation breaks the
dependency graph (cyclic imports, layer-skip); high when a
contract is exceeded; medium for "should be cleaner".

## Security (sec-*)

Look for:

- Input validation gaps at trust boundaries (HTTP / KTSP / CLI /
  file paths from user input)
- Tool calls that bypass per-spawn permission profiles
- Secrets in code (search for "token", "key", "password" string
  literals — env-var references are fine, baked secrets are not)
- Path-traversal risks (`filepath.Join` with user input not
  validated)

**Severity guidance**: critical for working bypasses, high for
unenforced invariants with theoretical bypasses, medium for
defense-in-depth gaps.

## Threat-model (threat-T*)

Read the module's threat-model document (when present) or
construct one. For each declared threat:

- Identify the mitigation in code
- File a finding when the mitigation is missing, weak, or removed
- Use `T1, T2, T3…` numbering to match Joram-style threat docs

## Red-team (redteam-*)

Adopt an attacker mindset. For each public verb / endpoint:

- What's the worst possible input? Is it rejected?
- What state can the attacker observe? What can they infer?
- What concurrent call pattern would deadlock / corrupt state?

This perspective often surfaces issues the other perspectives
miss because they're framed as "find what's wrong"; red-team is
framed as "find how to break it."

## Correctness (corr-*)

Look for:

- Off-by-one errors in slicing / range loops
- Goroutine leaks (unbounded goroutine spawn, missing cancel)
- Race conditions (read+write to shared map without lock; check
  the `Go RWMutex Lock+RLock re-entry` memory entry)
- Wrong error wrapping (`%v` instead of `%w`, swallowed errors)

## Performance (perf-*)

Look for:

- Unbounded allocation in hot paths (string concatenation in
  loops, append-to-slice in goroutines without preallocation)
- Synchronous calls in handlers that should be async
- O(n²) algorithms over inputs that grow with users / items

## Quality (quality-*)

Look for:

- Hardcoded magic numbers / strings without explanation
- Functions exceeding ~60 lines (smell, not always wrong)
- Inconsistent error messages (some wrap with file:line, others
  don't)
- godoc gaps on public symbols

## Dead-code (dead-*)

Look for:

- Exported symbols never imported (run `staticcheck`-style scan)
- Internal helpers used by exactly one caller (inline candidate)
- TODO / FIXME comments older than tier-promote window

## Library (lib-*)

Look for:

- Third-party imports without a corresponding `decision` item
- Versions of dependencies that conflict with project rules
- Use of deprecated APIs in transitive deps

## Package-layout (pkg-*)

Look for:

- Violation of the `public/internal/local` triple convention
- Files that don't belong to their package's stated responsibility
- Mixed concerns (e.g. http handlers + business logic in same file)

## CLI-UX (cli-*)

Look for:

- Inconsistent flag names across sibling commands
- Help text that doesn't explain WHEN to use the command
- Error messages without remediation hints
- Missing dry-run for destructive verbs

## Complexity (complex-*)

Look for:

- Cyclomatic complexity > 15 in single functions
- Deep nesting (5+ levels)
- Mutual recursion without clear termination

## Data-model (data-*)

Look for:

- Items / nodes with fields that should be relations (e.g.
  storing a node UUID as a string instead of via a typed edge)
- Optional fields with no documented null semantics
- Schema migrations that aren't reversible

## Test-coverage (test-*)

Look for:

- UCs at severity `core` or `security` without an L1 test
- Negative-DoD missing on security UCs
- L2 scenarios that use real LLMs unnecessarily (cost-incurring)

## Summary (summary-*)

The summary pass rolls up the others. After the 14 specific
perspectives have run:

1. Read all open findings
2. Group by theme (architectural / security / quality)
3. Author a `summary.md` document under the audit's review
4. Highlight the **promote-gate blockers** (critical + high
   findings) explicitly

## When done

The audit's review has:

- [ ] One or more `finding`s per non-empty perspective (`found_by`: the perspective)
- [ ] A `summary.md` document rolling up the findings
- [ ] Severity-tagged findings so the promote-gate can count them
- [ ] No "ran the perspective but didn't write anything" gaps —
      if a perspective produced no findings, file an info-severity
      placeholder ("perspective: clean")

## References

- `ktw-code:code-review` skill — the parent multi-agent review pattern
- The reviews pack — the `review` and `finding` types an audit writes
