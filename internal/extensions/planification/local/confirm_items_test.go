// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// A WANT THE WORK RECORDED DONE WAITS ON THE PERSON: a
// realized feature is one validate item of « À faire » with what it is judged
// on — its acceptance criteria with their last result, the requests that built
// it — and Accept and Reject; Accept (its confirm, as the person) takes it to
// done, Reject sends it back with the reason; either way it leaves the queue.
func TestARealizedFeatureWaitsOnThePersonInAFaire(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := world(t)
	realized := func(title string) string {
		t.Helper()
		f := want(t, ctx, cwm, "feature", title)
		if _, err := cwm.ObjectTransition(ctx, cwmapi.ObjectTransitionInput{ID: f, To: "realized", By: session, Note: "built and tested"}); err != nil {
			t.Fatal(err)
		}
		return f
	}
	mailed, printed := realized("Mail the invoices"), realized("Print the invoices")

	ac := cwmapi.Object{Type: "acceptance-criterion", Parent: mailed, Name: "An invoice is mailed the day it is issued"}
	_ = ac.SetCapability("checkable", map[string]any{"kind": "command", "probe": "go version"})
	crit, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "billing", Object: ac})
	if err != nil {
		t.Fatal(err)
	}
	v := cwmapi.Object{Type: "verdict", Parent: crit.Object.ID, Name: "Verdict: mailed"}
	_ = v.SetCapability("verdict", map[string]any{"status": "pass", "ran_at": "2026-10-16T08:00:00Z", "kind": "command"})
	if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "billing", Object: v}); err != nil {
		t.Fatal(err)
	}
	built := file(t, ctx, req, api.FileInput{Title: "Mail the invoices", Module: "billing", Kind: api.KindBuild})
	if _, err := cwm.EdgeCreate(ctx, cwmapi.EdgeCreateInput{From: built.ID, To: mailed, Kind: api.EdgeServes}); err != nil {
		t.Fatal(err)
	}

	q := personQueue(t, ctx, "")
	got := itemsOf(q, mailed)
	if len(got) != 1 || got[0].Action != api.ActionValidate || got[0].Confirm == nil ||
		got[0].Confirm.Accept != "confirm" || got[0].Confirm.AcceptTo != "done" || got[0].Confirm.Reject != "reopen" {
		t.Fatalf("the realized feature's items: %+v; want one validate item with its confirm and reopen", got)
	}
	if cs := got[0].Criteria; len(cs) != 1 || cs[0].ID != crit.Object.ID || cs[0].Status != "pass" || cs[0].RanAt == "" {
		t.Errorf("its criteria: %+v; want its one criterion, passed, with when it ran", cs)
	}
	if rs := got[0].Refs; len(rs) != 1 || rs[0].ID != built.ID {
		t.Errorf("what built it: %+v; want %s", rs, built.ID)
	}
	if got := itemsOf(q, printed); len(got) != 1 || len(got[0].Criteria) != 0 {
		t.Errorf("a realized feature with no criterion: %+v; want its item, no criteria", got)
	}

	views, err := application.UseService[api.ViewService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	def, err := views.PersonQueuePage(ctx, api.PersonQueueInput{Language: "fr"})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(def.DSLTree)
	page := string(raw)
	for _, want := range []string{"Critères d'acceptation", crit.Object.ID, "réussi", "Aucun critère d'acceptation",
		`"transition":"confirm"`, `"transition":"reopen"`, "Rejeter", built.ID} {
		if !strings.Contains(page, want) {
			t.Errorf("« À faire » lacks %s", want)
		}
	}

	// Accept, as the person: done, out of the queue.
	if _, err := cwm.ObjectTransition(ctx, cwmapi.ObjectTransitionInput{ID: mailed, To: "done", Transition: "confirm", By: person, Note: "accepted from « À faire »"}); err != nil {
		t.Fatalf("accept: %v", err)
	}
	// Reject, with the reason: back to proposed, out of the queue.
	if _, err := cwm.ObjectTransition(ctx, cwmapi.ObjectTransitionInput{ID: printed, To: "proposed", Transition: "reopen", By: person, Note: "the PDF has no logo"}); err != nil {
		t.Fatalf("reject: %v", err)
	}
	if st := stateOf(t, ctx, cwm, mailed); st != "done" {
		t.Errorf("accepted, %s is %q", mailed, st)
	}
	if st := stateOf(t, ctx, cwm, printed); st != "proposed" {
		t.Errorf("rejected, %s is %q", printed, st)
	}
	q = personQueue(t, ctx, "")
	for _, id := range []string{mailed, printed} {
		for _, it := range itemsOf(q, id) {
			if it.Confirm != nil {
				t.Errorf("%s still waits on the person: %+v", id, it)
			}
		}
	}
}
