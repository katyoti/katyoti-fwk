// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/appcore"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/eventbus"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	journalcontrib "github.com/katyoti/katyoti-fwk/internal/extensions/journal/contrib"
	journallocal "github.com/katyoti/katyoti-fwk/internal/extensions/journal/local"
	"github.com/katyoti/katyoti-fwk/internal/extensions/objectpane/api"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// THE PANE ASKS THE JOURNAL BY THE QUALIFIED ID (
// ). A universe serves alpha and beta, both holding the bare id
// : alpha's is created first and never moves, beta's is created later
// and accepted. The detail of each qualified id shows its own project's
// created and changed times, and its options its own history - alpha's none,
// beta's the accept. Asked by the bare id, the journal reads both projects'
// records merged: alpha would show beta's change and beta's move.
func TestTheTimesAndHistoryOfAQualifiedIdAreItsProjects(t *testing.T) {
	root := t.TempDir()
	t.Setenv("KTW_KATYOTI_ROOT", filepath.Join(root, "installation"))
	application.ResetGlobals()
	t.Cleanup(application.ResetGlobals)

	ctx, cwm, svc := paneMembersServed(t, root, map[string]string{
		"object-types/thing.yaml": "name: thing\ntree: node\nworkflow: ladder\n",
		"workflows/ladder.yaml": "name: ladder\ninitial: proposed\nstates: [{name: proposed}, {name: accepted}, {name: closed, terminal: true}]\n" +
			"transitions: [{name: accept, from: [proposed], to: accepted}, {name: close, from: [accepted], to: closed}]\n",
	}, []application.Extension{
		application.DeclareSimpleService[eventbus.EventBus](func(context.Context) (eventbus.EventBus, error) {
			return eventbus.NewMemoryEventBus(), nil
		}),
		journalcontrib.Pack(""), cwmapp.SystemModules(t), journallocal.Extension(),
	}, "alpha", "beta")
	// The journal records on its own goroutine: drain it before the
	// workspace is removed.
	t.Cleanup(func() { appcore.CloseApplicationScope(ctx) })

	// waitDetail reads the detail of id until ok (the journal's observer
	// writes after the event).
	waitDetail := func(id string, ok func(api.DetailOutput) bool) api.DetailOutput {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for {
			d, err := svc.Detail(ctx, api.DetailInput{ID: id})
			if err != nil {
				t.Fatalf("detail %s: %v", id, err)
			}
			if ok(d) || time.Now().After(deadline) {
				return d
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
	created := func(d api.DetailOutput) bool { return !d.CreatedAt.IsZero() }

	createIn(t, ctx, cwm, "alpha", cwmapi.Object{ID: "m.F-1", Type: "thing", Name: "alpha feature"})
	alphaCreated := waitDetail("alpha:m.F-1", created).CreatedAt
	time.Sleep(50 * time.Millisecond) // beta's is created later
	createIn(t, ctx, cwm, "beta", cwmapi.Object{ID: "m.F-1", Type: "thing", Name: "beta feature"})
	betaCreated := waitDetail("beta:m.F-1", created).CreatedAt
	time.Sleep(50 * time.Millisecond) // and accepted later still
	if _, err := cwm.ObjectTransition(ctx, cwmapi.ObjectTransitionInput{ID: "beta:m.F-1", To: "accepted", By: cwmapi.Actor{ID: "p", Kind: "person"}}); err != nil {
		t.Fatal(err)
	}
	beta := waitDetail("beta:m.F-1", func(d api.DetailOutput) bool { return d.UpdatedAt.After(betaCreated) })
	time.Sleep(200 * time.Millisecond) // a stray record would land by now

	if alphaCreated.IsZero() || !betaCreated.After(alphaCreated) {
		t.Fatalf("created alpha %s, beta %s: want beta's after alpha's", alphaCreated, betaCreated)
	}
	alpha := waitDetail("alpha:m.F-1", created)
	if !alpha.CreatedAt.Equal(alphaCreated) || !alpha.UpdatedAt.Equal(alphaCreated) {
		t.Errorf("alpha:m.F-1 created %s changed %s, want its own creation %s for both", alpha.CreatedAt, alpha.UpdatedAt, alphaCreated)
	}
	if !beta.CreatedAt.Equal(betaCreated) || !beta.UpdatedAt.After(betaCreated) {
		t.Errorf("beta:m.F-1 created %s changed %s, want its own creation %s and its accept after", beta.CreatedAt, beta.UpdatedAt, betaCreated)
	}

	for _, c := range []struct {
		id   string
		want []string
	}{{"alpha:m.F-1", nil}, {"beta:m.F-1", []string{"accepted"}}} {
		out, err := svc.TransitionOptions(ctx, api.TransitionOptionsInput{ID: c.id, By: cwmapi.Actor{ID: "p", Kind: "person"}})
		if err != nil || len(out.Lifecycles) != 1 {
			t.Fatalf("%s options: %+v, %v", c.id, out, err)
		}
		var moves []string
		for _, e := range out.Lifecycles[0].History {
			moves = append(moves, e.To)
		}
		if len(moves) != len(c.want) || (len(moves) == 1 && moves[0] != c.want[0]) {
			t.Errorf("%s history %v, want its own project's %v", c.id, moves, c.want)
		}
	}
}
