// SPDX-License-Identifier: Apache-2.0

package contrib_test

import (
	"encoding/json"
	"testing"
	"testing/fstest"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	docsapi "github.com/katyoti/katyoti-fwk/internal/extensions/docs/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/docs/contrib"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// A document written for someone carries the `deliverable` capability: what
// wrote it, for whom, in which format, its own workflow starting at draft,
// its produced-from edges to what it was written from, and the actions that
// regenerate and export it. Only a person validates it, and only a
// deliverable is produced from something.
func TestDocumentObjectIsADeliverable(t *testing.T) {
	ctx, cwm := cwmapp.NewStrict(t,
		// `document` and `module` are ktw-agents' and ktw-methods'; the test
		// carries their names only.
		cwmapp.Pack(fstest.MapFS{
			"object-types/document.yaml": {Data: []byte("name: document\n")},
			"object-types/module.yaml":   {Data: []byte("name: module\n")},
		}),
		contrib.Pack("ktw-docs"),
	)
	cwmapp.Module(t, ctx, cwm, "dossier")

	study, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "dossier",
		Object: cwmapi.Object{Type: "document", Name: "As-is study", Body: "What is in place."}})
	if err != nil {
		t.Fatalf("create the study: %v", err)
	}
	doc := cwmapi.Object{Type: "document", Name: "Architecture study", Body: "# Architecture study",
		Capabilities: map[string]map[string]any{docsapi.CapabilityDeliverable: {
			"template": "etude-architecture", "audience": "architecture board", "language": "fr", "format": "markdown"}}}
	created, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "dossier", Object: doc})
	if err != nil {
		t.Fatalf("create the deliverable: %v", err)
	}
	id := created.Object.ID
	// A deliverable that never moved is in its workflow's initial state
	// (draft): nothing stored, or draft.
	if got, stored := created.Object.Capabilities[docsapi.CapabilityDeliverable]["state"]; stored && got != "draft" {
		t.Errorf("a new deliverable starts at draft, got %v", got)
	}

	// What it was written from is an edge; a document that is no deliverable
	// is produced from nothing.
	if _, err := cwm.EdgeCreate(ctx, cwmapi.EdgeCreateInput{From: id, To: study.Object.ID, Kind: docsapi.EdgeProducedFrom}); err != nil {
		t.Fatalf("produced-from: %v", err)
	}
	if _, err := cwm.EdgeCreate(ctx, cwmapi.EdgeCreateInput{From: study.Object.ID, To: id, Kind: docsapi.EdgeProducedFrom}); err == nil {
		t.Errorf("a document without the deliverable capability was given a produced-from edge")
	}

	// Its workflow: an agent submits, only a person validates, then it is published.
	agent := cwmapi.Actor{ID: "writer", Kind: cwmapi.ActorAgent}
	person := cwmapi.Actor{ID: "architect", Kind: cwmapi.ActorPerson}
	move := func(to string, by cwmapi.Actor) error {
		_, err := cwm.ObjectTransition(ctx, cwmapi.ObjectTransitionInput{ID: id, To: to, By: by, Capability: docsapi.CapabilityDeliverable})
		return err
	}
	if err := move("in-review", agent); err != nil {
		t.Fatalf("submit: %v", err)
	}
	if err := move("validated", agent); err == nil {
		t.Errorf("an agent validated a deliverable")
	}
	if err := move("validated", person); err != nil {
		t.Fatalf("validate by a person: %v", err)
	}
	if err := move("published", agent); err != nil {
		t.Fatalf("publish: %v", err)
	}
	got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{ID: id})
	if err != nil {
		t.Fatal(err)
	}
	if s := got.Object.Capabilities[docsapi.CapabilityDeliverable]["state"]; s != "published" {
		t.Errorf("state %v, want published", s)
	}

	// Its pane offers to regenerate and to export it.
	surface, err := cwm.ObjectEffectiveSurface(ctx, cwmapi.ObjectEffectiveSurfaceInput{ID: id})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"ktw-docs:tasks/deliverable-regenerate": false, "ktw-docs:tasks/deliverable-export": false}
	for _, a := range surface.Actions {
		if _, ok := want[a.Ref.URI]; ok {
			want[a.Ref.URI] = true
		}
	}
	for uri, seen := range want {
		if !seen {
			t.Errorf("the deliverable does not offer %s: %+v", uri, surface.Actions)
		}
	}
}

// : a deliverable keeps its moves - who submitted it, who
// validated it and when - on the object, its trail.
func TestADeliverableKeepsWhoMovedItAndWhen(t *testing.T) {
	ctx, cwm := cwmapp.NewStrict(t,
		cwmapp.Pack(fstest.MapFS{
			"object-types/document.yaml": {Data: []byte("name: document\n")},
			"object-types/module.yaml":   {Data: []byte("name: module\n")},
		}),
		contrib.Pack("ktw-docs"),
	)
	cwmapp.Module(t, ctx, cwm, "study")
	created, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "study", Object: cwmapi.Object{Type: "document", Name: "00 Contexte",
		Capabilities: map[string]map[string]any{docsapi.CapabilityDeliverable: {"template": "archi-contexte", "format": "markdown"}}}})
	if err != nil {
		t.Fatal(err)
	}
	id := created.Object.ID
	agent := cwmapi.Actor{ID: "writer", Kind: cwmapi.ActorAgent}
	person := cwmapi.Actor{ID: "architect", Kind: cwmapi.ActorPerson}
	for _, m := range []struct {
		to string
		by cwmapi.Actor
	}{{"in-review", agent}, {"validated", person}} {
		if _, err := cwm.ObjectTransition(ctx, cwmapi.ObjectTransitionInput{ID: id, To: m.to, By: m.by, Capability: docsapi.CapabilityDeliverable}); err != nil {
			t.Fatalf("to %s: %v", m.to, err)
		}
	}
	got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{ID: id})
	if err != nil {
		t.Fatal(err)
	}
	grp, _ := got.Object.Capability(docsapi.CapabilityDeliverable)
	raw, _ := json.Marshal(grp)
	var cap docsapi.CapDeliverable
	if err := json.Unmarshal(raw, &cap); err != nil {
		t.Fatal(err)
	}
	if cap.State != "validated" || len(cap.Trail) != 2 {
		t.Fatalf("state %q, trail %+v: want validated and both moves", cap.State, cap.Trail)
	}
	last := cap.Trail[1]
	if last.To != "validated" || last.By.ID != "architect" || last.By.Kind != cwmapi.ActorPerson || last.At == "" {
		t.Errorf("who validated it and when: %+v", last)
	}
}
