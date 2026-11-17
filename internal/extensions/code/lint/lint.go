// SPDX-License-Identifier: Apache-2.0

// Package lint implements the code methodology's LANGUAGE-NEUTRAL
// lint detectors — pure logic on typed inputs. The wiring layer
// (gocode/internal/core/methods_lint.go today) loads CWM items via
// the projection loader and feeds them here. Language packs compose
// their language-specific detectors on top via RunDetectors (see
// gocode/internal/lint — the Go pack adds SidecarMissingAppFolder).
//
// Neutral detector set:
//
//   - MissingDoDBody       — error, UC items with empty body
//   - DeferredWithoutOD    — error, fail-deferred UC missing OD link
//   - InvalidFacets        — error, facet kind outside the closed set / empty path
//   - FacetPathMissing     — warn,  declared facet path absent on disk
//   - OrphanUseCases       — warn,  UC items with status_cache=no-test
//   - StatusCacheDrift     — warn,  UC stamped without a paired test-run
//   - CharterProjectionDrift — warn, charter.md differs from regenerated
//   - ManifestDrift        — warn,  manifest.json differs from regenerated
//
// Deferred to follow-ups (edge-dependent or substrate-blocked):
//
//   - StaleEdges, UnaddressedTasks, DeferredWithoutOD edge
//   - UnenforceableRules, RuleKindConflict
package lint

import (
	"fmt"

	gcapi "github.com/katyoti/katyoti-fwk/internal/extensions/code/api"
)

// Inputs is the typed bundle the wiring layer feeds in. Mirrors
// `projection.ManifestInputs` so the same loader serves verify +
// status + lint.
type Inputs struct {
	Module        gcapi.Module
	UseCases      []gcapi.UseCase
	OpenDecisions []gcapi.OpenDecision
	TestCases     []gcapi.TestCase
	TestRuns      []gcapi.TestRun // optional — populated by methods_lint when StatusCacheDrift is enabled
	// CharterDrift / ManifestDrift are pre-computed by the wiring
	// layer (it has the file-system access). The detector functions
	// only consume the booleans.
	CharterDrift  bool
	ManifestDrift bool
	// MissingFacetPaths is pre-computed by the wiring layer (os.Stat
	// per facet path) — same FS-access split as the drift booleans.
	MissingFacetPaths []string
}

// Severity constants matching the strings the LintViolation API
// surfaces. Kept as `string` aliases here rather than a separate
// typed enum in gocode/api/ because the rest of the codebase already
// treats `LintViolation.Severity` as a free-form string field.
const (
	SeverityError = "error"
	SeverityWarn  = "warn"
	SeverityInfo  = "info"
)

// Detector represents a single lint check. The lint runner iterates
// the registry and concatenates violations.
type Detector struct {
	Name     string
	Severity string
	Check    func(Inputs) []gcapi.LintViolation
}

// BaseDetectors returns the language-neutral detector registry in
// stable order (errors first so high-signal violations surface at the
// top). Language packs append their own detectors and dispatch the
// merged list through RunDetectors.
func BaseDetectors() []Detector {
	return []Detector{
		{Name: "MissingDoDBody", Severity: SeverityError, Check: missingDoDBody},
		{Name: "DeferredWithoutOD", Severity: SeverityError, Check: deferredWithoutOD},
		{Name: "InvalidFacets", Severity: SeverityError, Check: invalidFacets},
		{Name: "FacetPathMissing", Severity: SeverityWarn, Check: facetPathMissing},
		{Name: "OrphanUseCases", Severity: SeverityWarn, Check: orphanUseCases},
		{Name: "StatusCacheDrift", Severity: SeverityWarn, Check: statusCacheDrift},
		{Name: "CharterProjectionDrift", Severity: SeverityWarn, Check: charterProjectionDrift},
		{Name: "ManifestDrift", Severity: SeverityWarn, Check: manifestDrift},
	}
}

// Run executes the requested NEUTRAL detectors against the inputs.
// Language-pack callers go through RunDetectors with their composed
// registry instead. Empty `requested` means "run all".
func Run(in Inputs, requested []string) ([]gcapi.LintViolation, int, int, int) {
	return RunDetectors(BaseDetectors(), in, requested)
}

// RunDetectors executes the requested detectors from the given
// registry against the inputs and returns the merged violation list +
// severity counts. Empty `requested` means "run all".
func RunDetectors(detectors []Detector, in Inputs, requested []string) (out []gcapi.LintViolation, errorCount, warnCount, infoCount int) {
	want := stringSet(requested)
	for _, d := range detectors {
		if len(want) > 0 && !want[d.Name] {
			continue
		}
		vs := d.Check(in)
		for i := range vs {
			vs[i].Detector = d.Name
			if vs[i].Severity == "" {
				vs[i].Severity = d.Severity
			}
		}
		out = append(out, vs...)
	}
	for _, v := range out {
		switch v.Severity {
		case SeverityError:
			errorCount++
		case SeverityWarn:
			warnCount++
		default:
			infoCount++
		}
	}
	return
}

// ── Detectors ────────────────────────────────────────────────────────────

// missingDoDBody flags UCs whose body field is empty. The body
// carries the DoD-per-UC markdown — a UC without it can't be
// verified, so this is an `error` severity that blocks promote.
func missingDoDBody(in Inputs) []gcapi.LintViolation {
	var out []gcapi.LintViolation
	for _, uc := range in.UseCases {
		if uc.Body == "" {
			out = append(out, gcapi.LintViolation{
				NodeID:        in.Module.ID,
				ItemID:        uc.ID,
				Reason:        fmt.Sprintf("UC %q has no DoD body — cannot be verified", titleOrID(uc)),
				FixSuggestion: "Author DoD per modules/code/concepts.md §3 (layer-tagged test refs + negative DoD)",
			})
		}
	}
	return out
}

// deferredWithoutOD flags UCs marked fail-deferred but without a
// FailDeferredBy link to an OD. The deferral has no rationale trail,
// so it's not auditable. `error` severity.
func deferredWithoutOD(in Inputs) []gcapi.LintViolation {
	var out []gcapi.LintViolation
	for _, uc := range in.UseCases {
		if uc.StatusCache != gcapi.UCStatusFailDeferred {
			continue
		}
		if uc.FailDeferredBy == "" {
			out = append(out, gcapi.LintViolation{
				NodeID:        in.Module.ID,
				ItemID:        uc.ID,
				Reason:        fmt.Sprintf("UC %q is fail-deferred but cites no decision", titleOrID(uc)),
				FixSuggestion: "Open an OD via `gocode od-open` and set fail_deferred_by to the OD id",
			})
		}
	}
	return out
}

// invalidFacets flags facet entries with an unknown kind or an empty
// path — the closed-kind contract of the facets list. `error` severity.
func invalidFacets(in Inputs) []gcapi.LintViolation {
	var out []gcapi.LintViolation
	for _, f := range in.Module.Facets {
		switch {
		case !gcapi.ValidFacetKinds[f.Kind]:
			out = append(out, gcapi.LintViolation{
				NodeID:        in.Module.ID,
				Reason:        fmt.Sprintf("facet kind %q is not in the closed set (code/docs/ucs/tests/app/charter/manifest)", f.Kind),
				FixSuggestion: "Fix the facet's `kind` to one of the supported values",
			})
		case f.Path == "":
			out = append(out, gcapi.LintViolation{
				NodeID:        in.Module.ID,
				Reason:        fmt.Sprintf("facet of kind %q has an empty path", f.Kind),
				FixSuggestion: "Set the facet's `path` (repo-relative) or remove the entry",
			})
		}
	}
	return out
}

// facetPathMissing reports facets whose path does not exist on disk.
// The wiring layer pre-computes MissingFacetPaths (it has the FS
// access). `warn` severity: a freshly declared projection target may
// legitimately not exist until its first publish.
func facetPathMissing(in Inputs) []gcapi.LintViolation {
	var out []gcapi.LintViolation
	for _, p := range in.MissingFacetPaths {
		out = append(out, gcapi.LintViolation{
			NodeID:        in.Module.ID,
			Reason:        fmt.Sprintf("facet path %q does not exist on disk", p),
			FixSuggestion: "Fix the path, create the target, or remove the facet",
		})
	}
	return out
}

// orphanUseCases flags UCs with status_cache=no-test. These are
// authored UCs no test-case claims to verify. `warn` because v0
// in-flight work legitimately starts here.
func orphanUseCases(in Inputs) []gcapi.LintViolation {
	var out []gcapi.LintViolation
	for _, uc := range in.UseCases {
		if uc.StatusCache == gcapi.UCStatusNoTest {
			out = append(out, gcapi.LintViolation{
				NodeID:        in.Module.ID,
				ItemID:        uc.ID,
				Reason:        fmt.Sprintf("UC %q has no test-case claiming to verify it (status_cache=no-test)", titleOrID(uc)),
				FixSuggestion: "Add a test-case with TCID matching the UC id (e.g. L1-uc042-…) and run module verify",
			})
		}
	}
	return out
}

// statusCacheDrift flags UCs whose status_cache value has no
// corresponding test-run record. Per modules/code-gocode/concepts.md §4 status_cache
// is honor-system protected — only the `verify` verb should write
// it. A drift signals a hand-edit and the verdict should be
// regenerated.
//
// v1 heuristic: when status_cache is set (any non-empty value other
// than no-test) AND last_verified_at is unset OR no test-run exists.
// Real per-UC test-run linkage will tighten when test-runs land.
func statusCacheDrift(in Inputs) []gcapi.LintViolation {
	hasAnyTestRun := len(in.TestRuns) > 0
	var out []gcapi.LintViolation
	for _, uc := range in.UseCases {
		if uc.StatusCache == "" || uc.StatusCache == gcapi.UCStatusNoTest {
			continue
		}
		// Has a non-trivial status but no last_verified_at AND no
		// test-runs on the module → almost certainly hand-edited.
		if uc.LastVerifiedAt == nil && !hasAnyTestRun {
			out = append(out, gcapi.LintViolation{
				NodeID:        in.Module.ID,
				ItemID:        uc.ID,
				Reason:        fmt.Sprintf("UC %q has status_cache=%q but no test-run record (likely hand-edited)", titleOrID(uc), uc.StatusCache),
				FixSuggestion: "Re-run `gocode module-verify` to regenerate status_cache from real test-case outcomes",
			})
		}
	}
	return out
}

// charterProjectionDrift fires when the wiring layer detected a
// drift between the persisted charter.md and the regenerated one.
// Pure passthrough so the boolean wins are kept in one place.
func charterProjectionDrift(in Inputs) []gcapi.LintViolation {
	if !in.CharterDrift {
		return nil
	}
	return []gcapi.LintViolation{{
		NodeID:        in.Module.ID,
		Reason:        "charter.md on disk differs from the regenerated projection — file was hand-edited",
		FixSuggestion: "Run `gocode charter-publish` to overwrite with the canonical projection (items are source-of-truth)",
	}}
}

// manifestDrift — same pattern as charterProjectionDrift.
func manifestDrift(in Inputs) []gcapi.LintViolation {
	if !in.ManifestDrift {
		return nil
	}
	return []gcapi.LintViolation{{
		NodeID:        in.Module.ID,
		Reason:        "manifest.json on disk differs from the regenerated projection — file was hand-edited",
		FixSuggestion: "Run `gocode manifest-write` to overwrite with the canonical projection",
	}}
}

// ── helpers ──────────────────────────────────────────────────────────────

func titleOrID(uc gcapi.UseCase) string {
	if uc.UCID != "" {
		return uc.UCID
	}
	if uc.Title != "" {
		return uc.Title
	}
	return uc.ID
}

func stringSet(ss []string) map[string]bool {
	if len(ss) == 0 {
		return nil
	}
	m := make(map[string]bool, len(ss))
	for _, s := range ss {
		m[s] = true
	}
	return m
}
