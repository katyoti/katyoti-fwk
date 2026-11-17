// SPDX-License-Identifier: Apache-2.0

package api

import "time"

// AuditPerspective is one of the automated checks a Go audit runs (the
// go-checks review type, ktw-gocode's ModuleReviewCheck): the toolchain and
// the lint, no model. The judgment perspectives (architecture, security,
// threat model …) are review types of their own — ktw-code's code reviews,
// run by an agent — never values here.
type AuditPerspective string

const (
	AuditPerspectiveCorrectness   AuditPerspective = "correctness"
	AuditPerspectiveQuality       AuditPerspective = "quality"
	AuditPerspectivePackageLayout AuditPerspective = "package-layout"
	AuditPerspectiveDeadCode      AuditPerspective = "dead-code"
	AuditPerspectiveLint          AuditPerspective = "lint"
)

// An audit is stored as a review of the module (the reviews pack) whose
// reviewer is the audit, holding a finding per thing found.
const (
	AuditReviewType  = "review"
	AuditFindingType = "finding"
	AuditReviewer    = "ktw-gocode audit"
)

// FindingSeverity classifies an audit finding. critical / high
// findings block tier promotion when state=open.
type FindingSeverity string

const (
	FindingSeverityCritical FindingSeverity = "critical"
	FindingSeverityHigh     FindingSeverity = "high"
	FindingSeverityMedium   FindingSeverity = "medium"
	FindingSeverityLow      FindingSeverity = "low"
	FindingSeverityInfo     FindingSeverity = "info"
)

// FindingState is the lifecycle of an finding.
type FindingState string

const (
	FindingStateOpen     FindingState = "open"
	FindingStateClosed   FindingState = "closed"
	FindingStateDeferred FindingState = "deferred"
)

// AuditBundle is the typed reading of an audit: the review of the module the
// audit held (AuditReviewType, its reviewer AuditReviewer).
type AuditBundle struct {
	ID           string             `json:"id"                          jsonschema:"Node UUID — immutable post-creation"`
	ParentID     string             `json:"parent_id"                   jsonschema:"Parent module-node UUID"`
	Name         string             `json:"name"                        jsonschema:"Canonical id (audit-YYYY-MM-DD_HH-MM)"`
	RanAt        time.Time          `json:"ran_at"`
	Perspectives []AuditPerspective `json:"perspectives,omitempty"     jsonschema:"Names of the perspective passes that produced this bundle"`
	SummaryMD    string             `json:"summary_md,omitempty"        jsonschema:"Rolled-up narrative (markdown)"`
	ArchivedTo   string             `json:"archived_to,omitempty"       jsonschema:"Optional repo-relative path the projection wrote to"`
	Immutable    bool               `json:"immutable"                   jsonschema:"Always true; substrate-enforced post-creation"`
}

// AuditFinding is the typed reading of a finding of an audit's review: its
// state is read from the finding's lifecycle.
type AuditFinding struct {
	ID     string `json:"id"                            jsonschema:"Item UUID"`
	NodeID string `json:"node_id"                       jsonschema:"Owning audit review UUID"`
	Title  string `json:"title"`
	Body   string `json:"body,omitempty"                jsonschema:"Markdown — finding details + suggested fix"`

	// Typed body
	FindingID   string           `json:"finding_id"               jsonschema:"Perspective-prefixed (sec-1, threat-T1, perf-2)"`
	Perspective AuditPerspective `json:"perspective"              jsonschema:"Which audit perspective surfaced this"`
	Severity    FindingSeverity  `json:"severity"                 jsonschema:"critical | high | medium | low | info"`
	State       FindingState     `json:"state"                    jsonschema:"open | closed | deferred"`
	ClosedAt    *time.Time       `json:"closed_at,omitempty"`
	ClosedByUC  string           `json:"closed_by_uc,omitempty"   jsonschema:"UC-id that resolved this finding"`
	DeferredBy  string           `json:"deferred_by,omitempty"    jsonschema:"OD-id when state=deferred"`
	Tags        []string         `json:"tags,omitempty"`
}
