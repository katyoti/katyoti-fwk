// SPDX-License-Identifier: Apache-2.0

package api

import "time"

// TestRunVerdict is one UC's verdict within a TestRun's verdicts map.
type TestRunVerdict struct {
	Layers  map[UCLayer]TestRunStatus `json:"layers,omitempty"  jsonschema:"Per-layer outcome (only the layers that ran)"`
	Verdict UCStatus                  `json:"verdict"            jsonschema:"Projected user-facing verdict (pass | fail | fail-deferred | no-test)"`
	Reason  string                    `json:"reason,omitempty"   jsonschema:"Optional one-line explanation (failure cause, deferral OD-id, etc.)"`

	// ACs is the per-criterion outcome, keyed by AC id (UC-001-AC1).
	//
	// Layers is NOT enough to hold it: several acceptance criteria routinely
	// share one layer, so a per-layer map silently collapses them and the
	// criterion is the method's atomic unit. Added when UC files moved to
	// objects — the file side carried a status PER AC, and without this the
	// conversion would lose it.
	//
	// It lives on the RUN, never on the criterion object. A verdict belongs to
	// the run that produced it; a criterion carrying its own status is the
	// authored verdict this refactor is removing.
	ACs map[string]UCStatus `json:"acs,omitempty"      jsonschema:"Per-acceptance-criterion outcome, keyed by AC id"`
}

// TestRunTotals aggregates verdict counts for a TestRun.
type TestRunTotals struct {
	Pass         int `json:"pass"`
	Fail         int `json:"fail"`
	FailDeferred int `json:"fail_deferred"`
	NoTest       int `json:"no_test"`
	Skipped      int `json:"skipped,omitempty"`
	UCTotal      int `json:"uc_total"`
}

// TestRun is the on-the-wire shape of a `test-run` item — immutable
// per `verify` invocation. Mirrors app/.../item_types/test-run.yaml.
type TestRun struct {
	ID     string `json:"id"                          jsonschema:"Item UUID"`
	NodeID string `json:"node_id"                     jsonschema:"Owning module-node UUID"`

	// Typed body
	Module        string                    `json:"module"                       jsonschema:"Module slug"`
	RanAt         time.Time                 `json:"ran_at"                       jsonschema:"When the verify verb fired"`
	LayerFilter   string                    `json:"layer_filter"                 jsonschema:"L1 | L2 | L3 | L1+L2 | all"`
	UCFilter      string                    `json:"uc_filter,omitempty"          jsonschema:"Optional UC-id pattern from --uc flag"`
	Verdicts      map[string]TestRunVerdict `json:"verdicts,omitempty"           jsonschema:"UC-id → verdict"`
	Totals        TestRunTotals             `json:"totals"`
	TierNDoDMet   bool                      `json:"tier_n_dod_met"               jsonschema:"Did this run satisfy the module's tier_target DoD?"`
	TierNBlockers []string                  `json:"tier_n_blockers,omitempty"    jsonschema:"Failure summaries blocking tier promotion"`
	ArchivedTo    string                    `json:"archived_to,omitempty"        jsonschema:"Optional path the projection wrote to"`
}
