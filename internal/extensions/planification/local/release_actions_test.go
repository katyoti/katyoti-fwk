// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// A RELEASE IS ACTED ON FROM ITS PAGE (feature): each act
// offered only in the stage where it applies, run as the person - planned, it
// is frozen; frozen, a candidate is delivered; on the candidate chosen on the
// page (the last by default, its candidates offered) its letter is written,
// then it is signed off; promoting it is offered only once every gate passes.
// Ended, nothing is offered. The page names no command to type.
func TestAReleaseIsActedOnFromItsPage(t *testing.T) {
	t.Parallel()
	ctx, _, _ := world(t)
	sm, rel := streams(t, ctx), releases(t, ctx)
	st, err := sm.Create(ctx, api.CreateStreamInput{Module: "billing", Name: "Product", StreamFields: api.StreamFields{VersionScheme: api.SchemeSemver, ReleaseMode: api.ModeManual}})
	if err != nil {
		t.Fatal(err)
	}
	r := plan(t, ctx, rel, api.PlanInput{Module: "billing", Version: "1.1.0", Stream: st.Stream.ID})
	page := func() string {
		t.Helper()
		def, err := releasePane(ctx, rel, r.ID)
		if err != nil {
			t.Fatal(err)
		}
		if err := def.Validate(); err != nil {
			t.Fatalf("the release's pane must display: %v", err)
		}
		b, _ := json.Marshal(def.DSLTree)
		return string(b)
	}
	offers := func(stage, raw string, want, not []string) {
		t.Helper()
		for _, w := range want {
			if !strings.Contains(raw, w) {
				t.Errorf("%s: the page does not offer %s", stage, w)
			}
		}
		for _, n := range not {
			if strings.Contains(raw, n) {
				t.Errorf("%s: the page offers %s", stage, n)
			}
		}
	}
	freeze, deliver := `"uri":"ktw-agents:releases/freeze"`, `"uri":"ktw-agents:releases/deliver"`
	letter, signOff, promote := `"uri":"ktw-agents:releases/letter"`, `"uri":"ktw-agents:releases/sign-off"`, `"uri":"ktw-agents:releases/promote"`

	raw := page()
	offers("planned", raw, []string{freeze, `"by":"${person}"`, "Act on it"}, []string{deliver, letter, signOff, promote})
	for _, cli := range []string{"(releases freeze)", "(releases deliver)", "(releases promote)"} {
		if strings.Contains(raw, cli) {
			t.Errorf("the page tells the person to type %s", cli)
		}
	}

	if _, err := rel.Freeze(ctx, api.MoveInput{ID: r.ID, By: person}); err != nil {
		t.Fatal(err)
	}
	offers("frozen, no candidate", page(), []string{deliver}, []string{freeze, letter, signOff, promote})

	if _, err := rel.Deliver(ctx, api.DeliverInput{ID: r.ID, Build: "b-1", By: person}); err != nil {
		t.Fatal(err)
	}
	got, err := rel.Get(ctx, api.GetInput{ID: r.ID})
	if err != nil || len(got.Release.Candidates) != 1 {
		t.Fatalf("the delivered candidate: %v %+v", err, got.Release.Candidates)
	}
	raw = page()
	if got.Release.Candidates[0].Letter == "" {
		offers("a candidate without its letter", raw, []string{deliver, letter, `"enum":["rc1"]`, `"candidate":"rc1"`}, []string{freeze, signOff, promote})
		if _, err := rel.Letter(ctx, api.CandidateInput{ID: r.ID, Candidate: "rc1", By: person}); err != nil {
			t.Fatal(err)
		}
		raw = page()
	}
	// Its letter written (at its delivery or by the act): a person signs it
	// off; its gates not passed (no end-to-end test), promoting is not offered.
	offers("a candidate with its letter, gates not passed", raw, []string{deliver, signOff, `"enum":["rc1"]`, `"candidate":"rc1"`}, []string{freeze, letter, promote})

	if _, err := rel.SignOff(ctx, api.CandidateInput{ID: r.ID, Candidate: "rc1", By: person}); err != nil {
		t.Fatal(err)
	}
	offers("signed off, gates not passed", page(), []string{deliver}, []string{freeze, letter, signOff, promote})

	// The end-to-end test green on its build: every gate passes - promote it.
	if _, err := rel.RecordGate(ctx, api.RecordGateInput{ID: r.ID, Candidate: "rc1", Gate: api.GateEndToEnd, Passed: true, Why: "clean machine run green", By: person}); err != nil {
		t.Fatal(err)
	}
	offers("every gate passed", page(), []string{promote, `"candidate":"rc1"`}, []string{freeze, letter, signOff})

	// Ended, nothing is offered: a dropped release.
	gone := plan(t, ctx, rel, api.PlanInput{Module: "billing", Version: "1.2.0", Stream: st.Stream.ID})
	if _, err := rel.Drop(ctx, api.MoveInput{ID: gone.ID, By: person}); err != nil {
		t.Fatal(err)
	}
	r = gone
	offers("dropped", page(), nil, []string{"Act on it", freeze, deliver, letter, signOff, promote})
}
