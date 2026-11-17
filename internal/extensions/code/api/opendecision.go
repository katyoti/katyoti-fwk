// SPDX-License-Identifier: Apache-2.0

package api

import "time"

// ODState is the lifecycle of a ktw-gocode decision item.
// Distinct from the CWM-generic decision item type — ktw-gocode
// uses the substrate's decision but adds a flat resolution-state
// view that consumers (verify gate, charter projection) read.
type ODState string

const (
	ODStateOpen     ODState = "open"
	ODStateResolved ODState = "resolved"
)

// OpenDecision is the on-the-wire shape of a ktw-gocode-flavored
// decision. The substrate-generic decision item carries
// title / question / options / becomes_decision; ktw-gocode adds
// the OD-NNN stable id and the resolved-with summary so the
// `verify` gate can flip fail-deferred UCs to pass when their
// linked OD resolves.
type OpenDecision struct {
	// Item-substrate fields
	ID     string `json:"id"                          jsonschema:"Item UUID"`
	NodeID string `json:"node_id"                     jsonschema:"Owning module or workspace node UUID"`
	Title  string `json:"title"                       jsonschema:"Short headline"`
	Body   string `json:"body,omitempty"              jsonschema:"Markdown body — context + options"`

	// Typed body
	ODID         string     `json:"od_id"                 jsonschema:"Stable identifier (OD-001, OD-042)"`
	State        ODState    `json:"state"                 jsonschema:"open | resolved"`
	Resolution   string     `json:"resolution,omitempty"  jsonschema:"Short one-line resolution (set when state=resolved)"`
	ResolvedAt   *time.Time `json:"resolved_at,omitempty"`
	BlockingTier *int       `json:"blocking_tier,omitempty" jsonschema:"If set, OD must resolve before this tier promote"`
}
