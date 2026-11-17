// SPDX-License-Identifier: Apache-2.0

package api

// Methodology verb I/O: thin by design — verbs reference graph entities
// by ID and return compact reports; the full domain shapes live in the
// CWM type YAMLs. The module, use-case, verify, lint, promote and
// tasktest verbs are GocodeService's (the Go language pack); CodeService
// keeps discovery, decisions and the projections.

// ── module.* ─────────────────────────────────────────────────────────

type ModuleDiscoverInput struct {
	Path           string `json:"path"                       jsonschema:"Code root to scan"`
	Language       string `json:"language,omitempty"         jsonschema:"Language-pack instance to dispatch to; inferred from extensions when empty (Q-B4)"`
	TargetModuleID string `json:"target_module_id,omitempty" jsonschema:"Existing module to extend; empty proposes a new one"`
	DryRun         bool   `json:"dry_run,omitempty"`
}

type ModuleDiscoverOutput struct {
	ModuleID  string         `json:"module_id,omitempty"`
	Discovery DiscoverOutput `json:"discovery"`
}

// LintViolation is one lint finding (language-neutral shape; packs
// contribute detectors via CheckKindSpec).
type LintViolation struct {
	Detector      string `json:"detector"`
	Severity      string `json:"severity" jsonschema:"error | warn | info"`
	NodeID        string `json:"node_id,omitempty"`
	ItemID        string `json:"item_id,omitempty"`
	Reason        string `json:"reason"`
	FixSuggestion string `json:"fix_suggestion,omitempty"`
}

// ── od.* ─────────────────────────────────────────────────────────────

type ODOpenInput struct {
	ModuleID string `json:"module_id"`
	Title    string `json:"title"`
	Proposal string `json:"proposal,omitempty"`
}

type ODOpenOutput struct {
	ODID string `json:"od_id" jsonschema:"OD-NNN"`
}

type ODResolveInput struct {
	ModuleID   string `json:"module_id"`
	ODID       string `json:"od_id"`
	Resolution string `json:"resolution"`
}

type ODResolveOutput struct {
	ODID string `json:"od_id"`
}

// ── projections ──────────────────────────────────────────────────────

type CharterPublishInput struct {
	ModuleID string `json:"module_id"`
	DryRun   bool   `json:"dry_run,omitempty"`
}

type CharterPublishOutput struct {
	Path     string `json:"path,omitempty"`
	Markdown string `json:"markdown,omitempty" jsonschema:"Rendered output when dry_run"`
}

type ManifestWriteInput struct {
	ModuleID string `json:"module_id"`
	DryRun   bool   `json:"dry_run,omitempty"`
}

type ManifestWriteOutput struct {
	Path string `json:"path"`
	JSON string `json:"json,omitempty" jsonschema:"Rendered output when dry_run"`
}
