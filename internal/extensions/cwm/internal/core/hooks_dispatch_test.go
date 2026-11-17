// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	actionsapi "github.com/katyoti/katyoti-fwk/internal/extensions/actions/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// hookActions answers hook calls per uri and records every call.
type hookActions struct {
	mu      sync.Mutex
	replies map[string]string // uri → JSON value
	calls   []hookCallSeen
	fired   chan string
}

type hookCallSeen struct {
	uri  string
	call api.HookCall
	raw  json.RawMessage // the input as sent
}

func (f *hookActions) Describe(stdctx.Context, actionsapi.DescribeInput) (actionsapi.DescribeOutput, error) {
	return actionsapi.DescribeOutput{}, nil
}

func (f *hookActions) Run(_ stdctx.Context, in actionsapi.RunInput) (actionsapi.RunOutput, error) {
	var c api.HookCall
	_ = json.Unmarshal(in.Input, &c)
	f.mu.Lock()
	f.calls = append(f.calls, hookCallSeen{in.URI, c, in.Input})
	f.mu.Unlock()
	if f.fired != nil {
		f.fired <- in.URI
	}
	return actionsapi.RunOutput{URI: in.URI, Value: json.RawMessage(f.replies[in.URI])}, nil
}

func (f *hookActions) callsTo(uri string) []api.HookCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []api.HookCall
	for _, c := range f.calls {
		if c.uri == uri {
			out = append(out, c.call)
		}
	}
	return out
}

// dispatchFixture: a `gated` type whose writes are hooked, a `plain` type with
// nothing, and a ctx whose ActionService is the fake (or none).
func dispatchFixture(t *testing.T, actions actionsapi.ActionService) (*Service, stdctx.Context, string) {
	t.Helper()
	project := t.TempDir()
	cfg := &CWMServiceConfig{WorkspaceRoot: freshWorkspace(t), ProjectRoot: project,
		ObjectTypeValidation: api.TypeValidationError}
	cfg.CatalogTypes.Capabilities = []api.Capability{{Name: "container"}}
	cfg.CatalogTypes.Objects = []api.ObjectType{
		{Name: "gated", Capabilities: []string{"container"}, Hooks: api.Hooks{
			api.HookBeforeCreate: {{URI: "x:gate/create"}},
			api.HookBeforeUpdate: {{URI: "x:gate/update"}},
			api.HookOnCreate:     {{URI: "x:after/create"}},
			api.HookOnDelete:     {{URI: "x:after/delete"}},
			api.HookBeforeDelete: {{URI: "x:gate/delete"}},
			api.HookOnUpdate:     {{URI: "x:after/doc-only", Filter: &api.ObjectFilter{Types: []string{"doc"}}}},
		}},
		{Name: "plain", Capabilities: []string{"container"}},
	}
	cfg.CatalogTypes.Edges = []api.EdgeTypeRegistration{{Name: "about", From: "any", To: "any", Cardinality: "M:N", Target: "object", ObjectStorage: "edges"}}
	svc, err := New(stdctx.Background(), cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	app := application.New(application.Config())
	if actions != nil {
		application.PutService[actionsapi.ActionService](app, "default", actions)
	}
	return svc, application.WithApplication(stdctx.Background(), app), project
}

func gatedObject(id string) api.Object {
	o := api.Object{ID: id, Type: "gated"}
	o.Name = id
	return o
}

func waitFired(t *testing.T, f *hookActions, uri string) {
	t.Helper()
	deadline := time.After(3 * time.Second)
	for {
		select {
		case got := <-f.fired:
			if got == uri {
				return
			}
		case <-deadline:
			t.Fatalf("%s never fired", uri)
		}
	}
}

// TestBeforeCreateRefusalBlocksTheWrite: a refusing gate refuses the create,
// its reason reaches the caller, and nothing is written.
func TestBeforeCreateRefusalBlocksTheWrite(t *testing.T) {
	f := &hookActions{replies: map[string]string{"x:gate/create": `{"accept":false,"reason":"not in this module"}`}}
	svc, ctx, project := dispatchFixture(t, f)
	_, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "billing", Object: gatedObject("billing.a")})
	if !errors.Is(err, api.ErrRefusedByHook) || !strings.Contains(err.Error(), "not in this module") {
		t.Fatalf("create: err = %v, want ErrRefusedByHook carrying the reason", err)
	}
	if _, err := os.Stat(filepath.Join(project, "modules", "billing")); err == nil {
		t.Fatal("a refused create wrote the module folder")
	}
}

// TestAcceptedCreateFiresOnCreate: an accepting gate lets the write through,
// and on_create fires after it with the object.
func TestAcceptedCreateFiresOnCreate(t *testing.T) {
	f := &hookActions{replies: map[string]string{"x:gate/create": `{"accept":true}`}, fired: make(chan string, 8)}
	svc, ctx, _ := dispatchFixture(t, f)
	if _, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "billing", Object: gatedObject("billing.a")}); err != nil {
		t.Fatalf("create: %v", err)
	}
	waitFired(t, f, "x:after/create")
	calls := f.callsTo("x:after/create")
	if len(calls) != 1 || calls[0].Object.ID != "billing.a" || calls[0].Point != api.HookOnCreate {
		t.Fatalf("on_create calls = %+v", calls)
	}
}

// TestBeforeUpdateSeesThePriorAndMayRefuse: the gate receives the object as it
// was; a refusal leaves the file unchanged.
func TestBeforeUpdateSeesThePriorAndMayRefuse(t *testing.T) {
	f := &hookActions{replies: map[string]string{
		"x:gate/create": `{"accept":true}`, "x:gate/update": `{"accept":false,"reason":"frozen"}`}}
	svc, ctx, _ := dispatchFixture(t, f)
	if _, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "billing", Object: gatedObject("billing.a")}); err != nil {
		t.Fatalf("create: %v", err)
	}
	changed := gatedObject("billing.a")
	changed.Name = "renamed"
	if _, err := svc.ObjectUpdate(ctx, api.ObjectUpdateInput{Module: "billing", Object: changed}); !errors.Is(err, api.ErrRefusedByHook) {
		t.Fatalf("update: err = %v, want ErrRefusedByHook", err)
	}
	calls := f.callsTo("x:gate/update")
	if len(calls) != 1 || calls[0].Prior == nil || calls[0].Prior.ParentID() != "" {
		t.Fatalf("before_update calls = %+v, want one carrying the prior object", calls)
	}
	if c := calls[0].Prior; c.Name != "billing.a" {
		t.Errorf("prior name %q, want the stored one", c.Name)
	}
	got, err := svc.ObjectGet(ctx, api.ObjectGetInput{Module: "billing", ID: "billing.a"})
	if c := got.Object; err != nil || c.Name != "billing.a" {
		t.Fatalf("a refused update changed the object: %+v (%v)", c, err)
	}
}

// TestOnDeleteFiresWithTheDeletedObject.
func TestOnDeleteFiresWithTheDeletedObject(t *testing.T) {
	f := &hookActions{replies: map[string]string{"x:gate/create": `{"accept":true}`, "x:gate/delete": `{"accept":true}`}, fired: make(chan string, 8)}
	svc, ctx, _ := dispatchFixture(t, f)
	if _, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "billing", Object: gatedObject("billing.a")}); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := svc.ObjectDelete(ctx, api.ObjectDeleteInput{Module: "billing", ID: "billing.a"}); err != nil {
		t.Fatalf("delete: %v", err)
	}
	waitFired(t, f, "x:after/delete")
	if calls := f.callsTo("x:after/delete"); len(calls) != 1 || calls[0].Object.ID != "billing.a" {
		t.Fatalf("on_delete calls = %+v", calls)
	}
}

// TestADeclaredGateWithNoActionServiceRefuses: a gate that cannot be asked
// refuses; an object with no hooks is written without any service.
func TestADeclaredGateWithNoActionServiceRefuses(t *testing.T) {
	svc, ctx, _ := dispatchFixture(t, nil)
	if _, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "billing", Object: gatedObject("billing.a")}); !errors.Is(err, api.ErrRefusedByHook) {
		t.Fatalf("gated create with no action service: err = %v, want ErrRefusedByHook", err)
	}
	plain := api.Object{ID: "billing.p", Type: "plain"}
	plain.Name = "p"
	if _, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "billing", Object: plain}); err != nil {
		t.Fatalf("an unhooked create needs no action service: %v", err)
	}
}

// TestAHookWhoseFilterDoesNotHoldIsNotRun: the doc-only on_update entry does not
// fire for a `gated` object.
func TestAHookWhoseFilterDoesNotHoldIsNotRun(t *testing.T) {
	f := &hookActions{replies: map[string]string{"x:gate/create": `{"accept":true}`, "x:gate/update": `{"accept":true}`}}
	svc, ctx, _ := dispatchFixture(t, f)
	if _, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "billing", Object: gatedObject("billing.a")}); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := svc.ObjectUpdate(ctx, api.ObjectUpdateInput{Module: "billing", Object: gatedObject("billing.a")}); err != nil {
		t.Fatalf("update: %v", err)
	}
	time.Sleep(100 * time.Millisecond) // an on_update would have fired by now
	if calls := f.callsTo("x:after/doc-only"); len(calls) != 0 {
		t.Fatalf("a doc-only hook fired for a gated object: %+v", calls)
	}
}

// bootstrapFixture: a project object of a project type - one carrying the
// project level, named otherwise (a dossier d'architecture) - and a policy on
// the project level whose bootstrap entry an extension serves, as the inbox
// extension declares its own (filter: capabilities [project]).
func bootstrapFixture(t *testing.T, actions actionsapi.ActionService, withProject bool) (*Service, stdctx.Context) {
	t.Helper()
	cfg := &CWMServiceConfig{WorkspaceRoot: freshWorkspace(t), ProjectRoot: t.TempDir()}
	cfg.CatalogTypes.Capabilities = []api.Capability{
		{Name: "container"}, {Name: api.CapProject},
		{Name: "inbox-bootstrap", Attach: &api.ShapeFilter{Capabilities: []string{api.CapProject}},
			Hooks: api.Hooks{api.HookBootstrap: {{URI: "x:inbox/ensure"}}}},
	}
	cfg.CatalogTypes.Objects = []api.ObjectType{{Name: "dossier-architecture", Capabilities: []string{api.CapProject, "container"}}}
	svc, err := New(stdctx.Background(), cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	if withProject {
		p := api.Object{ID: "demo", Type: "dossier-architecture"}
		p.Name = "demo"
		if _, err := svc.ObjectCreate(stdctx.Background(), api.ObjectCreateInput{Module: "demo", Object: p}); err != nil {
			t.Fatalf("create the project object: %v", err)
		}
	}
	app := application.New(application.Config())
	if actions != nil {
		application.PutService[actionsapi.ActionService](app, "default", actions)
	}
	return svc, application.WithApplication(stdctx.Background(), app)
}

// TestBootstrapRunsWhatThePolicyDeclares: the project's bootstrap entry (from a
// policy attached to the project type) runs once, with the project object.
func TestBootstrapRunsWhatThePolicyDeclares(t *testing.T) {
	f := &hookActions{replies: map[string]string{}}
	svc, ctx := bootstrapFixture(t, f, true)
	if errs := svc.RunBootstrapHooks(ctx, BootstrapAll); len(errs) != 0 {
		t.Fatalf("RunBootstrapHooks: %v", errs)
	}
	calls := f.callsTo("x:inbox/ensure")
	if len(calls) != 1 || calls[0].Object.ID != "demo" || calls[0].Point != api.HookBootstrap {
		t.Fatalf("bootstrap calls = %+v, want one with the project object", calls)
	}
}

// TestBootstrapSplitsOwnFromOthers: the binary that holds CWM runs the
// bootstrap entries it serves at boot, and the ones another sidecar serves
// once it answers on the bus — each pass runs its own entries and only those.
func TestBootstrapSplitsOwnFromOthers(t *testing.T) {
	f := &hookActions{replies: map[string]string{}}
	cfg := &CWMServiceConfig{WorkspaceRoot: freshWorkspace(t), ProjectRoot: t.TempDir()}
	cfg.CatalogTypes.Capabilities = []api.Capability{
		{Name: "container"}, {Name: api.CapProject},
		{Name: "both-bootstraps", Attach: &api.ShapeFilter{Capabilities: []string{api.CapProject}},
			Hooks: api.Hooks{api.HookBootstrap: {{URI: "x:inbox/ensure"}, {URI: "y:review/ensure-desk"}}}},
	}
	cfg.CatalogTypes.Objects = []api.ObjectType{{Name: "software-project", Capabilities: []string{api.CapProject, "container"}}}
	svc, err := New(stdctx.Background(), cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	for _, id := range []string{"one", "two"} {
		p := api.Object{ID: id, Type: "software-project"}
		p.Name = id
		if _, err := svc.ObjectCreate(stdctx.Background(), api.ObjectCreateInput{Module: "demo", Object: p}); err != nil {
			t.Fatalf("create project %s: %v", id, err)
		}
	}
	app := application.New(application.Config())
	app.SetProvider("x")
	application.PutService[actionsapi.ActionService](app, "default", f)
	ctx := application.WithApplication(stdctx.Background(), app)

	if errs := svc.RunBootstrapHooks(ctx, BootstrapOwn); len(errs) != 0 {
		t.Fatalf("own pass: %v", errs)
	}
	if own, others := len(f.callsTo("x:inbox/ensure")), len(f.callsTo("y:review/ensure-desk")); own != 2 || others != 0 {
		t.Fatalf("own pass ran x:inbox/ensure %d time(s) and y:review/ensure-desk %d; want 2 and 0", own, others)
	}
	if errs := svc.RunBootstrapHooks(ctx, BootstrapOthers); len(errs) != 0 {
		t.Fatalf("others pass: %v", errs)
	}
	if own, others := len(f.callsTo("x:inbox/ensure")), len(f.callsTo("y:review/ensure-desk")); own != 2 || others != 2 {
		t.Fatalf("after the others pass: x:inbox/ensure %d time(s), y:review/ensure-desk %d; want 2 and 2", own, others)
	}
}

// TestBootstrapWithNoActionServiceReportsIt: a declared bootstrap that cannot
// run is returned as an error, not swallowed.
func TestBootstrapWithNoActionServiceReportsIt(t *testing.T) {
	svc, ctx := bootstrapFixture(t, nil, true)
	if errs := svc.RunBootstrapHooks(ctx, BootstrapAll); len(errs) != 1 {
		t.Fatalf("errs = %v, want one", errs)
	}
}

// TestARefusedMoveLeavesTheIndexUntouched: a move whose update a hook refuses
// changes nothing — not the file, and not the object the index holds in memory.
func TestARefusedMoveLeavesTheIndexUntouched(t *testing.T) {
	f := &hookActions{replies: map[string]string{
		"x:gate/create": `{"accept":true}`, "x:gate/update": `{"accept":false,"reason":"frozen"}`}}
	svc, ctx, _ := dispatchFixture(t, f)
	a, b, x := gatedObject("billing.a"), gatedObject("billing.b"), gatedObject("billing.x")
	x.Parent = "billing.a"
	for _, o := range []api.Object{a, b, x} {
		if _, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "billing", Object: o}); err != nil {
			t.Fatalf("create %s: %v", o.ID, err)
		}
	}
	if _, err := svc.ObjectMove(ctx, api.ObjectMoveInput{ID: "billing.x", NewParent: "billing.b", Reason: "try"}); !errors.Is(err, api.ErrRefusedByHook) {
		t.Fatalf("move: err = %v, want the update refused", err)
	}
	kids, _ := svc.ObjectList(ctx, api.ObjectListInput{Parent: "billing.a"})
	if len(kids.Objects) != 1 || kids.Objects[0].ID != "billing.x" {
		t.Fatalf("after a refused move, a's children = %+v, want billing.x still there", kids.Objects)
	}
}
