// SPDX-License-Identifier: Apache-2.0

// Package cwmprojection produces the canonical artifact set
// (charter.md, manifest.json, audit review summaries) from CWM
// items. Items are the source of truth; these projections are
// regenerated on every `module charter publish` / `module manifest
// write` invocation. Hand-edits to projected files surface as drift
// via the gocode-lint-detectors task.
package cwmprojection

import "github.com/katyoti/katyoti-fwk/internal/extensions/code/internal/projection"

// Manifest is the projected shape of a module's manifest.json.
// Mirrors the reference at modules/tasks/manifest.json so a
// regenerated file is drop-in compatible with the hand-built
// originals.
//
// Field-ordering note: the JSON output is deterministic — see
// MarshalCanonicalJSON. Adding a field also adds it to the
// canonical key list.
type Manifest struct {
	// --- module identity --------------------------------------------------
	Module      string   `json:"module"`
	SourcePaths []string `json:"source_paths,omitempty"`
	ModuleKind  string   `json:"module_kind"`

	// --- tier -------------------------------------------------------------
	CharterVersion string `json:"charter_version,omitempty"`
	CurrentTier    int    `json:"current_tier"`
	TierTarget     int    `json:"tier_target"`
	TierState      string `json:"tier_state,omitempty"`

	// --- linked goals -----------------------------------------------------
	LinkedGoals []string `json:"linked_goals,omitempty"`

	// --- open decisions ---------------------------------------------------
	OpenDecisions     []ManifestDecision `json:"open_decisions"`
	ResolvedDecisions []ManifestDecision `json:"resolved_decisions"`

	// --- coverage (the use cases + the latest test-run verdicts) ----------
	Coverage *projection.Coverage `json:"coverage"`

	// --- audit ------------------------------------------------------------
	LastAudit         string         `json:"last_audit,omitempty"`
	AuditFindingsOpen map[string]int `json:"audit_findings_open,omitempty"`

	// --- lint / promotion -------------------------------------------------
	LintStatus       string   `json:"lint_status,omitempty"`
	ForcedPromotions []string `json:"forced_promotions,omitempty"`
}

// ManifestDecision is the per-OD entry in OpenDecisions /
// ResolvedDecisions arrays.
type ManifestDecision struct {
	ID         string `json:"id"`
	Resolution string `json:"resolution,omitempty"`
}
