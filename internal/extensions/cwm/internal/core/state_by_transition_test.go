// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"errors"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// A STATE CHANGES ONLY BY A TRANSITION. An empty state is the workflow's
// initial one: an update leaving it, or a capability's state appearing
// anywhere but in its initial state, is refused like any other state change;
// a write storing a state its workflow does not have is refused unless the
// object already had it.

func stateFixture(t *testing.T) (*Service, stdctx.Context) {
	t.Helper()
	cfg := &CWMServiceConfig{WorkspaceRoot: freshWorkspace(t), ProjectRoot: t.TempDir(), ObjectTypeValidation: api.TypeValidationError}
	cfg.CatalogTypes.Capabilities = []api.Capability{
		{Name: "lifecycle"},
		{Name: "review", Workflow: &api.CapabilityWorkflow{Name: "review", State: "status", History: "log"}},
	}
	cfg.CatalogTypes.Workflows = []api.Workflow{
		{Name: "flow", Initial: "proposed",
			States: []api.WorkflowState{{Name: "proposed"}, {Name: "accepted"}, {Name: "done", Terminal: true}},
			Transitions: []api.WorkflowTransition{
				{Name: "accept", From: []string{"proposed"}, To: "accepted", Gate: "by.kind == 'person'"},
				{Name: "finish", From: []string{"accepted"}, To: "done"}}},
		{Name: "review", Initial: "open",
			States:      []api.WorkflowState{{Name: "open"}, {Name: "closed", Terminal: true}},
			Transitions: []api.WorkflowTransition{{Name: "close", From: []string{"open"}, To: "closed"}}},
	}
	cfg.CatalogTypes.Objects = []api.ObjectType{{Name: "goal", Capabilities: []string{"lifecycle"}, Workflow: "flow"}}
	svc, err := New(stdctx.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	ctx := stdctx.Background()
	if _, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "m", Object: api.Object{ID: "m.g", Type: "goal", Name: "g"}}); err != nil {
		t.Fatal(err)
	}
	return svc, ctx
}

func update(t *testing.T, svc *Service, ctx stdctx.Context, edit func(*api.Object)) error {
	t.Helper()
	got, err := svc.ObjectGet(ctx, api.ObjectGetInput{Module: "m", ID: "m.g"})
	if err != nil {
		t.Fatal(err)
	}
	o := got.Object
	edit(&o)
	_, err = svc.ObjectUpdate(ctx, api.ObjectUpdateInput{Module: "m", Object: o})
	return err
}

func refusedTransitionOnly(err error) bool {
	return errors.Is(err, api.ErrInvalidInput) && strings.Contains(err.Error(), "changes only through ObjectTransition")
}

// A new object has no stored state: writing one past the initial is refused;
// writing the initial itself changes nothing and passes.
func TestANewObjectsStateIsNotWrittenPastTheInitial(t *testing.T) {
	svc, ctx := stateFixture(t)
	for _, to := range []string{"done", "accepted"} {
		if err := update(t, svc, ctx, func(o *api.Object) { o.SetLifecycle(api.Lifecycle{State: to}) }); !refusedTransitionOnly(err) {
			t.Errorf("update a new goal to %s: %v — want refused, only a transition moves it", to, err)
		}
	}
	if err := update(t, svc, ctx, func(o *api.Object) { o.SetLifecycle(api.Lifecycle{State: "proposed"}) }); err != nil {
		t.Errorf("writing the initial state: %v", err)
	}
	if _, err := svc.ObjectTransition(ctx, api.ObjectTransitionInput{ID: "m.g", To: "accepted", By: api.Actor{ID: "ana", Kind: api.ActorPerson}}); err != nil {
		t.Fatalf("the transition still moves it: %v", err)
	}
	if err := update(t, svc, ctx, func(o *api.Object) { o.SetLifecycle(api.Lifecycle{State: "done"}) }); !refusedTransitionOnly(err) {
		t.Errorf("update an accepted goal to done: %v", err)
	}
}

// A capability's own workflow: its group appearing in a state past the
// initial, or leaving an empty state by update, is refused; given in its
// initial state, it starts.
func TestACapabilitysStateIsNotWrittenPastTheInitial(t *testing.T) {
	svc, ctx := stateFixture(t)
	if err := update(t, svc, ctx, func(o *api.Object) { _ = o.SetCapability("review", map[string]any{"status": "closed"}) }); !refusedTransitionOnly(err) {
		t.Errorf("a review group appearing closed: %v", err)
	}
	if err := update(t, svc, ctx, func(o *api.Object) { _ = o.SetCapability("review", map[string]any{"status": ""}) }); err != nil {
		t.Fatalf("a review group with no state yet: %v", err)
	}
	if err := update(t, svc, ctx, func(o *api.Object) { _ = o.SetCapability("review", map[string]any{"status": "closed"}) }); !refusedTransitionOnly(err) {
		t.Errorf("its empty state written closed: %v", err)
	}
	if err := update(t, svc, ctx, func(o *api.Object) { _ = o.SetCapability("review", map[string]any{"status": "open"}) }); err != nil {
		t.Errorf("its initial state: %v", err)
	}
}

// A state the workflow does not have is refused on write — but an object
// stored with one is still writable as long as the write adds no new one.
func TestAStateOutsideItsWorkflowIsRefusedOnWrite(t *testing.T) {
	svc, ctx := stateFixture(t)
	if err := update(t, svc, ctx, func(o *api.Object) { _ = o.SetCapability("review", map[string]any{"status": "bogus"}) }); err == nil || !strings.Contains(err.Error(), `"bogus" is not a state of workflow review`) {
		t.Errorf("a state outside the workflow: %v", err)
	}

	// Stored invalid by hand (a git pull, an edit): reported, still writable.
	path, _ := svc.index().pathOf("m.g")
	got, err := svc.ObjectGet(ctx, api.ObjectGetInput{Module: "m", ID: "m.g"})
	if err != nil {
		t.Fatal(err)
	}
	stored := got.Object
	_ = stored.SetCapability("review", map[string]any{"status": "bogus"})
	if err := WriteObjectFile(path, stored); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ObjectScan(ctx, api.ObjectScanInput{Force: true}); err != nil {
		t.Fatal(err)
	}
	if err := update(t, svc, ctx, func(o *api.Object) { o.Description = "repaired later" }); err != nil {
		t.Errorf("an object already outside its workflow must stay writable: %v", err)
	}
}
