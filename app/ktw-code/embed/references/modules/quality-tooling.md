# Quality Tooling — SonarQube and the Go-Native Stack

Question: can SonarQube (or equivalent) plug into this method to add automated quality scoring?

**Short answer**: yes, but for a Go-only codebase the value-add is modest. A Go-native stack (`golangci-lint` + `go test -race -cover` + Codecov-style coverage tracking) typically gives better day-to-day signal at lower operational cost. SonarQube earns its keep when the **portfolio view** matters more than the per-package depth — which is exactly what this method is about. So the recommended setup is a **hybrid**: Go-native for module-level gates, SonarCloud (or Sonar OnDemand-style alternative) for the portfolio dashboard.

## What SonarQube would buy you

| Capability | SonarQube | Go-native | Audit method |
|---|---|---|---|
| Cyclomatic complexity per function | ✓ (out-of-the-box) | ✓ (`gocyclo`/`cyclop` in golangci-lint) | indirect |
| Code duplication | ✓ | partial (`dupl` linter; less polished) | — |
| Maintainability index | ✓ | — | — |
| Security hotspots (Go) | partial (Go support is OK, not stellar) | ✓ (`gosec` in golangci-lint) + `govulncheck` | ✓ (threat-model review) |
| Bug detection (linters bundled) | ✓ (subset; less than golangci-lint) | ✓ (50+ linters via golangci-lint) | indirect |
| Test coverage tracking over time | ✓ | manual / Codecov / Coveralls | — |
| Quality gate that fails CI | ✓ | ✓ (configure CI to gate on lint/test/cover) | ✓ (promote verb) |
| **Portfolio dashboard** | ✓ ★ | — | partial (factory-rollup) |
| **Multi-language support** (TS, Java, Python) | ✓ ★ | — | — |
| Architecture / data-model review | — | — | ✓ ★ |
| Threat-model coverage | — | partial | ✓ ★ |
| Dimension-matrix coverage (per-spawn modes) | — | — | ✓ ★ |
| Per-module charter / UC tracking | — | — | ✓ ★ |
| Tier promotion / quality gates | — | — | ✓ ★ |

★ = where this column wins outright.

The key observation: **SonarQube and the audit method don't overlap much**. Sonar covers what static analysis can find; the audit covers what only humans (or a fan-out of LLMs) can find. They're complementary, not redundant.

## The Go-native day-to-day stack

For module-level gates, this stack is usually enough:

```
┌─────────────────────────────────────────────────────────────────┐
│ golangci-lint (one runner)                                      │
│   ├── staticcheck      → bugs, unused, redundant code           │
│   ├── govet            → standard-library-grade checks          │
│   ├── errcheck         → unhandled errors                       │
│   ├── gosec            → security smells (regex, perms, etc.)   │
│   ├── gocyclo / cyclop → complexity per function                │
│   ├── revive / gocritic→ style + idiomatic Go                   │
│   ├── dupl             → duplication                            │
│   └── ... (20+ linters available)                               │
│                                                                 │
│ go test ./... -race -coverprofile=coverage.out                  │
│   └── built-in test + race detector + coverage                  │
│                                                                 │
│ govulncheck ./...                                               │
│   └── known CVEs in dependencies                                │
└─────────────────────────────────────────────────────────────────┘
```

Adding all three is ~30 minutes of setup. The result:

- One CI step runs the linters
- One step runs tests with race + coverage
- One step checks for known CVEs
- A `coverage.out` file flows to Codecov / Coveralls / SonarCloud (any of them, pick one)

This is what most production Go projects ship with.

### What golangci-lint catches that's invisible today

Running golangci-lint on `internal/extensions/task/` (post-cleanup) likely surfaces:

- Several `errcheck` violations (the audit's correctness review noted error-swallowing)
- `gocyclo` violations in `wkflow/task_strategy.go` (the 910-LOC god-method)
- `gosec` warnings on path operations (the WorkspacePathHook gaps)
- `unused` exports (would have caught the `MachineRegistry` dead code)
- `goconst` (repeated string literals — the inline topic strings)

Many of these are **the same findings the audit produced**, but at a fraction of the cost (seconds vs minutes vs hours of LLM time). Don't skip cheap signal.

## When SonarQube earns its keep

- **Polyglot codebases** (Java + JS + Go) — Sonar's main strength. The repo has TS sidecars (`app/ktw-display`, `app/ktw-gui` Wails bindings) — Sonar can cover them with the same tool.
- **Compliance-heavy environments** — auditors recognize SonarQube reports; some certifications expect them.
- **Multi-team factories** — when many teams ship many modules, the portfolio dashboard with consistent quality scoring is hard to replicate cheaply.
- **Hosted (SonarCloud)** — free for OSS, low operational cost vs self-hosting.

## When SonarQube is overkill

- **Solo or small team, Go-only** — golangci-lint + Codecov gives 90% of the signal with no server.
- **You'd ignore the dashboard** — a portfolio view that nobody opens is dead weight.
- **You already have the audit method** — for the questions Sonar can't answer (architecture, threat model, dimension coverage), this method is the answer; Sonar adds at the margins.

## Recommendation for this codebase

**Hybrid, in this order**:

1. **Now (free / fast)**: add `golangci-lint` config at repo root. Wire into CI. The `code-module lint` verb shells out to `golangci-lint run --config .golangci.yml ./<module>/...` as one of its checks. This gets module-level static analysis on every charter run.
2. **Next (a few hours)**: add `go test -race -coverprofile=...` to CI; upload to Codecov (free for public; cheap for private). Per-module coverage history.
3. **Maybe later (operational decision)**: connect SonarCloud for the portfolio dashboard if/when:
   - The TS sidecar bundles need their own quality scoring (multi-language is Sonar's win)
   - A non-engineer stakeholder asks for "the dashboard"
   - A compliance requirement makes Sonar reports useful

If you skip step 3, the `code-module factory-status` verb gives you a portfolio view of the rock-solid metrics (UCs, audits, tiers). If you adopt step 3, both views coexist — Sonar for code-level metrics, factory-status for charter-level metrics.

## Concrete next steps if you adopt Sonar

If you decide to go ahead:

1. **Pick a flavor**:
   - **SonarCloud** — hosted, free for public repos, paid per LOC for private. Recommended unless privacy/network constraints force self-hosted.
   - **SonarQube Community Edition** — self-hosted, free, basic features. PostgreSQL + Java backend; ~30-min setup.
   - **SonarQube Developer/Enterprise** — paid; adds branch analysis, security review depth, etc.
2. **Add `sonar-project.properties`** at repo root:
   ```properties
   sonar.projectKey=ktw-agents
   sonar.organization=<your-org>
   sonar.sources=app,internal
   sonar.tests=app,internal
   sonar.test.inclusions=**/*_test.go
   sonar.go.coverage.reportPaths=coverage.out
   sonar.exclusions=**/*.pb.go,**/_ktw_workspace/**,**/node_modules/**
   ```
3. **Add a CI step** that runs `sonar-scanner` after the test/coverage steps.
4. **Configure a quality gate** in Sonar matching the rock-solid tiers (e.g. "no new criticals; coverage ≥ 70%").
5. **Map Sonar projects to module charters** — one Sonar project per `/modules/<name>/` so the portfolio aligns.

If you want a pure Go-native alternative dashboard:

- **GitHub Actions + a Pages site** rendering `code-module factory-status --format json` as a static page.
- **Codecov dashboards** for coverage trends.
- **Internal Grafana** scraping the JSON.

Both routes work. The audit method is unchanged either way.

## Recommended `.golangci.yml` (starter)

A pragmatic config to start with — not minimal, not maximal:

```yaml
run:
  timeout: 5m
  tests: true

linters:
  enable:
    - errcheck
    - govet
    - staticcheck
    - gosec
    - gocyclo
    - cyclop
    - unused
    - goconst
    - gocritic
    - revive
    - misspell
    - bodyclose
    - rowserrcheck
    - sqlclosecheck
    - gofmt
    - goimports
    - dupl

linters-settings:
  cyclop:
    max-complexity: 15      # promote to 12 once the 910-LOC god-method is broken up
  gocyclo:
    min-complexity: 15
  errcheck:
    check-type-assertions: true
  gosec:
    excludes:
      - G104                # errcheck overlap; let errcheck own this
  dupl:
    threshold: 100

issues:
  exclude-use-default: false
  exclude-rules:
    - path: _test\.go
      linters:
        - gosec             # tests can have permissive practices
        - errcheck
```

Drop this in `.golangci.yml` at repo root. Run `golangci-lint run` from the repo root and watch what surfaces. Expect the first run to surface a meaningful number of issues — that's expected; the audit findings have been accumulating.

## Go tooling catalog

`golangci-lint` is the heaviest single tool in the kit. Several other Go-native tools earn their place at different tiers. Categorized by cost and tier relevance:

### Must-have (Tier 1 — wire alongside golangci-lint)

| Tool | What | Why for this codebase |
|---|---|---|
| `go test -race -count=1` | Built-in race detector | The audit found races (subscribe-after-spawn, recorder leaks). Non-negotiable for executor / channel / spawner code. |
| `go test -coverprofile=coverage.out` | Built-in coverage | Tier-1 DoD requires ≥80% UC coverage. Without numbers, you can't gate. |
| `gotestsum --junitfile=report.xml` | Better test runner output + JUnit XML | Free upgrade over `go test`. JUnit XML feeds CI dashboards and flaky-test detection. |
| `govulncheck ./...` | Official Go CVE scanner (Go team) | One command. Catches deps with known CVEs. Fast enough to run every audit. |

### Should-have (Tier 1-2 — addresses specific audit gaps)

| Tool | What | Why |
|---|---|---|
| `go-arch-lint` (or `depguard` linter inside golangci-lint) | Enforce package-import layering | The audit found one reverse import (`api/engine/spawner_impl.go` → `api/`). A layering linter would have caught it pre-commit. Define rules: `api/` cannot import `internal/`; `local/` only depends on `api/` + `internal/`. Maps directly to the rules in [`standards-enforcement.md`](standards-enforcement.md). |
| `apidiff` (`golang.org/x/exp/cmd/apidiff`) | Detect public-API breaking changes | The charter's "Public API contract" becomes mechanically enforceable. Run between charter versions; failures gate `promote` if the version isn't bumping major. |
| `benchstat` | Compare benchmark results across runs | Tier-2 DoD says "p99 step latency under <BUDGET>." Without statistical comparison, perf claims are guesses. `benchstat` reads `go test -bench` output and gives confidence intervals. |
| Coverage trend tracker (Codecov / `factory-status` JSON / Coveralls) | Track coverage across commits | Tier-2 wants coverage trending up. Without trend data, you're flying blind. The simplest path: ship coverage-percent as a `manifest.json` field; let `factory-status` rollup show the delta. |

### Nice-to-have (Tier 2-3 — defer until earlier tiers shipped)

| Tool | What | Why later |
|---|---|---|
| `go test -fuzz` | Built-in fuzz testing (Go 1.18+) | High value for the workflow YAML loader, TASK.md frontmatter parser, mailbox-topic parsing. Fuzz seeds bloat tier-1 ergonomics; defer to tier 2. |
| `gomutator` / `go-mutesting` | Mutation testing | Tier-3 DoD wants ≥80% mutation kill rate on security-critical code. Slow; reserve for tier 3 push. |
| `go-deadlock` | Runtime deadlock detector (instrumentation) | Reactive — only when chasing a specific concurrency bug. Don't run by default. |
| `pprof` / `go tool pprof` | Profiling | Reactive — when perf review surfaces something, `pprof` confirms. Not part of the routine pipeline. |
| `go-licenses` / `trivy fs` | License extraction / SCA | Once before any external release. Not needed for internal velocity. |

### Project-specific (already exists or will be built)

| Tool | What | Status |
|---|---|---|
| `ktw-test` | Virtual terminal harness for L2 scenarios | Exists. See [`test-layers.md`](test-layers.md). The `verify` verb shells out to it. |
| `ktw-utils session show/aggregate` | Journal inspection for L3 assertions | Exists. Used by L3 tests. |
| `code-module lint` | Custom rules + alignment checks | Spec'd in [`standards-enforcement.md`](standards-enforcement.md) + [`alignment.md`](alignment.md). Built when the skill ships. |
| Charter-driven test stub generator | Convert charter UCs → starter test files | Doesn't exist; consider at tier 2 once UC patterns stabilize. |

## When to run the gates — on-request, not on-commit

The rock-solid program runs quality gates **on request**, not on every commit. Three reasons:

- **Cost**: race tests + coverage + lint can take 1-3 minutes. Multiplied across every push, that's developer friction with diminishing returns.
- **Audit ≠ continuous**: the method's audit cadence (≥1 per tier, +90-day staleness gate) is deliberately periodic. Continuous gates contradict that rhythm and create alert fatigue.
- **Trust the developer**: invoking `code-module verify` locally is the primary feedback loop. CI is for verification before promotion, not for catching mistakes during exploration.

### Trigger model

| Trigger | What runs | Who fires it |
|---|---|---|
| **Local: `code-module verify <module>`** | Full L1 + L2 (+ L3 if applicable); golangci-lint; coverage | Developer, before opening a PR or before `audit` |
| **Local: `code-module lint <module>`** | Rules + alignment checks; fast | Developer, ad-hoc during refactor |
| **Local: `code-module audit <module>`** | The full 13-agent review | Developer, when they want a fresh audit (blocks on confirmation per OD-B) |
| **CI: manual `workflow_dispatch`** | Same as `code-module verify` but from a GitHub button | Anyone with repo access; e.g. before merging a charter-bumping PR |
| **CI: nightly schedule (optional, light)** | Only `govulncheck` + `factory-status --stale 90 --format json` | Cron, posts result to Slack/issue |
| **CI: pre-promotion check** | Same as `verify` + lint; gates the `promote` verb | The skill itself when invoked |

### Sketch — manual-trigger CI

```yaml
# .github/workflows/quality.yml — runs on request only
name: Quality (manual)

on:
  workflow_dispatch:
    inputs:
      module:
        description: 'Module to verify (or "all" for portfolio rollup)'
        required: false
        default: 'all'
      run_audit:
        description: 'Also run a fresh audit (expensive)'
        type: boolean
        default: false

jobs:
  verify:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version: '1.22' }
      - run: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
      - run: go install gotest.tools/gotestsum@latest
      - run: go install golang.org/x/vuln/cmd/govulncheck@latest

      - name: Lint
        run: golangci-lint run --config .golangci.yml ./${{ inputs.module || '' }}/...
      - name: Test (race + coverage)
        run: gotestsum --junitfile=report.xml --format=short -- -race -coverprofile=coverage.out ./${{ inputs.module || '' }}/...
      - name: Coverage summary
        run: go tool cover -func=coverage.out | tail -1
      - name: Vulnerability scan
        run: govulncheck ./...

      - name: Upload coverage artifact
        uses: actions/upload-artifact@v4
        with: { name: coverage, path: coverage.out }
      - name: Upload junit artifact
        uses: actions/upload-artifact@v4
        with: { name: junit, path: report.xml }

  audit:
    if: ${{ inputs.run_audit }}
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      # ...invoke ktw-code:code-review against the module path...
      # writes to /modules/<module>/audits/<date>/
      # commits the audit bundle back if a deploy key is configured
```

The `workflow_dispatch` trigger means the action only runs when someone clicks "Run workflow" in the GitHub UI (or invokes via `gh workflow run quality.yml -f module=tasks`). No commit triggers it.

### What about a nightly cadence

The 90-day audit staleness gate ([`workflow.md`](workflow.md) Cadence section) suggests one **light** nightly job: run `code-module factory-status --stale 90 --format json` and `govulncheck`, post results to a status channel. That's not "running on every commit" — it's a daily portfolio check. Recommended but optional.

### What stays out of CI entirely

- The 13-agent audit (cost-heavy; user invokes via `code-module audit` deliberately).
- L3 self-hosted scenarios (need a stable build pinned; better run on demand against a release tag).
- Mutation testing (slow; tier 3 only).

## Summary

- **Yes**, SonarQube is configurable. Yes, it adds value — especially for portfolio view and multi-language.
- For Go-only day-to-day quality, **`golangci-lint` + `go test -race -cover` + `gotestsum` + `govulncheck`** covers most of what Sonar would at a fraction of the operational cost.
- The audit method covers what neither tool does (architecture, threat model, dimension coverage).
- **Trigger model is on-request, not on-commit**: developers run `code-module verify` locally; CI runs only via `workflow_dispatch`; the skill itself gates `promote`. No push-triggered CI for these gates — by design.
- **Recommended setup**: Tier-1 must-haves immediately; `go-arch-lint` / `apidiff` / `benchstat` as Tier 1-2 needs them; mutation testing and SonarCloud later if/when the portfolio view in `factory-status` proves insufficient.
- The `code-module lint` and `code-module verify` verbs are the integration points — they shell out to whichever tools you adopt and treat their output as one more rule category.
