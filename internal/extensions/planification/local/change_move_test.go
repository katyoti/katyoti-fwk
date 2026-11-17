// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"strings"
	"testing"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// An entry moves to another release: its number kept
// listed and rendered under the release it moved to and no longer under the
// one it left; nothing moves into a dropped or released release, nor out of a
// released one - what it shipped stays in its change log.
func TestAChangeLogEntryMovesToAnotherRelease(t *testing.T) {
	t.Parallel()
	ctx, cwm, _ := world(t)
	ch := changesOf(t, ctx)
	from, _ := changeLogRelease(t, ctx)
	to := plan(t, ctx, releases(t, ctx), api.PlanInput{Version: "1.5.0"})
	plan(t, ctx, releases(t, ctx), api.PlanInput{Version: "1.6.0"})

	added, err := ch.Add(ctx, api.AddChangeInput{Release: from, Title: "Les relances partent seules", Domain: "Interface", Value: api.ValueNotable, Text: "**Ce que ça vous apporte :** une relance par facture."})
	if err != nil {
		t.Fatal(err)
	}
	moved, err := ch.Update(ctx, api.UpdateChangeInput{ID: added.Change.ID, Release: "1.5.0"})
	if err != nil {
		t.Fatal(err)
	}
	if c := moved.Change; c.Release != to.ID || c.Version != "1.5.0" || c.Number != added.Change.Number || c.Title != added.Change.Title || c.Value != api.ValueNotable {
		t.Fatalf("moved: %+v; want under %s, number %d and everything else kept", c, to.ID, added.Change.Number)
	}
	count := func(rel string) int {
		out, err := ch.List(ctx, api.ListChangesInput{Release: rel})
		if err != nil {
			t.Fatal(err)
		}
		return len(out.Changes)
	}
	if count(from) != 0 || count(to.ID) != 1 {
		t.Errorf("listed %d under the release it left, %d under the one it moved to; want 0 and 1", count(from), count(to.ID))
	}
	left, _ := ch.Render(ctx, api.RenderChangeLogInput{Release: from})
	now, _ := ch.Render(ctx, api.RenderChangeLogInput{Release: to.ID})
	if strings.Contains(left.Markdown, "Les relances partent seules") || !strings.Contains(now.Markdown, "Les relances partent seules") {
		t.Errorf("the pages: the one it left lists it %v, the one it moved to %v; want only the second", strings.Contains(left.Markdown, "Les relances"), strings.Contains(now.Markdown, "Les relances"))
	}
	if _, err := ch.Update(ctx, api.UpdateChangeInput{ID: added.Change.ID, Release: "9.9.9"}); err == nil {
		t.Error("an entry moved to a release that does not exist")
	}

	move := func(id string, states ...string) {
		t.Helper()
		for _, st := range states {
			if _, err := cwm.ObjectTransition(ctx, cwmapi.ObjectTransitionInput{ID: id, To: st, By: person, Note: "for the test", Force: true}); err != nil {
				t.Fatal(err)
			}
		}
	}
	move(from, api.ReleaseDropped)
	if _, err := ch.Update(ctx, api.UpdateChangeInput{ID: added.Change.ID, Release: from}); err == nil || !strings.Contains(err.Error(), "dropped") {
		t.Errorf("moved into a dropped release: %v; want refused", err)
	}
	move(to.ID, api.ReleaseFrozen, api.ReleaseReleased)
	if _, err := ch.Update(ctx, api.UpdateChangeInput{ID: added.Change.ID, Release: "1.6.0"}); err == nil || !strings.Contains(err.Error(), "released") {
		t.Errorf("moved out of a released release: %v; want refused", err)
	}
	if _, err := ch.Update(ctx, api.UpdateChangeInput{ID: added.Change.ID, Maturity: "Disponible"}); err != nil {
		t.Errorf("an update that moves nothing, in a released release: %v; want it kept as before", err)
	}
}
