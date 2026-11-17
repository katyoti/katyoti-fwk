// SPDX-License-Identifier: Apache-2.0

package core

import (
	"testing"
	"time"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// The Done tile counts today, the current week (from Monday) or the current
// month: a request ended yesterday, a Wednesday, is in the week and the month,
// not the day; one ended the month before is in none.
func TestDoneCountsThePeriodAsked(t *testing.T) {
	now := time.Date(2026, 10, 8, 15, 0, 0, 0, time.UTC) // a Thursday
	ended := func(id, at string) api.PlannedObject {
		return api.PlannedObject{ID: id, PlanFields: api.PlanFields{State: api.PlanDone, Trail: []cwmapi.LifecycleEntry{{At: at}}}}
	}
	plans := []api.PlannedObject{
		ended("today", "2026-10-08T09:00:00Z"),
		ended("yesterday", "2026-10-07T09:00:00Z"),
		ended("last-week", "2026-10-02T09:00:00Z"),
		ended("last-month", "2026-09-30T09:00:00Z"),
	}
	for period, want := range map[string]int{api.PeriodDay: 1, api.PeriodWeek: 2, api.PeriodMonth: 3} {
		if got := len(doneIn(plans, period, now)); got != want {
			t.Errorf("%s: %d done; want %d", period, got, want)
		}
	}
	if !periodStart("year", now).IsZero() {
		t.Error("an unknown period has a start")
	}
}
