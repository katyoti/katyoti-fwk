// SPDX-License-Identifier: Apache-2.0

package api

// ModuleKind is the polymorphism axis for the `module` node type.
// Rules condition on kind via `when_kind: [...]` rather than the
// substrate using subtype inheritance. See modules/code-gocode/concepts.md §2.
type ModuleKind string

const (
	ModuleKindFramework ModuleKind = "framework"
	ModuleKindLibrary   ModuleKind = "library"
	ModuleKindSidecar   ModuleKind = "sidecar"
)

// TierState is one slot in the tier-promotion FSM driving the
// `module promote` gate. See modules/code-gocode/concepts.md §6.
type TierState string

const (
	TierStateBootstrapping TierState = "bootstrapping"
	TierStateSpecifying    TierState = "specifying"
	TierStatePlanning      TierState = "planning"
	TierStateExecuting     TierState = "executing"
	TierStateVerifying     TierState = "verifying"
	TierStateAuditing      TierState = "auditing"
	TierStateGated         TierState = "gated"
	TierStatePromoted      TierState = "promoted"
)

// ModuleHealth aggregates per-module health derived from UC verdicts
// + audit findings + lint state. Concepts.md §10.
type ModuleHealth string

const (
	ModuleHealthHealthy  ModuleHealth = "healthy"
	ModuleHealthDegraded ModuleHealth = "degraded"
	ModuleHealthBroken   ModuleHealth = "broken"
)

// Module is the on-the-wire shape of a `module` node. Mirrors the
// type_specific_schema declared in
// internal/extensions/gocode/local/embed/cwm/node_types/module.yaml.
type Module struct {
	// Standard node fields
	ID          string `json:"id"                              jsonschema:"UUID v7, immutable"`
	Name        string `json:"name"                            jsonschema:"Module slug (lowercase, hyphens)"`
	Slug        string `json:"slug,omitempty"                  jsonschema:"URL/CLI-safe slug; defaults to Name"`
	Description string `json:"description,omitempty"           jsonschema:"Free-form prose, ≤ 800 chars"`
	ParentID    string `json:"parent_id,omitempty"             jsonschema:"Parent node UUID (project or parent module)"`

	// Module-specific (type_specific)
	Kind        ModuleKind   `json:"kind"                       jsonschema:"framework | library | sidecar"`
	Facets      []Facet      `json:"facets,omitempty"           jsonschema:"Typed paths to everything the module points at (code / docs / ucs / tests / app / charter / manifest)"`
	TierCurrent int          `json:"tier_current"               jsonschema:"Current achieved tier (0..N)"`
	TierTarget  int          `json:"tier_target"                jsonschema:"Goal tier; drives plan-tier"`
	TierState   TierState    `json:"tier_state"                 jsonschema:"Tier-FSM state — see TierState enum"`
	Health      ModuleHealth `json:"health,omitempty"           jsonschema:"Derived health (computed-on-read)"`
	LinkedGoals []string     `json:"linked_goals,omitempty"     jsonschema:"References to workspace-level goal items"`
	LastAuditID string       `json:"last_audit_id,omitempty"    jsonschema:"Node id of the most recent audit review child"`
}

// CodeCapability is the capability a software module's code lives in: its
// kind, packages, tiers and facets (ktw-code's cap-code).
const CodeCapability = "code"

// CodePackagesField is the `code` capability field (declared by the cap-code
// data-type) that lists the source packages a module is realized by. It is the one source of a module's code facets: the manifest's
// source_paths, the packages `module verify` runs in, and gocode's scans all
// read it. A `code` entry under the undeclared `facets` group is ignored.
const CodePackagesField = "code_packages"

// FacetPaths returns every path declared for the kind, in declaration
// order.
func (m *Module) FacetPaths(kind FacetKind) []string {
	return FacetPathsOf(m.Facets, kind)
}

// FirstFacet returns the first path declared for the kind, or "".
func (m *Module) FirstFacet(kind FacetKind) string {
	return FirstFacetOf(m.Facets, kind)
}

// ModuleSummary is the briefing-shape view of a Module — drops
// timestamps, used by listings + tree/projection output.
type ModuleSummary struct {
	ID          string       `json:"id"`
	Name        string       `json:"name"`
	Kind        ModuleKind   `json:"kind"`
	TierCurrent int          `json:"tier_current"`
	TierTarget  int          `json:"tier_target"`
	TierState   TierState    `json:"tier_state"`
	Health      ModuleHealth `json:"health,omitempty"`
}
