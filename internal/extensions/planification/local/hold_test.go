// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/cause"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	objectpaneapi "github.com/katyoti/katyoti-fwk/internal/extensions/objectpane/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// ON HOLD — the person puts an item of « À faire » on hold: it leaves its
// action and the count, is listed last under « En attente » with who, when and
// why, and « Reprendre » takes it back. Nothing else changes: the request keeps
// its state, what waits on it stays waiting. The hold is a file of its own: it
// outlives the process.

// queuePage is the « À faire » page, in French.
func queuePage(t *testing.T, ctx context.Context) string {
	t.Helper()
	return queuePageOf(t, ctx, "")
}

// queuePageOf is the « À faire » page with one tile chosen ("": « Tout »).
func queuePageOf(t *testing.T, ctx context.Context, action string) string {
	t.Helper()
	views, err := application.UseService[api.ViewService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	def, err := views.PersonQueuePage(ctx, api.PersonQueueInput{Language: "fr", Action: action})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(def.DSLTree)
	return string(raw)
}

func TestAnItemOnHoldLeavesItsActionAndComesBack(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := world(t)
	r := file(t, ctx, req, api.FileInput{Title: "Print invoices", Module: "billing", Kind: api.KindBuild,
		Deliverables: []api.Deliverable{{Title: "seen on paper", ByHand: "Print one; read it."}}})
	toReview(t, ctx, r.ID)
	before := personQueue(t, ctx, "")
	i := slices.IndexFunc(before.Items, func(it api.PersonItem) bool { return it.Action == api.ActionValidate && it.Request == r.ID })
	if i < 0 {
		t.Fatalf("no review waits on the person: %+v", before.Items)
	}
	key := before.Items[i].Key
	review := stepOf(t, ctx, r.ID, "review")
	state := planState(t, ctx, r.ID)
	if !strings.Contains(queuePage(t, ctx), `"uri":"ktw-agents:requests/hold"`) {
		t.Error("a card offers no « Mettre en attente »")
	}

	if _, err := req.Hold(ctx, api.HoldInput{Key: key, Reason: "after the audit", By: person}); err != nil {
		t.Fatal(err)
	}
	q := personQueue(t, ctx, "")
	if q.Count != before.Count-1 || q.ByAction[api.ActionValidate] != before.ByAction[api.ActionValidate]-1 || q.Held != 1 {
		t.Errorf("held: count %d (was %d), validate %d, held %d; want it out of its action and the count", q.Count, before.Count, q.ByAction[api.ActionValidate], q.Held)
	}
	last := q.Items[len(q.Items)-1]
	if last.Key != key || last.Held == nil || last.Held.Reason != "after the audit" || last.Held.By.ID != person.ID {
		t.Errorf("the last item = %+v; want the held review with who and why", last)
	}
	// « Tout » shows it nowhere: only the « En attente » tile counts it.
	page := queuePage(t, ctx)
	if !strings.Contains(page, "Tester à la main — 1") || !strings.Contains(page, `"label":"En attente","value":1`) {
		t.Errorf("« Tout »: want the hand test listed and the « En attente » tile counting 1: %s", page)
	}
	for _, not := range []string{"Valider — 1", "En attente — 1", "after the audit"} {
		if strings.Contains(page, not) {
			t.Errorf("« Tout » lists the held review (%s): it shows only under its tile", not)
		}
	}
	// Its tile chosen: the held item alone, with what takes it back.
	held := queuePageOf(t, ctx, "held")
	for _, want := range []string{"En attente — 1", "after the audit", `"uri":"ktw-agents:requests/unhold"`} {
		if !strings.Contains(held, want) {
			t.Errorf("« En attente » chosen: the page lacks %s", want)
		}
	}
	if strings.Contains(held, "Tester à la main — 1") {
		t.Error("« En attente » chosen: another action's section shows")
	}
	// Nothing else moved: the request, its review step.
	if st := planState(t, ctx, r.ID); st != state {
		t.Errorf("the request went %s → %s on a hold", state, st)
	}
	if now := stepOf(t, ctx, r.ID, "review"); now.State != review.State {
		t.Errorf("the review went %s → %s on a hold", review.State, now.State)
	}
	// The hold is a file of its own: another process reads it.
	sys, err := cwm.ObjectResolve(ctx, cwmapi.ObjectResolveInput{Ref: api.SystemModule})
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Dir(sys.Path)
	if filepath.Base(dir) == "objects" {
		dir = filepath.Dir(dir)
	}
	if raw, err := os.ReadFile(filepath.Join(dir, ".ktw", "holds.yaml")); err != nil || !strings.Contains(string(raw), key) {
		t.Errorf("the holds file: %v; want it to hold %s", err, key)
	}

	if _, err := req.Unhold(ctx, api.UnholdInput{Key: key, By: person}); err != nil {
		t.Fatal(err)
	}
	if q := personQueue(t, ctx, ""); q.Count != before.Count || q.Held != 0 {
		t.Errorf("taken back: count %d held %d; want %d and none", q.Count, q.Held, before.Count)
	}
}

func TestOnlyThePersonHoldsTheirItems(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	r := file(t, ctx, req, api.FileInput{Title: "Print invoices", Module: "billing", Kind: api.KindBuild,
		Deliverables: []api.Deliverable{{Title: "seen on paper", ByHand: "Print one; read it."}}})
	toReview(t, ctx, r.ID)
	key := personQueue(t, ctx, r.ID).Items[0].Key
	if _, err := req.Hold(ctx, api.HoldInput{Key: key, By: session}); err == nil {
		t.Error("an agent put the person's item on hold")
	}
	agent := cause.WithCause(ctx, cause.Cause{Kind: cause.KindAgent, ID: "claude-code"})
	if _, err := req.Hold(agent, api.HoldInput{Key: key, By: person}); err == nil {
		t.Error("an agent's session put the person's item on hold under their name")
	}
	if _, err := req.Hold(ctx, api.HoldInput{Key: "validate:nothing", By: person}); err == nil {
		t.Error("an unknown item was put on hold")
	}
	if _, err := req.Unhold(ctx, api.UnholdInput{Key: key, By: person}); err == nil {
		t.Error("an item not on hold was taken back")
	}
}

// A decision's own pane shows its items, those on hold among them.
func TestHoldsShowOnADecisionsPaneWithItsQueue(t *testing.T) {
	t.Parallel()
	ctx, cwm, _ := world(t)
	id := openDecision(t, ctx, cwm, "Keep the totals", true)
	pane, err := application.UseService[objectpaneapi.ObjectPaneService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	d, err := pane.Detail(ctx, objectpaneapi.DetailInput{ID: id})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(d.Panels, func(p objectpaneapi.ObjectPanel) bool { return p.ID == "tab:person-queue" }) {
		t.Errorf("« À faire » is not on the decision's pane")
	}
}
