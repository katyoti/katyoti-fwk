// SPDX-License-Identifier: Apache-2.0

package api

// WHERE IT STANDS — one place names where an increment and a release stand
// : every view reads these functions, never its own
// switch over the states. Until the workflow engine carries these stages as
// lifecycles, they are computed here from the states, the hold and the counts.

// The standings of an increment, in its life's order.
const (
	// StandingDefined: its delivery goals are written; no person ran it.
	StandingDefined = "defined"
	// StandingRunning: a person ran it; its proposal or its work is open.
	StandingRunning = "running"
	// StandingDelivering: everything it holds is done; its delivery awaits
	// the person's approval.
	StandingDelivering = "delivering"
	// StandingOnHold: held - its work is not pulled, no release waits for it.
	StandingOnHold = "on-hold"
	// StandingDone: its delivery was approved.
	StandingDone = "done"
	// StandingShipped: done, and the release it ships in is released.
	StandingShipped = "shipped"
	// StandingDropped: given up.
	StandingDropped = "dropped"
)

// StageCandidate is a release's stage once frozen with a delivery.
const StageCandidate = "candidate"

// IncrementStandingInput is what an increment's standing is read from: the
// increment, and - when the view has them - the counts of what it holds and
// the state of the release it ships in.
type IncrementStandingInput struct {
	Increment Increment
	// Done and Total are the goals and features it holds done, of all; zero
	// when unknown (then it is never read as delivering).
	Done, Total int
	// ReleaseState is the state of the release it ships in; empty when none
	// or unknown (then a done increment is done, not shipped).
	ReleaseState string
}

// IncrementStanding is where an increment stands.
func IncrementStanding(in IncrementStandingInput) string {
	inc := in.Increment
	switch {
	case inc.State == IncrementDropped:
		return StandingDropped
	case inc.State == IncrementDone && in.ReleaseState == ReleaseReleased:
		return StandingShipped
	case inc.State == IncrementDone:
		return StandingDone
	case inc.Held != nil:
		return StandingOnHold
	case !inc.Running:
		return StandingDefined
	case in.Total > 0 && in.Done >= in.Total:
		return StandingDelivering
	}
	return StandingRunning
}

// ReleaseStage is a release's stage: planned, frozen, candidate (frozen with
// a delivery), released or dropped.
func ReleaseStage(r Release) string {
	if r.State == ReleaseFrozen && len(r.Candidates) > 0 {
		return StageCandidate
	}
	return r.State
}

// ReleaseLabel is how every view names a release: « 1.4.0 — Spring » - its
// version once when its name is its version, or when it has none.
func ReleaseLabel(version, name string) string {
	switch {
	case version == "" || version == name:
		return name
	case name == "":
		return version
	}
	return version + " — " + name
}

// standingWords are each standing's and stage's words and colour, the same in
// every view.
var standingWords = map[string]struct{ en, fr, color string }{
	StandingDefined:    {"defined", "défini", "gray"},
	StandingRunning:    {"running", "en cours", "blue"},
	StandingDelivering: {"delivering", "en livraison", "grape"},
	StandingOnHold:     {"on hold", "en pause", "orange"},
	StandingDone:       {"done", "terminé", "green"},
	StandingShipped:    {"shipped", "livré", "teal"},
	StandingDropped:    {"dropped", "abandonné", "gray"},
	ReleasePlanned:     {"planned", "prévue", "blue"},
	ReleaseFrozen:      {"frozen", "gelée", "cyan"},
	StageCandidate:     {"candidate", "candidate", "grape"},
	ReleaseReleased:    {"released", "publiée", "green"},
}

// StandingLabel is how every view names a standing or a stage, and its colour.
func StandingLabel(standing string, french bool) (label, color string) {
	w, ok := standingWords[standing]
	if !ok {
		return standing, "gray"
	}
	if french {
		return w.fr, w.color
	}
	return w.en, w.color
}
