// SPDX-License-Identifier: Apache-2.0

// Package realization implements the feature realization-status
// derivation (method-core): "is feature F delivered?" computed from the
// status of the Use Cases it `wants` (worst-of). The UC's own status IS the
// leaf signal — there is no module tier-state hop (a US reaches its delivered
// behavior through US→`wants`→Use Case, and each UC `lives-in` its Module
// structurally).
//
// Pure logic on typed inputs. The wiring layer (gocode/internal/core/
// methods_realization.go) walks `wants` edges via EdgeList, reads each wanted
// UC's status_cache, and feeds the result here.
//
// Compute-on-read: no caching layer.
package realization

// Status is the per-UC derived value, rolled up to the feature/US.
type Status string

const (
	// StatusNotStarted — UC has no test yet (no-test / skipped / empty).
	StatusNotStarted Status = "not-started"
	// StatusInProgress — UC failing / in-dev / deferred — work to do.
	StatusInProgress Status = "in-progress"
	// StatusVerifying — reserved (a UC under validation); not currently
	// produced by DeriveUCStatus but kept for the worst-of ladder.
	StatusVerifying Status = "verifying"
	// StatusRegressed — UC was passing, now red. Dominates the rollup.
	StatusRegressed Status = "regressed"
	// StatusDone — UC passing (pass / validated / covered).
	StatusDone Status = "done"
)

// DeriveUCStatus maps a Use Case's `status_cache` to a realization Status.
// The UC's 9-state status_cache collapses to the 5 realization values:
//
//   - done        — pass / validated / covered
//   - regressed   — regressed
//   - in-progress — fail / in-dev / fail-deferred
//   - not-started — no-test / skipped / empty (otherwise)
func DeriveUCStatus(statusCache string) Status {
	switch statusCache {
	case "pass", "validated", "covered":
		return StatusDone
	case "regressed":
		return StatusRegressed
	case "fail", "in-dev", "fail-deferred":
		return StatusInProgress
	default: // no-test, skipped, ""
		return StatusNotStarted
	}
}

// AggregateFeatureStatus reduces a US's wanted-UC statuses to a single
// feature-level verdict via the worst-of rule:
//
//   - any regressed     ⇒ regressed
//   - else any not-started ⇒ not-started
//   - else any in-progress ⇒ in-progress
//   - else any verifying   ⇒ verifying
//   - all done             ⇒ done
//
// An empty list (a US that `wants` no UC) returns `not-started` — the US
// has no delivered behavior claimed yet.
func AggregateFeatureStatus(ucs []Status) Status {
	if len(ucs) == 0 {
		return StatusNotStarted
	}
	hasRegressed := false
	hasNotStarted := false
	hasInProgress := false
	hasVerifying := false
	allDone := true
	for _, s := range ucs {
		switch s {
		case StatusRegressed:
			hasRegressed = true
		case StatusNotStarted:
			hasNotStarted = true
			allDone = false
		case StatusInProgress:
			hasInProgress = true
			allDone = false
		case StatusVerifying:
			hasVerifying = true
			allDone = false
		case StatusDone:
			// keep allDone alive
		default:
			allDone = false
		}
	}
	switch {
	case hasRegressed:
		return StatusRegressed
	case hasNotStarted:
		return StatusNotStarted
	case hasInProgress:
		return StatusInProgress
	case hasVerifying:
		return StatusVerifying
	case allDone:
		return StatusDone
	default:
		return StatusInProgress
	}
}
