---
name: test-go
description: "Test code changes with the Go and tasktest layers: L1 unit tests, L2 token-free simulator scenarios, L3 self-hosted scenarios. Load when code changed and regressions must be caught at CI grade."
advertised: false
hashtags: [testing, go, tasktest, regression]
category: guide
---

# Go + tasktest layers

The regression-pinning method: deterministic, CI-grade, no LLM cost at L1/L2.

## The layers

| Layer | What | Where | Cost |
|---|---|---|---|
| **L1** | Go unit/integration tests | `internal/**/*_test.go` | free, instant |
| **L2** | scenario files, `type: simulator` children — token-free determinism | `modules/<module>/tests/L2/*.tasktest` | free |
| **L3** | self-hosted scenarios with real LLMs — ONLY on explicit user request | `modules/<module>/tests/L3/` | tokens |
| **Corpus** | every in-repo skill must strict-parse | skills corpus test | free |

## Run: two lanes

This section is the one place the test lanes are said; the other skills and briefs point
here.

| Lane | When | Command |
|---|---|---|
| **quick** | every change, every agent step | `bash scripts/test-affected.sh --paths <the files the step changed>` - their packages and the packages importing them; or `go test -short -p 2 <packages>` (`scripts/test-quick.sh`, `make test-quick PKGS=…`) |
| **full** | the final gate of an increment, the night - one at a time | `make test-full` (`COMMIT=<ref>`, default HEAD): on a clean snapshot of that commit, never the working tree other sessions edit - its `bin/` built there, every Go test again (`-count=1`), two packages at a time, under the installation guard, then the frontend suites one at a time; a second full run is refused while one runs; its report names the commit tested |

- **The quick lane runs only what the change touches**, with Go's test cache kept:
  `scripts/test-affected.sh` names the packages of the files changed and their importers
  (`--deep`: every dependent) and runs them with `-short -p 2`. Without `--paths` it reads
  git (the working tree and the untracked files against HEAD, `--since <ref>`) - in a tree
  other sessions change too, name your own files. Never `-count=1` (except to prove a
  flake, on the one test), never a whole `go test ./...` for a step.
- **An everyday run stays minutes long**: an importer that took more than 120 s
  (`SLOW_SKIP_SECONDS`) in the last full run's report (`bin/test-runs/full-*.txt`) is left
  out and named at the start and the end of the run - its owner tags its heavy tests
  `long.Test`. A package that changed itself always runs.
- **The quick lane leaves the full lane's work to it**, with no full report needed:
  a **whole-repo suite** - a package under `internal/test/` holding tests only
  (`internal/test/arch`, `internal/test/catalog`, …) - is left out and named unless it
  changed itself; and a changed package with more than 20 importers (`FANOUT`) brings
  only the importers of its own module (the modules' `code_packages`), the others named
  for the full lane. `--all` runs everything left out (slow importers, suites, wide
  fan-outs).
- **Every run ends on its wall time** (`test-affected: wall time N s`): a step's quick
  lane is meant to stay well under the Bash tool's limit.
- **Slow is named, never failed**: every lane passes `go test -timeout 30m`
  (`KTW_TEST_TIMEOUT`, `scripts/test-env.sh`), not go's 10-minute default.
- **`-short` skips the heavy tests**: real-corpus scans, every-diagram layouts, programs
  started, a real screen - each one listed, with what it costs, in `internal/test/long`
  (`long.Tests`). A heavy test says so on its first line - `long.Test(t)` - and has its row;
  the package's own test refuses a call with no row and a row with no call.
- **A private entry goes in a `//go:build !oss` file**: a test table that names a
  private package, sidecar or extension (a heavy-test row of a private package, an arch
  guard's deviation, an extra command file) gets that entry from a file built only
  without the tag `oss` - `internal/test/long/private.go`, `internal/test/arch/private_test.go` -
  which adds it in `init()` to the table the published file declares. The development
  tree keeps every row; the open-source export leaves the file out, so the published
  tests neither name nor expect it. Check the published form with
  `go vet -tags oss <packages>` (`modules/katyoti/developer-cycle.md` §1).
- **Where the time goes**: every lane ends on the packages over 30 s
  (`scripts/test-report.sh`); one slow in the quick lane is a candidate for a `long.Test`
  row or a cheaper fixture. Each full run keeps its report in `bin/test-runs/` and names the
  packages slower than the previous run.
- **The lanes run the tests without fsync**: `scripts/test-env.sh`, sourced by both lanes'
  scripts (and set by the Makefile's `test`, `test-race`, `cover`), exports
  `KTW_ATOMICIO_NO_SYNC=1` (`atomicio.NoSyncEnv`) - `atomicio.WriteAtomic` stays an atomic
  replace but skips its sync, most of a file-writing package's time on Windows. A test
  that proves the sync itself clears it (`internal/core/atomicio`'s `TestMain`); a bare
  `go test` keeps the syncs.
- Frontend suites (`bun test`) run one at a time.

```bash
go build ./...                                                  # the whole tree compiles
bash scripts/test-affected.sh --paths <files changed>           # the quick lane on what changed
go test -short -p 2 ./internal/extensions/skills/...            # skills corpus gate (after ANY skill edit)
```

## Shipped content is held by tests too

A change to a pack, a skill or a data-type — no Go touched — still has gates:

| Test | Holds | When it fails |
|---|---|---|
| `go test ./app/<sidecar>/ -run TestCensus` | every contribution the binary publishes, against `testdata/census.golden` | an intended change: rerun with `KTW_CENSUS_UPDATE=1` and review the golden's diff |
| `go test ./internal/extensions/manifest/lint/ -run TestShippedPacks_TranslatedForEveryLanguage` | no missing translation in the shipped CWM packs | fill the `<name>.i18n.yaml` (`ktw-compose i18n <pack> --scaffold`) |
| `go test ./internal/test/arch/ -run TestShippedObjectPanelNamesAreUnique` | one tab name per shipped panel | rename the panel |
| `go test ./internal/extensions/cwm/internal/core/ -run Shipped` | every shipped `when` / guard compiles; a pack's gates touch only what it owns | fix the expression; scope the hook or grant it |
| `go test ./internal/core/contribution/ -run TestDataTypeLedger` | a system data-type's fields per version | bump the version (minor to add, major to remove), then `KTW_LEDGER_UPDATE=1` |
| an extension's datatype test | a generated data-type equals its Go type | `KTW_DATATYPE_UPDATE=1 go test ./internal/extensions/<ext>/contrib/` |

## Rules

- After changing anything under `internal/`, rebuild the SIDECARS that embed
  it (`go build -o bin/<sidecar>.exe ./app/<sidecar>`) — a green test against
  a stale binary proves nothing about the live surface. Windows: always build
  with the `.exe` suffix or a stale extension-less twin shadows the build.
- L2 before L3: if a simulator scenario can answer it, never pay tokens.
- L3 runs ONLY when the user explicitly asks — never on your own initiative,
  never as part of routine verify. L1+L2 are the automated gate; L3 verdicts
  come from programmable post-checks (never LLM judging LLM).
- Authoring scenarios: the e2e guide is
  `ktw-code:/reference/modules/e2e-test-authoring.md`; security scenarios
  need `journal_must_not_contain` (negative assertions).
- Pin every fix with the test that would have caught it — a fix without a
  regression test is a fix on loan.
