// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// PLANIFICATION'S RESILIENCE MEASURED - `planning check
// --measure`: a stall the reconciler found, unblocked after a known time, is
// counted and its time to unblock reported; an unclassified wait is counted
// with its age; what no store records yet is named, never guessed.
func TestTheCheckMeasuresAStallsTimeToUnblockAndTheCounts(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := worldConfigured(t, nil, reconcilerOn())
	uc := want(t, ctx, cwm, "use-case", "Mail the invoices")
	if _, err := req.File(ctx, api.FileInput{On: uc, Kind: api.KindBuild, By: person}); err != nil {
		t.Fatal(err)
	}
	if err := move(ctx, cwm, uc, api.StateDone, person); err != nil {
		t.Fatal(err)
	}
	journaledDone(t, ctx, uc)
	key := api.RepairEndPlan + ":" + uc
	// The stall: seen by a sweep, cleared, closed by two clean sweeps.
	if r := openOn(t, reconcile(t, ctx, api.ReconcileInput{}).Open, key); r == nil {
		t.Fatalf("no incident %s raised", key)
	}
	removeAll(t, ctx, uc)
	reconcile(t, ctx, api.ReconcileInput{})
	reconcile(t, ctx, api.ReconcileInput{})

	// Its record says it was raised 90 minutes ago and closed 30 minutes ago:
	// one hour to unblock.
	root := projectRootOf(t, ctx, cwm, uc)
	list := filepath.Join(application.EnvOf(root).HomeDir(), "planification", "incidents.json")
	now := time.Now().UTC()
	since, closed := now.Add(-90*time.Minute), now.Add(-30*time.Minute)
	setIncident(t, list, key, since, closed)

	// An unclassified wait on another piece of work.
	stuck := file(t, ctx, req, api.FileInput{Title: "Archive the invoices", Module: "billing"})
	reportBlocked(t, ctx, stepOf(t, ctx, stuck.ID, api.StepIntake).ID, "something is off", nil)

	if out := check(t, ctx, api.PlanningCheckInput{Census: true}); out.Measures != nil {
		t.Errorf("measures without --measure: %+v", out.Measures)
	}
	m := check(t, ctx, api.PlanningCheckInput{Census: true, Measure: true}).Measures
	if m == nil {
		t.Fatal("no measures with --measure")
	}
	if m.Days != api.DefaultMeasureDays {
		t.Errorf("window %d days; want %d", m.Days, api.DefaultMeasureDays)
	}
	if m.Stalls != 1 || m.StallsOpen != 0 || m.IncidentsPerDay[since.Format(time.DateOnly)] != 1 {
		t.Errorf("stalls %d, open %d, a day %v; want one stall raised %s, none open", m.Stalls, m.StallsOpen, m.IncidentsPerDay, since.Format(time.DateOnly))
	}
	if got := m.UnblockSystem; got.Count != 1 || got.MedianS != 3600 || got.MaxS != 3600 {
		t.Errorf("time to unblock, system causes: %+v; want one of 3600 s", got)
	}
	if m.Unclassified != 1 || m.UnclassifiedOver1 != 0 {
		t.Errorf("unclassified waits %d (over a day %d); want 1 (0)", m.Unclassified, m.UnclassifiedOver1)
	}
	if m.SpendUSD != nil || m.Unmeasured["spend"] == "" || m.Unmeasured["hub_starts"] == "" {
		t.Errorf("spend %v, unmeasured %v; want the spend and the hub's starts named as unmeasured", m.SpendUSD, m.Unmeasured)
	}

	// A window that ends before the stall was raised counts nothing of it -
	// the window is read, not ignored.
	setIncident(t, list, key, now.Add(-72*time.Hour), now.Add(-71*time.Hour))
	m = check(t, ctx, api.PlanningCheckInput{Census: true, Measure: true, Days: 1}).Measures
	if m.Stalls != 0 || m.UnblockSystem.Count != 0 {
		t.Errorf("a one-day window over a stall of three days ago: stalls %d, unblocked %+v; want none", m.Stalls, m.UnblockSystem)
	}
}

// setIncident rewrites one record's since and closed in the incidents' list.
func setIncident(t *testing.T, list, key string, since, closed time.Time) {
	t.Helper()
	raw, err := os.ReadFile(list)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, r := range doc["incidents"].([]any) {
		rec := r.(map[string]any)
		if rec["key"] == key {
			rec["since"], rec["closed"] = since.Format(time.RFC3339), closed.Format(time.RFC3339)
			found = true
		}
	}
	if !found {
		t.Fatalf("%s not in %s:\n%s", key, list, raw)
	}
	out, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(list, out, 0o644); err != nil {
		t.Fatal(err)
	}
}
