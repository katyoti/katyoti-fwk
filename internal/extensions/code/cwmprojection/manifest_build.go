// SPDX-License-Identifier: Apache-2.0

package cwmprojection

import (
	"sort"

	gcapi "github.com/katyoti/katyoti-fwk/internal/extensions/code/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/code/internal/projection"
	"github.com/katyoti/katyoti-fwk/internal/extensions/code/internal/ucfile"
)

// ManifestInputs is the typed bundle a caller assembles from CWM
// before building the Manifest. Tested independently of CWM via
// BuildManifest (pure function on typed values).
type ManifestInputs struct {
	Module         gcapi.Module
	CharterVersion string // empty when no charter item exists yet
	UseCases       []gcapi.UseCase
	OpenDecisions  []gcapi.OpenDecision
	// UCFiles are the module's use cases with their acceptance criteria,
	// carrying the statuses of the latest test-run verdicts
	// (ucobj.LatestVerdicts). They are the one source of a use case's status
	// for the charter, the test plan and the manifest's coverage block.
	UCFiles []*ucfile.UCFile
	// LatestTestRun is nil when no verify has ever run on this module.
	LatestTestRun *gcapi.TestRun
	// LatestAudit is nil when no audit review exists yet.
	LatestAudit *gcapi.AuditBundle
	// OpenFindings is the un-closed audit findings rollup at the
	// LatestAudit (or across all bundles if the caller chooses).
	OpenFindings []gcapi.AuditFinding
	// LintErrorCount surfaces from the latest lint run; 0 if not yet run.
	LintErrorCount int
	// ForcedPromotions records every forced tier-flip in the
	// module's history (audit log entries).
	ForcedPromotions []string
}

// BuildManifest is a pure function that produces the projected
// Manifest from typed inputs. Deterministic — same inputs always
// produce the same output. Tested in manifest_build_test.go.
func BuildManifest(in ManifestInputs) Manifest {
	m := Manifest{
		Module:            in.Module.Name,
		SourcePaths:       cloneAndSortStrings(in.Module.FacetPaths(gcapi.FacetCode)),
		ModuleKind:        string(in.Module.Kind),
		CharterVersion:    in.CharterVersion,
		CurrentTier:       in.Module.TierCurrent,
		TierTarget:        in.Module.TierTarget,
		TierState:         string(in.Module.TierState),
		LinkedGoals:       cloneAndSortStrings(in.Module.LinkedGoals),
		OpenDecisions:     []ManifestDecision{}, // always present (empty array, not null)
		ResolvedDecisions: []ManifestDecision{},
		ForcedPromotions:  cloneAndSortStrings(in.ForcedPromotions),
	}

	// Open / resolved decisions
	for _, od := range in.OpenDecisions {
		entry := ManifestDecision{ID: od.ODID}
		switch od.State {
		case gcapi.ODStateOpen:
			m.OpenDecisions = append(m.OpenDecisions, entry)
		case gcapi.ODStateResolved:
			entry.Resolution = od.Resolution
			m.ResolvedDecisions = append(m.ResolvedDecisions, entry)
		}
	}
	sort.Slice(m.OpenDecisions, func(i, j int) bool {
		return m.OpenDecisions[i].ID < m.OpenDecisions[j].ID
	})
	sort.Slice(m.ResolvedDecisions, func(i, j int) bool {
		return m.ResolvedDecisions[i].ID < m.ResolvedDecisions[j].ID
	})

	// The coverage block — the one coverage surface, rendered from the use
	// cases and the latest test-run verdicts.
	m.Coverage = projection.ComputeCoverage(in.UCFiles)

	// Audit rollup
	if in.LatestAudit != nil {
		m.LastAudit = in.LatestAudit.Name
	}
	if len(in.OpenFindings) > 0 {
		m.AuditFindingsOpen = map[string]int{}
		for _, f := range in.OpenFindings {
			if f.State != gcapi.FindingStateOpen {
				continue
			}
			m.AuditFindingsOpen[string(f.Severity)]++
		}
	}

	// Lint
	if in.LintErrorCount > 0 {
		m.LintStatus = "errors"
	} else {
		m.LintStatus = "clean"
	}

	return m
}

// UCStatuses maps each use case (UC-NNN) to its 4-verdict status, read from
// the latest test-run verdicts its UCFiles carry — never from a field on the
// use case. A use case without a verdict is absent and reads as no-test.
func UCStatuses(files []*ucfile.UCFile) map[string]gcapi.UCStatus {
	out := make(map[string]gcapi.UCStatus, len(files))
	for _, f := range files {
		out[f.Meta.ID] = verdictOf(gcapi.UCStatus(f.Meta.StatusCache))
	}
	return out
}

// verdictOf maps a 9-state UCStatus into the 4-verdict projection
// the manifest exposes. Mirrors the modules/code-gocode/concepts.md
// §4 mapping.
func verdictOf(s gcapi.UCStatus) gcapi.UCStatus {
	switch s {
	case gcapi.UCStatusValidated, gcapi.UCStatusCovered, gcapi.UCStatusPass:
		return gcapi.UCStatusPass
	case gcapi.UCStatusInDev, gcapi.UCStatusRegressed, gcapi.UCStatusFail:
		return gcapi.UCStatusFail
	case gcapi.UCStatusFailDeferred:
		return gcapi.UCStatusFailDeferred
	default:
		return gcapi.UCStatusNoTest
	}
}

// cloneAndSortStrings returns a sorted copy so callers can't
// accidentally mutate ManifestInputs after build, and so the
// canonical JSON output is byte-stable across runs.
func cloneAndSortStrings(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, len(in))
	copy(out, in)
	sort.Strings(out)
	return out
}
