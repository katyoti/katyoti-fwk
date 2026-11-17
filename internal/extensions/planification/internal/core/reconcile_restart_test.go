// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// : a restart changes nothing a sweep does - what the
// reconciler keeps (the repairs applied to each element, the day's puts and
// its own dry-run) goes into the incidents' list and a fresh state reads it
// back: past the day's limit, the same day, it stays in dry-run; the next
// day it starts afresh.

// putFindings are n findings each putting one to-do, on distinct elements.
func putFindings(n int) []finding {
	out := make([]finding, n)
	for i := range out {
		work := fmt.Sprintf("ops.RQ-9%02d", i)
		out[i] = finding{
			repair: api.Repair{Repair: api.RepairPutStep, Work: work, Reason: "a step is missing"},
			key:    api.RepairPutStep + ":" + work,
			still:  func(context.Context) bool { return true },
			apply:  func(context.Context) (int, error) { return 1, nil },
		}
	}
	return out
}

func TestARestartKeepsTheDaysPutsAndDryRun(t *testing.T) {
	cfg := api.ReconcilerConfig{Repairs: []string{api.RepairPutStep}}
	in := api.ReconcileInput{Apply: true}
	const today, tomorrow = "2026-11-05", "2026-11-06"

	// The process that ran first: past the day's limit, it stopped itself.
	first := &reconState{applied: map[string]int{}}
	first.newDay(today)
	first.settle(context.Background(), in, cfg, putFindings(putLimit+2), nil)
	if !strings.HasPrefix(first.stopped, "the day's limit") {
		t.Fatalf("past %d puts: stopped %q; want the day's limit", putLimit, first.stopped)
	}
	var doc incidentsDoc
	first.keep(&doc)

	// Restarted the same day: a fresh state reads the list back and stays in
	// dry-run, its counts as they were.
	again := &reconState{applied: map[string]int{}}
	again.restore(doc, today)
	again.newDay(today)
	out := again.settle(context.Background(), in, cfg, putFindings(1), nil)
	if !out.DryRun || !strings.Contains(out.Why, "day's limit") || out.Repairs[0].Applied {
		t.Fatalf("restarted the same day: dry-run %v (%q), applied %v; want dry-run for the day's limit", out.DryRun, out.Why, out.Repairs[0].Applied)
	}
	if again.puts != first.puts || again.applied[api.RepairPutStep+":ops.RQ-900"] != 1 {
		t.Errorf("restarted: puts %d (was %d), attempts %v; want them read back", again.puts, first.puts, again.applied)
	}

	// Restarted the next day: the day's count starts afresh, attempts kept.
	next := &reconState{applied: map[string]int{}}
	next.restore(doc, tomorrow)
	next.newDay(tomorrow)
	out = next.settle(context.Background(), in, cfg, putFindings(1), nil)
	if out.DryRun || !out.Repairs[0].Applied || next.puts != 1 {
		t.Errorf("restarted the next day: dry-run %v (%q), applied %v, puts %d; want applied, one put", out.DryRun, out.Why, out.Repairs[0].Applied, next.puts)
	}
	if next.applied[api.RepairPutStep+":ops.RQ-900"] != 2 {
		t.Errorf("restarted the next day: attempts %v; want the kept one plus today's", next.applied)
	}
}

// : the flapping limit holds across a restart - an element
// repaired up to flapLimit times before it is reported flapping by the first
// sweep of a fresh state, nothing applied.
func TestARestartKeepsAnElementFlapping(t *testing.T) {
	cfg := api.ReconcilerConfig{Repairs: []string{api.RepairPutStep}}
	in := api.ReconcileInput{Apply: true}
	const today = "2026-11-06"
	key := api.RepairPutStep + ":ops.RQ-900"

	// The process that ran first repaired the same element up to the limit.
	first := &reconState{applied: map[string]int{}}
	first.newDay(today)
	for i := 0; i < flapLimit; i++ {
		out := first.settle(context.Background(), in, cfg, putFindings(1), nil)
		if !out.Repairs[0].Applied {
			t.Fatalf("repair %d before the restart: skipped %q; want applied", i+1, out.Repairs[0].Skipped)
		}
	}
	var doc incidentsDoc
	first.keep(&doc)

	// Restarted: the first sweep after it reports the element flapping.
	again := &reconState{applied: map[string]int{}}
	again.restore(doc, today)
	again.newDay(today)
	out := again.settle(context.Background(), in, cfg, putFindings(1), nil)
	if out.Repairs[0].Applied || !strings.HasPrefix(out.Repairs[0].Skipped, "flapping") {
		t.Fatalf("first sweep after the restart: applied %v, skipped %q; want flapping, nothing applied", out.Repairs[0].Applied, out.Repairs[0].Skipped)
	}
	flapping := false
	for _, inc := range out.Incidents {
		if inc.Check == api.IncidentFlapping && inc.Key == api.IncidentFlapping+":"+key {
			flapping = true
		}
	}
	if !flapping {
		t.Errorf("first sweep after the restart: incidents %v; want a flapping incident on %s", out.Incidents, key)
	}
	if again.applied[key] != flapLimit {
		t.Errorf("after the restart: attempts %d; want the kept %d, none added", again.applied[key], flapLimit)
	}
}
