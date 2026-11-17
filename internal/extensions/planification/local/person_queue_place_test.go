// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// WHAT WAITS ON THE PERSON IS PLACED BY RELEASE THEN INCREMENT (feature
// ): each item of « À faire » carries the increment and
// the release of the work it moves on; the page names each group - release
// then increment, what is in none last - over its items; the queue scoped to
// a release keeps that release's items, and the release page's count of what
// awaits a person is that same list.
func TestToDoIsPlacedByReleaseThenIncrementAndTheReleaseCountsTheSame(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	sm, rel, inc, plans := streams(t, ctx), releases(t, ctx), increments(t, ctx), plansOf(t, ctx)
	st, err := sm.Create(ctx, api.CreateStreamInput{Module: "billing", Name: "Product", StreamFields: api.StreamFields{VersionScheme: api.SchemeSemver, ReleaseMode: api.ModeManual}})
	if err != nil {
		t.Fatal(err)
	}
	r := plan(t, ctx, rel, api.PlanInput{Module: "billing", Version: "1.1.0", Stream: st.Stream.ID})
	shipped := func(title string) string {
		t.Helper()
		id := increment(t, ctx, title).ID
		if _, err := sm.AimIncrement(ctx, api.StreamInInput{ID: id, Stream: st.Stream.ID}); err != nil {
			t.Fatal(err)
		}
		if _, err := inc.ShipIn(ctx, api.ShipInInput{ID: id, In: r.ID}); err != nil {
			t.Fatal(err)
		}
		return id
	}
	mail, csv := shipped("Invoices by mail"), shipped("Exports")
	blocked := func(title, increment string) string {
		t.Helper()
		f := file(t, ctx, req, api.FileInput{Title: title, Module: "billing", Kind: api.KindBuild, Increment: increment})
		if _, err := plans.Block(ctx, api.BlockInput{ID: f.ID, Note: "waits on a person", By: session}); err != nil {
			t.Fatal(err)
		}
		return f.ID
	}
	a, b := blocked("Mail the invoices", mail), blocked("Export to CSV", csv)
	alone := blocked("Fix a typo", "")

	q := personQueue(t, ctx, "")
	byReq := map[string]api.PersonItem{}
	for _, it := range q.Items {
		byReq[it.Request] = it
	}
	if it := byReq[a]; it.Increment != mail || it.Release != r.ID || it.IncrementTitle != "Invoices by mail" || !strings.Contains(it.ReleaseTitle, "1.1.0") {
		t.Errorf("the item of %s: %+v; want its increment and release with their titles", a, it)
	}
	if it := byReq[alone]; it.Increment != "" || it.Release != "" {
		t.Errorf("the item outside any increment is placed: %+v", it)
	}

	scoped := personQueue(t, ctx, r.ID)
	var in []string
	for _, it := range scoped.Items {
		in = append(in, it.Request)
	}
	if len(in) != 2 || !strings.Contains(strings.Join(in, ","), a) || !strings.Contains(strings.Join(in, ","), b) {
		t.Errorf("the queue scoped to the release holds %v; want %s and %s", in, a, b)
	}

	views, err := application.UseService[api.ViewService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	def, err := views.PersonQueuePage(ctx, api.PersonQueueInput{Language: "en"})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(def.DSLTree)
	page := string(raw)
	// Within the release, by increment (Exports before Invoices by mail);
	// what is in no release last.
	g1, g2, g3 := strings.Index(page, "› Exports"), strings.Index(page, "› Invoices by mail"), strings.Index(page, "In no release › outside any increment")
	if g1 < 0 || g2 < 0 || g3 < 0 || !(g1 < g2 && g2 < g3) {
		t.Errorf("the groups are not named release then increment, what is in none last (Exports %d, Invoices %d, none %d)", g1, g2, g3)
	}

	rp, err := releasePane(ctx, rel, r.ID)
	if err != nil {
		t.Fatal(err)
	}
	b2, _ := json.Marshal(rp.DSLTree)
	if want := `"label":"Awaiting a person","value":2`; !strings.Contains(string(b2), want) {
		t.Errorf("the release page does not count the queue's 2 items for it (%s)", want)
	}
}
