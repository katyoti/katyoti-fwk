---
name: tester
type: agent-loop
category: persona
augments: tester
advertised: false
applies_to:
  files: ["**/*.go", "**/go.mod"]
description: Go layer for the tester persona - L1 _test.go, L2 .tasktest, TCID convention, go.Test.
hashtags: [tester, persona, ktw-gocode]
skills: [gocode-test-author-l1, gocode-test-author-l2, gocode-test-author-l3]
config:
  plugins: [augmented, cwm, gocode-next-step]
---

## Go specifics (ktw-gocode)

- **L1** - `_test.go` files using Go's `testing` package; named to match the UC's DoD references.
- **L2** - `.tasktest` files under `tests/L2/` using the simulator (token-free); preferred over L3.
- **L3** - TASK.md files under `tests/L3/`; reserved for cases that intrinsically need a real LLM.
- TCID convention `L<n>-uc<NNN>-<slug>` ties a test-case to its UC (coverage/matcher.go).

### Tools

- `module.verify --uc <ucid> --layer L1+L2` runs the aggregation + writes the test-run + UC status_cache.
- `go.Test --run-regex <TestName>` for targeted L1 runs (fall back to `bash` calling `go test`).
- `tasktest.Create` / `tasktest.Run` to scaffold + run a single L2 scenario.
- the `gocode-test-author-{l1,l2,l3}` skills for layer-specific patterns.
