// SPDX-License-Identifier: Apache-2.0

package api_test

import (
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// WHERE IT STANDS - one function names an increment's
// standing and a release's stage; each case is one step of their lives.
func TestWhereAnIncrementAndAReleaseStand(t *testing.T) {
	held := &api.Hold{Reason: "waiting for the provider"}
	for _, c := range []struct {
		name string
		in   api.IncrementStandingInput
		want string
	}{
		{"defined: no person ran it", api.IncrementStandingInput{Increment: api.Increment{State: api.IncrementOpen}}, api.StandingDefined},
		{"running: its work is open", api.IncrementStandingInput{Increment: api.Increment{State: api.IncrementOpen, Running: true}, Done: 1, Total: 3}, api.StandingRunning},
		{"delivering: all it holds is done", api.IncrementStandingInput{Increment: api.Increment{State: api.IncrementOpen, Running: true}, Done: 3, Total: 3}, api.StandingDelivering},
		{"running when nothing is held yet", api.IncrementStandingInput{Increment: api.Increment{State: api.IncrementOpen, Running: true}}, api.StandingRunning},
		{"on hold wins over running", api.IncrementStandingInput{Increment: api.Increment{State: api.IncrementOpen, Running: true, Held: held}, Done: 3, Total: 3}, api.StandingOnHold},
		{"done, never running", api.IncrementStandingInput{Increment: api.Increment{State: api.IncrementDone, Running: true}, ReleaseState: api.ReleaseFrozen}, api.StandingDone},
		{"shipped: its release is released", api.IncrementStandingInput{Increment: api.Increment{State: api.IncrementDone, Running: true}, ReleaseState: api.ReleaseReleased}, api.StandingShipped},
		{"dropped", api.IncrementStandingInput{Increment: api.Increment{State: api.IncrementDropped, Running: true}}, api.StandingDropped},
	} {
		if got := api.IncrementStanding(c.in); got != c.want {
			t.Errorf("%s: %s; want %s", c.name, got, c.want)
		}
	}
	for _, c := range []struct {
		r    api.Release
		want string
	}{
		{api.Release{State: api.ReleasePlanned}, api.ReleasePlanned},
		{api.Release{State: api.ReleaseFrozen}, api.ReleaseFrozen},
		{api.Release{State: api.ReleaseFrozen, ReleaseFields: api.ReleaseFields{Candidates: []api.Candidate{{Name: "rc1"}}}}, api.StageCandidate},
		{api.Release{State: api.ReleaseReleased, ReleaseFields: api.ReleaseFields{Candidates: []api.Candidate{{Name: "rc1", Promoted: true}}}}, api.ReleaseReleased},
		{api.Release{State: api.ReleaseDropped}, api.ReleaseDropped},
	} {
		if got := api.ReleaseStage(c.r); got != c.want {
			t.Errorf("release %s with %d candidate(s): stage %s; want %s", c.r.State, len(c.r.Candidates), got, c.want)
		}
	}
	// Every standing and stage has its words in both languages.
	for _, s := range []string{api.StandingDefined, api.StandingRunning, api.StandingDelivering, api.StandingOnHold, api.StandingDone, api.StandingShipped, api.StandingDropped,
		api.ReleasePlanned, api.ReleaseFrozen, api.StageCandidate, api.ReleaseReleased} {
		en, color := api.StandingLabel(s, false)
		fr, _ := api.StandingLabel(s, true)
		if en == "" || fr == "" || color == "gray" && s != api.StandingDefined && s != api.StandingDropped {
			t.Errorf("%s: words %q / %q, colour %s", s, en, fr, color)
		}
	}
}

// A RELEASE IS NAMED ONCE - « 1.4.0 — Spring », and
// « 1.3.0 » alone when its name is its version, never « 1.3.0 — 1.3.0 ».
func TestAReleaseIsNamedOnce(t *testing.T) {
	for _, c := range []struct{ version, name, want string }{
		{"1.4.0", "Spring", "1.4.0 — Spring"},
		{"1.3.0", "1.3.0", "1.3.0"},
		{"", "Spring", "Spring"},
		{"1.3.0", "", "1.3.0"},
	} {
		if got := api.ReleaseLabel(c.version, c.name); got != c.want {
			t.Errorf("ReleaseLabel(%q, %q) = %q; want %q", c.version, c.name, got, c.want)
		}
	}
}
