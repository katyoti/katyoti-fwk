// SPDX-License-Identifier: Apache-2.0

// Package ucfile is the in-memory shape of a use case — its identity,
// severity, acceptance criteria with their probe refs, and the statuses a run
// gave them — and the authoring lint over it. The use cases themselves are
// `use-case` and `acceptance-criterion` objects; package ucobj rebuilds this
// shape from them (statuses from the latest test-run) for verify, the lint
// and the projections.
//
// Rules: modules/code/concepts.md §3 (ISA authoring rules) and §4 (5-state
// model, layer assignment).
package ucfile

import "regexp"

// UCFile is one use case, rebuilt from its objects.
type UCFile struct {
	// Path is the use case's object file (for findings).
	Path string

	// Meta is the decoded frontmatter.
	Meta UCMeta

	// Body is the markdown prose after the frontmatter (trimmed).
	Body string
}

// UCMeta is the YAML frontmatter of a UC file.
type UCMeta struct {
	ID          string   `yaml:"id"`   // UC-NNN
	Kind        string   `yaml:"kind"` // always "use-case"
	Title       string   `yaml:"title"`
	Description string   `yaml:"description"` // what the UC covers and HOW it is tested
	Module      string   `yaml:"module"`
	Severity    string   `yaml:"severity"` // core | security | qol | cosmetic
	Realizes    []string `yaml:"realizes,omitempty"`

	// StatusCache is derived (worst-of the ACs). Written only by verify.
	StatusCache    string `yaml:"status_cache,omitempty"`
	LastVerifiedAt string `yaml:"last_verified_at,omitempty"`
	FailDeferredBy string `yaml:"fail_deferred_by,omitempty"` // OD id when status is fail-deferred

	AcceptanceCriteria []AC `yaml:"acceptance_criteria"`
}

// AC is one inline acceptance criterion — the atomic testable claim.
// Exactly one binary probe: one test_ref at one layer.
type AC struct {
	ID      string `yaml:"id"` // UC-NNN-ACn (splits: ACn.m)
	Claim   string `yaml:"claim"`
	Kind    string `yaml:"kind,omitempty"` // "" (= normal) | normal | anti
	Layer   string `yaml:"layer"`          // L1 | L2 | L3
	TestRef string `yaml:"test_ref"`
	Status  string `yaml:"status"` // pass | fail | no-test | fail-deferred | skipped
	Note    string `yaml:"note,omitempty"`
}

// IsAnti reports whether the criterion is an ISA anti-criterion.
func (a AC) IsAnti() bool { return a.Kind == "anti" }

// EffectiveStatus is the criterion's status in the 5-state set: a status
// never recorded (a new criterion no run has reached yet) is no-test — an
// open gap, not an invalid value.
func (a AC) EffectiveStatus() string {
	if a.Status == "" {
		return StatusNoTest
	}
	return a.Status
}

// The 5-state model (modules/code/concepts.md §4.1).
const (
	StatusPass         = "pass"
	StatusFail         = "fail"
	StatusNoTest       = "no-test"
	StatusFailDeferred = "fail-deferred"
	StatusSkipped      = "skipped"
)

// ValidStatuses is the closed set an AC status (and the derived
// status_cache) must belong to.
var ValidStatuses = map[string]bool{
	StatusPass:         true,
	StatusFail:         true,
	StatusNoTest:       true,
	StatusFailDeferred: true,
	StatusSkipped:      true,
}

// ValidSeverities is the closed severity set.
var ValidSeverities = map[string]bool{
	"core":     true,
	"security": true,
	"qol":      true,
	"cosmetic": true,
}

// ValidLayers is the closed probe-layer set.
var ValidLayers = map[string]bool{
	"L1": true,
	"L2": true,
	"L3": true,
}

// KindUseCase is the only accepted frontmatter `kind`.
const KindUseCase = "use-case"

// UCIDRe matches a use-case identifier (UC-001, UC-114, ...).
var UCIDRe = regexp.MustCompile(`^UC-\d+$`)

// ACIDRe matches an acceptance-criterion identifier scoped to its UC:
// UC-NNN-ACn, with split children UC-NNN-ACn.m (ID-stability rule —
// splits extend, never renumber).
var ACIDRe = regexp.MustCompile(`^UC-\d+-AC\d+(\.\d+)*$`)

// L1TestNameRe matches a Go test function name (the L1 probe form).
var L1TestNameRe = regexp.MustCompile(`^Test[A-Z][A-Za-z0-9_]*$`)

// WorstOf derives the UC status_cache from its ACs: the worst status
// present, ordered fail > no-test > skipped > fail-deferred > pass.
// fail-deferred ranks below skipped because it is an explicitly recorded
// decision (anchored to an OD), not an open gap. Returns no-test for a UC
// with zero ACs.
func WorstOf(acs []AC) string {
	rank := map[string]int{
		StatusFail:         0,
		StatusNoTest:       1,
		StatusSkipped:      2,
		StatusFailDeferred: 3,
		StatusPass:         4,
	}
	worst := ""
	worstRank := len(rank)
	for _, a := range acs {
		st := a.EffectiveStatus()
		r, ok := rank[st]
		if !ok {
			// Unknown status is treated as worst — lint flags it anyway.
			return StatusFail
		}
		if r < worstRank {
			worstRank = r
			worst = st
		}
	}
	if worst == "" {
		return StatusNoTest
	}
	return worst
}
