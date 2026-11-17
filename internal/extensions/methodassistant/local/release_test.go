// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/methodassistant/api"
	macore "github.com/katyoti/katyoti-fwk/internal/extensions/methodassistant/internal/core"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// WHAT'S NEXT ON A RELEASE keeps in sight only the work aimed at it: the goal
// its scope holds, the feature serving that goal and its sub-feature, their
// criteria; a backlog feature (aimed at no release) and a criterion under no
// feature fall out. Without a release everything is there; a release whose
// scope cannot be read is refused; the page offers the releases to narrow to.
func TestWhatsNextOnAReleaseKeepsOnlyTheWorkAimedAtIt(t *testing.T) {
	ctx, cwm := cwmapp.New(t, methodPack(nil))
	cwmapp.Module(t, ctx, cwm, "m")
	g := put(t, ctx, cwm, "g1", "goal", "Customers pay on time", nil)
	f := put(t, ctx, cwm, "f1", "feature", "Invoices go out", nil, cwmapi.ObjectEdge{Kind: "serves", To: g})
	sub := putUnder(t, ctx, cwm, f, "f1-1", "feature", "By mail", nil)
	criterion(t, ctx, cwm, f, "f1-AC1", "An invoice goes out", "pass")
	criterion(t, ctx, cwm, sub, "f1-1-AC1", "It is mailed", "")
	backlog := put(t, ctx, cwm, "f9", "feature", "Someday", nil)
	criterion(t, ctx, cwm, backlog, "f9-AC1", "Some check", "")
	criterion(t, ctx, cwm, "m", "loose-AC1", "Loose", "")

	scopes := map[string][]string{"m.RL-1": {g}}
	svc := macore.New(nil).WithScope(
		func(_ context.Context, id string) ([]string, error) {
			if ids, ok := scopes[id]; ok {
				return ids, nil
			}
			return nil, fmt.Errorf("%s is no release", id)
		},
		func(context.Context, string) ([]macore.ReleaseChoice, error) {
			return []macore.ReleaseChoice{{ID: "m.RL-1", Label: "1.0 - First"}}, nil
		})

	features := func(out api.OverviewOutput) []string {
		var ids []string
		for _, s := range out.Stories {
			ids = append(ids, s.ID)
		}
		slices.Sort(ids)
		return ids
	}
	all, err := svc.Overview(ctx, api.ObjectInput{ID: "m"})
	if err != nil {
		t.Fatal(err)
	}
	if got := features(all); len(got) != 3 {
		t.Fatalf("without a release: features %v; want all three", got)
	}
	scoped, err := svc.Overview(ctx, api.ObjectInput{ID: "m", Release: "m.RL-1"})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := features(scoped), []string{f, sub}; !slices.Equal(got, want) {
		t.Errorf("on m.RL-1: features %v; want %v (the goal's feature and its sub-feature, never the backlog's)", got, want)
	}
	if len(scoped.Goals) != 1 || scoped.Goals[0].ID != g {
		t.Errorf("on m.RL-1: goals %+v; want only %s", scoped.Goals, g)
	}
	if len(scoped.LooseCriteria) != 0 {
		t.Errorf("on m.RL-1: loose criteria %+v; want none", scoped.LooseCriteria)
	}
	if b, _ := json.Marshal(scoped); strings.Contains(string(b), "f9-AC1") {
		t.Error("on m.RL-1: the backlog feature's criterion is still in sight")
	}
	if _, err := svc.Overview(ctx, api.ObjectInput{ID: "m", Release: "m.RL-404"}); err == nil {
		t.Error("a release whose scope cannot be read: accepted; want it refused")
	}
	page, err := svc.NextPage(ctx, api.ObjectInput{ID: "m", Release: "m.RL-1"})
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := json.Marshal(page.DSLTree); !strings.Contains(string(b), `"m.RL-1"`) || !strings.Contains(string(b), "1.0 - First") {
		t.Error("the page offers no release to narrow to")
	}
}
