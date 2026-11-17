// SPDX-License-Identifier: Apache-2.0

package verify

import (
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/code/internal/projection"
	"github.com/katyoti/katyoti-fwk/internal/extensions/code/internal/ucfile"
)

// TestRenderTestPlan_ParserRoundTrip pins the projection's generated
// Test-plan table to verify's legacy parser (pin-every-wire-format rule):
// a generated charter section must parse back with every ref intact —
// the legacy verify path and uc-explode both read charters through
// ParseTestPlan, and a column-layout change silently shifts their cell
// offsets (this exact drift happened once: a Status column broke UC-150's
// L2 extraction).
func TestRenderTestPlan_ParserRoundTrip(t *testing.T) {
	files := []*ucfile.UCFile{
		{Meta: ucfile.UCMeta{
			ID: "UC-150", Kind: ucfile.KindUseCase, Title: "Simulator basic", Module: "tasks",
			Severity: "core", StatusCache: ucfile.StatusPass,
			AcceptanceCriteria: []ucfile.AC{
				{ID: "UC-150-AC1", Claim: "l1", Layer: "L1", TestRef: "TestSimulator_Basic", Status: ucfile.StatusPass},
				{ID: "UC-150-AC2", Claim: "l2", Kind: "anti", Layer: "L2", TestRef: "tests/L2/uc-150-simulator-basic.tasktest", Status: ucfile.StatusPass},
				{ID: "UC-150-AC3", Claim: "l3", Layer: "L3", TestRef: "self-hosted-regression", Status: ucfile.StatusSkipped},
			},
		}},
	}

	section := projection.RenderTestPlanSection("tasks", files)
	rows := ParseTestPlan(section)
	if len(rows) != 1 {
		t.Fatalf("want 1 parsed row, got %d\nsection:\n%s", len(rows), section)
	}
	row := rows[0]
	if len(row.IDs) != 1 || row.IDs[0] != "UC-150" {
		t.Fatalf("IDs = %v", row.IDs)
	}
	if len(row.L1Tests) != 1 || row.L1Tests[0] != "TestSimulator_Basic" {
		t.Fatalf("L1 refs lost in round-trip: %v", row.L1Tests)
	}
	if len(row.L2Scenarios) != 1 || row.L2Scenarios[0] != "tests/L2/uc-150-simulator-basic.tasktest" {
		t.Fatalf("L2 scenario lost in round-trip: %v", row.L2Scenarios)
	}
	if len(row.L3Scenarios) == 0 {
		t.Fatalf("L3 cell lost in round-trip: %+v", row)
	}
}
