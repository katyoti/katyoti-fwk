// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"strings"
	"testing"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// A TARGET COUNTS BY HOW ITS OWN LIFECYCLE ENDS - a
// decision in force (applies) is done; a superseded decision and a split
// request count in neither done nor total, their work carried elsewhere; none
// of them is waited on. In an increment, and as a release's direct work.
func TestAnEndedTargetCountsByItsEnd(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := world(t)
	inc, rel := increments(t, ctx), releases(t, ctx)
	in := increment(t, ctx, "Invoices reach their customers by mail")
	hold := func(id string) {
		t.Helper()
		if _, err := inc.Target(ctx, api.IncrementTargetInput{ID: id, Increment: in.ID}); err != nil {
			t.Fatal(err)
		}
	}
	decided := func(title string) string {
		t.Helper()
		id := decision(t, ctx, cwm, req, title)
		choose(t, ctx, cwm, id, "keep", person)
		if err := move(ctx, cwm, id, "applies", person); err != nil && !strings.Contains(err.Error(), "from applies") {
			t.Fatal(err)
		}
		return id
	}
	uc := usecase(t, ctx, cwm, "Send invoices by mail")
	hold(uc)
	if _, err := cwm.ObjectTransition(ctx, cwmapi.ObjectTransitionInput{ID: uc, To: "done", By: person, Note: "mailed"}); err != nil {
		t.Fatal(err)
	}
	// An increment takes a decision as an applied proposal leaves it (the
	// increment's verb aims wants only).
	aim := func(id string) {
		t.Helper()
		taken(t, ctx, cwm, in.ID, id)
	}
	applies := decided("Mail through the gateway?")
	aim(applies)
	old := decided("Mail from the server?")
	aim(old)
	if _, err := cwm.ObjectTransition(ctx, cwmapi.ObjectTransitionInput{ID: old, To: "superseded", By: person, Force: true, Note: "replaced by the gateway"}); err != nil {
		t.Fatal(err)
	}
	split := file(t, ctx, req, api.FileInput{Title: "Mail the invoices and the reminders", Module: "billing", Kind: api.KindBuild, Increment: in.ID})
	if _, err := req.Split(ctx, api.SplitInput{ID: split.ID, By: intake, Note: "two", Parts: []api.SplitPart{{Module: "billing", Title: "Mail the invoices"}, {Module: "billing", Title: "Mail the reminders"}}}); err != nil {
		t.Fatal(err)
	}

	sc, err := inc.Scope(ctx, api.GetInput{ID: in.ID})
	if err != nil {
		t.Fatal(err)
	}
	if sc.Done != 2 || sc.Total != 2 {
		t.Errorf("increment scope %d/%d; want 2/2: the use case and the decision in force done, the superseded decision and the split request in neither", sc.Done, sc.Total)
	}
	// The increment's card reads its summary: the same counts, and its
	// requests' bar counts the split request nowhere.
	sum, err := inc.Summary(ctx, api.GetInput{ID: in.ID})
	if err != nil {
		t.Fatal(err)
	}
	if sum.Done != sc.Done || sum.Total != sc.Total {
		t.Errorf("the card's bar %d/%d; want the scope's %d/%d", sum.Done, sum.Total, sc.Done, sc.Total)
	}
	if n := sum.RequestsDone + sum.Running + sum.ToDo + sum.Awaiting + sum.Blocked; n != 2 {
		t.Errorf("the card counts %d requests (%+v); want the split request's two parts only", n, sum)
	}
	waits, err := inc.Waits(ctx, api.GetInput{ID: in.ID})
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(waits.Waits, "\n")
	for _, ended := range []string{"Mail through the gateway?", "Mail from the server?", "Mail the invoices and the reminders"} {
		if strings.Contains(got, ended) {
			t.Errorf("the increment waits on the ended %q: %q", ended, got)
		}
	}

	// A release's direct work: a split fix counts in neither.
	r := plan(t, ctx, rel, api.PlanInput{Module: "billing", Version: "1.4.0"})
	fix := file(t, ctx, req, api.FileInput{Title: "Totals and taxes", Module: "billing", Kind: api.KindFix})
	if _, err := rel.Target(ctx, api.TargetInput{ID: fix.ID, Release: r.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := req.Split(ctx, api.SplitInput{ID: fix.ID, By: intake, Note: "two", Parts: []api.SplitPart{{Module: "billing", Title: "Totals"}, {Module: "billing", Title: "Taxes"}}}); err != nil {
		t.Fatal(err)
	}
	rs, err := rel.Scope(ctx, api.GetInput{ID: r.ID})
	if err != nil {
		t.Fatal(err)
	}
	for _, tg := range rs.Targets {
		if tg.ID == fix.ID && rs.Total != 0 {
			t.Errorf("release scope %d/%d with the split fix counted; want it in neither", rs.Done, rs.Total)
		}
	}
}
