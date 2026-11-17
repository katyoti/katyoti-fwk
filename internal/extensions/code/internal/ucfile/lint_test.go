// SPDX-License-Identifier: Apache-2.0

package ucfile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// mkUC builds a valid file; tests then break one aspect at a time.
func mkUC() *UCFile {
	return &UCFile{
		Path: "UC-014-resume.md",
		Meta: UCMeta{
			ID: "UC-014", Kind: KindUseCase, Title: "Resume", Module: "tasks",
			Severity:    "core",
			Description: "Resume contract; probed at L1 (restore) + L2 anti (ordering).",
			AcceptanceCriteria: []AC{
				{ID: "UC-014-AC1", Claim: "restores state", Layer: "L1",
					TestRef: "TestResume_RestoresProcessedIDs", Status: StatusPass},
				{ID: "UC-014-AC2", Claim: "no early publish", Kind: "anti", Layer: "L2",
					TestRef: "tests/L2/resume.tasktest", Status: StatusNoTest},
			},
		},
	}
}

func rules(fs []Finding) map[string]string { // rule -> severity
	m := map[string]string{}
	for _, f := range fs {
		m[f.Rule] = f.Severity
	}
	return m
}

func TestLint_CleanFile(t *testing.T) {
	fs := Lint([]*UCFile{mkUC()}, Options{})
	if len(Errors(fs)) != 0 {
		t.Fatalf("clean file produced errors: %v", fs)
	}
	for _, f := range fs {
		t.Logf("warn ok: %s", f) // none expected, but log any
	}
	if len(fs) != 0 {
		t.Fatalf("clean file produced findings: %v", fs)
	}
}

func TestLint_ErrorRules(t *testing.T) {
	cases := []struct {
		rule string
		mut  func(*UCFile)
	}{
		{"uc-id", func(f *UCFile) { f.Meta.ID = "NOT-AN-ID" }},
		{"kind", func(f *UCFile) { f.Meta.Kind = "usecase" }},
		{"title", func(f *UCFile) { f.Meta.Title = "" }},
		{"module", func(f *UCFile) { f.Meta.Module = "" }},
		{"severity", func(f *UCFile) { f.Meta.Severity = "major" }},
		{"status-cache", func(f *UCFile) { f.Meta.StatusCache = "green" }},
		{"ac-id", func(f *UCFile) { f.Meta.AcceptanceCriteria[0].ID = "AC1" }},
		{"ac-id-scope", func(f *UCFile) { f.Meta.AcceptanceCriteria[0].ID = "UC-099-AC1" }},
		{"duplicate-ac-id", func(f *UCFile) { f.Meta.AcceptanceCriteria[1].ID = "UC-014-AC1" }},
		{"claim", func(f *UCFile) { f.Meta.AcceptanceCriteria[0].Claim = "" }},
		{"ac-kind", func(f *UCFile) { f.Meta.AcceptanceCriteria[0].Kind = "negative" }},
		{"layer", func(f *UCFile) { f.Meta.AcceptanceCriteria[0].Layer = "L4" }},
		{"status", func(f *UCFile) { f.Meta.AcceptanceCriteria[0].Status = "green" }},
		{"probe", func(f *UCFile) {
			f.Meta.AcceptanceCriteria[0].TestRef = ""
			f.Meta.AcceptanceCriteria[0].Status = StatusPass // gap not declared
		}},
		{"test-ref-l1", func(f *UCFile) { f.Meta.AcceptanceCriteria[0].TestRef = "lowercase_test" }},
		{"test-ref-l2", func(f *UCFile) { f.Meta.AcceptanceCriteria[1].TestRef = "tests/L2/resume.yaml" }},
		{"deferred-without-od", func(f *UCFile) { f.Meta.AcceptanceCriteria[0].Status = StatusFailDeferred }},
	}
	for _, c := range cases {
		f := mkUC()
		c.mut(f)
		got := rules(Lint([]*UCFile{f}, Options{}))
		if got[c.rule] != SeverityError {
			t.Errorf("%s: want error finding, got %v", c.rule, got)
		}
	}
}

func TestLint_PhasedWarnRules(t *testing.T) {
	f := mkUC()
	f.Meta.AcceptanceCriteria = f.Meta.AcceptanceCriteria[:1] // drop the anti
	f.Meta.Description = "(generated — refine)"

	got := rules(Lint([]*UCFile{f}, Options{}))
	if got["anti-criterion"] != SeverityWarn || got["description"] != SeverityWarn {
		t.Fatalf("want warn for anti-criterion + description, got %v", got)
	}

	// Per-module flip: the same findings become errors.
	got = rules(Lint([]*UCFile{f}, Options{AntiRuleError: true}))
	if got["anti-criterion"] != SeverityError || got["description"] != SeverityError {
		t.Fatalf("AntiRuleError must promote to error, got %v", got)
	}
}

func TestLint_StatusCacheDriftAndNoACs(t *testing.T) {
	f := mkUC()
	f.Meta.StatusCache = StatusPass // worst-of is no-test (AC2)
	got := rules(Lint([]*UCFile{f}, Options{}))
	if got["status-cache-drift"] != SeverityWarn {
		t.Fatalf("want status-cache-drift warn, got %v", got)
	}

	empty := mkUC()
	empty.Meta.AcceptanceCriteria = nil
	got = rules(Lint([]*UCFile{empty}, Options{}))
	if got["no-acceptance-criteria"] != SeverityWarn {
		t.Fatalf("want no-acceptance-criteria warn, got %v", got)
	}
}

// A criterion no run has reached yet carries no status: that is no-test (an
// open gap), never "not in the 5-state set" — a new module must pass the
// verify pre-flight before its first run.
func TestLint_UnrecordedStatusIsNoTest(t *testing.T) {
	f := mkUC()
	for i := range f.Meta.AcceptanceCriteria {
		f.Meta.AcceptanceCriteria[i].Status = ""
	}
	if errs := Errors(Lint([]*UCFile{f}, Options{})); len(errs) != 0 {
		t.Fatalf("unrecorded statuses must lint clean, got %v", errs)
	}
	// Without a probe, an unrecorded criterion is an open gap too.
	f.Meta.AcceptanceCriteria[0].TestRef = ""
	if errs := Errors(Lint([]*UCFile{f}, Options{})); len(errs) != 0 {
		t.Fatalf("unrecorded, probe-less criterion must lint clean, got %v", errs)
	}
	if got := WorstOf(f.Meta.AcceptanceCriteria); got != StatusNoTest {
		t.Fatalf("WorstOf(unrecorded) = %q, want no-test", got)
	}
}

func TestLint_DuplicateUCID(t *testing.T) {
	a, b := mkUC(), mkUC()
	b.Path = "UC-014-copy.cwm.yaml"
	got := rules(Lint([]*UCFile{a, b}, Options{}))
	if got["duplicate-uc-id"] != SeverityError {
		t.Fatalf("want duplicate-uc-id, got %v", got)
	}
}

func TestLint_L2ExistenceWithModuleDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "tests", "L2"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "tests", "L2", "resume.tasktest"), []byte("uc: UC-014"), 0o644); err != nil {
		t.Fatal(err)
	}
	f := mkUC()
	if errs := Errors(Lint([]*UCFile{f}, Options{ModuleDir: dir})); len(errs) != 0 {
		t.Fatalf("existing scenario flagged: %v", errs)
	}
	f.Meta.AcceptanceCriteria[1].TestRef = "tests/L2/missing.tasktest"
	if errs := Errors(Lint([]*UCFile{f}, Options{ModuleDir: dir})); len(errs) != 1 || errs[0].Rule != "test-ref-l2" {
		t.Fatalf("missing scenario not flagged: %v", errs)
	}
}

func TestOrphanTests(t *testing.T) {
	fs := OrphanTests([]*UCFile{mkUC()},
		[]string{"TestResume_RestoresProcessedIDs", "TestUnmapped_Thing"})
	if len(fs) != 1 || !strings.Contains(fs[0].Msg, "TestUnmapped_Thing") {
		t.Fatalf("want 1 orphan for TestUnmapped_Thing, got %v", fs)
	}
}
