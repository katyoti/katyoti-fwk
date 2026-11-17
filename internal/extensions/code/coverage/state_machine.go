// SPDX-License-Identifier: Apache-2.0

// Package coverage implements the per-use-case coverage state machine
// (5 states + fail-deferred) and the verify-time aggregator that drives
// the `module verify` verb. See modules/code-gocode/concepts.md §4.
//
// Items are the source of truth; this package is pure logic on typed
// inputs. The wiring layer (gocode/internal/core/methods_verify.go)
// loads CWM items, calls the pure functions here, and writes back
// status_cache + a test-run record.
//
// Status_cache is honor-system protected — only the `verify` verb
// should write it. The lint-detectors task will flag hand-edits.
package coverage

import gcapi "github.com/katyoti/katyoti-fwk/internal/extensions/code/api"

// DeriveStatus is the pure state transition. Given a use-case's
// previous status_cache value and the run-states of every test-case
// claiming to verify it, returns the new status_cache.
//
// Inputs:
//
//   - prev: the use-case's last-known status_cache value
//   - runStates: last_run_status of every test-case linked to this UC
//   - deferringODOpen: true iff the UC links to a `deferred-by` OD
//     that's currently in state=open
//
// Output is the 9-state status_cache value. Use VerdictOf to project
// down to the 4-verdict user-facing view.
//
// The rules (per modules/code-gocode/concepts.md §4):
//
//   - deferringODOpen → fail-deferred (regardless of test state)
//   - no test-cases at all → no-test
//   - any harness-error → fail (treated as failure)
//   - any fail → fail (regressed if previously passing)
//   - all pass + previously passing → validated/covered (preserved)
//   - all pass + previously not-passing → covered (newly green)
//   - all not-run → no-test
//
// Note: the v1 state machine collapses validated and covered into
// "covered" until the audit review layer lands (validated requires
// an audit-validated tier signal; without it, every passing UC is
// just "covered"). This degrades gracefully — the user-facing
// verdict is still "pass" either way.
func DeriveStatus(prev gcapi.UCStatus, runStates []gcapi.TestRunStatus, deferringODOpen bool) gcapi.UCStatus {
	if deferringODOpen {
		return gcapi.UCStatusFailDeferred
	}
	if len(runStates) == 0 {
		return gcapi.UCStatusNoTest
	}

	var anyFail, anyPass, allNotRun = false, false, true
	for _, s := range runStates {
		switch s {
		case gcapi.TestRunPass:
			anyPass = true
			allNotRun = false
		case gcapi.TestRunFail, gcapi.TestRunHarnessError:
			anyFail = true
			allNotRun = false
		case gcapi.TestRunNotRun, "":
			// allNotRun stays true unless something else flips it
		}
	}

	if allNotRun {
		return gcapi.UCStatusNoTest
	}
	if anyFail {
		// Regressed if previously passing; in-dev if not yet green
		if prev == gcapi.UCStatusValidated || prev == gcapi.UCStatusCovered || prev == gcapi.UCStatusPass {
			return gcapi.UCStatusRegressed
		}
		return gcapi.UCStatusInDev
	}
	// All matter passing (anyPass and no fail). Preserve validated
	// when previously validated (covers "still passing post-audit"
	// case). Otherwise: covered.
	if prev == gcapi.UCStatusValidated && anyPass {
		return gcapi.UCStatusValidated
	}
	return gcapi.UCStatusCovered
}

// VerdictOf projects the 9-state status_cache into the 4-verdict
// view that user-facing surfaces use (verify --format=text output,
// charter.md status column, manifest counters).
//
// Mapping per modules/code-gocode/concepts.md §4:
//
//	pass          ← validated / covered / pass
//	fail          ← in-dev / regressed / fail
//	no-test       ← no-test / (empty)
//	fail-deferred ← fail-deferred
//	skipped       ← skipped (filter passthrough)
func VerdictOf(s gcapi.UCStatus) gcapi.UCStatus {
	switch s {
	case gcapi.UCStatusValidated, gcapi.UCStatusCovered, gcapi.UCStatusPass:
		return gcapi.UCStatusPass
	case gcapi.UCStatusInDev, gcapi.UCStatusRegressed, gcapi.UCStatusFail:
		return gcapi.UCStatusFail
	case gcapi.UCStatusFailDeferred:
		return gcapi.UCStatusFailDeferred
	case gcapi.UCStatusSkipped:
		return gcapi.UCStatusSkipped
	default:
		return gcapi.UCStatusNoTest
	}
}
