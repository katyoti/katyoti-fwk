// SPDX-License-Identifier: Apache-2.0

package verify

import (
	"regexp"
	"strings"
)

// Verdict is the per-UC outcome surfaced by `module verify`. Mirrors the
// verdict semantics in [verbs/verify.md](app/ktw-code/embed/references/modules/verbs/verify.md).
type Verdict int

const (
	VerdictUnknown      Verdict = iota
	VerdictPass                 // 🟢 — all applicable layers green
	VerdictFail                 // 🔴 — bug; blocks promotion
	VerdictFailDeferred         // 🟠 — known-failing by explicit OD; doesn't block
	VerdictNoTest               // ⚪ — UC has no Test plan row
	VerdictSkipped              // ⏸ — filtered out by --uc / --severity / DoR
)

func (v Verdict) String() string {
	switch v {
	case VerdictPass:
		return "pass"
	case VerdictFail:
		return "fail"
	case VerdictFailDeferred:
		return "fail-deferred"
	case VerdictNoTest:
		return "no-test"
	case VerdictSkipped:
		return "skipped"
	}
	return "unknown"
}

// Symbol returns the emoji used in the text-tree output.
func (v Verdict) Symbol() string {
	switch v {
	case VerdictPass:
		return "🟢"
	case VerdictFail:
		return "🔴"
	case VerdictFailDeferred:
		return "🟠"
	case VerdictNoTest:
		return "⚪"
	case VerdictSkipped:
		return "⏸"
	}
	return "❓"
}

// BlocksPromotion reports whether this verdict prevents tier promotion.
// Per the verb spec: only fail and no-test block; fail-deferred is documented.
func (v Verdict) BlocksPromotion() bool {
	return v == VerdictFail || v == VerdictNoTest
}

// UCResult is the per-UC rollup the orchestrator produces and the emitters
// consume. Carries enough info for any output format (text, JUnit, JSON).
type UCResult struct {
	UCID     string
	Title    string
	Severity string
	Group    string // charter subsection label, e.g. "Permissions (UC-050 to UC-058)"

	// L1, L2, L3 are the per-layer rollups. Phase B populates L1; Phase C.1
	// populates L2 from .tasktest scenarios; Phase C.2 strengthens with
	// journal + dimension assertions.
	// Empty CombinedOutcome.UCID signals "layer not applicable to this UC".
	L1 CombinedOutcome
	L2 CombinedOutcome
	L3 CombinedOutcome

	Verdict    Verdict
	Reason     string // populated on fail / fail-deferred / no-test
	DeferredBy string // OD-XXX when verdict is fail-deferred

	// ACs carries the per-acceptance-criterion results when the run was
	// driven by UC files (Report.Source == "use-cases"). Empty on
	// charter-driven runs.
	ACs []ACResult
}

// ResolveVerdict reduces an L1-only UCResult into a final Verdict per the
// verb spec's rules. Phase B-only — Phase C extends with L2/L3 logic.
func ResolveVerdict(r *UCResult, row *UCRow) Verdict {
	// Detect fail-deferred from notes/title (OD-004, OD-005, etc.). The cite
	// can live in either the Test plan row's trailing Notes or the UC table's
	// Title (e.g., UC-073's title says "see OD-004"). If either explicitly
	// cites a deferral, treat failures as documented limitations rather than bugs.
	if row != nil && hasDeferralCite(row.Notes) {
		r.DeferredBy = extractDeferralOD(row.Notes)
		return VerdictFailDeferred
	}
	if hasDeferralCite(r.Title) {
		r.DeferredBy = extractDeferralOD(r.Title)
		return VerdictFailDeferred
	}

	// Aggregate L1 + L2 status when both ran. Severity-worst-wins matches
	// the per-test rollup logic so a mixed pass/fail surface flips the UC red.
	statuses := []TestStatus{}
	if r.L1.UCID != "" {
		statuses = append(statuses, r.L1.Status)
	}
	if r.L2.UCID != "" {
		statuses = append(statuses, r.L2.Status)
	}
	if len(statuses) == 0 {
		return VerdictNoTest
	}
	worst := worstStatus(statuses)
	switch worst {
	case StatusPass:
		return VerdictPass
	case StatusFail, StatusError:
		r.Reason = firstFailReason(r)
		return VerdictFail
	case StatusNoMatch:
		r.Reason = "no test matched the named L1/L2 tests"
		return VerdictFail
	case StatusSkip:
		r.Reason = "all tests skipped"
		return VerdictNoTest
	}
	return VerdictUnknown
}

// worstStatus picks the most severe status from the list (Error > Fail >
// NoMatch > Skip > Pass > Unknown). Mirrors the per-UC Combine() logic so
// L1 + L2 + future L3 surfaces aggregate consistently.
func worstStatus(list []TestStatus) TestStatus {
	severity := func(s TestStatus) int {
		switch s {
		case StatusError:
			return 5
		case StatusFail:
			return 4
		case StatusNoMatch:
			return 3
		case StatusSkip:
			return 2
		case StatusPass:
			return 1
		}
		return 0
	}
	worst := list[0]
	for _, s := range list[1:] {
		if severity(s) > severity(worst) {
			worst = s
		}
	}
	return worst
}

// firstFailReason returns the L1 fail reason if present, else L2's. Used by
// the verdict resolver to populate UCResult.Reason.
func firstFailReason(r *UCResult) string {
	if r.L1.FailReason != "" {
		return r.L1.FailReason
	}
	if r.L2.FailReason != "" {
		return r.L2.FailReason
	}
	return ""
}

// hasDeferralCite returns true if Notes/Title mentions an OD-driven deferral.
// Detects either the explicit verbiage ("fail-deferred", "deferred-by-decision")
// OR a "see OD-XXX" / "OD-XXX" cite — covers both Test plan notes and UC titles.
var ucDeferralCiteRe = regexp.MustCompile(`(?i)(see\s+)?\bOD-\d+\b`)

func hasDeferralCite(notes string) bool {
	if notes == "" {
		return false
	}
	low := strings.ToLower(notes)
	if strings.Contains(low, "fail-deferred") || strings.Contains(low, "deferred-by-decision") {
		return true
	}
	// "see OD-004" / "OD-005" / similar — common in UC titles for deferred UCs.
	return ucDeferralCiteRe.MatchString(notes) &&
		(strings.Contains(low, "deferred") || strings.Contains(low, "see "))
}

// extractDeferralOD pulls the OD ID out of a notes string when present.
func extractDeferralOD(notes string) string {
	for _, token := range strings.Fields(notes) {
		t := strings.Trim(token, "(),.;:")
		if strings.HasPrefix(strings.ToUpper(t), "OD-") {
			return strings.ToUpper(t)
		}
	}
	return ""
}
