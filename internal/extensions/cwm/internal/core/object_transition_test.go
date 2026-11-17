// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/eventbus"
	actionsapi "github.com/katyoti/katyoti-fwk/internal/extensions/actions/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/internal/events"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/internal/storage"
)

// LIFECYCLES: ObjectTransition is the one way a state changes, and the workflow
// — transitions, gates, terminal states — is what it checks.

var person = api.Actor{ID: "jdoe", Kind: api.ActorPerson}
var agent = api.Actor{ID: "coder-1", Kind: api.ActorAgent}

func approvalWorkflow() api.Workflow {
	return api.Workflow{
		Name: "approval", Initial: "idea",
		States: []api.WorkflowState{
			{Name: "idea"}, {Name: "proposed"}, {Name: "in-review"}, {Name: "accepted"},
			{Name: "active"}, {Name: "done", Terminal: true}, {Name: "dropped", Terminal: true},
		},
		Transitions: []api.WorkflowTransition{
			{Name: "propose", From: []string{"idea"}, To: "proposed"},
			{Name: "review", From: []string{"proposed"}, To: "in-review"},
			{Name: "accept", From: []string{"in-review"}, To: "accepted", Gate: "by.kind == 'person'"},
			{Name: "rush", From: []string{"proposed"}, To: "accepted", Gate: "false", Force: api.ForceAnyone},
			{Name: "typo", From: []string{"accepted"}, To: "active", Gate: "bye.kind == 'person'"},
			{Name: "drop", From: []string{"*"}, To: "dropped"},
		},
	}
}

func transitionFixture(t *testing.T, actions actionsapi.ActionService) (*Service, stdctx.Context, *eventbus.MemoryEventBus) {
	t.Helper()
	cfg := &CWMServiceConfig{WorkspaceRoot: freshWorkspace(t), ProjectRoot: t.TempDir()}
	cfg.CatalogTypes.Capabilities = []api.Capability{{Name: "container"}, {Name: "lifecycle"}}
	cfg.CatalogTypes.Objects = []api.ObjectType{{Name: "goal", Capabilities: []string{"container", "lifecycle"}, Workflow: "approval"}}
	cfg.CatalogTypes.Workflows = []api.Workflow{approvalWorkflow()}
	svc, err := New(stdctx.Background(), cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	bus := eventbus.NewMemoryEventBus()
	svc.Events = events.New("t", "u", "w", bus, nil)
	app := application.New(application.Config())
	if actions != nil {
		application.PutService[actionsapi.ActionService](app, "default", actions)
	}
	ctx := application.WithApplication(stdctx.Background(), app)
	g := api.Object{ID: "billing.g1", Type: "goal"}
	g.Name = "g1"
	if _, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "billing", Object: g}); err != nil {
		t.Fatalf("create: %v", err)
	}
	return svc, ctx, bus
}

func move(t *testing.T, svc *Service, ctx stdctx.Context, to string, by api.Actor) (api.ObjectTransitionOutput, error) {
	t.Helper()
	return svc.ObjectTransition(ctx, api.ObjectTransitionInput{ID: "billing.g1", To: to, By: by, Note: "because"})
}

// TestATransitionWritesTheStateAndOneEvent: a new object starts in the
// workflow's initial state; the transition writes the state — and only the
// state: who and why are the event's (the journal records it as the object's
// history) — and publishes one transitioned event.
func TestATransitionWritesTheStateAndOneEvent(t *testing.T) {
	svc, ctx, bus := transitionFixture(t, nil)
	var c captured
	_ = bus.Subscribe(api.ObjectTransitionedSubject("t", "u", "w", api.SubjectToken("billing.g1")), c.on)
	out, err := move(t, svc, ctx, "proposed", agent)
	if err != nil || out.From != "idea" || out.To != "proposed" || out.Transition != "propose" {
		t.Fatalf("transition = %+v (%v)", out, err)
	}
	got, _ := svc.ObjectGet(ctx, api.ObjectGetInput{Module: "billing", ID: "billing.g1"})
	if g := got.Object.Capabilities[api.CapLifecycle]; len(g) != 1 || g["state"] != "proposed" {
		t.Fatalf("stored lifecycle = %+v, want the state alone", g)
	}
	evs := c.all()
	if len(evs) != 1 {
		t.Fatalf("transitioned events = %d, want 1", len(evs))
	}
	if e := evs[0].(events.ObjectTransitionedPayload); e.From != "idea" || e.To != "proposed" || e.By != agent || e.Note != "because" || e.Workflow != "approval" {
		t.Fatalf("event = %+v", e)
	}
}

// TestAGateDecidesWhoMayMove: only a person may accept.
func TestAGateDecidesWhoMayMove(t *testing.T) {
	svc, ctx, _ := transitionFixture(t, nil)
	for _, to := range []string{"proposed", "in-review"} {
		if _, err := move(t, svc, ctx, to, agent); err != nil {
			t.Fatalf("to %s: %v", to, err)
		}
	}
	if _, err := move(t, svc, ctx, "accepted", agent); !errors.Is(err, api.ErrTransitionRefused) {
		t.Fatalf("an agent accepted: err = %v, want refused", err)
	}
	// accept does not allow force: forcing it changes nothing.
	if _, err := svc.ObjectTransition(ctx, api.ObjectTransitionInput{ID: "billing.g1", To: "accepted", By: agent, Force: true}); !errors.Is(err, api.ErrTransitionRefused) {
		t.Fatalf("an agent forced a transition that allows no force: err = %v, want refused", err)
	}
	if _, err := move(t, svc, ctx, "accepted", person); err != nil {
		t.Fatalf("a person could not accept: %v", err)
	}
}

// TestNoTransitionNoMoveAndTerminalIsFinal: a move the workflow does not have
// is refused, and nothing leaves a terminal state.
func TestNoTransitionNoMoveAndTerminalIsFinal(t *testing.T) {
	svc, ctx, _ := transitionFixture(t, nil)
	if _, err := move(t, svc, ctx, "active", person); !errors.Is(err, api.ErrTransitionRefused) {
		t.Fatalf("idea → active: err = %v, want refused (no such transition)", err)
	}
	if _, err := move(t, svc, ctx, "dropped", person); err != nil {
		t.Fatalf("drop: %v", err)
	}
	if _, err := move(t, svc, ctx, "dropped", person); !errors.Is(err, api.ErrTransitionRefused) {
		t.Fatalf("out of a terminal state: err = %v, want refused", err)
	}
}

// TestForceIsAllowedOnlyWhereTheTransitionSaysAndIsRecorded.
func TestForceIsAllowedOnlyWhereTheTransitionSaysAndIsRecorded(t *testing.T) {
	svc, ctx, _ := transitionFixture(t, nil)
	if _, err := move(t, svc, ctx, "proposed", person); err != nil {
		t.Fatal(err)
	}
	if _, err := move(t, svc, ctx, "accepted", person); !errors.Is(err, api.ErrTransitionRefused) {
		t.Fatalf("rush without force: err = %v, want refused", err)
	}
	out, err := svc.ObjectTransition(ctx, api.ObjectTransitionInput{ID: "billing.g1", To: "accepted", Transition: "rush", By: person, Force: true})
	if err != nil || !out.Forced {
		t.Fatalf("forced rush = %+v (%v), want forced", out, err)
	}
	got, _ := svc.ObjectGet(ctx, api.ObjectGetInput{Module: "billing", ID: "billing.g1"})
	if life, _ := got.Object.Lifecycle(); life.State != "accepted" {
		t.Fatalf("the forced pass did not move: %+v (it is recorded on its event, the journal's)", life)
	}
}

// TestAGateTypoIsRefusedNotReadAsNil: `bye.kind` names nothing the vocabulary
// declares — refused, never a gate that silently passes or fails.
func TestAGateTypoIsRefusedNotReadAsNil(t *testing.T) {
	svc, ctx, _ := transitionFixture(t, nil)
	for _, to := range []string{"proposed", "in-review"} {
		_, _ = move(t, svc, ctx, to, person)
	}
	_, _ = move(t, svc, ctx, "accepted", person)
	_, err := move(t, svc, ctx, "active", person)
	if !errors.Is(err, api.ErrTransitionRefused) || !strings.Contains(err.Error(), "bye") {
		t.Fatalf("a gate naming an undeclared variable: err = %v, want refused", err)
	}
}

// TestTheStateChangesOnlyThroughATransition: an update carrying another state
// is refused; one carrying the same state is fine.
func TestTheStateChangesOnlyThroughATransition(t *testing.T) {
	svc, ctx, _ := transitionFixture(t, nil)
	if _, err := move(t, svc, ctx, "proposed", person); err != nil {
		t.Fatal(err)
	}
	got, _ := svc.ObjectGet(ctx, api.ObjectGetInput{Module: "billing", ID: "billing.g1"})
	same := got.Object
	if _, err := svc.ObjectUpdate(ctx, api.ObjectUpdateInput{Module: "billing", Object: same}); err != nil {
		t.Fatalf("an update keeping the state was refused: %v", err)
	}
	cheat := got.Object
	cheat.Capabilities = cloneCaps(got.Object.Capabilities)
	life, _ := cheat.Lifecycle()
	life.State = "accepted"
	cheat.SetLifecycle(life)
	if _, err := svc.ObjectUpdate(ctx, api.ObjectUpdateInput{Module: "billing", Object: cheat}); !errors.Is(err, api.ErrInvalidInput) {
		t.Fatalf("a raw state change: err = %v, want refused", err)
	}
}

// TestOnTransitionMayRefuse: an on_transition hook that refuses stops the move.
func TestOnTransitionMayRefuse(t *testing.T) {
	f := &hookActions{replies: map[string]string{"x:gate/transition": `{"accept":false,"reason":"frozen for the release"}`}}
	svc, ctx, _ := transitionFixture(t, f)
	g, _ := svc.ObjectGet(ctx, api.ObjectGetInput{Module: "billing", ID: "billing.g1"})
	o := g.Object
	o.Hooks = api.Hooks{api.HookOnTransition: {{URI: "x:gate/transition"}}}
	if _, err := svc.ObjectUpdate(ctx, api.ObjectUpdateInput{Module: "billing", Object: o}); err != nil {
		t.Fatal(err)
	}
	if _, err := move(t, svc, ctx, "proposed", person); !errors.Is(err, api.ErrRefusedByHook) {
		t.Fatalf("err = %v, want refused by the on_transition hook", err)
	}
	got, _ := svc.ObjectGet(ctx, api.ObjectGetInput{Module: "billing", ID: "billing.g1"})
	if life, _ := got.Object.Lifecycle(); life.State != "" {
		t.Fatalf("a refused transition changed the state: %+v", life)
	}
}

// shippedWorkflows reads the workflows ktw-methods ships — the files, not a
// fixture, so a broken shipped definition fails here.
func shippedWorkflows(t *testing.T) []api.Workflow {
	t.Helper()
	dir := filepath.Join("..", "..", "..", "..", "..", "app", "ktw-methods", "embed", "cwm")
	flows, err := storage.LoadWorkflowsLayered(os.DirFS(dir))
	if err != nil || len(flows) < 2 {
		t.Fatalf("shipped workflows = %d (%v)", len(flows), err)
	}
	return flows
}

func shippedFixture(t *testing.T, workflow string) (*Service, stdctx.Context) {
	t.Helper()
	cfg := &CWMServiceConfig{WorkspaceRoot: freshWorkspace(t), ProjectRoot: t.TempDir()}
	cfg.CatalogTypes.Capabilities = []api.Capability{{Name: "container"}, {Name: "lifecycle"}}
	cfg.CatalogTypes.Objects = []api.ObjectType{{Name: "goal", Capabilities: []string{"container", "lifecycle"}, Workflow: workflow}}
	cfg.CatalogTypes.Workflows = shippedWorkflows(t)
	svc, err := New(stdctx.Background(), cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	g := api.Object{ID: "billing.g1", Type: "goal"}
	g.Name = "g1"
	if _, err := svc.ObjectCreate(stdctx.Background(), api.ObjectCreateInput{Module: "billing", Object: g}); err != nil {
		t.Fatalf("create: %v", err)
	}
	return svc, stdctx.Background()
}

// TestShippedGoalOnlyAPersonAccepts: the shipped `goal` workflow — an agent
// proposes and puts up for review, only a person accepts, dropped is final.
func TestShippedGoalOnlyAPersonAccepts(t *testing.T) {
	svc, ctx := shippedFixture(t, "goal")
	for _, to := range []string{"proposed", "in-review"} {
		if _, err := move(t, svc, ctx, to, agent); err != nil {
			t.Fatalf("agent → %s: %v", to, err)
		}
	}
	if _, err := move(t, svc, ctx, "accepted", agent); !errors.Is(err, api.ErrTransitionRefused) {
		t.Fatalf("an agent accepted under the shipped goal: %v", err)
	}
	for _, to := range []string{"accepted", "dropped"} {
		if _, err := move(t, svc, ctx, to, person); err != nil {
			t.Fatalf("person → %s: %v", to, err)
		}
	}
	if _, err := move(t, svc, ctx, "idea", person); !errors.Is(err, api.ErrTransitionRefused) {
		t.Fatalf("out of dropped under goal: %v, want refused (terminal)", err)
	}
}

// TestShippedManualReachesAnyState: the shipped `manual` workflow — any state from
// any state, done included.
func TestShippedManualReachesAnyState(t *testing.T) {
	svc, ctx := shippedFixture(t, "manual")
	for _, to := range []string{"done", "idea", "active", "dropped", "proposed"} {
		if _, err := move(t, svc, ctx, to, agent); err != nil {
			t.Fatalf("manual → %s: %v", to, err)
		}
	}
}

// TestAnObjectIsBornInItsInitialState: creating an object already "accepted"
// would skip every gate; only the initial state (or none) is allowed at birth.
func TestAnObjectIsBornInItsInitialState(t *testing.T) {
	svc, ctx, _ := transitionFixture(t, nil)
	born := func(id, state string) error {
		o := api.Object{ID: id, Type: "goal"}
		o.Name = id
		o.SetLifecycle(api.Lifecycle{State: state})
		_, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "billing", Object: o})
		return err
	}
	if err := born("billing.g2", "accepted"); !errors.Is(err, api.ErrInvalidInput) {
		t.Fatalf("created already accepted: err = %v, want refused", err)
	}
	if err := born("billing.g3", "idea"); err != nil {
		t.Fatalf("created in the initial state: %v", err)
	}
}

// TestShippedGoalRethinkIsAPersonsOnly: the shipped `goal` workflow — a person
// sends a goal back to idea from any stage it is not retired from, an agent
// may not, and a retired goal stays retired. Its decompose guard calls
// gate('ready-to-plan'), answered here as not passing: a person forces it.
func TestShippedGoalRethinkIsAPersonsOnly(t *testing.T) {
	cfg := &CWMServiceConfig{WorkspaceRoot: freshWorkspace(t), ProjectRoot: t.TempDir()}
	cfg.CatalogTypes.Capabilities = []api.Capability{{Name: "container"}, {Name: "lifecycle"},
		{Name: "gates", Functions: []api.CapabilityFunction{
			{Name: "gate", Action: "x:gates/check", Args: []string{"name"}, Returns: api.FunctionReturnsObject}}}}
	cfg.CatalogTypes.Objects = []api.ObjectType{{Name: "goal", Capabilities: []string{"container", "lifecycle", "gates"}, Workflow: "goal"}}
	cfg.CatalogTypes.Workflows = shippedWorkflows(t)
	svc, err := New(stdctx.Background(), cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	acts := newFnActions()
	acts.replies["x:gates/check"] = `{"passes": false}`
	app := application.New(application.Config())
	application.PutService[actionsapi.ActionService](app, "default", acts)
	ctx := application.WithApplication(stdctx.Background(), app)
	g := api.Object{ID: "billing.g1", Type: "goal"}
	g.Name = "g1"
	if _, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "billing", Object: g}); err != nil {
		t.Fatalf("create: %v", err)
	}
	step := func(to string, by api.Actor, force bool) error {
		_, err := svc.ObjectTransition(ctx, api.ObjectTransitionInput{ID: "billing.g1", To: to, By: by, Note: "because", Force: force})
		return err
	}
	climb := []string{"proposed", "in-review", "accepted", "decomposed", "active"}
	for i, stage := range climb {
		for _, to := range climb[:i+1] {
			if err := step(to, person, to == "decomposed"); err != nil {
				t.Fatalf("person → %s: %v", to, err)
			}
		}
		if err := step("idea", agent, false); !errors.Is(err, api.ErrTransitionRefused) {
			t.Fatalf("an agent rethought a goal from %s: %v, want refused", stage, err)
		}
		if err := step("idea", person, false); err != nil {
			t.Fatalf("person rethink from %s: %v", stage, err)
		}
	}
	if err := step("dropped", person, false); err != nil {
		t.Fatalf("drop: %v", err)
	}
	if err := step("idea", person, false); !errors.Is(err, api.ErrTransitionRefused) {
		t.Fatalf("rethink out of dropped: %v, want refused (terminal)", err)
	}
}
