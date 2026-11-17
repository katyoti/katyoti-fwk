// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/render"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// WHERE AN INCREMENT STANDS, READ BY EVERY VIEW - its
// summary, its pane and the release's rows say the same standing as it moves:
// defined, on hold, done, then shipped once its release is released - a done
// increment never reads running.
func TestEveryViewSaysWhereAnIncrementStands(t *testing.T) {
	t.Parallel()
	ctx, cwm, _ := world(t)
	inc := increments(t, ctx)
	rel := releases(t, ctx)
	r := plan(t, ctx, rel, api.PlanInput{Module: "billing", Version: "1.4.0"})
	in := increment(t, ctx, "Invoices reach their customers by mail")
	uc := usecase(t, ctx, cwm, "Send invoices by mail")
	if _, err := inc.Target(ctx, api.IncrementTargetInput{ID: uc, Increment: in.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := inc.ShipIn(ctx, api.ShipInInput{ID: in.ID, In: r.ID}); err != nil {
		t.Fatal(err)
	}
	standing := func(when, want string) {
		t.Helper()
		sum, err := inc.Summary(ctx, api.GetInput{ID: in.ID})
		if err != nil {
			t.Fatal(err)
		}
		if sum.Standing != want {
			t.Errorf("%s: its summary says %q; want %q", when, sum.Standing, want)
		}
		label, _ := api.StandingLabel(want, false)
		page, err := inc.PanePage(ctx, api.IncrementPaneInput{ID: in.ID, Tab: "page"})
		if err != nil {
			t.Fatal(err)
		}
		if got := pageJSON(t, page); !strings.Contains(got, `"text":"`+want+`"`) && !strings.Contains(got, `"text":"`+want+` · `) {
			t.Errorf("%s: its pane does not say %q", when, want)
		}
		rp, err := releasePane(ctx, rel, r.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got := pageJSON(t, rp); want != api.StandingOnHold && !strings.Contains(got, `"state":"`+label+`"`) {
			t.Errorf("%s: its release's row does not say %q", when, label)
		}
	}
	standing("defined", api.StandingDefined)

	if _, err := inc.Hold(ctx, api.IncrementHoldInput{ID: in.ID, Reason: "the mail provider is not chosen", By: person}); err != nil {
		t.Fatal(err)
	}
	standing("held", api.StandingOnHold)
	if _, err := inc.Resume(ctx, api.IncrementHoldInput{ID: in.ID, By: person}); err != nil {
		t.Fatal(err)
	}
	if _, err := inc.ShipIn(ctx, api.ShipInInput{ID: in.ID, In: r.ID}); err != nil {
		t.Fatal(err)
	}

	if _, err := rel.Freeze(ctx, api.MoveInput{ID: r.ID, By: intake}); err != nil {
		t.Fatal(err)
	}
	changelogListed(t, ctx, cwm, r.ID)
	finish(t, ctx, cwm, uc)
	if _, err := inc.Done(ctx, api.IncrementMoveInput{ID: in.ID, By: person, Force: true, Note: forcedNote}); err != nil {
		t.Fatal(err)
	}
	standing("done, its release frozen", api.StandingDone)
	if _, err := rel.Release(ctx, api.MoveInput{ID: r.ID, By: person}); err != nil {
		t.Fatal(err)
	}
	standing("its release released", api.StandingShipped)
}

func pageJSON(t *testing.T, def render.Def) string {
	t.Helper()
	raw, err := json.Marshal(def.DSLTree)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// NO VIEW NAMES A STANDING ITSELF - the words of the
// standings live in api/standing.go only: a view that words one itself
// computes its own, the drift the one function ends.
func TestNoViewWordsAStandingItself(t *testing.T) {
	t.Parallel()
	words := []string{`"en pause"`, `"on hold"`, `"défini"`, `"en livraison"`, `"delivering"`,
		`Version + " — " + next.Name`, `Version + " — " + r.Name`, `v + " — " + o.Name`} // a release named by hand: api.ReleaseLabel
	for _, root := range []string{"..", filepath.Join("..", "..", "portfolio"), filepath.Join("..", "..", "inbox")} {
		if _, err := os.Stat(root); os.IsNotExist(err) {
			continue // a sibling package the tree does not hold (the open-source copy has no portfolio)
		}
		err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") || filepath.Base(path) == "standing.go" {
				return err
			}
			b, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			for _, w := range words {
				if strings.Contains(string(b), w) {
					t.Errorf("%s words a standing itself (%s): read it from api.StandingLabel", path, w)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}
