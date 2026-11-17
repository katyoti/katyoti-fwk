// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"errors"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	actionsapi "github.com/katyoti/katyoti-fwk/internal/extensions/actions/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// twoWaysFixture is a workflow with two transitions to one state, as a
// feature's realized -> done: confirm (a person) first, verify (a check) next.
func twoWaysFixture(t *testing.T, verifyGate string) (*Service, stdctx.Context) {
	t.Helper()
	cfg := &CWMServiceConfig{WorkspaceRoot: freshWorkspace(t), ProjectRoot: t.TempDir()}
	cfg.CatalogTypes.Capabilities = []api.Capability{
		{Name: "lifecycle"},
		{Name: "gates", Functions: []api.CapabilityFunction{{Name: "score", Action: "x:score/get", Returns: api.FunctionReturnsNumber}}},
	}
	cfg.CatalogTypes.Workflows = []api.Workflow{{Name: "flow", Initial: "realized",
		States: []api.WorkflowState{{Name: "realized"}, {Name: "done"}},
		Transitions: []api.WorkflowTransition{
			{Name: "confirm", From: []string{"realized"}, To: "done", Gate: "by.kind == 'person'"},
			{Name: "verify", From: []string{"realized"}, To: "done", Gate: verifyGate},
		}}}
	cfg.CatalogTypes.Objects = []api.ObjectType{{Name: "thing", Capabilities: []string{"lifecycle", "gates"}, Workflow: "flow"}}
	svc, err := New(stdctx.Background(), cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	app := application.New(application.Config())
	application.PutService[actionsapi.ActionService](app, "default", newFnActions())
	ctx := application.WithApplication(stdctx.Background(), app)
	if _, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "m", Object: api.Object{ID: "m.t", Type: "thing", Name: "T"}}); err != nil {
		t.Fatalf("create: %v", err)
	}
	return svc, ctx
}

// A move asked by its target state takes the first matching transition whose
// guard holds: an agent's through verify once its check holds, a person's
// through confirm; a named transition runs only itself; none holding is
// refused on the first one's guard.
func TestAMoveByItsTargetTakesTheFirstTransitionThatHolds(t *testing.T) {
	svc, ctx := twoWaysFixture(t, "score() > 2") // score() answers 3
	out, err := svc.ObjectTransition(ctx, api.ObjectTransitionInput{ID: "m.t", To: "done", By: agent})
	if err != nil {
		t.Fatalf("an agent whose check holds should move it through verify: %v", err)
	}
	if out.Transition != "verify" {
		t.Errorf("moved through %q, want verify", out.Transition)
	}

	svc, ctx = twoWaysFixture(t, "score() > 2")
	if out, err := svc.ObjectTransition(ctx, api.ObjectTransitionInput{ID: "m.t", To: "done", By: ana}); err != nil || out.Transition != "confirm" {
		t.Errorf("a person moves it through confirm, the first that holds: %+v, %v", out, err)
	}

	svc, ctx = twoWaysFixture(t, "score() > 2")
	var refused *api.TransitionRefusedError
	_, err = svc.ObjectTransition(ctx, api.ObjectTransitionInput{ID: "m.t", To: "done", Transition: "confirm", By: agent})
	if !errors.As(err, &refused) || refused.Transition != "confirm" {
		t.Errorf("a named transition runs only itself: %v", err)
	}

	svc, ctx = twoWaysFixture(t, "score() > 5")
	_, err = svc.ObjectTransition(ctx, api.ObjectTransitionInput{ID: "m.t", To: "done", By: agent})
	if !errors.As(err, &refused) || refused.Transition != "confirm" {
		t.Errorf("neither holding, the first is refused: %v", err)
	}
}
