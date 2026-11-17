// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"strings"
	"testing"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// A FOLLOW-UP NEVER LOCKS THE WORK IT WAITS ON: a
// parent ends only once its follow-ups have, so ordering a follow-up after
// work that leads back to its parent closes a cycle - refused, naming it, as
// a depends-on cycle is.
func TestAFollowUpOrderedAfterWorkLeadingBackToItsParentIsRefused(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := world(t)
	parent := file(t, ctx, req, api.FileInput{Title: "Send invoices", Module: "billing", Kind: api.KindBuild, By: person})
	follow := file(t, ctx, req, api.FileInput{Title: "The project type is gone", Module: "billing", Kind: api.KindBuild, By: person})
	if _, err := cwm.EdgeCreate(ctx, cwmapi.EdgeCreateInput{From: follow.ID, To: parent.ID, Kind: cwmapi.EdgeType(api.EdgeFollowsUp)}); err != nil {
		t.Fatal(err)
	}
	later := file(t, ctx, req, api.FileInput{Title: "Delete what the increment replaces", Module: "billing", Kind: api.KindBuild, By: person})
	if _, err := req.Depend(ctx, api.DependInput{ID: later.ID, On: parent.ID}); err != nil {
		t.Fatal(err)
	}
	// later waits on parent, which cannot end before its follow-up: the
	// follow-up waiting on later locks all three.
	_, err := req.Depend(ctx, api.DependInput{ID: follow.ID, On: later.ID})
	if err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("a follow-up ordered after work leading back to its parent: %v; want refused, naming the cycle", err)
	}
	// An unrelated order is still taken.
	other := file(t, ctx, req, api.FileInput{Title: "Print invoices", Module: "billing", Kind: api.KindBuild, By: person})
	if _, err := req.Depend(ctx, api.DependInput{ID: follow.ID, On: other.ID}); err != nil {
		t.Errorf("a follow-up ordered after unrelated work: %v", err)
	}
}

// A REQUEST WAITING ON A FEATURE GOES ON ONCE THE FEATURE IS DONE, even while
// the feature's own plan is left open (idle or blocked) - its lifecycle says it
// is delivered.
func TestARequestWaitingOnADoneFeatureGoesOn(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := world(t)
	f := want(t, ctx, cwm, "feature", "Mail the invoices")
	if _, err := req.File(ctx, api.FileInput{On: f, Kind: api.KindBuild, By: person}); err != nil {
		t.Fatal(err)
	}
	r := file(t, ctx, req, api.FileInput{Title: "Remind late payers", Module: "billing", Kind: api.KindBuild, By: person})
	if _, err := req.Depend(ctx, api.DependInput{ID: r.ID, On: f}); err != nil {
		t.Fatal(err)
	}
	pullAndReport(t, ctx, stepOf(t, ctx, r.ID, api.StepIntake).ID, session, schedapi.Result{Summary: "a build"})
	if st := stepOf(t, ctx, r.ID, "analyse"); st.State != schedapi.StateWaiting || !strings.Contains(st.Waits, f) {
		t.Fatalf("before the feature is done: %s (%q); want waiting on %s", st.State, st.Waits, f)
	}
	for _, to := range []struct{ state, transition string }{{"realized", ""}, {"done", "confirm"}} {
		if _, err := cwm.ObjectTransition(ctx, cwmapi.ObjectTransitionInput{ID: f, To: to.state, Transition: to.transition, By: person, Note: "delivered"}); err != nil {
			t.Fatal(err)
		}
	}
	// Its plan is still open (its build request's steps not run). A lifecycle
	// move settles every to-do again in the running sidecar (its transition
	// event); this world has no bus, so the order given again settles r.
	if _, err := req.Depend(ctx, api.DependInput{ID: r.ID, On: f}); err != nil {
		t.Fatal(err)
	}
	if st := stepOf(t, ctx, r.ID, "analyse"); st.State != schedapi.StateReady {
		t.Errorf("the feature done, its plan open: %s (%q); want ready", st.State, st.Waits)
	}
}
