// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	actionsapi "github.com/katyoti/katyoti-fwk/internal/extensions/actions/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// A MOVE KEEPS THE OBJECT. Renamed or refiled into another module, an object
// past its initial state keeps its state and history — the move is not a
// creation, so it is not sent back to the initial state, and no create or
// delete hook fires — and what pointed at it follows.

func moveFixture(t *testing.T) (*Service, stdctx.Context, *fnActions) {
	t.Helper()
	acts := newFnActions()
	acts.replies["x:hook/before"] = `{"accept":true}`
	cfg := &CWMServiceConfig{WorkspaceRoot: freshWorkspace(t), ProjectRoot: t.TempDir(), ObjectTypeValidation: api.TypeValidationError}
	cfg.CatalogTypes.Capabilities = []api.Capability{{Name: "lifecycle"}}
	cfg.CatalogTypes.Workflows = []api.Workflow{{Name: "flow", Initial: "proposed",
		States:      []api.WorkflowState{{Name: "proposed"}, {Name: "accepted"}},
		Transitions: []api.WorkflowTransition{{Name: "accept", From: []string{"proposed"}, To: "accepted"}}}}
	cfg.CatalogTypes.Objects = []api.ObjectType{
		{Name: "module"},
		{Name: "goal", Capabilities: []string{"lifecycle"}, Workflow: "flow", Hooks: api.Hooks{
			api.HookBeforeCreate: {{URI: "x:hook/before"}},
			api.HookOnCreate:     {{URI: "x:hook/created"}},
			api.HookOnDelete:     {{URI: "x:hook/deleted"}},
		}},
		{Name: "note"},
	}
	cfg.CatalogTypes.Edges = []api.EdgeTypeRegistration{{Name: "about", From: "any", To: "any", Cardinality: "M:N", Target: "object", ObjectStorage: "edges"}}
	svc, err := New(stdctx.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	app := application.New(application.Config())
	application.PutService[actionsapi.ActionService](app, "default", acts)
	ctx := application.WithApplication(stdctx.Background(), app)
	for _, o := range []api.Object{
		{ID: "a", Type: "module", Name: "a"}, {ID: "b", Type: "module", Name: "b"},
		{ID: "a.g", Type: "goal", Name: "g", Parent: "a"},
		{ID: "a.child", Type: "note", Name: "child", Parent: "a.g"},
		{ID: "a.n", Type: "note", Name: "n", Parent: "a", Edges: []api.ObjectEdge{{Kind: "about", To: "a.g"}}},
	} {
		module, _ := api.ModuleOfObjectID(o.ID)
		if _, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: module, Object: o}); err != nil {
			t.Fatalf("create %s: %v", o.ID, err)
		}
	}
	if _, err := svc.ObjectTransition(ctx, api.ObjectTransitionInput{ID: "a.g", To: "accepted", By: api.Actor{ID: "ana", Kind: api.ActorPerson}, Note: "agreed"}); err != nil {
		t.Fatal(err)
	}
	return svc, ctx, acts
}

func lifecycleOf(t *testing.T, svc *Service, ctx stdctx.Context, id string) api.Lifecycle {
	t.Helper()
	module, _ := api.ModuleOfObjectID(id)
	got, err := svc.ObjectGet(ctx, api.ObjectGetInput{Module: module, ID: id})
	if err != nil {
		t.Fatalf("get %s: %v", id, err)
	}
	life, _ := got.Object.Lifecycle()
	return life
}

func TestAMovedObjectKeepsItsStateAndItsReferrers(t *testing.T) {
	svc, ctx, acts := moveFixture(t)
	settle := func() { time.Sleep(150 * time.Millisecond) } // on_* hooks run detached
	settle()
	before := acts.count("x:hook/before")
	created, deleted := acts.count("x:hook/created"), acts.count("x:hook/deleted")

	// Renamed within its module.
	if _, err := svc.ObjectMove(ctx, api.ObjectMoveInput{ID: "a.g", NewLocalID: "g2", Reason: "clearer name"}); err != nil {
		t.Fatalf("rename an accepted goal: %v", err)
	}
	// Refiled into another module.
	if _, err := svc.ObjectMove(ctx, api.ObjectMoveInput{ID: "a.g2", NewModule: "b", NewParent: "b", Reason: "belongs to b"}); err != nil {
		t.Fatalf("move an accepted goal to another module: %v", err)
	}
	life := lifecycleOf(t, svc, ctx, "b.g2")
	if life.State != "accepted" {
		t.Errorf("after the moves: %+v — want accepted", life)
	}
	// What pointed at it follows: the child's parent and the inbound edge.
	child, _ := svc.ObjectGet(ctx, api.ObjectGetInput{Module: "a", ID: "a.child"})
	if child.Object.Parent != "b.g2" {
		t.Errorf("the child's parent: %q, want b.g2", child.Object.Parent)
	}
	n, _ := svc.ObjectGet(ctx, api.ObjectGetInput{Module: "a", ID: "a.n"})
	if len(n.Object.Edges) != 1 || n.Object.Edges[0].To != "b.g2" {
		t.Errorf("the inbound edge: %+v, want to b.g2", n.Object.Edges)
	}
	// A move is no birth and no death: no create or delete hook ran.
	settle()
	if d := acts.count("x:hook/before") - before; d != 0 {
		t.Errorf("before_create ran %d times for two moves", d)
	}
	if d := acts.count("x:hook/created") - created; d != 0 {
		t.Errorf("on_create ran %d times for two moves", d)
	}
	if d := acts.count("x:hook/deleted") - deleted; d != 0 {
		t.Errorf("on_delete ran %d times for two moves", d)
	}
}

// The creation path is unchanged: a new object still may not be written
// past its initial state, and its create hooks still run.
func TestACreationIsStillACreation(t *testing.T) {
	svc, ctx, acts := moveFixture(t)
	o := api.Object{ID: "a.late", Type: "goal", Name: "late", Parent: "a"}
	o.SetLifecycle(api.Lifecycle{State: "accepted"})
	if _, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "a", Object: o}); err == nil {
		t.Error("a goal created accepted was let through")
	}
	if acts.count("x:hook/before") == 0 {
		t.Error("before_create no longer runs on a creation")
	}
}
