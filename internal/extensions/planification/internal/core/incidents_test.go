// SPDX-License-Identifier: Apache-2.0

package core

import (
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// The incidents' list: the same incident seen by two
// sweeps is one record; it closes after two clean sweeps in a row; raised
// again after it closed, it is opened anew; an old closed one is dropped.
func TestTheIncidentsListDeduplicatesAndClosesAfterTwoCleanSweeps(t *testing.T) {
	t0 := time.Date(2026, 10, 24, 10, 0, 0, 0, time.UTC)
	stall := api.Incident{Key: "check:billing.RQ-001", Check: "check", Work: "billing.RQ-001", Reason: "a cycle"}
	other := api.Incident{Key: "end-plan:billing.F-001", Check: "end-plan", Work: "billing.F-001", Reason: "open steps"}

	list := foldIncidents(nil, []api.Incident{stall, stall}, t0)
	list = foldIncidents(list, []api.Incident{stall, other}, t0.Add(10*time.Minute))
	if len(list) != 2 {
		t.Fatalf("two sweeps of one stall and one other: %d records %+v; want 2", len(list), list)
	}
	r := recordOf(t, list, stall.Key)
	if r.Sweeps != 2 || r.Since != t0.Format(time.RFC3339) || r.Closed != "" {
		t.Errorf("the stall seen twice: %+v; want one open record, 2 sweeps, since the first", r)
	}

	list = foldIncidents(list, []api.Incident{other}, t0.Add(20*time.Minute))
	if r := recordOf(t, list, stall.Key); r.Clean != 1 || r.Closed != "" {
		t.Errorf("after one clean sweep: %+v; want still open, clean 1", r)
	}
	list = foldIncidents(list, []api.Incident{other}, t0.Add(30*time.Minute))
	if r := recordOf(t, list, stall.Key); r.Closed == "" {
		t.Errorf("after two clean sweeps: %+v; want closed", r)
	}
	if got := openIncidents(list); len(got) != 1 || got[0].Key != other.Key {
		t.Errorf("open: %+v; want only %s", got, other.Key)
	}
	if r := recordOf(t, list, other.Key); r.Sweeps != 3 || r.Clean != 0 {
		t.Errorf("the other, seen by every sweep: %+v", r)
	}

	// A clean sweep between two sightings starts the count again.
	list = foldIncidents(list, nil, t0.Add(40*time.Minute))
	list = foldIncidents(list, []api.Incident{other}, t0.Add(50*time.Minute))
	list = foldIncidents(list, nil, t0.Add(60*time.Minute))
	if r := recordOf(t, list, other.Key); r.Closed != "" || r.Clean != 1 {
		t.Errorf("clean, seen, clean: %+v; want open, clean 1", r)
	}

	// Raised again after it closed: a new record.
	list = foldIncidents(list, []api.Incident{stall}, t0.Add(70*time.Minute))
	if r := recordOf(t, list, stall.Key); r.Closed != "" || r.Sweeps != 1 || r.Since != t0.Add(70*time.Minute).Format(time.RFC3339) {
		t.Errorf("raised again: %+v; want opened anew", r)
	}

	// A closed record a week old is dropped.
	list = foldIncidents(list, nil, t0.Add(80*time.Minute))
	list = foldIncidents(list, nil, t0.Add(90*time.Minute))
	list = foldIncidents(list, nil, t0.Add(8*24*time.Hour))
	if len(list) != 0 {
		t.Errorf("a week later: %+v; want the closed ones dropped", list)
	}
}

func recordOf(t *testing.T, list []api.IncidentRecord, key string) api.IncidentRecord {
	t.Helper()
	n, out := 0, api.IncidentRecord{}
	for _, r := range list {
		if r.Key == key {
			n++
			out = r
		}
	}
	if n != 1 {
		t.Fatalf("%d records of %s in %+v; want one", n, key, list)
	}
	return out
}
