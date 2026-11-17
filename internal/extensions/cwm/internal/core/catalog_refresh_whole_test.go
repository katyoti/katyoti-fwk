// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	actionsapi "github.com/katyoti/katyoti-fwk/internal/extensions/actions/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// A PACK DEPLOYED WHILE CWM RUNS IS SEEN WHOLE. A refresh that brings a type
// together with its capability, its workflow and a hook makes the three work
// at once — no restart — and the capability's functions with it.
func TestARefreshBringsEveryDeclaration(t *testing.T) {
	cfg := &CWMServiceConfig{WorkspaceRoot: freshWorkspace(t), ProjectRoot: t.TempDir(), ObjectTypeValidation: api.TypeValidationError}
	cfg.CatalogTypes.Capabilities = []api.Capability{{Name: "lifecycle"}}
	cfg.CatalogTypes.Objects = []api.ObjectType{{Name: "module"}}
	svc, err := New(stdctx.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	f := &hookActions{replies: map[string]string{"x:pack/create": `{"accept":true}`}}
	app := application.New(application.Config())
	application.PutService[actionsapi.ActionService](app, "default", f)
	ctx := application.WithApplication(stdctx.Background(), app)
	if _, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "m", Object: api.Object{ID: "m", Type: "module", Name: "m"}}); err != nil {
		t.Fatal(err)
	}

	svc.RefreshCatalogTypes(CatalogTypeSet{
		Capabilities: []api.Capability{{Name: "notes", Functions: []api.CapabilityFunction{
			{Name: "ready", Action: "x:notes/ready", Returns: api.FunctionReturnsBoolean}}}},
		Workflows: []api.Workflow{{Name: "review", Initial: "draft",
			States:      []api.WorkflowState{{Name: "draft"}, {Name: "done"}},
			Transitions: []api.WorkflowTransition{{Name: "finish", From: []string{"draft"}, To: "done"}}}},
		Objects: []api.ObjectType{{Name: "note", Capabilities: []string{"lifecycle", "notes"}, Workflow: "review"}},
		Hooks:   []api.Hook{{Name: "pack-gate", Type: api.HookBeforeCreate, URI: "x:pack/create", Filter: api.ObjectFilter{Types: []string{"note"}}}},
	})

	n := api.Object{ID: "m.n", Type: "note", Name: "n", Parent: "m", Capabilities: map[string]map[string]any{"notes": {}}}
	if _, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "m", Object: n}); err != nil {
		t.Fatalf("an object of the new type, with the new capability: %v", err)
	}
	if calls := f.callsTo("x:pack/create"); len(calls) != 1 {
		t.Errorf("the new hook ran %d times, want once", len(calls))
	}
	if _, err := svc.ObjectTransition(ctx, api.ObjectTransitionInput{ID: "m.n", To: "done", By: api.Actor{ID: "ana", Kind: api.ActorPerson}}); err != nil {
		t.Errorf("a move along the new workflow: %v", err)
	}
	if fns := svc.fns.Load(); fns == nil || fns.byName["ready"].decl.Name != "ready" {
		t.Error("the new capability's function is not callable in expressions")
	}
}

// A HOOK REDECLARED UNDER THE SAME NAME IS IN FORCE AT THE REFRESH. A sidecar
// rebuilt with a changed hook (another action, async made synchronous) is
// followed at once, without a restart; one the fresh catalog
// does not name stays.
func TestARefreshReplacesAChangedHook(t *testing.T) {
	cfg := &CWMServiceConfig{WorkspaceRoot: freshWorkspace(t), ProjectRoot: t.TempDir(), ObjectTypeValidation: api.TypeValidationError}
	cfg.CatalogTypes.Objects = []api.ObjectType{{Name: "module"}, {Name: "note"}}
	cfg.CatalogTypes.Hooks = []api.Hook{
		{Name: "gate", Type: api.HookBeforeCreate, URI: "x:pack/old", Filter: api.ObjectFilter{Types: []string{"note"}}},
		{Name: "kept", Type: api.HookBeforeCreate, URI: "x:pack/kept", Filter: api.ObjectFilter{Types: []string{"note"}}},
		{Name: "propose", Type: api.HookOnCreate, URI: "x:pack/propose", Async: &api.AsyncSpec{}},
	}
	svc, err := New(stdctx.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	f := &hookActions{replies: map[string]string{"x:pack/old": `{"accept":true}`, "x:pack/new": `{"accept":true}`, "x:pack/kept": `{"accept":true}`}}
	app := application.New(application.Config())
	application.PutService[actionsapi.ActionService](app, "default", f)
	ctx := application.WithApplication(stdctx.Background(), app)
	if _, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "m", Object: api.Object{ID: "m", Type: "module", Name: "m"}}); err != nil {
		t.Fatal(err)
	}

	svc.RefreshCatalogTypes(CatalogTypeSet{Hooks: []api.Hook{
		{Name: "gate", Type: api.HookBeforeCreate, URI: "x:pack/new", Filter: api.ObjectFilter{Types: []string{"note"}}},
		{Name: "propose", Type: api.HookOnCreate, URI: "x:pack/propose"},
	}})

	if _, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "m", Object: api.Object{ID: "m.n", Type: "note", Name: "n", Parent: "m"}}); err != nil {
		t.Fatal(err)
	}
	if n := len(f.callsTo("x:pack/old")); n != 0 {
		t.Errorf("the hook's former action ran %d times after the refresh, want none", n)
	}
	if n := len(f.callsTo("x:pack/new")); n != 1 {
		t.Errorf("the redeclared hook's action ran %d times, want once", n)
	}
	if n := len(f.callsTo("x:pack/kept")); n != 1 {
		t.Errorf("a hook the refresh does not name ran %d times, want once (it stays)", n)
	}
	for _, h := range svc.decl().Hooks {
		if h.Name == "propose" && h.Async != nil {
			t.Error("a hook made synchronous is still async after the refresh")
		}
	}
}

// DECLARATIONS BY URI. A refresh replaces the declarations whole
// keyed by where each comes from: a changed one is in force, one its member's
// manifest no longer names goes; the last good one stays while its member is
// absent from the snapshot or while its new version was refused (named, not read).
func TestARefreshReplacesDeclarationsByURI(t *testing.T) {
	src := func(provider, name string) api.DeclarationSource {
		return api.DeclarationSource{URI: provider + ":/object-type/" + name, Provider: provider}
	}
	cfg := &CWMServiceConfig{WorkspaceRoot: freshWorkspace(t), ProjectRoot: t.TempDir(), ObjectTypeValidation: api.TypeValidationError}
	cfg.CatalogTypes.Objects = []api.ObjectType{{Name: "module"}, {Name: "changed"}, {Name: "gone"}, {Name: "refused"}, {Name: "away"}}
	cfg.CatalogTypes.Sources = map[string]api.DeclarationSource{
		api.DeclarationKey("object-type", "changed"): src("ktw-p", "changed"),
		api.DeclarationKey("object-type", "gone"):    src("ktw-p", "gone"),
		api.DeclarationKey("object-type", "refused"): src("ktw-p", "refused"),
		api.DeclarationKey("object-type", "away"):    src("ktw-q", "away"),
	}
	svc, err := New(stdctx.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	ctx := stdctx.Background()
	if _, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "m", Object: api.Object{ID: "m", Type: "module", Name: "m"}}); err != nil {
		t.Fatal(err)
	}

	// ktw-p's manifest names "changed" (now abstract) and "refused" (which did
	// not read); it no longer names "gone"; ktw-q is absent from the snapshot.
	svc.RefreshCatalogTypes(CatalogTypeSet{
		Objects: []api.ObjectType{{Name: "changed", Abstract: true}},
		Sources: map[string]api.DeclarationSource{api.DeclarationKey("object-type", "changed"): src("ktw-p", "changed")},
		Members: map[string]bool{"ktw-p": true},
		Named:   map[string]bool{src("ktw-p", "changed").URI: true, src("ktw-p", "refused").URI: true},
	})

	create := func(typ string) error {
		_, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "m", Object: api.Object{ID: "m." + typ, Type: typ, Name: typ, Parent: "m"}})
		return err
	}
	if create("changed") == nil {
		t.Error("the changed declaration (now abstract) is not in force")
	}
	if create("gone") == nil {
		t.Error("a type its member no longer names is still declared")
	}
	if err := create("refused"); err != nil {
		t.Errorf("the last good declaration of a type whose new version was refused is gone: %v", err)
	}
	if err := create("away"); err != nil {
		t.Errorf("the declaration of a member absent from the snapshot is gone: %v", err)
	}
	svc.objTypesMu.RLock()
	_, kept := svc.objTypes["module"]
	svc.objTypesMu.RUnlock()
	if !kept {
		t.Error("a declaration read from no catalog is dropped")
	}
}
