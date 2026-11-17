// SPDX-License-Identifier: Apache-2.0

// Package modulehealth implements the per-module Health derivation
// (3 states: healthy / degraded / broken). The state is computed,
// not stored — it's projected from UC verdicts + open audit findings
// (critical/high) + lint state on every read. See modules/code/concepts.md §4.3.
//
// Pure logic on typed inputs. The wiring layer (ModuleStatus +
// ModuleVerify) gathers the inputs and stamps the result onto the
// output's `Module.Health` field.
package modulehealth

import (
	gcapi "github.com/katyoti/katyoti-fwk/internal/extensions/code/api"
)

// Inputs is the typed bundle the wiring layer feeds in. All fields
// are optional in the sense that missing data degrades to "safe"
// defaults (e.g. zero counts → no penalty).
type Inputs struct {
	// UCTotals — the 4-verdict projection already computed by the
	// coverage state machine. Health only consumes the counts.
	UCTotals gcapi.TestRunTotals

	// AuditCritOpen / AuditHighOpen — open findings on the latest
	// audit review, severity-bucketed. Older bundles are historical.
	AuditCritOpen int
	AuditHighOpen int

	// BuildFailing — true when the module's package(s) fail to compile
	// or `go test` reports harness errors. Wiring layer leaves this
	// false until the GoTool integration lands (Phase F).
	BuildFailing bool

	// LintBroken — true when the lint-detectors verb flags rule-
	// violations on this module. Wiring layer leaves this false until
	// the lint-detectors task lands.
	LintBroken bool
}

// Derive returns the 3-state health verdict. Rules per modules/code/concepts.md §4.3:
//
//   - broken    — any UC regressed/failing, OR critical/high audit
//     findings open, OR build failing
//   - degraded  — some UCs covered/in-dev (not yet validated), OR
//     medium audit findings open (deferred — once lint
//   - medium-severity tracking land), OR lint broken
//   - healthy   — all UCs validated/pass, no open audit critical/
//     high findings, build green, lint clean
//
// Defaults to `healthy` when Inputs is empty (zero-value modules).
func Derive(in Inputs) gcapi.ModuleHealth {
	// Broken — any of: UCs failing (non-deferred), open critical/
	// high audit findings, build failing.
	if in.UCTotals.Fail > 0 {
		return gcapi.ModuleHealthBroken
	}
	if in.AuditCritOpen > 0 || in.AuditHighOpen > 0 {
		return gcapi.ModuleHealthBroken
	}
	if in.BuildFailing {
		return gcapi.ModuleHealthBroken
	}

	// Degraded — UCs without tests (work-in-progress), fail-deferred
	// UCs (known gaps tracked by an OD), or lint flagged.
	if in.UCTotals.NoTest > 0 || in.UCTotals.FailDeferred > 0 {
		return gcapi.ModuleHealthDegraded
	}
	if in.LintBroken {
		return gcapi.ModuleHealthDegraded
	}

	return gcapi.ModuleHealthHealthy
}
