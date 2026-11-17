// SPDX-License-Identifier: Apache-2.0

package api

// RealizationStatus is the derived realization value for a wanted Use Case
// (and the worst-of rollup at the feature level) — computed-on-read from
// the UC's status_cache (method-core §1/§3).
type RealizationStatus string

const (
	RealizationNotStarted RealizationStatus = "not-started"
	RealizationInProgress RealizationStatus = "in-progress"
	RealizationVerifying  RealizationStatus = "verifying"
	RealizationRegressed  RealizationStatus = "regressed"
	RealizationDone       RealizationStatus = "done"
)

// WantedUCEdge is one feature→Use Case link from a feature's `wants` edge map,
// with the UC's derived realization status surfaced.
type WantedUCEdge struct {
	UCID              string            `json:"uc_id"               jsonschema:"Wanted Use Case item UUID"`
	UCTitle           string            `json:"uc_title,omitempty"  jsonschema:"Display title for the wanted UC"`
	RealizationStatus RealizationStatus `json:"realization_status"  jsonschema:"Derived from the UC's status_cache"`
}

// FeatureSummary is a feature's display-friendly view with its
// aggregate realization-status (worst-of its wanted UCs).
type FeatureSummary struct {
	ID              string            `json:"id"`
	Name            string            `json:"name"`
	Description     string            `json:"description,omitempty"`
	InitiativeID    string            `json:"initiative_id,omitempty"`
	Priority        string            `json:"priority,omitempty"`
	TeamTag         string            `json:"team_tag,omitempty"`
	Wants           []WantedUCEdge    `json:"wants,omitempty"`
	AggregateStatus RealizationStatus `json:"aggregate_status"     jsonschema:"Worst-of wanted UCs; method-core §1/§3"`
}
