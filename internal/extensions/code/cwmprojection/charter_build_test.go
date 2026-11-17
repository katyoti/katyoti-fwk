// SPDX-License-Identifier: Apache-2.0

package cwmprojection

import (
	"strings"
	"testing"
	"time"

	gcapi "github.com/katyoti/katyoti-fwk/internal/extensions/code/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/code/internal/ucfile"
)

func charterFixture() CharterInputs {
	return CharterInputs{
		Module: gcapi.Module{
			Name:        "acceptance",
			Kind:        gcapi.ModuleKindLibrary,
			Facets:      []gcapi.Facet{{Kind: gcapi.FacetCode, Path: "internal/test-modules/acceptance/"}},
			TierCurrent: 0,
			TierTarget:  1,
			TierState:   gcapi.TierStateVerifying,
		},
		CharterItem: &Charter{
			Version:       "0.1",
			Status:        "draft",
			ConceptsMD:    "## Glossary\n\n- **Foo** — a thing.\n- **Bar** — another thing.",
			APIContractMD: "Exports: `Foo()`, `Bar()`. Stability: stable.",
		},
		UseCases: []gcapi.UseCase{
			{UCID: "UC-001", Title: "Foo does X", Severity: gcapi.UCSeverityCore, Body: "L1: `pkg/foo_test.go::TestFooX`"},
			{UCID: "UC-002", Title: "Bar guards Y", Severity: gcapi.UCSeveritySecurity, Body: "L1: `pkg/bar_test.go::TestBarY`\nNegative: Y must not panic on nil"},
			{UCID: "UC-003", Title: "Baz logs Z", Severity: gcapi.UCSeverityQoL, FailDeferredBy: "OD-001", Body: ""},
		},
		// The statuses: the latest test-run verdicts the rebuilt use cases carry.
		UCFiles: []*ucfile.UCFile{
			{Meta: ucfile.UCMeta{ID: "UC-001", StatusCache: "pass", AcceptanceCriteria: []ucfile.AC{{ID: "UC-001-AC1", Layer: "L1", TestRef: "TestFooX", Status: "pass"}}}},
			{Meta: ucfile.UCMeta{ID: "UC-002", StatusCache: "pass"}},
			{Meta: ucfile.UCMeta{ID: "UC-003", StatusCache: "fail-deferred"}},
		},
		OpenDecisions: []gcapi.OpenDecision{
			{ODID: "OD-001", Title: "Pick logging library", State: gcapi.ODStateOpen},
			{ODID: "OD-002", Title: "Repo layout for tests", State: gcapi.ODStateResolved, Resolution: "use tests/L{1,2,3}/ convention"},
		},
		LatestAudit: &gcapi.AuditBundle{
			Name:  "audit-2026-05-18_09-00",
			RanAt: time.Date(2026, 5, 18, 9, 0, 0, 0, time.UTC),
		},
		OpenFindings: []gcapi.AuditFinding{
			{FindingID: "sec-1", Severity: gcapi.FindingSeverityHigh, State: gcapi.FindingStateOpen},
			{FindingID: "perf-1", Severity: gcapi.FindingSeverityMedium, State: gcapi.FindingStateOpen},
			{FindingID: "perf-2", Severity: gcapi.FindingSeverityMedium, State: gcapi.FindingStateClosed},
		},
		LatestTestRun: &gcapi.TestRun{
			RanAt:       time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC),
			Totals:      gcapi.TestRunTotals{Pass: 2, Fail: 0, FailDeferred: 1, NoTest: 0, UCTotal: 3},
			TierNDoDMet: true,
		},
	}
}

// TestBuildCharter_AllSectionsPresent asserts every documented
// section appears in the rendered markdown.
func TestBuildCharter_AllSectionsPresent(t *testing.T) {
	t.Parallel()
	out := BuildCharter(charterFixture())

	required := []string{
		"# acceptance — Module Charter",
		"## Concepts & Glossary",
		"## Use Cases",
		"## Definition of Done — per UC",
		"## Test plan",
		"## Open Decisions",
		"## Public API Contract",
		"## Audit History",
		"## Snapshot — Manifest Stats",
	}
	for _, want := range required {
		if !strings.Contains(out, want) {
			t.Errorf("missing required section %q\n--- rendered ---\n%s", want, out)
		}
	}
}

// TestBuildCharter_UCTableShape verifies the UC table renders rows
// with the right status badge mapping.
func TestBuildCharter_UCTableShape(t *testing.T) {
	t.Parallel()
	out := BuildCharter(charterFixture())

	// UC-001 (validated) → 🟢 pass
	if !strings.Contains(out, "| `UC-001` |") || !strings.Contains(out, "🟢 pass") {
		t.Errorf("UC-001 table row missing or wrong status: %s", excerpt(out, "UC-001", 1))
	}
	// UC-003 (fail-deferred) → 🟠 fail-deferred
	if !strings.Contains(out, "🟠 fail-deferred") {
		t.Errorf("expected 🟠 fail-deferred badge for UC-003")
	}
}

// TestBuildCharter_AuthoredProsePassesThrough confirms charter.concepts_md
// and api_contract_md land in the output verbatim.
func TestBuildCharter_AuthoredProsePassesThrough(t *testing.T) {
	t.Parallel()
	in := charterFixture()
	out := BuildCharter(in)

	if !strings.Contains(out, "## Glossary") {
		t.Errorf("authored concepts_md content missing from output")
	}
	if !strings.Contains(out, "Exports: `Foo()`, `Bar()`") {
		t.Errorf("authored api_contract_md content missing from output")
	}
}

// TestBuildCharter_Idempotent_ByteStable asserts re-builds with
// identical inputs produce byte-identical markdown. Drift detector
// depends on this.
func TestBuildCharter_Idempotent_ByteStable(t *testing.T) {
	t.Parallel()
	in := charterFixture()
	first := BuildCharter(in)
	second := BuildCharter(in)
	if first != second {
		t.Errorf("BuildCharter not byte-stable across runs")
	}
}

// TestBuildCharter_EmptyModuleStillValid produces a charter for a
// fresh module with no items.
func TestBuildCharter_EmptyModuleStillValid(t *testing.T) {
	t.Parallel()
	out := BuildCharter(CharterInputs{
		Module: gcapi.Module{
			Name:        "fresh",
			Kind:        gcapi.ModuleKindFramework,
			TierCurrent: 0,
			TierTarget:  1,
			TierState:   gcapi.TierStateBootstrapping,
		},
	})
	if !strings.Contains(out, "# fresh — Module Charter") {
		t.Errorf("empty-module title missing")
	}
	if !strings.Contains(out, "_No use-case items at this module yet") {
		t.Errorf("empty-UC placeholder missing")
	}
	if !strings.Contains(out, "_(no audit yet)_") {
		t.Errorf("empty-audit placeholder missing")
	}
}

// TestBuildCharter_PromoteGateWarning surfaces when critical/high
// findings are open.
func TestBuildCharter_PromoteGateWarning(t *testing.T) {
	t.Parallel()
	out := BuildCharter(charterFixture())
	if !strings.Contains(out, "Promote gate blocked") {
		t.Errorf("expected promote-gate warning given open high finding; got:\n%s",
			excerpt(out, "Audit History", 30))
	}
}

// excerpt returns the lines surrounding the first occurrence of
// needle for error-message context.
func excerpt(s, needle string, lines int) string {
	idx := strings.Index(s, needle)
	if idx < 0 {
		return "(not found)"
	}
	end := idx
	for n := 0; n < lines && end < len(s); n++ {
		next := strings.Index(s[end:], "\n")
		if next < 0 {
			return s[idx:]
		}
		end += next + 1
	}
	return s[idx:end]
}
