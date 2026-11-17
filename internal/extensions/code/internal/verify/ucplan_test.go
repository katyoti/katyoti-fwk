// SPDX-License-Identifier: Apache-2.0

package verify

import (
	"path/filepath"
	"testing"
	"testing/fstest"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/extensions/code/internal/ucfile"
	"github.com/katyoti/katyoti-fwk/internal/extensions/code/internal/ucobj"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

func TestACStatusFromOutcome(t *testing.T) {
	cases := []struct {
		in   TestStatus
		want string
	}{
		{StatusPass, ucfile.StatusPass},
		{StatusSkip, ucfile.StatusSkipped},
		{StatusNoMatch, ucfile.StatusFail}, // dead ref surfaces red, never silently green
		{StatusFail, ucfile.StatusFail},
		{StatusError, ucfile.StatusFail},
	}
	for _, c := range cases {
		got, _ := acStatusFromOutcome(TestOutcome{Name: "TestX", Status: c.in})
		if got != c.want {
			t.Errorf("%v: got %s want %s", c.in, got, c.want)
		}
	}
}

func TestVerdictFromACs(t *testing.T) {
	ac := func(s string) ACResult { return ACResult{ID: "UC-001-AC1", Status: s} }
	cases := []struct {
		name string
		acs  []ACResult
		want Verdict
	}{
		{"empty", nil, VerdictNoTest},
		{"all pass", []ACResult{ac("pass"), ac("pass")}, VerdictPass},
		{"fail wins", []ACResult{ac("pass"), ac("fail")}, VerdictFail},
		{"no-test over skipped", []ACResult{ac("skipped"), ac("no-test")}, VerdictNoTest},
		{"deferred over pass", []ACResult{ac("pass"), ac("fail-deferred")}, VerdictFailDeferred},
		{"skipped over deferred", []ACResult{ac("fail-deferred"), ac("skipped")}, VerdictSkipped},
	}
	for _, c := range cases {
		got, _ := verdictFromACs(c.acs)
		if got != c.want {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}

// TestRunFromUseCases_NoProbePathRecordsTheRun exercises the per-AC loop
// without shelling go test: ACs are fail-deferred / no-probe / L3 — verify
// must classify them, never run them, and record the run as a test-run object
// that carries forward what it did not run.
func TestRunFromUseCases_NoProbePathRecordsTheRun(t *testing.T) {
	dir := t.TempDir()
	ctx, cwm := cwmapp.NewAt(t, dir, cwmapp.Pack(fstest.MapFS{
		"object-types/module.yaml":   {Data: []byte("name: module\ncapabilities: [container]\n")},
		"object-types/test-run.yaml": {Data: cwmapp.ShippedFile(t, "app/ktw-code/embed/cwm/object-types/test-run.yaml")},
	}))
	cwmapp.Module(t, ctx, cwm, "m")
	f := &ucfile.UCFile{
		Path: "UC-001.cwm.yaml",
		Meta: ucfile.UCMeta{
			ID: "UC-001", Kind: ucfile.KindUseCase, Title: "X", Module: "m", Severity: "core",
			Description:    "d",
			FailDeferredBy: "OD-001",
			AcceptanceCriteria: []ucfile.AC{
				{ID: "UC-001-AC1", Claim: "a", Layer: "L1", TestRef: "TestA", Status: ucfile.StatusFailDeferred},
				{ID: "UC-001-AC2", Claim: "b", Layer: "L1", TestRef: "", Status: ucfile.StatusNoTest},
				{ID: "UC-001-AC3", Claim: "c", Kind: "anti", Layer: "L3", TestRef: "tests/L3/x.tasktest", Status: ucfile.StatusSkipped},
			},
		},
	}
	skip := &ucfile.UCFile{
		Path: "UC-002.cwm.yaml",
		Meta: ucfile.UCMeta{ID: "UC-002", Kind: ucfile.KindUseCase, Title: "Y", Module: "m", Severity: "qol"},
	}

	report := &Report{RanAt: time.Now(), Layer: "L1"}
	opts := Options{Module: "m", ProjectRoot: dir, Layer: "L1", SeverityFilter: []string{"core"}, CWM: cwm}
	var runPath string
	opts.RunPath = &runPath
	got, err := runFromUseCases(ctx, opts, &Manifest{Module: "m"}, nil, dir,
		[]*ucfile.UCFile{f, skip}, report)
	if err != nil {
		t.Fatal(err)
	}
	if got.Source != "use-cases" {
		t.Fatalf("source = %q", got.Source)
	}
	if len(got.Results) != 2 {
		t.Fatalf("want 2 results, got %d", len(got.Results))
	}
	// UC-002 filtered out by severity.
	var r1, r2 *UCResult
	for _, r := range got.Results {
		switch r.UCID {
		case "UC-001":
			r1 = r
		case "UC-002":
			r2 = r
		}
	}
	if r2.Verdict != VerdictSkipped {
		t.Fatalf("UC-002 must be filter-skipped, got %v", r2.Verdict)
	}
	if len(r1.ACs) != 3 {
		t.Fatalf("want 3 AC results, got %d", len(r1.ACs))
	}
	if r1.ACs[0].Status != ucfile.StatusFailDeferred ||
		r1.ACs[1].Status != ucfile.StatusNoTest ||
		r1.ACs[2].Status != ucfile.StatusSkipped {
		t.Fatalf("AC statuses wrong: %+v", r1.ACs)
	}
	// worst-of(fail-deferred, no-test, skipped) = no-test.
	if r1.Verdict != VerdictNoTest {
		t.Fatalf("UC-001 verdict = %v", r1.Verdict)
	}
	if r1.DeferredBy != "OD-001" {
		t.Fatalf("DeferredBy = %q", r1.DeferredBy)
	}

	// The run is recorded as a test-run object: what was not run this time
	// (deferred, L3) keeps its last status, so the next run still reads it.
	if runPath == "" {
		t.Fatal("the run was not recorded")
	}
	// Recorded THROUGH CWM, in its workspace: the index knows the run.
	listed, err := cwm.ObjectList(ctx, cwmapi.ObjectListInput{Type: "test-run"})
	if err != nil || len(listed.Objects) != 1 {
		t.Fatalf("test-runs in CWM = %+v, %v; want the one run", listed.Objects, err)
	}
	runs, err := ucobj.LoadVerdicts(filepath.Join(dir, "modules", "m"), "m")
	if err != nil {
		t.Fatal(err)
	}
	v := runs["UC-001"]
	if v.Verdict != "no-test" || v.ACs["UC-001-AC1"] != "fail-deferred" || v.ACs["UC-001-AC2"] != "no-test" || v.ACs["UC-001-AC3"] != "skipped" {
		t.Fatalf("recorded run = %+v", v)
	}
	if _, filtered := runs["UC-002"]; filtered {
		t.Fatal("a use case filtered out of the run was recorded")
	}
}
