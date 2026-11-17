// SPDX-License-Identifier: Apache-2.0

package core

import (
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// A release's days left are calendar days: one the day before, none on the
// day, negative once late.
func TestDaysLeftAreCalendarDays(t *testing.T) {
	now := time.Date(2026, 10, 30, 23, 30, 0, 0, time.Local)
	for date, want := range map[string]int{"2026-10-31": 1, "2026-10-30": 0, "2026-10-28": -2, "2026-11-30": 31} {
		if got, ok := calendarDaysLeft(date, now); !ok || got != want {
			t.Errorf("days left to %s on 2026-10-30: %d, %v; want %d", date, got, ok, want)
		}
	}
	if _, ok := calendarDaysLeft("soon", now); ok {
		t.Error("a date that does not read gives days left")
	}
}

// A release's stepper marks the stages before its own done and its own
// current; a dropped release is dropped alone.
func TestAReleaseStepperFollowsItsStage(t *testing.T) {
	s := stepperOf(api.StageCandidate)
	if len(s) != 4 || s[1].State != "done" || s[2].Name != api.StageCandidate || s[2].State != "current" || s[3].State != "todo" {
		t.Errorf("a candidate's stepper: %+v", s)
	}
	if d := stepperOf(api.ReleaseDropped); len(d) != 1 || d[0].Name != api.ReleaseDropped || d[0].State != "current" {
		t.Errorf("a dropped release's stepper: %+v; want dropped alone", d)
	}
}

// The release level's remaining words in French
// « 0 jour restant » singular for zero, « Livrer une version candidate », the
// gate reasons, Change release's option modes and what an increment waits
// on.
func TestTheReleaseLevelsLastWordsAreFrench(t *testing.T) {
	frozen := api.Release{State: api.ReleaseFrozen}
	for got, want := range map[string]string{
		deliveryCountf("fr", 0, "days_left"):            " · 0 jour restant",
		deliveryCountf("fr", 1, "days_left"):            " · 1 jour restant",
		deliveryCountf("fr", 2, "days_left"):            " · 2 jours restants",
		deliveryCountf("en", 0, "days_left"):            " · 0 days left",
		gateWhy("fr", "needs a candidate"):              "attend une version candidate",
		gateWhy("fr", "no candidate delivered yet"):     "aucune version candidate livrée",
		gateWhy("fr", "open: Invoices, Reminders"):      "ouverts : Invoices, Reminders",
		gateWhy("fr", "not run on 1.1.0-rc1"):           "non lancé sur 1.1.0-rc1",
		gateWhy("fr", "no person signed 1.1.0-rc1 off"): "personne n'a validé 1.1.0-rc1",
		gateWhy("en", "open: Invoices"):                 "open: Invoices",
		deliveryWord("fr", "mode_planned-for"):          "prévu pour",
		deliveryWord("fr", "mode_ships-in"):             "livré dans",
		waitWord("fr", "on_hold") + "legal":             "suspendu : legal",
		waitWord("fr", "waits_on") + "Reminders":        "attend Reminders",
	} {
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	}
	if _, label := nextStep("fr", frozen, nil); label != "Livrer une version candidate" {
		t.Errorf("a frozen release's next step in French: %q", label)
	}

}
