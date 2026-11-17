// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// ObjectSearch answers the lifecycle state an object is in — the stored one,
// else its workflow's initial one — coloured by the lifecycle badge, and keeps
// only the objects in the states asked: a list's state filter and its state
// badge read the same answer.
func TestObjectSearchAnswersAndFiltersLifecycleStates(t *testing.T) {
	cfg := &CWMServiceConfig{WorkspaceRoot: freshWorkspace(t), ProjectRoot: t.TempDir()}
	cfg.CatalogTypes.Capabilities = []api.Capability{
		{Name: "container"},
		{Name: api.CapLifecycle, Badges: []api.BadgeDef{{Field: "state", Colors: map[string]string{"idea": "gray", "done": "green"}}}},
	}
	cfg.CatalogTypes.Workflows = []api.Workflow{{
		Name: "ladder", Initial: "idea",
		States:      []api.WorkflowState{{Name: "idea"}, {Name: "done"}},
		Transitions: []api.WorkflowTransition{{Name: "finish", From: []string{"idea"}, To: "done"}},
	}}
	cfg.CatalogTypes.Objects = []api.ObjectType{
		{Name: "goal", Capabilities: []string{"container", api.CapLifecycle}, Workflow: "ladder"},
		{Name: "note", Capabilities: []string{"container"}},
	}
	svc, err := New(stdctx.Background(), cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	ctx := stdctx.Background()
	for _, o := range []api.Object{
		{ID: "m.new", Type: "goal", Name: "never moved"},
		{ID: "m.old", Type: "goal", Name: "finished"},
		{ID: "m.note", Type: "note", Name: "no workflow"},
	} {
		if _, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "m", Object: o}); err != nil {
			t.Fatalf("create %s: %v", o.ID, err)
		}
	}
	if _, err := svc.ObjectTransition(ctx, api.ObjectTransitionInput{ID: "m.old", To: "done", By: api.Actor{ID: "p", Kind: "person"}}); err != nil {
		t.Fatalf("transition: %v", err)
	}

	got, err := svc.ObjectSearch(ctx, api.ObjectSearchInput{WithStates: true})
	if err != nil {
		t.Fatal(err)
	}
	if s := got.States["m.new"]; s.State != "idea" || s.Color != "gray" {
		t.Errorf("never moved: %+v, want the workflow's initial state, gray", s)
	}
	if s := got.States["m.old"]; s.State != "done" || s.Color != "green" {
		t.Errorf("moved: %+v, want done, green", s)
	}
	if _, ok := got.States["m.note"]; ok {
		t.Errorf("an object following no workflow got a state: %+v", got.States["m.note"])
	}
	if plain, _ := svc.ObjectSearch(ctx, api.ObjectSearchInput{}); plain.States != nil {
		t.Errorf("states answered unasked: %v", plain.States)
	}

	ids := func(out api.ObjectSearchOutput) string {
		var s []string
		for _, o := range out.Objects {
			s = append(s, o.ID)
		}
		return strings.Join(s, ",")
	}
	if got, _ := svc.ObjectSearch(ctx, api.ObjectSearchInput{States: []string{"idea"}}); ids(got) != "m.new" {
		t.Errorf("states [idea] = %s, want the never-moved goal only", ids(got))
	}
	if got, _ := svc.ObjectSearch(ctx, api.ObjectSearchInput{States: []string{"done", "idea"}}); ids(got) != "m.new,m.old" {
		t.Errorf("states [done idea] = %s", ids(got))
	}
}
