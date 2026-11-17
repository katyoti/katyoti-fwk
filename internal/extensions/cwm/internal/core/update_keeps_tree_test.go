// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"errors"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/eventbus"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/internal/events"
)

// AN UPDATE KEEPS THE TREE AND THE MODULE. A new parent may not be the object
// or one of its descendants; the module is the object's own (an empty one is
// filled, another refused — ObjectMove refiles); ProjectInit publishes its
// object like any write.

func treeFixture(t *testing.T) (*Service, stdctx.Context) {
	t.Helper()
	cfg := &CWMServiceConfig{WorkspaceRoot: freshWorkspace(t), ProjectRoot: t.TempDir(), ObjectTypeValidation: api.TypeValidationError}
	cfg.CatalogTypes.Objects = []api.ObjectType{{Name: "module"}, {Name: "note"}}
	svc, err := New(stdctx.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	ctx := stdctx.Background()
	for _, o := range []api.Object{
		{ID: "m", Type: "module", Name: "m"}, {ID: "other", Type: "module", Name: "other"},
		{ID: "m.a", Type: "note", Name: "a", Parent: "m"},
		{ID: "m.b", Type: "note", Name: "b", Parent: "m.a"},
		{ID: "m.c", Type: "note", Name: "c", Parent: "m.b"},
		{ID: "m.d", Type: "note", Name: "d", Parent: "m"},
	} {
		module, _ := api.ModuleOfObjectID(o.ID)
		if _, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: module, Object: o}); err != nil {
			t.Fatalf("create %s: %v", o.ID, err)
		}
	}
	return svc, ctx
}

func reparent(svc *Service, ctx stdctx.Context, module, id, parent string) error {
	got, err := svc.ObjectGet(ctx, api.ObjectGetInput{Module: "m", ID: id})
	if err != nil {
		return err
	}
	o := got.Object
	o.Parent = parent
	_, err = svc.ObjectUpdate(ctx, api.ObjectUpdateInput{Module: module, Object: o})
	return err
}

func TestAnUpdateCannotDetachASubtree(t *testing.T) {
	svc, ctx := treeFixture(t)
	for _, p := range []string{"m.c", "m.b", "m.a"} { // a descendant, a child, itself
		if err := reparent(svc, ctx, "m", "m.a", p); !errors.Is(err, api.ErrCircularReference) {
			t.Errorf("m.a under %s: %v — want refused", p, err)
		}
	}
	if err := reparent(svc, ctx, "m", "m.a", "m.d"); err != nil {
		t.Errorf("m.a under its sibling m.d: %v", err)
	}
	if err := reparent(svc, ctx, "m", "m.c", "m.c"); !errors.Is(err, api.ErrCircularReference) {
		t.Errorf("m.c under itself: %v", err)
	}
}

func TestAnUpdateKeepsTheObjectsModule(t *testing.T) {
	svc, ctx := treeFixture(t)
	// No module given: taken from the index, recorded right.
	if err := reparent(svc, ctx, "", "m.d", "m.a"); err != nil {
		t.Fatalf("update with no module: %v", err)
	}
	if m, _ := svc.index().moduleOf("m.d"); m != "m" {
		t.Errorf("module recorded %q, want m", m)
	}
	// Another module: refused, nothing recorded.
	if err := reparent(svc, ctx, "other", "m.d", "m"); !errors.Is(err, api.ErrInvalidInput) {
		t.Errorf("update naming another module: %v — want refused", err)
	}
	if m, _ := svc.index().moduleOf("m.d"); m != "m" {
		t.Errorf("module after a refused update %q, want m", m)
	}
}

func TestProjectInitPublishesTheProject(t *testing.T) {
	// The slot is given (WorkspaceID), so minting the id keeps it.
	cfg := &CWMServiceConfig{WorkspaceRoot: freshWorkspace(t), ProjectRoot: t.TempDir(), WorkspaceID: "w"}
	svc, err := New(stdctx.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	bus := eventbus.NewMemoryEventBus()
	svc.Events = events.New("t", "u", "w", bus, nil)
	var c captured
	_ = bus.Subscribe(api.ObjectCreatedSubject("t", "u", "w", api.SubjectToken("acme")), c.on)
	if _, err := svc.ProjectInit(stdctx.Background(), api.ProjectInitInput{Name: "acme"}); err != nil {
		t.Fatal(err)
	}
	if len(c.all()) != 1 {
		t.Errorf("created events for the project: %d, want 1", len(c.all()))
	}
}
