// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// A type's declared folder is where NEW objects of that type are written.
//
// This is the whole of D1. Without it every object lands in `objects/` and the
// grouping has to be done by hand — which, since a hand-moved object is still
// found by the suffix scan, nobody would ever be told they needed to do.
func TestDeclaredFolderIsWhereNewObjectsLand(t *testing.T) {
	svc, root := newSvc(t)
	ctx := stdctx.Background()

	svc.mergeObjectTypes([]api.ObjectType{
		{Name: "goal", Folder: "goals"},
		{Name: "plain"},
	}, nil)

	for _, tc := range []struct {
		typ  string
		id   string
		want string
	}{
		{"goal", "demo.g1", "modules/demo/goals/g1.cwm.yaml"},
		{"plain", "demo.p1", "modules/demo/objects/p1.cwm.yaml"},
	} {
		if _, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{
			Module: "demo",
			Object: api.Object{ID: tc.id, Type: tc.typ},
		}); err != nil {
			t.Fatalf("create %s: %v", tc.id, err)
		}
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(tc.want))); err != nil {
			t.Errorf("type %q: expected the object at %s: %v", tc.typ, tc.want, err)
		}
	}

	// And the folder is a WRITE-SIDE DEFAULT, never a read rule: both objects
	// are listed, from two different folders, by one flat call.
	out, err := svc.ObjectList(ctx, api.ObjectListInput{Module: "demo"})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(out.Objects) != 2 {
		t.Fatalf("expected both objects found regardless of folder, got %d", len(out.Objects))
	}
}

// Changing a type's folder does NOT move the objects already written, and they
// stay findable.
//
// Stated as a test because it is the thing someone will "fix": seeing goals in
// two folders looks like a bug, and moving them would be a no-op dressed as a
// repair.
func TestChangingATypesFolderStrandsNothing(t *testing.T) {
	svc, root := newSvc(t)
	ctx := stdctx.Background()

	svc.mergeObjectTypes([]api.ObjectType{{Name: "goal", Folder: "goals"}}, nil)
	if _, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{
		Module: "demo", Object: api.Object{ID: "demo.before", Type: "goal"},
	}); err != nil {
		t.Fatalf("create before: %v", err)
	}

	// The type changes its mind. mergeObjectTypes is add-only, so replace the
	// registry entry directly — the point under test is the read path, not how
	// a redeclaration is merged.
	svc.objTypesMu.Lock()
	svc.objTypes["goal"] = api.ObjectType{Name: "goal", Folder: "wants"}
	svc.objTypesMu.Unlock()

	if _, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{
		Module: "demo", Object: api.Object{ID: "demo.after", Type: "goal"},
	}); err != nil {
		t.Fatalf("create after: %v", err)
	}

	for _, rel := range []string{
		"modules/demo/goals/before.cwm.yaml",
		"modules/demo/wants/after.cwm.yaml",
	} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); err != nil {
			t.Errorf("expected %s to exist untouched: %v", rel, err)
		}
	}

	out, err := svc.ObjectList(ctx, api.ObjectListInput{Module: "demo"})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(out.Objects) != 2 {
		t.Fatalf("the earlier object must still be found after the folder changed, got %d", len(out.Objects))
	}
}

// A `_`-prefixed folder is refused AT DECLARATION.
//
// `_`-prefixed folders are never scanned, so a type declaring one would write
// objects its own scanner skips — invisible from the moment they are created.
// Refusing at declaration is the difference between an error and a silent loss.
func TestUnscannableFolderIsRefusedAtDeclaration(t *testing.T) {
	for _, tc := range []struct {
		name, folder, because string
	}{
		{"underscore", "_generated", "never scanned"},
		{"path", "a/b", "ONE segment"},
		{"backslash", `a\b`, "ONE segment"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := api.ObjectType{Name: "x", Folder: tc.folder}.ValidateFolder()
			if err == nil {
				t.Fatalf("folder %q must be refused", tc.folder)
			}
			if !errors.Is(err, api.ErrInvalidInput) {
				t.Errorf("want ErrInvalidInput, got %v", err)
			}
			if !strings.Contains(err.Error(), tc.because) {
				t.Errorf("the error must say WHY (%q): %v", tc.because, err)
			}
		})
	}
	if err := (api.ObjectType{Name: "x", Folder: "goals"}).ValidateFolder(); err != nil {
		t.Errorf("an ordinary folder must be accepted: %v", err)
	}
	if err := (api.ObjectType{Name: "x"}).ValidateFolder(); err != nil {
		t.Errorf("an unset folder must be accepted: %v", err)
	}
}

// The refusal REACHES the registry: a rejected type is not registered, so it
// cannot go on to write invisible objects.
func TestRejectedTypeNeverReachesTheRegistry(t *testing.T) {
	svc, root := newSvc(t)
	ctx := stdctx.Background()

	svc.mergeObjectTypes([]api.ObjectType{{Name: "ghost", Folder: "_generated"}}, nil)

	if _, ok := svc.lookupObjectType("ghost"); ok {
		t.Fatal("a type with an unscannable folder must not be registered")
	}

	// If it HAD been registered, this object would land in `_generated/` and
	// never be seen again. It must fall back to `objects/` instead.
	if _, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{
		Module: "demo", Object: api.Object{ID: "demo.x", Type: "ghost"},
	}); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash("modules/demo/objects/x.cwm.yaml"))); err != nil {
		t.Errorf("expected the fallback folder to be used: %v", err)
	}
	out, err := svc.ObjectList(ctx, api.ObjectListInput{Module: "demo"})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(out.Objects) != 1 {
		t.Fatalf("the object must be findable, got %d", len(out.Objects))
	}
}
