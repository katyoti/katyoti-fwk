// SPDX-License-Identifier: Apache-2.0

package verify

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// fixtureCharter is a minimal markdown charter shaped like a real one. Used
// by parser + emitter tests so they don't depend on the live tasks charter.
const fixtureCharter = `# Test Module Charter

## Specifications

### Use Cases

#### Engine primitives (UC-001 to UC-019)

| UC ID | Title | Severity | DoD |
|---|---|---|---|
| UC-001 | First UC | core | ✓ |

#### Permissions (UC-050 to UC-058)

| UC ID | Title | Severity | DoD |
|---|---|---|---|
| UC-002 | Second UC, security-critical | security | — |

#### Context (UC-070 to UC-074)

| UC ID | Title | Severity | DoD |
|---|---|---|---|
| UC-003 | A deferred UC | qol | — |

### Function inventory

(unrelated table)

## Test plan

| UC ID | L1 (Go unit) | L2 (CLI black-box) | L3 (self-hosted) |
|---|---|---|---|
` + "| UC-001 | `pkg/file_test.go::TestFirst, TestFirstAlt` | — | — |\n" +
	"| UC-002 | `pkg/file_test.go::TestSecond` | tests/L2/uc-002.tasktest | — | (closed by some-task 2026-05-07) |\n" +
	"| UC-003 | `pkg/file_test.go::TestThird` | — | — | (fail-deferred OD-099) |\n" + `
## Audit ledger
`

func TestParseUseCases_Tasks(t *testing.T) {
	ucs := ParseUseCases(fixtureCharter)
	if len(ucs) != 3 {
		t.Fatalf("expected 3 UCs, got %d", len(ucs))
	}
	want := []struct {
		id       string
		severity string
		dodOK    bool
		group    string
	}{
		{"UC-001", "core", true, "Engine primitives (UC-001 to UC-019)"},
		{"UC-002", "security", false, "Permissions (UC-050 to UC-058)"},
		{"UC-003", "qol", false, "Context (UC-070 to UC-074)"},
	}
	for i, w := range want {
		got := ucs[i]
		if got.ID != w.id || got.Severity != w.severity || got.DoDOK != w.dodOK {
			t.Errorf("UC %d: got id=%s sev=%s dod=%v; want %+v",
				i, got.ID, got.Severity, got.DoDOK, w)
		}
		if got.Group != w.group {
			t.Errorf("UC %d: Group = %q, want %q", i, got.Group, w.group)
		}
	}
}

func TestParseTestPlan_ExtractsTestNames(t *testing.T) {
	rows := ParseTestPlan(fixtureCharter)
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}

	if got := rows[0].L1Tests; len(got) != 2 || got[0] != "TestFirst" || got[1] != "TestFirstAlt" {
		t.Errorf("UC-001 L1 = %v, want [TestFirst TestFirstAlt]", got)
	}
	if got := rows[1].L2Scenarios; len(got) != 1 || got[0] != "tests/L2/uc-002.tasktest" {
		t.Errorf("UC-002 L2 = %v", got)
	}
	if !strings.Contains(rows[2].Notes, "OD-099") {
		t.Errorf("UC-003 notes should contain OD-099, got %q", rows[2].Notes)
	}
}

func TestResolveVerdict_Pass(t *testing.T) {
	row := &UCRow{IDs: []string{"UC-001"}, L1Tests: []string{"TestFirst"}}
	res := &UCResult{
		UCID: "UC-001",
		L1: CombinedOutcome{
			UCID:   "UC-001",
			Status: StatusPass,
		},
	}
	got := ResolveVerdict(res, row)
	if got != VerdictPass {
		t.Errorf("got %v, want pass", got)
	}
}

func TestResolveVerdict_FailDeferred(t *testing.T) {
	row := &UCRow{IDs: []string{"UC-073"}, Notes: "fail-deferred OD-004"}
	res := &UCResult{UCID: "UC-073"}
	got := ResolveVerdict(res, row)
	if got != VerdictFailDeferred {
		t.Errorf("got %v, want fail-deferred", got)
	}
	if res.DeferredBy != "OD-004" {
		t.Errorf("DeferredBy = %q, want OD-004", res.DeferredBy)
	}
}

func TestResolveVerdict_NoTest(t *testing.T) {
	res := &UCResult{UCID: "UC-999"}
	got := ResolveVerdict(res, nil)
	if got != VerdictNoTest {
		t.Errorf("got %v, want no-test", got)
	}
}

func TestEmitJSON_StructureValid(t *testing.T) {
	report := makeFixtureReport()
	var buf bytes.Buffer
	if err := EmitJSON(&buf, report); err != nil {
		t.Fatalf("EmitJSON: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, buf.String())
	}
	for _, key := range []string{"module", "by_uc", "totals", "tier_1_dod_met", "ran_at"} {
		if _, ok := decoded[key]; !ok {
			t.Errorf("JSON missing key %q", key)
		}
	}
	totals := decoded["totals"].(map[string]any)
	if totals["pass"].(float64) != 1 {
		t.Errorf("totals.pass = %v, want 1", totals["pass"])
	}
}

func TestEmitJUnit_ParsesValid(t *testing.T) {
	report := makeFixtureReport()
	var buf bytes.Buffer
	if err := EmitJUnit(&buf, report); err != nil {
		t.Fatalf("EmitJUnit: %v", err)
	}
	body := buf.String()
	if !strings.HasPrefix(body, `<?xml`) {
		t.Errorf("missing XML declaration; got %s", body[:50])
	}
	// Parse to validate structure.
	dec := xml.NewDecoder(strings.NewReader(body))
	for {
		_, err := dec.Token()
		if err != nil {
			if err.Error() == "EOF" {
				break
			}
			t.Fatalf("XML parse error: %v", err)
		}
	}
	if !strings.Contains(body, `<testsuites name="test-mod"`) {
		t.Errorf("missing testsuites root; got: %s", body)
	}
	if !strings.Contains(body, `classname="test-mod.UC-001"`) {
		t.Errorf("missing UC-001 testcase classname; got: %s", body)
	}
	if !strings.Contains(body, `<failure`) {
		t.Errorf("expected at least one <failure> tag; got: %s", body)
	}
}

func TestEmitText_GroupedTreeCollapses(t *testing.T) {
	report := makeFixtureReport()
	var buf bytes.Buffer
	if err := EmitText(&buf, report); err != nil {
		t.Fatalf("EmitText: %v", err)
	}
	body := buf.String()
	// Group headers must appear with their counts.
	for _, line := range []string{"Engine primitives", "Permissions", "Context", "🟢 1", "🔴 1", "🟠 1", "Tier-1 DoD met?"} {
		if !strings.Contains(body, line) {
			t.Errorf("text output missing %q\n---\n%s", line, body)
		}
	}
	// Default mode must collapse passes — the EXPANDED row for UC-001
	// (a leading "🟢 UC-001" marker line) must be absent, even though
	// "UC-001" appears inside the "Engine primitives (UC-001 to UC-019)"
	// group label.
	if strings.Contains(body, "🟢 UC-001") {
		t.Errorf("default tree should COLLAPSE the passing UC-001 row; got:\n%s", body)
	}
	// UC-002 (fail) and UC-003 (fail-deferred) must be expanded with their
	// verdict-glyph row markers.
	for _, mustHave := range []string{"🔴 UC-002", "🟠 UC-003"} {
		if !strings.Contains(body, mustHave) {
			t.Errorf("expected expanded row %q under its group; got:\n%s", mustHave, body)
		}
	}
}

func TestEmitText_DetailExpandsAllRows(t *testing.T) {
	report := makeFixtureReport()
	var buf bytes.Buffer
	if err := EmitTextDetail(&buf, report); err != nil {
		t.Fatalf("EmitTextDetail: %v", err)
	}
	body := buf.String()
	for _, line := range []string{"UC-001", "UC-002", "UC-003"} {
		if !strings.Contains(body, line) {
			t.Errorf("--detail mode should expand %q; got:\n%s", line, body)
		}
	}
}

func TestGroupResults_PreservesFirstSeenOrder(t *testing.T) {
	results := []*UCResult{
		{UCID: "A", Group: "second", Verdict: VerdictPass},
		{UCID: "B", Group: "first", Verdict: VerdictPass},
		{UCID: "C", Group: "second", Verdict: VerdictFail},
		{UCID: "D", Group: "first", Verdict: VerdictPass},
	}
	groups := groupResults(results)
	if len(groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(groups))
	}
	if groups[0].label != "second" || groups[1].label != "first" {
		t.Errorf("group order = [%s, %s], want [second, first]",
			groups[0].label, groups[1].label)
	}
	if groups[0].totals.Pass != 1 || groups[0].totals.Fail != 1 {
		t.Errorf("second group totals = %+v", groups[0].totals)
	}
}

// TestRun_L2Filter probes the full orchestrator with --layer L2 + --uc UC-150
// to track down why the integration says no-test even though the direct
// runL2Scenarios returns pass.
func TestRun_L2Filter(t *testing.T) {
	if testing.Short() {
		t.Skip("filesystem + go run")
	}
	// It runs against the REAL repository, so it is read-only: before NoWrite
	// it recorded a test-run into modules/tasks/objects at every `go test`.
	defer repoModulesUnchanged(t)()
	report, err := Run(context.Background(), Options{
		Module:      "tasks",
		ProjectRoot: repoRootForTest,
		Layer:       "L2",
		UCFilter:    []string{"UC-150"},
		NoWrite:     true,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	for _, res := range report.Results {
		if res.UCID != "UC-150" {
			continue
		}
		t.Logf("UC-150 result: Verdict=%v Reason=%q L1.UCID=%q L1.Status=%v L2.UCID=%q L2.Status=%v",
			res.Verdict, res.Reason, res.L1.UCID, res.L1.Status, res.L2.UCID, res.L2.Status)
	}
}

// TestRunL2_RealCanary directly drives runL2Scenarios against the live UC-150
// canary path. Bypasses the orchestrator so we can see exactly what
// l2runner produces vs what the verdict resolver consumes.
func TestRunL2_RealCanary(t *testing.T) {
	if testing.Short() {
		t.Skip("network/filesystem-touching")
	}
	out := runL2Scenarios(context.Background(), "../../../../..", "tasks", "UC-150", []string{"tests/L2/uc-150-simulator-basic.tasktest"}, false)
	t.Logf("L2 outcome: UCID=%q Status=%v Tests=%d FailReason=%q",
		out.UCID, out.Status, len(out.Tests), out.FailReason)
	for i, tc := range out.Tests {
		t.Logf("  test[%d] Name=%q Status=%v Output=%.200q", i, tc.Name, tc.Status, tc.Output)
	}
	if out.UCID != "UC-150" {
		t.Errorf("UCID = %q, want UC-150 (Combine should set it from input)", out.UCID)
	}
}

func TestMatchUCPattern(t *testing.T) {
	cases := []struct {
		pat, id string
		match   bool
	}{
		{"UC-053", "UC-053", true},
		{"UC-053", "UC-054", false},
		{"UC-04*", "UC-040", true},
		{"UC-04*", "UC-049", true},
		{"UC-04*", "UC-050", false},
		{"UC-1*", "UC-100", true},
	}
	for _, c := range cases {
		got := matchUCPattern(c.pat, c.id)
		if got != c.match {
			t.Errorf("matchUCPattern(%q, %q) = %v, want %v", c.pat, c.id, got, c.match)
		}
	}
}

// TestParseTestOutput_Mixed exercises parseTestOutput with a fixture that
// mixes PASS / FAIL / SKIP — the canonical batched-go-test output shape.
func TestParseTestOutput_Mixed(t *testing.T) {
	output := `=== RUN   TestPassA
--- PASS: TestPassA (0.01s)
=== RUN   TestPassB
--- PASS: TestPassB (0.02s)
=== RUN   TestFailC
=== PAUSE TestFailC
=== CONT  TestFailC
    file_test.go:42: expected X, got Y
--- FAIL: TestFailC (0.05s)
=== RUN   TestSkipD
    file_test.go:99: missing fixture
--- SKIP: TestSkipD (0.00s)
PASS
ok      github.com/katyoti/katyoti-fwk/pkg/foo  0.234s`

	parsed := parseTestOutput(output)
	if len(parsed) != 4 {
		t.Fatalf("expected 4 outcomes, got %d: %v", len(parsed), parsed)
	}
	if parsed["TestPassA"].Status != StatusPass {
		t.Errorf("TestPassA = %v, want pass", parsed["TestPassA"].Status)
	}
	if parsed["TestPassB"].Duration != 20*time.Millisecond {
		t.Errorf("TestPassB duration = %v, want 20ms", parsed["TestPassB"].Duration)
	}
	if parsed["TestFailC"].Status != StatusFail {
		t.Errorf("TestFailC = %v, want fail", parsed["TestFailC"].Status)
	}
	if !strings.Contains(parsed["TestFailC"].Output, "expected X, got Y") {
		t.Errorf("TestFailC output should capture failure body; got: %q", parsed["TestFailC"].Output)
	}
	if parsed["TestSkipD"].Status != StatusSkip {
		t.Errorf("TestSkipD = %v, want skip", parsed["TestSkipD"].Status)
	}
}

// TestBuildOrPattern checks the regex shape for single + multiple names,
// including a name that contains regex metacharacters.
func TestBuildOrPattern(t *testing.T) {
	cases := map[string]string{
		"":          "^$", // edge — buildOrPattern of one empty
		"TestA":     "^TestA$",
		"Test_With": "^Test_With$",
	}
	for name, want := range cases {
		got := buildOrPattern([]string{name})
		if got != want {
			t.Errorf("buildOrPattern([%q]) = %q, want %q", name, got, want)
		}
	}
	multi := buildOrPattern([]string{"TestA", "TestB", "TestC_Edge"})
	if multi != "^(TestA|TestB|TestC_Edge)$" {
		t.Errorf("buildOrPattern multi = %q", multi)
	}
}

// TestRun_NoNamesSkipsExec confirms the runner returns immediately on empty
// input — important so the orchestrator's "no L1 tests" branch never shells out.
func TestRun_NoNamesSkipsExec(t *testing.T) {
	r := &L1Runner{SourceRoots: []string{"./does-not-exist/..."}}
	got := r.Run(nil, nil)
	if got != nil {
		t.Errorf("expected nil for empty input, got %+v", got)
	}
}

func makeFixtureReport() *Report {
	r := &Report{
		Module:         "test-mod",
		CharterVersion: "0.1",
		Layer:          "L1",
		RanAt:          time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC),
		Results: []*UCResult{
			{
				UCID: "UC-001", Title: "First UC", Severity: "core",
				Group:   "Engine primitives (UC-001 to UC-019)",
				L1:      CombinedOutcome{UCID: "UC-001", Status: StatusPass, TotalTime: 12 * time.Millisecond},
				Verdict: VerdictPass,
			},
			{
				UCID: "UC-002", Title: "Second UC", Severity: "security",
				Group: "Permissions (UC-050 to UC-058)",
				L1: CombinedOutcome{
					UCID:       "UC-002",
					Status:     StatusFail,
					TotalTime:  8 * time.Millisecond,
					FailReason: "TestSecond: assertion failed",
					Tests:      []TestOutcome{{Name: "TestSecond", Status: StatusFail, Output: "FAIL: assertion"}},
				},
				Verdict: VerdictFail,
				Reason:  "TestSecond: assertion failed",
			},
			{
				UCID: "UC-003", Title: "Deferred UC", Severity: "qol",
				Group:   "Context (UC-070 to UC-074)",
				Verdict: VerdictFailDeferred, DeferredBy: "OD-099",
			},
		},
	}
	r.ComputeTotals()
	return r
}

// repoRootForTest is the repository this package's tests may READ.
const repoRootForTest = "../../../../.."

// repoModulesUnchanged snapshots the repository's modules/tasks tree — the
// module the real-repository tests verify — and returns the check that it is
// unchanged: a test run against the real repository must leave it as it found
// it — a test writes to t.TempDir(), never to the tree under version control.
// The tree is the workspace's own: where it is absent (the open-source export
// does not publish modules/tasks) the test that needs it is skipped.
func repoModulesUnchanged(t *testing.T) func() {
	t.Helper()
	root := filepath.Join(repoRootForTest, "modules", "tasks")
	if _, err := os.Stat(root); err != nil {
		t.Skipf("no %s in this tree: %v", root, err)
	}
	return treeUnchanged(t, root)
}

// treeUnchanged snapshots root (every path with its size and modification
// time) and returns the check that it is unchanged.
func treeUnchanged(t *testing.T, root string) func() {
	t.Helper()
	before := snapshotTree(t, root)
	return func() {
		t.Helper()
		after := snapshotTree(t, root)
		for p, v := range after {
			if before[p] != v {
				t.Errorf("the test wrote %s/%s (%s)", root, p, v)
			}
		}
		for p := range before {
			if _, ok := after[p]; !ok {
				t.Errorf("the test removed %s/%s", root, p)
			}
		}
	}
}

func snapshotTree(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		out[filepath.ToSlash(rel)] = info.ModTime().String() + "/" + strconv.FormatInt(info.Size(), 10)
		return nil
	})
	if err != nil {
		t.Fatalf("snapshot %s: %v", root, err)
	}
	return out
}

// A run that would record without a CWM service is refused before any test
// runs and before anything is written: there is no file-writing fallback that
// could land a test-run in the repository a caller's ProjectRoot points at.
func TestRun_RecordingWithoutCWMIsRefused(t *testing.T) {
	// A synthetic project: a module folder with its manifest, charter and a
	// use case, so a run that went past the refusal would find work to do.
	root := t.TempDir()
	moduleDir := filepath.Join(root, "modules", "m")
	for rel, body := range map[string]string{
		"manifest.json":   `{"module":"m","charter_version":"0.1"}`,
		"charter.md":      fixtureCharter,
		"use-cases/x.txt": "a file the refusal must leave as it is",
	} {
		p := filepath.Join(moduleDir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	defer treeUnchanged(t, root)()
	_, err := Run(context.Background(), Options{
		Module:      "m",
		ProjectRoot: root,
		Layer:       "L1",
		UCFilter:    []string{"UC-001"},
	})
	if err == nil || !strings.Contains(err.Error(), "or run with NoWrite") {
		t.Fatalf("err = %v, want the no-CWM refusal", err)
	}
}
