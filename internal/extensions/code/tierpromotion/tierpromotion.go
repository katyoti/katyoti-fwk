// SPDX-License-Identifier: Apache-2.0

// Package tierpromotion implements the per-module tier-promotion FSM
// (8 states) + the promote-gate evaluation that the `module promote`
// verb consults. See modules/code-gocode/concepts.md §6.
//
// Pure logic on typed inputs. The wiring layer (gocode/internal/core/
// methods_promote.go) gathers inputs from CWM, calls EvaluateGates +
// NextStateAfter, writes back the tier-state transition.
package tierpromotion

import (
	"fmt"

	gcapi "github.com/katyoti/katyoti-fwk/internal/extensions/code/api"
)

// GateInputs is the typed bundle the promote gate evaluates.
type GateInputs struct {
	// Module state — what we are promoting.
	TierCurrent int
	TierTarget  int

	// Charter — projection v ≥ 0.1 means the charter has been
	// published at least once.
	CharterVersion string

	// UC counters — populated from the latest verify run (or live).
	// Gate passes when all UCs are pass-or-fail-deferred (no fail,
	// no no-test).
	UCTotalFail   int
	UCTotalNoTest int
	UCTotal       int

	// Audit — latest audit review's open critical / high findings.
	// Gate passes when both are zero.
	AuditCritOpen int
	AuditHighOpen int

	// Lint + drift — placeholders until lint-detectors and
	// go-ast-discovery land. Wiring layer leaves these false; the
	// gate evaluator surfaces them as "pending" entries in the
	// Pending output so an operator sees what's still manual.
	LintBroken bool
	DriftFound bool
}

// GateOutcome is the result of evaluating the 5-condition promote gate.
type GateOutcome struct {
	// Passes is true iff every gate is satisfied. Forced promotion
	// can still proceed when Passes=false (caller decides).
	Passes bool

	// Blockers lists the unmet gates in evaluation order. Each entry
	// is a human-readable string like "3 UC failing (must pass or
	// fail-defer)" suitable for direct rendering to the operator.
	Blockers []string

	// GatesMet lists the gates that were satisfied. Useful for
	// reporting progress after a successful promotion ("4/5 gates
	// passed; auto-promoted").
	GatesMet []string

	// Pending lists gates that are documented but not yet checkable
	// (lint, drift). Surfaces so operators see what's still manual.
	Pending []string

	// HasUCBlocker / HasAuditBlocker are pre-computed booleans the
	// NextStateAfter function consults to decide whether a verb
	// transition lands in its happy-path state or falls back to
	// `gated`. Set by EvaluateGates as it goes.
	HasUCBlocker    bool
	HasAuditBlocker bool
}

// EvaluateGates runs the 5-condition promote-gate per modules/code-gocode/concepts.md §6.
func EvaluateGates(in GateInputs) GateOutcome {
	out := GateOutcome{Passes: true}

	// Gate 1: Charter v ≥ 0.1
	if in.CharterVersion == "" || in.CharterVersion < "0.1" {
		out.Passes = false
		out.Blockers = append(out.Blockers, "charter version < 0.1")
	} else {
		out.GatesMet = append(out.GatesMet, "charter version ≥ 0.1")
	}

	// Gate 2: All UCs pass-or-deferred (no fail, no no-test)
	switch {
	case in.UCTotalFail > 0:
		out.Passes = false
		out.HasUCBlocker = true
		out.Blockers = append(out.Blockers,
			fmt.Sprintf("%d UC failing (must pass or fail-defer)", in.UCTotalFail))
	case in.UCTotalNoTest > 0:
		out.Passes = false
		out.HasUCBlocker = true
		out.Blockers = append(out.Blockers,
			fmt.Sprintf("%d UC with no test", in.UCTotalNoTest))
	default:
		out.GatesMet = append(out.GatesMet, "all UCs pass or fail-deferred")
	}

	// Gate 3: No critical/high audit findings open
	if in.AuditCritOpen > 0 {
		out.Passes = false
		out.HasAuditBlocker = true
		out.Blockers = append(out.Blockers,
			fmt.Sprintf("%d critical audit finding(s) open", in.AuditCritOpen))
	}
	if in.AuditHighOpen > 0 {
		out.Passes = false
		out.HasAuditBlocker = true
		out.Blockers = append(out.Blockers,
			fmt.Sprintf("%d high-severity audit finding(s) open", in.AuditHighOpen))
	}
	if in.AuditCritOpen == 0 && in.AuditHighOpen == 0 {
		out.GatesMet = append(out.GatesMet, "0 critical / 0 high audit findings")
	}

	// Gate 4: Lint clean
	if in.LintBroken {
		out.Passes = false
		out.Blockers = append(out.Blockers, "lint rule violations open (severity: error)")
	} else {
		out.Pending = append(out.Pending,
			"lint (pending: ktw-gocode-lint-detectors task — defaults to clean)")
	}

	// Gate 5: No API/code drift
	if in.DriftFound {
		out.Passes = false
		out.Blockers = append(out.Blockers, "API/code-symbol drift detected")
	} else {
		out.Pending = append(out.Pending,
			"API contract drift (pending: go-ast-discovery task — defaults to clean)")
	}

	return out
}

// NextStateAfter computes the tier-FSM transition resulting from a
// successful verb invocation. Returns the new state; caller writes it
// back to the module's `tier_state` field via NodeUpdate.
//
// Per modules/code-gocode/concepts.md §6 the transitions are:
//
//	plan-tier  → planning
//	verify     → verifying  (when all UCs pass-or-deferred)
//	audit      → auditing   (when 0 critical + 0 high)
//	promote    → promoted   (gate passes; tier_current advances)
//
// Other states are entered defensively:
//
//	bootstrapping — no charter yet
//	specifying    — charter exists but no UCs
//	gated         — verb's preconditions failed (UC/audit blocker)
func NextStateAfter(verb string, prev gcapi.TierState, gates GateOutcome, charterVersion string, ucTotal int) gcapi.TierState {
	switch verb {
	case "plan-tier":
		return gcapi.TierStatePlanning
	case "verify":
		if !gates.HasUCBlocker {
			return gcapi.TierStateVerifying
		}
		return gcapi.TierStateGated
	case "audit":
		if !gates.HasAuditBlocker {
			return gcapi.TierStateAuditing
		}
		return gcapi.TierStateGated
	case "promote":
		if gates.Passes {
			return gcapi.TierStatePromoted
		}
		return gcapi.TierStateGated
	}

	// No verb match — recompute the default state from observable
	// inputs. Useful for ModuleStatus to report a sensible state
	// even when no verb has run yet.
	if charterVersion == "" {
		return gcapi.TierStateBootstrapping
	}
	if ucTotal == 0 {
		return gcapi.TierStateSpecifying
	}
	if prev == "" {
		return gcapi.TierStateBootstrapping
	}
	return prev
}
