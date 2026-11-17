// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	actionsapi "github.com/katyoti/katyoti-fwk/internal/extensions/actions/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// EXPRESSION FUNCTIONS (b6): a capability's function is callable in a guard, a
// gate check, an action `when:` and a view; called once per evaluation; a
// failure refuses a guard and is never a pass; only read-only actions answer;
// a hook's `when:` and a task's `filter:` cannot call one.

// fnActions answers functions: a reply per action URI, read-only unless
// listed in writes, slow when listed in slow.
type fnActions struct {
	mu      sync.Mutex
	replies map[string]string
	writes  map[string]bool
	slow    map[string]time.Duration
	calls   map[string]int
	seen    []api.FunctionCall
}

func (f *fnActions) Describe(_ stdctx.Context, in actionsapi.DescribeInput) (actionsapi.DescribeOutput, error) {
	return actionsapi.DescribeOutput{URI: in.URI, ReadOnly: !f.writes[in.URI]}, nil
}

func (f *fnActions) Run(ctx stdctx.Context, in actionsapi.RunInput) (actionsapi.RunOutput, error) {
	var c api.FunctionCall
	_ = json.Unmarshal(in.Input, &c)
	f.mu.Lock()
	f.calls[in.URI]++
	f.seen = append(f.seen, c)
	d := f.slow[in.URI]
	f.mu.Unlock()
	if d > 0 {
		select {
		case <-time.After(d):
		case <-ctx.Done():
			return actionsapi.RunOutput{}, ctx.Err()
		}
	}
	return actionsapi.RunOutput{URI: in.URI, Value: json.RawMessage(f.replies[in.URI])}, nil
}

func (f *fnActions) count(uri string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[uri]
}

func newFnActions() *fnActions {
	return &fnActions{
		replies: map[string]string{"x:gates/check": "true", "x:score/get": "3", "x:write/it": "true", "x:slow/one": "true", "x:heavy/get": "true"},
		writes:  map[string]bool{"x:write/it": true, "x:heavy/get": true},
		slow:    map[string]time.Duration{"x:slow/one": 2 * time.Second},
		calls:   map[string]int{},
	}
}

// fnFixture: a `gates` capability declaring four functions; a `thing` type
// carrying it, whose workflow moves proposed → accepted under a guard.
func fnFixture(t *testing.T, guard string, actions actionsapi.ActionService) (*Service, stdctx.Context) {
	t.Helper()
	cfg := &CWMServiceConfig{WorkspaceRoot: freshWorkspace(t), ProjectRoot: t.TempDir()}
	cfg.CatalogTypes.Capabilities = []api.Capability{
		{Name: "lifecycle"},
		{Name: "gates", Functions: []api.CapabilityFunction{
			{Name: "gate", Action: "x:gates/check", Args: []string{"name"}, Returns: api.FunctionReturnsBoolean},
			{Name: "score", Action: "x:score/get", Returns: api.FunctionReturnsNumber},
			{Name: "writer", Action: "x:write/it", Returns: api.FunctionReturnsBoolean},
			{Name: "slow", Action: "x:slow/one", Timeout: "50ms"},
			{Name: "heavy", Action: "x:heavy/get", Returns: api.FunctionReturnsBoolean, Costly: true},
		}},
	}
	cfg.CatalogTypes.Workflows = []api.Workflow{{Name: "flow", Initial: "proposed",
		States:      []api.WorkflowState{{Name: "proposed"}, {Name: "accepted"}},
		Transitions: []api.WorkflowTransition{{Name: "accept", From: []string{"proposed"}, To: "accepted", Gate: guard}}}}
	cfg.CatalogTypes.Objects = []api.ObjectType{{Name: "thing", Capabilities: []string{"lifecycle", "gates"}, Workflow: "flow",
		Actions: []api.ActionRef{{URI: "x:act/one", When: "gate('ready') && score() > 2"}}}}
	svc, err := New(stdctx.Background(), cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	app := application.New(application.Config())
	if actions != nil {
		application.PutService[actionsapi.ActionService](app, "default", actions)
	}
	ctx := application.WithApplication(stdctx.Background(), app)
	o := api.Object{ID: "m.t", Type: "thing", Name: "T"}
	if _, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "m", Object: o}); err != nil {
		t.Fatalf("create: %v", err)
	}
	return svc, ctx
}

func fnMove(svc *Service, ctx stdctx.Context) error {
	_, err := svc.ObjectTransition(ctx, api.ObjectTransitionInput{ID: "m.t", To: "accepted", By: api.Actor{ID: "ana", Kind: "person"}})
	return err
}

func TestAFunctionAnswersInAGuard(t *testing.T) {
	acts := newFnActions()
	svc, ctx := fnFixture(t, "gate('ready') && gate('ready') && score() == 3", acts)
	if err := fnMove(svc, ctx); err != nil {
		t.Fatalf("the guard should hold: %v", err)
	}
	if n := acts.count("x:gates/check"); n != 1 {
		t.Errorf("gate('ready') named twice was called %d times, want once per evaluation", n)
	}
	if len(acts.seen) == 0 || acts.seen[0].Object != "m.t" || acts.seen[0].Function != "gate" ||
		len(acts.seen[0].Args) != 1 || acts.seen[0].Args[0] != "ready" {
		t.Errorf("the call: %+v — the object and the arguments as given", acts.seen)
	}
}

func TestAFailingFunctionRefusesTheGuard(t *testing.T) {
	for name, guard := range map[string]string{
		"timeout":       "slow() == true",
		"not read-only": "writer()",
		"wrong kind":    "score() == 'three'",
	} {
		t.Run(name, func(t *testing.T) {
			acts := newFnActions()
			if name == "wrong kind" {
				acts.replies["x:score/get"] = `"three"`
			}
			svc, ctx := fnFixture(t, guard, acts)
			err := fnMove(svc, ctx)
			if !errors.Is(err, api.ErrTransitionRefused) {
				t.Fatalf("a failed function must refuse the guard, got %v", err)
			}
		})
	}
	acts := newFnActions()
	svc, ctx := fnFixture(t, "writer()", acts)
	err := fnMove(svc, ctx)
	if err == nil || !strings.Contains(err.Error(), "not a read-only method") || acts.count("x:write/it") != 0 {
		t.Errorf("a non-read-only action: %v, run %d times — refused before it runs", err, acts.count("x:write/it"))
	}
	svc, ctx = fnFixture(t, "slow() == true", newFnActions())
	if err := fnMove(svc, ctx); err == nil || !strings.Contains(err.Error(), "no answer within 50ms") {
		t.Errorf("a slow function: %v — refused naming its timeout", err)
	}
}

// A REFUSAL FROM A CATALOG NOT YET CURRENT IS NOT KEPT. A method the catalog
// does not know yet is described « not read-only »; once its sidecar is
// indexed the same function answers, without a restart.
func TestANotReadOnlyRefusalIsAskedAgain(t *testing.T) {
	acts := newFnActions()
	acts.mu.Lock()
	acts.writes["x:gates/check"] = true
	acts.mu.Unlock()
	svc, ctx := fnFixture(t, "gate('ready')", acts)
	if err := fnMove(svc, ctx); err == nil || !strings.Contains(err.Error(), "not a read-only method") {
		t.Fatalf("while the catalog says not read-only: %v — refused", err)
	}
	acts.mu.Lock()
	delete(acts.writes, "x:gates/check")
	acts.mu.Unlock()
	if err := fnMove(svc, ctx); err != nil {
		t.Errorf("once the catalog says read-only the guard should hold: %v", err)
	}
}

// An action's `when:` is an object expression: evaluated (ObjectEvaluate, as
// the action surface asks it) with the capabilities' functions.
func TestAFunctionAnswersInAnActionWhen(t *testing.T) {
	acts := newFnActions()
	svc, ctx := fnFixture(t, "true", acts)
	when := "gate('ready') && score() > 2"
	out, err := svc.ObjectEvaluate(ctx, api.ObjectEvaluateInput{ID: "m.t", Expressions: []string{when}})
	if err != nil || out.Results[0].Value != true {
		t.Fatalf("%s holds: %+v, %v", when, out, err)
	}
	acts.replies["x:gates/check"] = "false"
	out, _ = svc.ObjectEvaluate(ctx, api.ObjectEvaluateInput{ID: "m.t", Expressions: []string{when}})
	if out.Results[0].Value != false {
		t.Errorf("gate('ready') answers false; the when must not hold: %+v", out.Results[0])
	}
}

func TestAnExpressionNamingNoFunctionCallsNothing(t *testing.T) {
	acts := newFnActions()
	svc, ctx := fnFixture(t, "by.kind == 'person'", acts)
	if err := fnMove(svc, ctx); err != nil {
		t.Fatal(err)
	}
	for uri, n := range acts.calls {
		if n > 0 {
			t.Errorf("%s was called %d times by a guard that names no function", uri, n)
		}
	}
}

func TestNoActionServiceRefusesTheGuard(t *testing.T) {
	svc, ctx := fnFixture(t, "gate('ready')", nil)
	if err := fnMove(svc, ctx); !errors.Is(err, api.ErrTransitionRefused) || !strings.Contains(err.Error(), "no ActionService") {
		t.Fatalf("a function nobody can ask is never a pass: %v", err)
	}
}

func TestAFunctionIsNotInAHookWhenNorATaskFilter(t *testing.T) {
	if _, err := objectExprs.Compile("gate('ready')"); err == nil {
		t.Error("the hook / named-file vocabulary compiled a function call")
	}
	svc, _ := fnFixture(t, "true", newFnActions())
	if _, err := svc.objectCache().Compile("gate('ready')"); err != nil {
		t.Errorf("the object vocabulary refuses a declared function: %v", err)
	}
	if _, err := svc.objectCache().Compile("nope('x')"); err == nil {
		t.Error("an undeclared function compiled")
	}
}

func TestAFunctionNameIsRefusedWhenItClashes(t *testing.T) {
	_, problems := buildFunctions([]api.Capability{
		{Name: "a", Functions: []api.CapabilityFunction{{Name: "gate", Action: "x:a/g"}, {Name: "by", Action: "x:a/v"}, {Name: "Bad-Name", Action: "x:a/b"}}},
		{Name: "b", Functions: []api.CapabilityFunction{{Name: "gate", Action: "x:b/g"}, {Name: "ok", Action: "x:b/ok"}}},
	})
	all := strings.Join(problems, "\n")
	for _, want := range []string{`function "gate" is declared by a and b`, `"by" would hide the vocabulary name`, `"Bad-Name" is not a lowercase identifier`} {
		if !strings.Contains(all, want) {
			t.Errorf("problems do not say %q:\n%s", want, all)
		}
	}
	r, _ := buildFunctions([]api.Capability{{Name: "b", Functions: []api.CapabilityFunction{{Name: "ok", Action: "x:b/ok"}}}})
	if _, ok := r.byName["ok"]; !ok {
		t.Error("a clean function was left out")
	}
}

// A type's accept_when may call a capability's function -
// a display switch decides whether « + New » offers the type: the
// picker offers it while the function answers true, leaves it out when it
// answers false, and a costly one is not run there (the type left out).
func TestAcceptWhenCallsAFunction(t *testing.T) {
	for _, tc := range []struct {
		name, when, reply string
		offered           bool
	}{
		{"true", "gate('on')", "true", true},
		{"false", "gate('on')", "false", false},
		{"costly", "heavy()", "true", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			acts := newFnActions()
			acts.replies["x:gates/check"] = tc.reply
			svc, ctx := fnFixture(t, "true", acts)
			svc.RefreshCatalogTypes(CatalogTypeSet{Objects: []api.ObjectType{
				{Name: "switched", AcceptParents: []string{"thing"}, AcceptWhen: tc.when},
			}})
			out, err := svc.ObjectAllowedChildren(ctx, api.ObjectAllowedChildrenInput{ParentID: "m.t"})
			if err != nil {
				t.Fatal(err)
			}
			if got := hasName(choiceNames(out.Choices), "switched"); got != tc.offered {
				t.Errorf("accept_when %q with the function answering %s: offered %v, want %v (%v)", tc.when, tc.reply, got, tc.offered, choiceNames(out.Choices))
			}
		})
	}
}
