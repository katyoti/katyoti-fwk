// SPDX-License-Identifier: Apache-2.0

// Package verify is the methods pack's verification toolkit: the
// per-domain probe-runner seam (the LanguagePack `RunUnitTest` analogue) that
// runs ONE acceptance-criterion probe, and the basic linters. A verdict is
// exactly a probe's result, never a judgment.
package verify

import "context"

// ProbeResult is one acceptance-criterion probe's outcome.
type ProbeResult struct {
	Status   string // "pass" | "fail" | "harness-error"
	Evidence string // a command, path, or message a reader can check
}

// ProbeRunner runs ONE acceptance-criterion probe by its `test_ref`. This is
// the per-domain SEAM — the LanguagePack `RunUnitTest` analogue. A files-only
// pack runs deterministic checks (file exists, a URI resolves, a corpus is
// green); a code pack runs `go test`; etc. A probe that cannot run returns
// `harness-error` (which counts as not-passing — never a silent pass).
type ProbeRunner interface {
	RunProbe(ctx context.Context, testRef string) ProbeResult
}
