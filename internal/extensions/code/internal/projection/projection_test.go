// SPDX-License-Identifier: Apache-2.0

package projection

import (
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/code/internal/ucfile"
)

func fixtureFiles() []*ucfile.UCFile {
	return []*ucfile.UCFile{
		{Meta: ucfile.UCMeta{
			ID: "UC-002", Kind: ucfile.KindUseCase, Title: "Resume", Module: "m",
			Severity: "core", StatusCache: ucfile.StatusPass,
			LastVerifiedAt: "2026-06-11T16:37:06Z",
			Realizes:       []string{"us-resume"},
			Description:    "Resume contract.",
			AcceptanceCriteria: []ucfile.AC{
				{ID: "UC-002-AC1", Claim: "restores", Layer: "L1", TestRef: "TestResume", Status: ucfile.StatusPass},
				{ID: "UC-002-AC2", Claim: "no silent restart", Kind: "anti", Layer: "L1", TestRef: "TestResumeMissing", Status: ucfile.StatusPass},
			},
		}},
		{Meta: ucfile.UCMeta{
			ID: "UC-001", Kind: ucfile.KindUseCase, Title: "Run", Module: "m",
			Severity: "core", StatusCache: ucfile.StatusNoTest,
			AcceptanceCriteria: []ucfile.AC{
				{ID: "UC-001-AC1", Claim: "runs", Layer: "L1", TestRef: "TestRun", Status: ucfile.StatusPass},
				{ID: "UC-001-AC2", Claim: "gap", Kind: "anti", Layer: "L2", TestRef: "", Status: ucfile.StatusNoTest},
				{ID: "UC-001-AC3", Claim: "l3", Layer: "L3", TestRef: "scenario", Status: ucfile.StatusSkipped},
			},
		}},
		{Meta: ucfile.UCMeta{
			ID: "UC-073", Kind: ucfile.KindUseCase, Title: "Deferred", Module: "m",
			Severity: "qol", StatusCache: ucfile.StatusFailDeferred, FailDeferredBy: "OD-004",
			AcceptanceCriteria: []ucfile.AC{
				{ID: "UC-073-AC1", Claim: "deferred", Layer: "L1", TestRef: "", Status: ucfile.StatusFailDeferred},
			},
		}},
	}
}

func TestRenderTestPlanSection(t *testing.T) {
	out := RenderTestPlanSection("m", fixtureFiles())
	if !strings.HasPrefix(out, "## Test plan\n") {
		t.Fatal("missing heading")
	}
	// Sorted by UC number: UC-001 row before UC-002.
	if strings.Index(out, "| UC-001 |") > strings.Index(out, "| UC-002 |") {
		t.Fatal("rows not sorted by UC number")
	}
	for _, want := range []string{
		"| UC-001 | `TestRun` 🟢 | (no probe) ⚪ [anti] | `scenario` ⏸ |",
		"| UC-002 | `TestResume` 🟢 · `TestResumeMissing` 🟢 [anti] | — | — |",
		"| UC-073 | (no probe) 🟠 |",
		"GENERATED",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	// Deterministic.
	if out != RenderTestPlanSection("m", fixtureFiles()) {
		t.Fatal("render not byte-stable")
	}
}

func TestComputeCoverage(t *testing.T) {
	c := ComputeCoverage(fixtureFiles())
	if c.UCTotal != 3 || c.ACTotal != 6 || c.AntiTotal != 2 {
		t.Fatalf("totals wrong: %+v", c)
	}
	if c.UCByStatus[ucfile.StatusPass] != 1 || c.UCByStatus[ucfile.StatusNoTest] != 1 || c.UCByStatus[ucfile.StatusFailDeferred] != 1 {
		t.Fatalf("uc_by_status wrong: %v", c.UCByStatus)
	}
	if c.ACByLayer["L1"] != 4 || c.ACByLayer["L2"] != 1 || c.ACByLayer["L3"] != 1 {
		t.Fatalf("ac_by_layer wrong: %v", c.ACByLayer)
	}
	// Denominator: 6 ACs − 1 fail-deferred − 1 skipped = 4; pass = 3 → 75%.
	if c.CoveragePercent != 75.0 {
		t.Fatalf("coverage = %v, want 75", c.CoveragePercent)
	}
	if len(c.DeferredUCIDs) != 1 || c.DeferredUCIDs[0] != "UC-073" {
		t.Fatalf("deferred ids wrong: %v", c.DeferredUCIDs)
	}
	r := c.Realizes["us-resume"]
	if r == nil || r.UCs != 1 || r.ACPass != 2 || r.ACTotal != 2 {
		t.Fatalf("realizes rollup wrong: %+v", r)
	}
	if c.LastVerifiedAt != "2026-06-11T16:37:06Z" {
		t.Fatalf("last_verified_at = %s", c.LastVerifiedAt)
	}
}

func TestRenderDocsUseCases(t *testing.T) {
	out := RenderDocsUseCases("m", "Tasks", fixtureFiles())
	for _, want := range []string{
		"# Tasks — Use Cases",
		"GENERATED",
		"### UC-002. Resume 🟢",
		"**Deferred by**: OD-004",
		"**Realizes**: us-resume",
		"*(anti)*",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q", want)
		}
	}
	if out != RenderDocsUseCases("m", "Tasks", fixtureFiles()) {
		t.Fatal("docs render not byte-stable")
	}
}
