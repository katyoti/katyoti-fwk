// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"testing"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// « À FAIRE » ACROSS THE SERVED PROJECTS: a universe
// serving several projects lists every project's person steps, each item
// named by its qualified id and the project holding it - two projects
// holding one request id never collapse into one card.

// plannedIn is a request filed in the module ref (qualified or bare) and
// planned: its reference.
func plannedIn(t *testing.T, ctx context.Context, req api.RequestService, module, title string) string {
	t.Helper()
	r := file(t, ctx, req, api.FileInput{Title: title, Module: module, Kind: api.KindBuild, Review: api.ReviewPerson,
		Deliverables: []api.Deliverable{{Title: "it builds", Check: "go version"}}})
	ref := r.ID
	if k := keyOfModule(module); k != "" {
		ref = k + ":" + bareRef(r.ID)
	}
	work := workOf(t, ctx)
	if got, err := work.List(ctx, api.ListWorkInput{Request: ref}); err != nil || len(got.Todos) == 0 {
		if _, err := work.Plan(ctx, api.PlanWorkInput{Request: ref, By: person}); err != nil {
			t.Fatalf("plan %s: %v", ref, err)
		}
	}
	return ref
}

// keyOfModule is the project key a module reference names, "" when bare.
func keyOfModule(module string) string {
	for i := 0; i < len(module); i++ {
		if module[i] == ':' {
			return module[:i]
		}
	}
	return ""
}

// TestTheTodoListsEveryServedProjectsPersonSteps: alpha and beta both hold
// billing and a request of one id, each worked to its person's review; beta
// also holds a decision ready to decide. « À faire » lists both reviews and
// the decision, each by its qualified id with its project, none collapsed; a
// hold on alpha's review holds only it. One project served: today's items,
// ids bare and no project.
func TestTheTodoListsEveryServedProjectsPersonSteps(t *testing.T) {
	ctx, cwm, req := membersWorld(t, map[string]string{"alpha": alphaProject, "beta": betaProject})
	refs := map[string]string{}
	for _, key := range []string{"alpha", "beta"} {
		refs[key] = plannedIn(t, ctx, req, key+":billing", key+"'s invoices")
	}
	if bareRef(refs["alpha"]) != bareRef(refs["beta"]) {
		t.Fatalf("the two projects' requests are %s and %s, want one id", refs["alpha"], refs["beta"])
	}
	for _, key := range []string{"alpha", "beta"} {
		toReview(t, ctx, refs[key])
	}
	d := cwmapi.Object{Type: "decision", Parent: "beta:billing", Name: "Keep the old format?"}
	_ = d.SetCapability("decision", map[string]any{"options": []string{"keep", "drop"},
		"proposal": map[string]any{"option": "keep", "why": "it costs nothing"}})
	made, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "beta:billing", Object: d})
	if err != nil {
		t.Fatalf("create beta's decision: %v", err)
	}
	decision := "beta:" + bareRef(made.Object.ID)

	q := personQueue(t, ctx, "")
	for _, key := range []string{"alpha", "beta"} {
		items := itemsOf(q, refs[key])
		if got := actionsOf(items); got != api.ActionValidate {
			t.Errorf("%s: items %q, want exactly one validate", refs[key], got)
			continue
		}
		it := items[0]
		if it.Object != refs[key] || it.Request != refs[key] || it.Project != key || it.Module != "billing" || it.Title != key+"'s invoices" {
			t.Errorf("%s's review: object %q request %q project %q module %q title %q - want qualified, %s, billing, its own title",
				key, it.Object, it.Request, it.Project, it.Module, it.Title, key)
		}
	}
	if items := itemsOf(q, decision); actionsOf(items) != api.ActionDecide || items[0].Project != "beta" {
		t.Errorf("beta's decision %s: items %+v, want one decide of project beta", decision, items)
	}
	for _, it := range q.Items {
		if it.Object == bareRef(refs["alpha"]) {
			t.Errorf("an item is named bare though two projects hold %s: %+v", it.Object, it)
		}
	}

	// A hold on alpha's review holds only it.
	alpha := itemsOf(q, refs["alpha"])
	if len(alpha) == 0 {
		t.Fatal("alpha's review is not listed")
	}
	if _, err := req.Hold(ctx, api.HoldInput{Key: alpha[0].Key, Reason: "after the audit", By: person}); err != nil {
		t.Fatalf("hold alpha's review: %v", err)
	}
	q = personQueue(t, ctx, "")
	for key, held := range map[string]bool{"alpha": true, "beta": false} {
		items := itemsOf(q, refs[key])
		if len(items) != 1 || (items[0].Held != nil) != held {
			t.Errorf("after holding alpha's review, %s's items: %+v - want one, held %v", key, items, held)
		}
	}
	// A module scope names a project's module, or the module of that name
	// in every project.
	if got := personQueue(t, ctx, "beta:billing"); len(itemsOf(got, refs["alpha"])) != 0 || len(itemsOf(got, refs["beta"])) != 1 {
		t.Errorf("beta:billing's « À faire » must hold beta's review only: %+v", got.Items)
	}

	// One project served: today's items.
	soloCtx, _, soloReq := membersWorld(t, map[string]string{"gamma": "0193a0d3-a5ed-71e3-9f71-d032d2688c03"})
	solo := plannedIn(t, soloCtx, soloReq, "billing", "gamma's invoices")
	toReview(t, soloCtx, solo)
	items := itemsOf(personQueue(t, soloCtx, ""), solo)
	if actionsOf(items) != api.ActionValidate || items[0].Object != solo || items[0].Project != "" {
		t.Errorf("one project's review %s: %+v - want one validate, its id bare and no project", solo, items)
	}
}
