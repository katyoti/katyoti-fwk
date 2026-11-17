// SPDX-License-Identifier: Apache-2.0

package core

import (
	"slices"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// A GOAL IS REWORDED ONLY WHEN ONE GOES AND ONE COMES:
// goals are matched by their words, never by position - a list collapsed to
// one goal reworks no operation of another goal; an applied operation keeps
// its goal, following it only through an unambiguous reword (the same goal in
// new words, so the pane keeps what it became).
func TestAGoalIsRewordedOnlyWhenOneGoesAndOneComes(t *testing.T) {
	paid, mailed, fast := "Customers pay on time", "Invoices reach them by mail", "Invoices leave the day they are made"
	before := []string{paid, mailed, fast}
	delta := []api.DeltaOp{
		{Op: api.OpFeatureAdd, Title: "Remind late payers", For: paid},
		{Op: api.OpFeatureAdd, Title: "Mail the invoices", For: mailed},
		{Op: api.OpFeatureAdd, Title: "Send at once", For: fast},
	}
	applied := []api.Applied{{Op: 0, ID: "shop.F-001"}, {Op: 1, ID: "shop.F-002"}}

	// Collapsed to one new goal: nothing is reworded, every gone goal removed.
	other := "A customer sees what they owe"
	renamed, changed, removed := goalChanges(before, []string{other}, before)
	if len(renamed) != 0 || !slices.Equal(changed, []string{other}) || !slices.Equal(removed, before) {
		t.Fatalf("collapsed list: renamed %v changed %v removed %v; want nothing renamed, the new goal added, the three removed", renamed, changed, removed)
	}
	d, a := regoal(delta, applied, renamed, append(append([]string{}, changed...), removed...))
	if len(d) != 2 || d[0].For != paid || d[1].For != mailed || len(a) != 2 {
		t.Fatalf("after a collapsed list: delta %+v applied %+v; want the two applied operations kept, each with its own goal", d, a)
	}

	// Two gone, two come: no pairing by position either.
	renamed, changed, removed = goalChanges(before, []string{"x", "y", fast}, nil)
	if len(renamed) != 0 || len(changed) != 2 || len(removed) != 2 {
		t.Fatalf("two for two: renamed %v changed %v removed %v; want two added and two removed", renamed, changed, removed)
	}

	// One gone, one come: a reword - the applied operation follows its goal.
	byPost := "Invoices reach them by post"
	renamed, changed, removed = goalChanges(before, []string{paid, byPost, fast}, nil)
	if renamed[mailed] != byPost || !slices.Equal(changed, []string{byPost}) || len(removed) != 0 {
		t.Fatalf("one for one: renamed %v changed %v removed %v; want %q reworded", renamed, changed, removed, mailed)
	}
	d, _ = regoal(delta, applied, renamed, changed)
	if len(d) != 3 || d[0].For != paid || d[1].For != byPost || d[2].For != fast {
		t.Fatalf("after a reword: %+v; want the applied operation following its goal's new words, the others untouched", d)
	}

	// The same one for one, the gone goal dropped: removed, then added.
	renamed, _, removed = goalChanges(before, []string{paid, byPost, fast}, []string{mailed})
	if len(renamed) != 0 || !slices.Equal(removed, []string{mailed}) {
		t.Fatalf("one for one dropped: renamed %v removed %v; want %q removed", renamed, removed, mailed)
	}
}

// A RUN INCREMENT'S GOAL IS DROPPED ONLY WHEN NAMED:
// a new list missing goals is refused unless the call names each in drop,
// the refusal naming what would drop; a drop naming a goal the change does
// not remove is refused; an unambiguous reword drops nothing.
func TestARunIncrementsGoalIsDroppedOnlyWhenNamed(t *testing.T) {
	a, b, c := "Customers pay on time", "Invoices reach them by mail", "Invoices leave the day they are made"
	before := []string{a, b, c}
	_, err := dropGuard("billing.IN-001", before, []string{"Something else"}, nil)
	if err == nil {
		t.Fatal("a list collapsed to one goal was accepted with no drop")
	}
	for _, g := range before {
		if !strings.Contains(err.Error(), g) {
			t.Errorf("the refusal %q does not name %q", err, g)
		}
	}
	if _, err := dropGuard("billing.IN-001", before, []string{a}, []string{b}); err == nil || !strings.Contains(err.Error(), c) || strings.Contains(err.Error(), `"`+b+`"`) {
		t.Errorf("one goal named of two dropped: %v; want refused naming %q alone", err, c)
	}
	dropped, err := dropGuard("billing.IN-001", before, []string{a}, []string{b, c})
	if err != nil || !slices.Equal(dropped, []string{b, c}) {
		t.Errorf("both named: dropped %v err %v; want both dropped", dropped, err)
	}
	if _, err := dropGuard("billing.IN-001", before, before, []string{"Not a goal"}); err == nil || !strings.Contains(err.Error(), "Not a goal") {
		t.Errorf("a drop naming no goal of the change: %v; want refused naming it", err)
	}
	if _, err := dropGuard("billing.IN-001", before, []string{a, b}, []string{a}); err == nil {
		t.Error("a drop naming a goal still in the list was accepted")
	}
	if dropped, err := dropGuard("billing.IN-001", before, []string{a, "Invoices reach them by post", c}, nil); err != nil || len(dropped) != 0 {
		t.Errorf("a reword: dropped %v err %v; want accepted, nothing dropped", dropped, err)
	}
}

// A ROUND LISTS ONLY THE GOALS ADDED OR REWORDED: the
// proposal planned again for changed goals names those and nothing else -
// neither the goals kept nor the goals removed.
func TestARoundListsOnlyTheGoalsAddedOrReworded(t *testing.T) {
	kept, gone1, gone2, added := "Customers pay on time", "Invoices reach them by mail", "Invoices leave at once", "A customer sees what they owe"
	_, changed, removed := goalChanges([]string{kept, gone1, gone2}, []string{kept, added}, []string{gone1, gone2})
	details := roundDetails(changed)
	if !strings.Contains(details, "- "+added+"\n") {
		t.Errorf("the round's details %q do not list the added goal", details)
	}
	for _, g := range append([]string{kept}, removed...) {
		if strings.Contains(details, g) {
			t.Errorf("the round's details %q list %q, neither added nor reworded", details, g)
		}
	}
	reworded := "Customers pay within thirty days"
	_, changed, _ = goalChanges([]string{kept, gone1}, []string{reworded, gone1}, nil)
	if details := roundDetails(changed); !strings.Contains(details, "- "+reworded+"\n") || strings.Contains(details, gone1) || strings.Contains(details, "- "+kept) {
		t.Errorf("the round's details after a reword %q; want the reworded goal alone", details)
	}
}

// TWO GOAL CHANGES BEFORE A ROUND IS WORKED LOSE NO GOAL:
// a proposal still open is planned again for each change of goals, which
// archives the round before it; the new round lists the goals the earlier one
// still waited to reconcile - in their new words, while still a goal - beside
// this change's, and the operations of the earlier round's goals are kept.
func TestTwoGoalChangesBeforeARoundIsWorkedLoseNoGoal(t *testing.T) {
	paid, mailed, fast, owed := "Customers pay on time", "Invoices reach them by mail", "Invoices leave the day they are made", "A customer sees what they owe"
	delivery := []string{paid, mailed, fast, owed}
	delta := []api.DeltaOp{
		{Op: api.OpFeatureAdd, Title: "Remind late payers", For: paid},
		{Op: api.OpFeatureAdd, Title: "Mail the invoices", For: mailed},
		{Op: api.OpFeatureAdd, Title: "Send at once", For: fast},
		{Op: api.OpFeatureAdd, Title: "Show the balance", For: owed},
	}
	applied := []api.Applied{{Op: 0, ID: "shop.F-001"}}
	details := ""
	// setGoals follows setDelivery on a proposal still open.
	setGoals := func(after, drop []string) {
		t.Helper()
		renamed, changed, removed := goalChanges(delivery, after, drop)
		delta, applied = regoal(delta, applied, renamed, append(append([]string{}, changed...), removed...))
		delivery = after
		details = roundDetails(roundGoals(details, renamed, delivery, changed))
	}

	paidSoon := "Customers pay within thirty days"
	setGoals([]string{paidSoon, mailed, fast, owed}, nil) // round 1: paid reworded
	byPost := "Invoices reach them by post"
	setGoals([]string{paidSoon, byPost, fast, owed}, nil) // round 2, round 1 never worked

	for _, g := range []string{paidSoon, byPost} {
		if !strings.Contains(details, "- "+g+"\n") {
			t.Errorf("the second round's details %q do not list %q", details, g)
		}
	}
	for _, g := range []string{paid, mailed, fast, owed} {
		if strings.Contains(details, "- "+g+"\n") {
			t.Errorf("the second round's details %q list %q, neither waiting nor changed", details, g)
		}
	}
	if len(delta) != 3 || delta[0].For != paidSoon || delta[1].For != fast || delta[2].For != owed || len(applied) != 1 || applied[0].Op != 0 {
		t.Fatalf("after two changes: delta %+v applied %+v; want the applied operation following the first round's goal, the others kept", delta, applied)
	}

	// The first round's goal reworded again, then removed: followed, then gone.
	paidNow := "Customers pay at once"
	setGoals([]string{paidNow, byPost, fast, owed}, nil)
	if !strings.Contains(details, "- "+paidNow+"\n") || strings.Contains(details, paidSoon) || !strings.Contains(details, "- "+byPost+"\n") {
		t.Errorf("after a second reword of a waiting goal: %q; want %q and %q listed, %q gone", details, paidNow, byPost, paidSoon)
	}
	if got := roundGoals(details, nil, []string{byPost, fast, owed}, nil); !slices.Equal(got, []string{byPost}) {
		t.Errorf("a waiting goal removed: the round keeps %v; want only %q", got, byPost)
	}
	if got := roundGoals("A demand's own words\n- not a round\n", nil, delivery, []string{owed}); !slices.Equal(got, []string{owed}) {
		t.Errorf("details that are no round of changed goals gave %v; want only this change's %q", got, owed)
	}
}
