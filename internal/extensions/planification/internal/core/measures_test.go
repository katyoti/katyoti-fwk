// SPDX-License-Identifier: Apache-2.0

package core

import (
	"strings"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// A person's step waits from the moment it can be acted on - made, and what
// it came after ended - to its report; work outside the scope is not counted;
// what waits on the person now is counted with its age.
func TestMeasureTimesThePersonsStepsFromWhenTheyCouldBeActedOn(t *testing.T) {
	now := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
	at := func(d time.Duration) string { return now.Add(-d).Format(time.RFC3339) }
	done := []schedapi.Todo{
		{ID: "a", Request: "billing.RQ-001", For: schedapi.ForAgent, Made: at(10 * time.Hour), Result: &schedapi.Result{At: at(5 * time.Hour)}},
		// Made 10 h ago, after a that ended 5 h ago, reported 3 h ago: 2 h.
		{ID: "p", Request: "billing.RQ-001", For: schedapi.ForPerson, Made: at(10 * time.Hour), After: []string{"a"}, Result: &schedapi.Result{At: at(3 * time.Hour)}},
		// Made 4 h ago with nothing before it, reported 0 h ago: 4 h.
		{ID: "q", Request: "billing.RQ-002", For: schedapi.ForPerson, Made: at(4 * time.Hour), Result: &schedapi.Result{At: at(0)}},
		// Outside the scope.
		{ID: "x", Request: "other.RQ-001", For: schedapi.ForPerson, Made: at(9 * time.Hour), Result: &schedapi.Result{At: at(1 * time.Hour)}},
		// Reported before the window.
		{ID: "old", Request: "billing.RQ-003", For: schedapi.ForPerson, Made: at(30 * 24 * time.Hour), Result: &schedapi.Result{At: at(20 * 24 * time.Hour)}},
	}
	inScope := func(work string, _ []string) bool { return work != "other.RQ-001" }
	blockers := []api.Blocker{
		{Kind: api.BlockerAct, ActorKind: api.ActorPerson, Since: at(6 * time.Hour), Reason: "validate"},
		{Kind: api.BlockerIncident, ActorKind: api.ActorAgent, Since: at(30 * time.Hour), Reason: unclassifiedWait + "something"},
		{Kind: api.BlockerReady, ActorKind: api.ActorAgent, Since: at(time.Hour), Reason: "ready"},
	}
	m := measure(now, 0, inScope, nil, done, blockers)
	if got := m.UnblockPerson; got.Count != 2 || got.MedianS != 3*3600 || got.MaxS != 4*3600 {
		t.Errorf("time to unblock, person causes: %+v; want 2 waits, median 3 h, longest 4 h", got)
	}
	if got := m.PersonWaiting; got.Count != 1 || got.MaxS != 6*3600 {
		t.Errorf("waiting on the person now: %+v; want one of 6 h", got)
	}
	if m.Unclassified != 1 || m.UnclassifiedOver1 != 1 || m.UnclassifiedMaxS != 30*3600 {
		t.Errorf("unclassified %d, over a day %d, oldest %d s; want 1, 1, %d", m.Unclassified, m.UnclassifiedOver1, m.UnclassifiedMaxS, 30*3600)
	}
}

// What the check cannot measure yet is named with the feature that will
// measure it, never left in F-092's done: the hub's
// starts under, the spend under RQ-328's sub-feature.
func TestMeasureNamesTheFeatureEachUnmeasuredMeasureWaitsOn(t *testing.T) {
	m := measure(time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC), 0, func(string, []string) bool { return true }, nil, nil, nil)
	if got := m.Unmeasured["hub_starts"]; !strings.Contains(got, "a feature of its own under the measures feature") || !strings.Contains(got, "waiting on the hub's record of its starts") {
		t.Errorf("unmeasured hub_starts %q; want it to name its feature and what it waits on", got)
	}
	if got := m.Unmeasured["spend"]; !strings.Contains(got, "a feature of its own under the measures feature") {
		t.Errorf("unmeasured spend %q; want it to name its feature", got)
	}
}
