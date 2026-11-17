// SPDX-License-Identifier: Apache-2.0

package cwmprojection

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	gcapi "github.com/katyoti/katyoti-fwk/internal/extensions/code/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/code/internal/ucfile"
)

// ucFile builds a rebuilt use case carrying its latest verdict, the way
// ucobj.FromObjects hands it to the projection.
func ucFile(id, verdict string) *ucfile.UCFile {
	return &ucfile.UCFile{Meta: ucfile.UCMeta{ID: id, Kind: ucfile.KindUseCase, Severity: "core", StatusCache: verdict}}
}

// fixtureInputs returns a representative ManifestInputs covering
// each branch of BuildManifest (open + resolved decisions, every
// UC verdict bucket, an audit with findings, a test-run, etc.).
func fixtureInputs() ManifestInputs {
	return ManifestInputs{
		Module: gcapi.Module{
			ID:          "m-fixture",
			Name:        "acceptance",
			Kind:        gcapi.ModuleKindLibrary,
			Facets:      []gcapi.Facet{{Kind: gcapi.FacetCode, Path: "internal/extensions/acceptance/"}, {Kind: gcapi.FacetCode, Path: "internal/test-modules/acceptance/"}},
			TierCurrent: 0,
			TierTarget:  1,
			TierState:   gcapi.TierStateVerifying,
			LinkedGoals: []string{"rock-solid-codebase.md"},
		},
		CharterVersion: "0.1",
		UseCases: []gcapi.UseCase{
			{UCID: "UC-001"}, {UCID: "UC-002"}, {UCID: "UC-003"}, {UCID: "UC-004"},
			{UCID: "UC-005", FailDeferredBy: "OD-002"}, {UCID: "UC-006"}, {UCID: "UC-007"},
		},
		// The statuses are the latest test-run verdicts the rebuilt use cases
		// carry; UC-007 has none.
		UCFiles: []*ucfile.UCFile{
			ucFile("UC-001", "pass"), ucFile("UC-002", "pass"),
			ucFile("UC-003", "fail"), ucFile("UC-004", "fail"),
			ucFile("UC-005", "fail-deferred"), ucFile("UC-006", "no-test"),
			ucFile("UC-007", ""),
		},
		OpenDecisions: []gcapi.OpenDecision{
			{ODID: "OD-001", State: gcapi.ODStateResolved, Resolution: "use library X"},
			{ODID: "OD-002", State: gcapi.ODStateOpen},
		},
		LatestTestRun: &gcapi.TestRun{
			RanAt: time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC),
			Totals: gcapi.TestRunTotals{
				Pass: 2, Fail: 2, FailDeferred: 1, NoTest: 2, UCTotal: 7,
			},
		},
		LatestAudit: &gcapi.AuditBundle{
			Name:  "audit-2026-05-18_09-00",
			RanAt: time.Date(2026, 5, 18, 9, 0, 0, 0, time.UTC),
		},
		OpenFindings: []gcapi.AuditFinding{
			{FindingID: "sec-1", Severity: gcapi.FindingSeverityHigh, State: gcapi.FindingStateOpen},
			{FindingID: "sec-2", Severity: gcapi.FindingSeverityHigh, State: gcapi.FindingStateOpen},
			{FindingID: "perf-1", Severity: gcapi.FindingSeverityMedium, State: gcapi.FindingStateOpen},
			{FindingID: "perf-2", Severity: gcapi.FindingSeverityMedium, State: gcapi.FindingStateClosed}, // filtered out
		},
		LintErrorCount: 0,
	}
}

// TestBuildManifest_Coverage asserts BuildManifest extracts every
// counter from a representative input set.
func TestBuildManifest_Coverage(t *testing.T) {
	t.Parallel()
	m := BuildManifest(fixtureInputs())

	// Identity
	if m.Module != "acceptance" {
		t.Errorf("Module = %q, want acceptance", m.Module)
	}
	if m.ModuleKind != "library" {
		t.Errorf("ModuleKind = %q, want library", m.ModuleKind)
	}
	if got, want := len(m.SourcePaths), 2; got != want {
		t.Errorf("SourcePaths len = %d, want %d", got, want)
	}
	if m.SourcePaths[0] != "internal/extensions/acceptance/" {
		t.Errorf("SourcePaths not sorted; got first = %q", m.SourcePaths[0])
	}

	// Tier
	if m.CurrentTier != 0 || m.TierTarget != 1 {
		t.Errorf("tier = (%d, %d), want (0, 1)", m.CurrentTier, m.TierTarget)
	}
	if m.TierState != "verifying" {
		t.Errorf("TierState = %q, want verifying", m.TierState)
	}

	// The coverage block, from the use cases' latest verdicts.
	if m.Coverage == nil {
		t.Fatal("coverage block missing")
	}
	c := m.Coverage
	if c.UCTotal != 7 || c.UCByStatus["pass"] != 2 || c.UCByStatus["fail"] != 2 ||
		c.UCByStatus["fail-deferred"] != 1 || c.UCByStatus["no-test"] != 2 {
		t.Errorf("coverage = %+v", c)
	}
	if got, want := c.DeferredUCIDs, []string{"UC-005"}; !equalStrings(got, want) {
		t.Errorf("DeferredUCIDs = %v, want %v", got, want)
	}

	// Decisions
	if len(m.OpenDecisions) != 1 || m.OpenDecisions[0].ID != "OD-002" {
		t.Errorf("OpenDecisions = %+v, want [OD-002]", m.OpenDecisions)
	}
	if len(m.ResolvedDecisions) != 1 || m.ResolvedDecisions[0].ID != "OD-001" ||
		m.ResolvedDecisions[0].Resolution != "use library X" {
		t.Errorf("ResolvedDecisions = %+v", m.ResolvedDecisions)
	}

	// Audit
	if m.LastAudit != "audit-2026-05-18_09-00" {
		t.Errorf("LastAudit = %q", m.LastAudit)
	}
	if got, want := m.AuditFindingsOpen["high"], 2; got != want {
		t.Errorf("AuditFindingsOpen[high] = %d, want %d", got, want)
	}
	if got, want := m.AuditFindingsOpen["medium"], 1; got != want {
		t.Errorf("AuditFindingsOpen[medium] = %d, want %d (closed perf-2 should be filtered)", got, want)
	}
	if _, present := m.AuditFindingsOpen["critical"]; present {
		t.Errorf("AuditFindingsOpen unexpectedly contains 'critical' bucket")
	}

	// Lint
	if m.LintStatus != "clean" {
		t.Errorf("LintStatus = %q, want clean (LintErrorCount=0)", m.LintStatus)
	}
}

// TestBuildManifest_Idempotent asserts re-building with identical
// inputs produces a byte-identical canonical JSON output. This is
// the contract the drift detector relies on.
func TestBuildManifest_Idempotent(t *testing.T) {
	t.Parallel()
	in := fixtureInputs()

	first, err := BuildManifest(in).MarshalCanonicalJSON()
	if err != nil {
		t.Fatalf("first marshal: %v", err)
	}
	second, err := BuildManifest(in).MarshalCanonicalJSON()
	if err != nil {
		t.Fatalf("second marshal: %v", err)
	}

	if !bytes.Equal(first, second) {
		t.Errorf("canonical JSON differs between runs:\n--- first ---\n%s\n--- second ---\n%s",
			first, second)
	}
}

// TestWriteManifestJSON_Roundtrip exercises the full write+read path
// against a tempdir, asserting the written bytes equal the marshaled
// bytes and the file lives at the expected path.
func TestWriteManifestJSON_Roundtrip(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "modules", "acceptance", "manifest.json")

	m := BuildManifest(fixtureInputs())
	n, err := WriteManifestJSON(m, path)
	if err != nil {
		t.Fatalf("WriteManifestJSON: %v", err)
	}

	expected, err := m.MarshalCanonicalJSON()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if n != len(expected) {
		t.Errorf("written bytes = %d, want %d", n, len(expected))
	}

	// Read back; assert byte-identical.
	read, err := readFile(t, path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if !bytes.Equal(read, expected) {
		t.Errorf("on-disk bytes differ from marshaled")
	}
}

// TestBuildManifest_EmptyModuleHasZeroCounters asserts a freshly
// created module with no items still produces a valid manifest
// (zero counters, empty arrays not null).
func TestBuildManifest_EmptyModuleHasZeroCounters(t *testing.T) {
	t.Parallel()
	m := BuildManifest(ManifestInputs{
		Module: gcapi.Module{
			Name:        "fresh",
			Kind:        gcapi.ModuleKindFramework,
			TierCurrent: 0,
			TierTarget:  1,
			TierState:   gcapi.TierStateBootstrapping,
		},
	})
	if m.Coverage == nil || m.Coverage.UCTotal != 0 || m.Coverage.ACTotal != 0 {
		t.Errorf("empty module not all zero counters: %+v", m.Coverage)
	}
	if m.OpenDecisions == nil || m.ResolvedDecisions == nil {
		t.Errorf("decisions arrays should be empty-not-nil for canonical JSON")
	}

	// Canonical JSON should contain `"open_decisions": []` not null.
	out, err := m.MarshalCanonicalJSON()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(out), `"open_decisions": []`) {
		t.Errorf("canonical JSON missing empty open_decisions array; got:\n%s", out)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func readFile(t *testing.T, path string) ([]byte, error) {
	t.Helper()
	return os.ReadFile(path)
}
