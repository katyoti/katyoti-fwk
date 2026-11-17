// SPDX-License-Identifier: Apache-2.0

package main

import (
	"testing"
	"testing/fstest"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// AN IN-REVIEW FEATURE REACHES DONE — the `feature` workflow as shipped: a
// person confirms a feature being reviewed as done, an agent cannot, and a
// feature found already in the code is recorded realized from review too.
func TestAnInReviewFeatureReachesDoneByAPerson(t *testing.T) {
	pack := cwmapp.Pack(fstest.MapFS{
		"object-types/feature.yaml": {Data: []byte("name: feature\nfolder: features\ncapabilities: [container, content, lifecycle]\nworkflow: feature\n")},
		"workflows/feature.yaml":    {Data: cwmapp.ShippedFile(t, "app/ktw-methods/embed/cwm/workflows/feature.yaml")},
	})
	ctx, cwm := cwmapp.New(t, pack)
	cwmapp.Module(t, ctx, cwm, "m")
	person := cwmapi.Actor{ID: "jdoe", Kind: cwmapi.ActorPerson}
	agent := cwmapi.Actor{ID: "claude", Kind: "agent"}
	inReview := func(local string) string {
		t.Helper()
		o := cwmapi.NewContainerObject("m", local, "feature", "m", "Feature "+local, "")
		if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "m", Object: o}); err != nil {
			t.Fatal(err)
		}
		for _, to := range []string{"proposed", "in-review"} {
			if _, err := cwm.ObjectTransition(ctx, cwmapi.ObjectTransitionInput{ID: o.ID, To: to, By: person, Note: to}); err != nil {
				t.Fatalf("%s -> %s: %v", o.ID, to, err)
			}
		}
		return o.ID
	}
	f := inReview("f1")
	if _, err := cwm.ObjectTransition(ctx, cwmapi.ObjectTransitionInput{ID: f, To: "done", By: agent, Note: "looks done"}); err == nil {
		t.Error("an agent moved an in-review feature to done; want it refused (a person confirms)")
	}
	if _, err := cwm.ObjectTransition(ctx, cwmapi.ObjectTransitionInput{ID: f, To: "done", By: person, Note: "reviewed and delivered"}); err != nil {
		t.Fatalf("a person confirming an in-review feature done: %v", err)
	}
	g := inReview("f2")
	if _, err := cwm.ObjectTransition(ctx, cwmapi.ObjectTransitionInput{ID: g, To: "realized", By: agent, Note: "already in the code"}); err != nil {
		t.Fatalf("an in-review feature recorded realized: %v", err)
	}
}

// A BUILT ACCEPTED FEATURE ENDS REALIZED — a feature accepted (or specified,
// or ready) whose build ran to its end has no way to done an agent takes: its
// build's end records it realized, and a person confirms it done.
func TestABuiltAcceptedFeatureIsRecordedRealized(t *testing.T) {
	pack := cwmapp.Pack(fstest.MapFS{
		"object-types/feature.yaml": {Data: []byte("name: feature\nfolder: features\ncapabilities: [container, content, lifecycle]\nworkflow: feature\n")},
		"workflows/feature.yaml":    {Data: cwmapp.ShippedFile(t, "app/ktw-methods/embed/cwm/workflows/feature.yaml")},
	})
	ctx, cwm := cwmapp.New(t, pack)
	cwmapp.Module(t, ctx, cwm, "m")
	person := cwmapi.Actor{ID: "jdoe", Kind: cwmapi.ActorPerson}
	agent := cwmapi.Actor{ID: "claude", Kind: "agent"}
	o := cwmapi.NewContainerObject("m", "f1", "feature", "m", "Feature f1", "")
	if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "m", Object: o}); err != nil {
		t.Fatal(err)
	}
	for _, to := range []string{"proposed", "in-review", "accepted"} {
		if _, err := cwm.ObjectTransition(ctx, cwmapi.ObjectTransitionInput{ID: o.ID, To: to, By: person, Note: to}); err != nil {
			t.Fatalf("%s -> %s: %v", o.ID, to, err)
		}
	}
	if _, err := cwm.ObjectTransition(ctx, cwmapi.ObjectTransitionInput{ID: o.ID, To: "realized", By: agent, Note: "its build ended"}); err != nil {
		t.Fatalf("an accepted feature whose build ended, recorded realized: %v", err)
	}
	if _, err := cwm.ObjectTransition(ctx, cwmapi.ObjectTransitionInput{ID: o.ID, To: "done", By: person, Note: "confirmed"}); err != nil {
		t.Fatalf("a person confirming the realized feature done: %v", err)
	}
}

// A DONE FEATURE IS REOPENED BY A PERSON — the `feature` workflow as shipped:
// a person sends a done feature back to specified (reopen-done), saying why in
// the note, so a fix round can plan it again; an agent cannot take that way.
func TestADoneFeatureIsReopenedToSpecifiedByAPerson(t *testing.T) {
	pack := cwmapp.Pack(fstest.MapFS{
		"object-types/feature.yaml": {Data: []byte("name: feature\nfolder: features\ncapabilities: [container, content, lifecycle]\nworkflow: feature\n")},
		"workflows/feature.yaml":    {Data: cwmapp.ShippedFile(t, "app/ktw-methods/embed/cwm/workflows/feature.yaml")},
	})
	ctx, cwm := cwmapp.New(t, pack)
	cwmapp.Module(t, ctx, cwm, "m")
	person := cwmapi.Actor{ID: "jdoe", Kind: cwmapi.ActorPerson}
	agent := cwmapi.Actor{ID: "claude", Kind: "agent"}
	o := cwmapi.NewContainerObject("m", "f1", "feature", "m", "Feature f1", "")
	if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "m", Object: o}); err != nil {
		t.Fatal(err)
	}
	for _, to := range []string{"proposed", "in-review", "done"} {
		if _, err := cwm.ObjectTransition(ctx, cwmapi.ObjectTransitionInput{ID: o.ID, To: to, By: person, Note: to}); err != nil {
			t.Fatalf("%s -> %s: %v", o.ID, to, err)
		}
	}
	if _, err := cwm.ObjectTransition(ctx, cwmapi.ObjectTransitionInput{ID: o.ID, To: "specified", By: agent, Note: "its check fails"}); err == nil {
		t.Error("an agent reopened a done feature to specified; want it refused (a person reopens it)")
	}
	out, err := cwm.ObjectTransition(ctx, cwmapi.ObjectTransitionInput{ID: o.ID, To: "specified", By: person, Note: "its fix round needs a build again"})
	if err != nil {
		t.Fatalf("a person reopening a done feature to specified: %v", err)
	}
	if out.Transition != "reopen-done" || out.To != "specified" {
		t.Errorf("took %q to %q, want reopen-done to specified", out.Transition, out.To)
	}
}
