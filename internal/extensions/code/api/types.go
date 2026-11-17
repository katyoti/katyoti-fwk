// SPDX-License-Identifier: Apache-2.0

package api

// Shared DTO vocabulary of the code methodology contract (Track B B1).
// Everything here is flat and JSON-schema-able — the contract is
// transport-agnostic by construction (in-process DI when a language
// pack is bundled, KTSP slot when it runs as its own sidecar).

// ── Facets — where a module's bytes live ─────────────────────────────
// Canonical home of the facet vocabulary (moved from gocode/api the
// day after it shipped there: the CONTRACT owns it; packs import it).

// Facet is one typed path entry on a module — the module is a logical
// record; facets say where its bytes live.
type Facet struct {
	Kind FacetKind `json:"kind"           jsonschema:"code | docs | ucs | tests | app | charter | manifest"`
	Path string    `json:"path"           jsonschema:"Repo-relative path"`
	Role string    `json:"role,omitempty" jsonschema:"Optional free-form qualifier (e.g. 'L1+L2+L3 corpora')"`
}

// FacetKind enumerates the facet kinds.
type FacetKind string

const (
	FacetCode     FacetKind = "code"     // Go package path the module owns (repeatable)
	FacetDocs     FacetKind = "docs"     // module doc folder
	FacetUCs      FacetKind = "ucs"      // modules/<m>/use-cases/ (per-UC files)
	FacetTests    FacetKind = "tests"    // tests corpora root
	FacetApp      FacetKind = "app"      // kind=sidecar: app/<name>/
	FacetCharter  FacetKind = "charter"  // published charter.md (projection target)
	FacetManifest FacetKind = "manifest" // manifest.json (projection target)
)

// ValidFacetKinds is the closed kind set (lint enforces membership).
var ValidFacetKinds = map[FacetKind]bool{
	FacetCode: true, FacetDocs: true, FacetUCs: true, FacetTests: true,
	FacetApp: true, FacetCharter: true, FacetManifest: true,
}

// FacetPathsOf returns every path declared for the kind, in
// declaration order.
func FacetPathsOf(facets []Facet, kind FacetKind) []string {
	var out []string
	for _, f := range facets {
		if f.Kind == kind {
			out = append(out, f.Path)
		}
	}
	return out
}

// FirstFacetOf returns the first path declared for the kind, or "".
func FirstFacetOf(facets []Facet, kind FacetKind) string {
	for _, f := range facets {
		if f.Kind == kind {
			return f.Path
		}
	}
	return ""
}

// ── The 5-state quality model ────────────────────────────────────────

// ACStatus is the 5-state model every acceptance criterion (and the
// derived UC status_cache) carries. Statuses are written ONLY by
// verify — code-driven, never LLM-judged, never hand-set.
type ACStatus string

const (
	ACPass         ACStatus = "pass"
	ACFail         ACStatus = "fail"
	ACNoTest       ACStatus = "no-test"
	ACFailDeferred ACStatus = "fail-deferred" // OD-anchored; excluded from coverage denominator
	ACSkipped      ACStatus = "skipped"       // excluded from coverage denominator
)

// ── Probes + test execution ──────────────────────────────────────────

// TestRef names one binary probe at one layer — the unit of the
// one-probe-per-AC rule.
type TestRef struct {
	Layer   string `json:"layer"    jsonschema:"L1 | L2 | L3 — L3 runs only on explicit user request, never via automated verify"`
	Ref     string `json:"ref"      jsonschema:"L1: Go test name; L2: .tasktest path; L3: scenario name/path"`
	Package string `json:"package,omitempty" jsonschema:"Optional package scope for L1 refs"`
}

// TestResult is the outcome of running one probe.
type TestResult struct {
	Status     ACStatus `json:"status"`
	Reason     string   `json:"reason,omitempty"`
	DurationMS int64    `json:"duration_ms,omitempty"`
	Output     string   `json:"output,omitempty" jsonschema:"Truncated failure output (≤ 1 KB)"`
}

// ── Gates (FSM transition checks contributed by language packs) ─────

// GateSpec declares one gate a language pack contributes (e.g.
// go-build-clean). Data only — the check itself is the GateCheck verb.
type GateSpec struct {
	Name        string `json:"name"            jsonschema:"Kebab gate id (e.g. go-build-clean)"`
	Label       string `json:"label,omitempty"`
	Description string `json:"description,omitempty"`
	Transition  string `json:"transition,omitempty" jsonschema:"FSM transition this gate guards (e.g. en-validation→livrable)"`
}

// GateCheckInput asks a pack to evaluate one gate against a module's
// code facets.
type GateCheckInput struct {
	Gate   string  `json:"gate"   jsonschema:"GateSpec.Name to evaluate"`
	Facets []Facet `json:"facets" jsonschema:"The module's facets (the pack reads the code/tests paths)"`
}

// GateCheckOutput is the gate verdict.
type GateCheckOutput struct {
	Allow  bool   `json:"allow"`
	Reason string `json:"reason,omitempty" jsonschema:"Failed checks summarized (capped output)"`
}

// ── Language-pack declarations (data, not behavior) ─────────────────

// CheckKindSpec declares one language-specific lint detector.
type CheckKindSpec struct {
	Name        string `json:"name"`
	Severity    string `json:"severity" jsonschema:"error | warn | info"`
	Description string `json:"description,omitempty"`
}

// LayoutRuleSpec declares one language-specific layout rule
// (e.g. the Go 3-package public/internal/local convention).
type LayoutRuleSpec struct {
	Name        string `json:"name"`
	AppliesKind string `json:"applies_kind,omitempty" jsonschema:"Module kind filter: framework | library | sidecar | * "`
	Description string `json:"description,omitempty"`
}

// PersonaAugmentation is language-specific guidance appended to one
// of the 6 baseline personas.
type PersonaAugmentation struct {
	Persona string `json:"persona" jsonschema:"specifier | architect | coder | reviewer | tester | documenter"`
	Prompt  string `json:"prompt"  jsonschema:"Markdown appended to the persona's system guidance"`
}

// ── Discovery / retro-engineering proposals ─────────────────────────

// Inventory is a pack's scan summary of a code root.
type Inventory struct {
	Packages      []string `json:"packages,omitempty"`
	PublicSymbols []string `json:"public_symbols,omitempty"`
	TestFunctions []string `json:"test_functions,omitempty"`
	SuggestedKind string   `json:"suggested_kind,omitempty" jsonschema:"framework | library | sidecar | unclear"`
}

// FacetProposal is a discovered typed path (never the retired
// flat path fields).
type FacetProposal struct {
	Facet      Facet  `json:"facet"`
	Confidence string `json:"confidence,omitempty" jsonschema:"high | medium | low"`
}

// UCProposal is a judgment-based use-case seed for specifier review —
// never auto-committed.
type UCProposal struct {
	Title       string   `json:"title"`
	Severity    string   `json:"severity"  jsonschema:"core | security | qol | cosmetic"`
	Description string   `json:"description,omitempty"`
	HintedBy    []string `json:"hinted_by,omitempty" jsonschema:"Symbols or test names that suggested this UC"`
}

// TestCaseProposal maps an existing test to a candidate probe.
type TestCaseProposal struct {
	Ref TestRef `json:"ref"`
}

// FeatureProposal is a reverse-engineered feature candidate
// (a feature seed). Confidence is always declared;
// never auto-committed.
type FeatureProposal struct {
	Title      string   `json:"title"`
	Rationale  string   `json:"rationale,omitempty"`
	Modules    []string `json:"modules,omitempty" jsonschema:"Module names/IDs the feature spans"`
	Confidence string   `json:"confidence" jsonschema:"high | medium | low"`
}
