// SPDX-License-Identifier: Apache-2.0

package coverage

import (
	"time"

	gcapi "github.com/katyoti/katyoti-fwk/internal/extensions/code/api"
)

// AggregateInputs is the typed bundle the verify verb assembles
// before calling Aggregate. All slices are owned by the caller.
type AggregateInputs struct {
	Module    gcapi.Module
	UseCases  []gcapi.UseCase
	TestCases []gcapi.TestCase
	// OpenODByID indexes decision items currently in state=open
	// by their OD-NNN id. Used to decide fail-deferred.
	OpenODByID map[string]gcapi.OpenDecision
	// LayerFilter restricts which test-case layers count toward the
	// aggregation. Empty / "all" → no filter. Per-layer values
	// follow the CLI: "L1" / "L2" / "L3" / "L1+L2" / "all".
	LayerFilter string
	// UCFilter restricts which UCs the run verifies. Empty → all.
	// Accepts exact ids ("UC-001"), prefix wildcards ("UC-04*"),
	// or ranges ("UC-053..UC-058"). v1 supports exact + prefix only;
	// range support deferred.
	UCFilter string
	// SeverityFilter: "core" | "security" | "core+security" | "all"
	// (empty = "all").
	SeverityFilter string
	// RanAt timestamp to stamp on per-UC verdicts and the test-run.
	RanAt time.Time
}

// AggregateOutput is what the verify verb gets back.
type AggregateOutput struct {
	// PerUC maps UC-NNN → derived new status_cache value (caller writes back).
	PerUC map[string]UCVerdict
	// Totals are the projected 4-verdict counts.
	Totals gcapi.TestRunTotals
	// TestRunVerdicts is the shape the test-run item's verdicts
	// field needs (UC-NNN → TestRunVerdict).
	TestRunVerdicts map[string]gcapi.TestRunVerdict
	// OrphanTestCases lists indices of test-cases whose TCID didn't
	// parse — surfaced via the lint detector.
	OrphanTestCases []int
}

// UCVerdict holds both the new internal status_cache and the
// projected user-facing verdict for one UC after a verify run.
type UCVerdict struct {
	PreviousStatus gcapi.UCStatus
	NewStatus      gcapi.UCStatus
	Verdict        gcapi.UCStatus // 4-verdict projection
	Reason         string         // e.g. "fail-deferred by OD-002" / "L1 test failed"
	Skipped        bool           // true if filtered out
}

// Aggregate runs the per-UC state machine across the inputs.
// Deterministic given identical inputs. Caller writes back each
// UC's NewStatus via cwm.ItemUpdate and creates a test-run item
// from PerUC + Totals + TestRunVerdicts.
func Aggregate(in AggregateInputs) AggregateOutput {
	out := AggregateOutput{
		PerUC:           map[string]UCVerdict{},
		TestRunVerdicts: map[string]gcapi.TestRunVerdict{},
	}

	// Index test-cases by UC via TCID convention.
	byUC, orphans := GroupTestCasesByUC(in.TestCases, func(tc gcapi.TestCase) string {
		return tc.TCID
	})
	out.OrphanTestCases = orphans

	for _, uc := range in.UseCases {
		ucID, ok := NormalizeUCID(uc.UCID)
		if !ok {
			// UC with a non-canonical id — skip; surfaces via lint.
			continue
		}

		// Filter
		if !ucMatchesFilters(uc, ucID, in.UCFilter, in.SeverityFilter) {
			out.PerUC[ucID] = UCVerdict{
				PreviousStatus: uc.StatusCache,
				NewStatus:      uc.StatusCache,
				Verdict:        gcapi.UCStatusSkipped,
				Skipped:        true,
			}
			continue
		}

		// Gather run states for this UC's linked test-cases.
		var runStates []gcapi.TestRunStatus
		layerOutcomes := map[gcapi.UCLayer]gcapi.TestRunStatus{}
		for _, idx := range byUC[ucID] {
			tc := in.TestCases[idx]
			if !layerInFilter(tc.Layer, in.LayerFilter) {
				continue
			}
			runStates = append(runStates, tc.LastRunStatus)
			layerOutcomes[tc.Layer] = worstLayerOutcome(layerOutcomes[tc.Layer], tc.LastRunStatus)
		}

		// Is this UC deferred by an open OD?
		deferringODOpen := false
		var reason string
		if uc.FailDeferredBy != "" {
			if od, ok := in.OpenODByID[uc.FailDeferredBy]; ok && od.State == gcapi.ODStateOpen {
				deferringODOpen = true
				reason = "fail-deferred by " + uc.FailDeferredBy
			}
		}

		newStatus := DeriveStatus(uc.StatusCache, runStates, deferringODOpen)
		verdict := VerdictOf(newStatus)
		if verdict == gcapi.UCStatusFail && reason == "" {
			reason = describeFailure(layerOutcomes)
		}

		out.PerUC[ucID] = UCVerdict{
			PreviousStatus: uc.StatusCache,
			NewStatus:      newStatus,
			Verdict:        verdict,
			Reason:         reason,
		}

		out.TestRunVerdicts[ucID] = gcapi.TestRunVerdict{
			Layers:  layerOutcomes,
			Verdict: verdict,
			Reason:  reason,
		}

		switch verdict {
		case gcapi.UCStatusPass:
			out.Totals.Pass++
		case gcapi.UCStatusFail:
			out.Totals.Fail++
		case gcapi.UCStatusFailDeferred:
			out.Totals.FailDeferred++
		case gcapi.UCStatusNoTest:
			out.Totals.NoTest++
		case gcapi.UCStatusSkipped:
			out.Totals.Skipped++
		}
		out.Totals.UCTotal++
	}

	return out
}

// --- filter / formatting helpers ---------------------------------------

func ucMatchesFilters(uc gcapi.UseCase, ucID, ucFilter, severityFilter string) bool {
	if !severityMatches(uc.Severity, severityFilter) {
		return false
	}
	if !ucIDMatches(ucID, ucFilter) {
		return false
	}
	return true
}

func severityMatches(sev gcapi.UCSeverity, filter string) bool {
	switch filter {
	case "", "all":
		return true
	case "core":
		return sev == gcapi.UCSeverityCore
	case "security":
		return sev == gcapi.UCSeveritySecurity
	case "core+security":
		return sev == gcapi.UCSeverityCore || sev == gcapi.UCSeveritySecurity
	}
	return true
}

func ucIDMatches(ucID, filter string) bool {
	if filter == "" || filter == "*" {
		return true
	}
	// Prefix wildcard: "UC-04*" matches "UC-040".."UC-049"
	if l := len(filter); l > 0 && filter[l-1] == '*' {
		return len(ucID) >= l-1 && ucID[:l-1] == filter[:l-1]
	}
	return ucID == filter
}

func layerInFilter(layer gcapi.UCLayer, filter string) bool {
	switch filter {
	case "", "all":
		return true
	case "L1":
		return layer == gcapi.UCLayerL1
	case "L2":
		return layer == gcapi.UCLayerL2
	case "L3":
		return layer == gcapi.UCLayerL3
	case "L1+L2":
		return layer == gcapi.UCLayerL1 || layer == gcapi.UCLayerL2
	}
	return true
}

// worstLayerOutcome reduces multiple test-case outcomes within the
// same layer to a single value (worst-of: fail > harness > not-run > pass).
func worstLayerOutcome(existing, incoming gcapi.TestRunStatus) gcapi.TestRunStatus {
	priority := func(s gcapi.TestRunStatus) int {
		switch s {
		case gcapi.TestRunFail:
			return 3
		case gcapi.TestRunHarnessError:
			return 2
		case gcapi.TestRunNotRun, "":
			return 1
		case gcapi.TestRunPass:
			return 0
		}
		return 0
	}
	if priority(incoming) > priority(existing) {
		return incoming
	}
	return existing
}

// describeFailure picks a short reason string from layer outcomes.
func describeFailure(layers map[gcapi.UCLayer]gcapi.TestRunStatus) string {
	for _, layer := range []gcapi.UCLayer{gcapi.UCLayerL1, gcapi.UCLayerL2, gcapi.UCLayerL3} {
		if s, ok := layers[layer]; ok {
			switch s {
			case gcapi.TestRunFail:
				return string(layer) + " test failed"
			case gcapi.TestRunHarnessError:
				return string(layer) + " harness error"
			case gcapi.TestRunNotRun:
				return string(layer) + " not run"
			}
		}
	}
	return "no passing tests"
}
