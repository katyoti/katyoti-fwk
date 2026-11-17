// SPDX-License-Identifier: Apache-2.0

package verify

import (
	"context"
	"fmt"
	"strings"
	"time"

	gcapi "github.com/katyoti/katyoti-fwk/internal/extensions/code/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/code/internal/ucfile"
	"github.com/katyoti/katyoti-fwk/internal/extensions/code/internal/ucobj"
)

// ACResult is the per-acceptance-criterion outcome of a UC-file-driven run.
type ACResult struct {
	ID       string
	Claim    string
	Layer    string
	TestRef  string
	Anti     bool
	Status   string // ucfile 5-state: pass | fail | no-test | fail-deferred | skipped
	Reason   string
	Duration time.Duration

	// Gated marks ACs not evaluated THIS RUN by policy (layer not selected,
	// L3 user-request-only). Gated ACs are excluded from the UC verdict —
	// an un-run layer must not degrade a passing one — and from write-back.
	Gated bool
}

// runFromUseCases drives verify from the module's use cases — the per-AC
// model: each AC's probe runs at its declared layer, the UC verdict is the
// worst of its AC results, and the run is recorded as ONE new `test-run`
// object (verify is the sole writer of verdicts; no use-case or criterion is
// ever edited). A criterion not evaluated this run (its layer not selected,
// L3) keeps its last recorded status in the record.
//
// Layer policy: L1+L2 are the automated gate. L3 ACs are ALWAYS reported
// `skipped` — verify never spends tokens; L3 evidence comes from the
// separate user-requested harness. fail-deferred ACs are preserved (probe
// not run) while their decision stands.
func runFromUseCases(ctx context.Context, opts Options, manifest *Manifest,
	charter *Charter, moduleDir string, files []*ucfile.UCFile, report *Report) (*Report, error) {

	runL1 := opts.Layer == "L1" || opts.Layer == "all"
	runL2 := opts.Layer == "L2" || opts.Layer == "all"

	// In-scope selection + batched L1 collection (one go-test invocation
	// across every L1 ref of every in-scope, non-deferred AC).
	inScope := make([]*ucfile.UCFile, 0, len(files))
	var allL1 []string
	seenL1 := map[string]bool{}
	for _, f := range files {
		if !filterMatch(f.Meta.ID, f.Meta.Severity, opts) {
			report.Results = append(report.Results, &UCResult{
				UCID: f.Meta.ID, Title: f.Meta.Title, Severity: f.Meta.Severity,
				Group: ucGroup(charter, f.Meta.ID), Verdict: VerdictSkipped,
			})
			continue
		}
		inScope = append(inScope, f)
		for _, ac := range f.Meta.AcceptanceCriteria {
			if ac.Layer == "L1" && ac.TestRef != "" && ac.Status != ucfile.StatusFailDeferred && !seenL1[ac.TestRef] {
				seenL1[ac.TestRef] = true
				allL1 = append(allL1, ac.TestRef)
			}
		}
	}

	runner := buildRunner(opts, manifest)
	batched := map[string]TestOutcome{}
	if runL1 && len(allL1) > 0 {
		for _, o := range runner.Run(ctx, allL1) {
			batched[o.Name] = o
		}
	}

	record := map[string]gcapi.TestRunVerdict{}
	for idx, f := range inScope {
		m := f.Meta
		res := &UCResult{
			UCID: m.ID, Title: m.Title, Severity: m.Severity,
			Group:      ucGroup(charter, m.ID),
			DeferredBy: m.FailDeferredBy,
		}
		acStatus := map[string]gcapi.UCStatus{}
		var ucDur time.Duration

		for _, ac := range m.AcceptanceCriteria {
			acr := ACResult{
				ID: ac.ID, Claim: ac.Claim, Layer: ac.Layer,
				TestRef: ac.TestRef, Anti: ac.IsAnti(),
			}
			switch {
			case ac.Status == ucfile.StatusFailDeferred:
				acr.Status = ucfile.StatusFailDeferred
				acr.Reason = "deferred by " + m.FailDeferredBy

			case ac.TestRef == "":
				acr.Status = ucfile.StatusNoTest
				acr.Reason = "no probe declared (open gap)"

			case ac.Layer == "L3":
				acr.Status = ucfile.StatusSkipped
				acr.Reason = "L3 — runs only on explicit user request"
				acr.Gated = true

			case ac.Layer == "L1":
				if !runL1 {
					acr.Status = ucfile.StatusSkipped
					acr.Reason = "L1 not selected this run"
					acr.Gated = true
					break
				}
				o, ok := batched[ac.TestRef]
				if !ok {
					o = TestOutcome{Name: ac.TestRef, Status: StatusNoMatch}
				}
				acr.Status, acr.Reason = acStatusFromOutcome(o)
				acr.Duration = o.Duration
				ucDur += o.Duration

			case ac.Layer == "L2":
				if !runL2 {
					acr.Status = ucfile.StatusSkipped
					acr.Reason = "L2 not selected this run"
					acr.Gated = true
					break
				}
				combined := runL2Scenarios(ctx, opts.ProjectRoot, opts.Module, m.ID, []string{ac.TestRef}, opts.KeepTemp)
				o := TestOutcome{Name: ac.TestRef, Status: combined.Status, Duration: combined.TotalTime, Output: combined.FailReason}
				acr.Status, acr.Reason = acStatusFromOutcome(o)
				acr.Duration = combined.TotalTime
				ucDur += combined.TotalTime

			default:
				acr.Status = ucfile.StatusNoTest
				acr.Reason = fmt.Sprintf("unknown layer %q", ac.Layer)
			}
			res.ACs = append(res.ACs, acr)
			switch {
			case !acr.Gated:
				acStatus[ac.ID] = gcapi.UCStatus(acr.Status)
			case ac.Status != "":
				acStatus[ac.ID] = gcapi.UCStatus(ac.Status) // not run: the last recorded status
			}
		}

		res.Verdict, res.Reason = verdictFromACs(res.ACs)
		report.Results = append(report.Results, res)
		record[m.ID] = gcapi.TestRunVerdict{Verdict: gcapi.UCStatus(res.Verdict.String()), Reason: res.Reason, ACs: acStatus}

		if opts.OnProgress != nil {
			opts.OnProgress(idx+1, len(inScope), m.ID, res.Verdict, ucDur)
		}
	}

	report.Source = "use-cases"
	report.ComputeTotals()
	if !opts.NoWrite && len(record) > 0 {
		path, err := ucobj.WriteRun(ctx, opts.CWM, opts.Module, ucobj.Run{
			RanAt: report.RanAt, LayerFilter: opts.Layer, UCFilter: strings.Join(opts.UCFilter, ","),
			Verdicts: record, UCTotal: len(record),
		})
		if err != nil {
			return nil, fmt.Errorf("record the run: %w", err)
		}
		if opts.RunPath != nil {
			*opts.RunPath = path
		}
	}
	return report, nil
}

// acStatusFromOutcome maps a runner outcome to the ucfile 5-state.
// Error / no-match collapse into fail — a probe that cannot run is a
// failing probe (drift surfaces red, never silently green).
func acStatusFromOutcome(o TestOutcome) (status, reason string) {
	switch o.Status {
	case StatusPass:
		return ucfile.StatusPass, ""
	case StatusSkip:
		return ucfile.StatusSkipped, "test skipped itself"
	case StatusNoMatch:
		return ucfile.StatusFail, fmt.Sprintf("probe %s matched no test (dead ref)", o.Name)
	default: // StatusFail, StatusError, unknown
		return ucfile.StatusFail, o.Output
	}
}

// verdictFromACs reduces AC statuses to the UC verdict using the same
// worst-of ranking as ucfile.WorstOf (fail > no-test > skipped >
// fail-deferred > pass). Gated ACs (layer not run this cycle) are
// excluded — an un-run layer must not degrade a passing one. A UC whose
// every AC is gated has no evidence this run → no-test.
func verdictFromACs(acs []ACResult) (Verdict, string) {
	if len(acs) == 0 {
		return VerdictNoTest, "UC declares no acceptance criteria"
	}
	evaluated := make([]ACResult, 0, len(acs))
	for _, a := range acs {
		if !a.Gated {
			evaluated = append(evaluated, a)
		}
	}
	if len(evaluated) == 0 {
		return VerdictNoTest, "no L1/L2 evidence this run (all criteria gated)"
	}
	acs = evaluated
	worst := VerdictUnknown
	reason := ""
	rank := func(v Verdict) int {
		switch v {
		case VerdictFail:
			return 0
		case VerdictNoTest:
			return 1
		case VerdictSkipped:
			return 2
		case VerdictFailDeferred:
			return 3
		case VerdictPass:
			return 4
		}
		return 5
	}
	for _, a := range acs {
		var v Verdict
		switch a.Status {
		case ucfile.StatusPass:
			v = VerdictPass
		case ucfile.StatusFail:
			v = VerdictFail
		case ucfile.StatusNoTest:
			v = VerdictNoTest
		case ucfile.StatusFailDeferred:
			v = VerdictFailDeferred
		case ucfile.StatusSkipped:
			v = VerdictSkipped
		default:
			v = VerdictFail
		}
		if worst == VerdictUnknown || rank(v) < rank(worst) {
			worst = v
			reason = a.Reason
			if v == VerdictFail && a.Reason == "" {
				reason = a.ID + " failed"
			}
		}
	}
	if worst == VerdictPass {
		reason = ""
	}
	return worst, reason
}

// ucGroup looks up the charter subsection label for display grouping —
// UC files don't carry groups; the charter (when present) still knows.
func ucGroup(charter *Charter, ucID string) string {
	if charter == nil {
		return ""
	}
	if info := charter.UCs[ucID]; info != nil {
		return info.Group
	}
	return ""
}
