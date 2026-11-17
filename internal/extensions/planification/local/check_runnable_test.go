// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// A check is a command that can run: a document path, a card name or a
// sentence is refused when filed and when revised, and one already on a
// request is never run nor counted as passed.
func TestANonRunnableCheckIsRefused(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := world(t)
	prose := []api.Deliverable{{Title: "The manual pass", Check: "modules/planification/tests/manual-tests.md §6, §10"}}
	if _, err := req.File(ctx, api.FileInput{Title: "Test releases", Module: "billing", Kind: api.KindReview, Deliverables: prose, By: person}); !errors.Is(err, cwmapi.ErrInvalidInput) || !strings.Contains(err.Error(), "not a runnable check") {
		t.Fatalf("filing a document path as a check: %v; want it refused as not runnable", err)
	}
	r := file(t, ctx, req, api.FileInput{Title: "Test releases", Module: "billing", Kind: api.KindReview,
		Deliverables: []api.Deliverable{{Title: "The manual pass"}}, By: person})
	if _, err := req.Revise(ctx, api.ReviseInput{ID: r.ID, Note: "a card name", By: person,
		Deliverables: []api.Deliverable{{Title: "The manual pass", Check: "ms-demo-install/dm03"}}}); !errors.Is(err, cwmapi.ErrInvalidInput) {
		t.Fatalf("revising to a card name: %v; want it refused", err)
	}
	// One written before the rule: never run, never passed.
	got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: "billing", ID: r.ID})
	if err != nil {
		t.Fatal(err)
	}
	o := got.Object
	f, _, err := cwmapi.DecodeCapability[api.RequestFields](o, api.CapRequest)
	if err != nil {
		t.Fatal(err)
	}
	f.Deliverables = prose
	if err := o.SetCapability(api.CapRequest, f); err != nil {
		t.Fatal(err)
	}
	if _, err := cwm.ObjectUpdate(ctx, cwmapi.ObjectUpdateInput{Module: "billing", Object: o}); err != nil {
		t.Fatal(err)
	}
	// Revising keeps an unchanged check as it is, and still refuses a new one.
	keep := append(slices.Clone(prose), api.Deliverable{Title: "The tests pass", Check: "go test ./..."})
	if _, err := req.Revise(ctx, api.ReviseInput{ID: r.ID, Note: "one more", By: person, Deliverables: keep}); err != nil {
		t.Fatalf("revising with the old check unchanged: %v; want it kept", err)
	}
	if _, err := req.Revise(ctx, api.ReviseInput{ID: r.ID, Note: "back", By: person, Deliverables: prose}); err != nil {
		t.Fatal(err)
	}
	out, err := req.Check(ctx, api.CheckInput{ID: r.ID, By: person})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Results) != 1 || out.Results[0].Passed || !strings.Contains(out.Results[0].Error, "not a runnable check") {
		t.Fatalf("results %+v; want the prose check not run and not passed", out.Results)
	}
	for _, d := range out.Request.Deliverables {
		if d.Verified {
			t.Errorf("%q verified by a check that cannot run", d.Title)
		}
	}
}

// An action URI is a check: filed as one word, it is accepted.
func TestAnActionURIIsARunnableCheck(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	if _, err := req.File(ctx, api.FileInput{Title: "Its methods answer", Module: "billing", Kind: api.KindBuild, By: person,
		Deliverables: []api.Deliverable{{Title: "The list answers", Check: "ktw-agents:requests/list"}}}); err != nil {
		t.Fatalf("filing an action URI as a check: %v; want it accepted", err)
	}
}

// Splitting a request that carries an old non-runnable check keeps it on the
// part that moves it, and still refuses a new one on a part.
func TestASplitKeepsAnOldCheckAndRefusesANewOne(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := world(t)
	prose := []api.Deliverable{{Title: "The manual pass", Check: "modules/planification/tests/manual-tests.md §6"}}
	r := file(t, ctx, req, api.FileInput{Title: "Test releases", Module: "billing", Kind: api.KindReview,
		Deliverables: []api.Deliverable{{Title: "The manual pass"}}, By: person})
	got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: "billing", ID: r.ID})
	if err != nil {
		t.Fatal(err)
	}
	o := got.Object
	f, _, err := cwmapi.DecodeCapability[api.RequestFields](o, api.CapRequest)
	if err != nil {
		t.Fatal(err)
	}
	f.Deliverables = prose
	if err := o.SetCapability(api.CapRequest, f); err != nil {
		t.Fatal(err)
	}
	if _, err := cwm.ObjectUpdate(ctx, cwmapi.ObjectUpdateInput{Module: "billing", Object: o}); err != nil {
		t.Fatal(err)
	}
	if _, err := req.Split(ctx, api.SplitInput{ID: r.ID, By: intake, Note: "a new prose check", Parts: []api.SplitPart{
		{Module: "billing", Title: "Test releases again", Deliverables: []api.Deliverable{{Title: "Looked at", Check: "the GUI shows it"}}},
	}}); !errors.Is(err, cwmapi.ErrInvalidInput) {
		t.Fatalf("a split part with a new prose check: %v; want it refused", err)
	}
	if _, err := req.Split(ctx, api.SplitInput{ID: r.ID, By: intake, Note: "a move", Parts: []api.SplitPart{
		{Module: "billing", Title: "Test releases in billing"},
	}}); err != nil {
		t.Fatalf("moving a request that carries an old check: %v; want it kept", err)
	}
}
