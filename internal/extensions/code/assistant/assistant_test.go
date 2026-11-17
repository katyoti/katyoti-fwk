// SPDX-License-Identifier: Apache-2.0

package assistant

import "testing"

// scenario: 2 goals (G1 served, G2 unserved=GAP), 2 features (F-A serves G1,
// its criteria AC-1[pass] + AC-2[no verdict]; F-B serves nothing=WASTE, its
// criterion AC-3[needs-harness]), 1 orphan AC-4 under no feature.
func sample() Snapshot {
	return Snapshot{
		Goals: []Goal{
			{ID: "G1", Title: "served", CoD: CoD{BusinessValue: 8}, ServingUS: []string{"F-A"}},
			{ID: "G2", Title: "unserved", CoD: CoD{BusinessValue: 5, TimeCriticality: 2}, ServingUS: nil},
		},
		Features: []Feature{
			{ID: "F-A", Title: "A", CoD: CoD{BusinessValue: 8, TimeCriticality: 3, RiskOpportunity: 2}, Serves: []string{"G1"}, Criteria: []string{"AC-1", "AC-2"}},
			{ID: "F-B", Title: "B", CoD: CoD{BusinessValue: 4}, Serves: nil, Criteria: []string{"AC-3"}},
		},
		Criteria: []Criterion{
			{ID: "AC-1", Status: "pass", FeatureID: "F-A"},
			{ID: "AC-2", Status: "", FeatureID: "F-A"},
			{ID: "AC-3", Status: "needs-harness", FeatureID: "F-B"},
			{ID: "AC-4", Status: "", ParentID: "m"}, // orphan
		},
	}
}

func idxOf(got []Action) func(string) int {
	return func(kind string) int {
		for i, a := range got {
			if a.Kind == kind {
				return i
			}
		}
		return -1
	}
}

func TestFindings(t *testing.T) {
	got := Findings(sample())
	kinds := map[string]string{}
	for _, f := range got {
		kinds[f.Kind] = f.Subject
	}
	if kinds["GAP"] != "G2" {
		t.Errorf("GAP: got %q want G2", kinds["GAP"])
	}
	if kinds["WASTE"] != "F-B" {
		t.Errorf("WASTE: got %q want F-B", kinds["WASTE"])
	}
	if kinds["ORPHAN"] != "AC-4" {
		t.Errorf("ORPHAN: got %q want AC-4", kinds["ORPHAN"])
	}
}

func TestComputeOverview(t *testing.T) {
	ov := ComputeOverview(sample())
	if ov.USTotal != 2 || ov.USDelivered != 0 {
		t.Errorf("features: total=%d delivered=%d, want 2/0", ov.USTotal, ov.USDelivered)
	}
	if ov.CriteriaTotal != 4 || ov.CriteriaPassing != 1 || ov.CriteriaNoTest != 2 || ov.CriteriaUnproven != 1 || ov.CriteriaFailing != 0 {
		t.Errorf("criteria tally: %+v (want 4 total, 1 pass, 2 no-test, 1 unproven)", ov)
	}
	for _, gg := range ov.GoalGauges {
		if gg.ID == "G1" && (gg.ServingUS != 1 || gg.DeliveredUS != 0) {
			t.Errorf("G1 gauge serving=%d delivered=%d want 1/0", gg.ServingUS, gg.DeliveredUS)
		}
	}
}

// A feature is delivered when every criterion under it passes - those of its
// sub-features included - and never with no criterion at all.
func TestDelivered(t *testing.T) {
	st := map[string]string{"a": "pass", "b": "pass", "c": "fail"}
	if !Delivered(Feature{Criteria: []string{"a", "b"}}, st) {
		t.Error("all passing: not delivered")
	}
	if Delivered(Feature{Criteria: []string{"a", "c"}}, st) {
		t.Error("one failing: delivered")
	}
	if Delivered(Feature{}, st) {
		t.Error("no criterion: delivered")
	}
}

func TestNextActions_Ranking(t *testing.T) {
	got := NextActions(sample())
	idx := idxOf(got)
	if got[0].Kind != "shape-us" || got[0].Subject != "G2" {
		t.Fatalf("top action = %q/%q want shape-us/G2", got[0].Kind, got[0].Subject)
	}
	if idx("write-probe") < 0 || idx("retro-link") < 0 {
		t.Fatalf("missing actions: %+v", got)
	}
	for _, a := range got {
		if a.Kind == "write-probe" && a.Subject != "AC-2" {
			t.Errorf("write-probe on %q: only a feature's criterion without verdict is one", a.Subject)
		}
	}
	if !(idx("shape-us") < idx("write-probe") && idx("write-probe") < idx("review-waste") && idx("review-waste") < idx("retro-link")) {
		t.Errorf("ranking order wrong: %+v", got)
	}
}

// A feature's criterion whose probe ran RED yields a `build` action — concrete
// delivery work. It ranks below GAP, above no-test and plan.
func TestNextActions_BuildFailing(t *testing.T) {
	s := Snapshot{
		Goals:    []Goal{{ID: "G1", ServingUS: []string{"F-A"}}},
		Features: []Feature{{ID: "F-A", CoD: CoD{BusinessValue: 8}, Serves: []string{"G1"}, Criteria: []string{"AC-pass", "AC-fail", "AC-nt"}}},
		Criteria: []Criterion{
			{ID: "AC-pass", Status: "pass", FeatureID: "F-A"},
			{ID: "AC-fail", Status: "fail", FeatureID: "F-A"},
			{ID: "AC-nt", Status: "no-probe", FeatureID: "F-A"},
		},
	}
	got := NextActions(s)
	idx := idxOf(got)
	if idx("build") < 0 {
		t.Fatalf("no build action for the failing criterion: %+v", got)
	}
	if !(idx("build") < idx("write-probe")) {
		t.Errorf("build (%d) should rank above write-probe (%d)", idx("build"), idx("write-probe"))
	}
	if idx("plan") >= 0 && !(idx("build") < idx("plan")) {
		t.Errorf("build (%d) should rank above plan (%d)", idx("build"), idx("plan"))
	}
	for _, a := range got {
		if a.Kind == "build" && a.Subject != "AC-fail" {
			t.Errorf("build subject = %q want AC-fail", a.Subject)
		}
	}
}

func TestNextActions_AllGreen(t *testing.T) {
	s := Snapshot{
		Goals:    []Goal{{ID: "G1", ServingUS: []string{"F-A"}}},
		Features: []Feature{{ID: "F-A", Serves: []string{"G1"}, Criteria: []string{"AC-1"}}},
		Criteria: []Criterion{{ID: "AC-1", Status: "pass", FeatureID: "F-A"}},
	}
	got := NextActions(s)
	if len(got) != 1 || got[0].Kind != "capture-want" {
		t.Errorf("all-green: got %+v want single capture-want", got)
	}
}

// A feature states its cost of delay as one number (its `cod`); the three
// primitives are the other way to state it. Either ranks.
func TestCoDDeclaredOrPrimitives(t *testing.T) {
	if got := (CoD{Declared: 26}).Sum(); got != 26 {
		t.Errorf("declared CoD = %d, want 26", got)
	}
	if got := (CoD{BusinessValue: 13, TimeCriticality: 8, RiskOpportunity: 5}).Sum(); got != 26 {
		t.Errorf("primitives CoD = %d, want 26", got)
	}
}
