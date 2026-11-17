// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// OWNED NAMED FILES LIVE WITH THEIR OBJECT: an owned file follows a move that
// changes its path and goes with a delete; a file the object only names is
// neither carried nor removed.
func TestOwnedNamedFilesLiveWithTheirObject(t *testing.T) {
	ctx := stdctx.Background()
	cfg := &CWMServiceConfig{WorkspaceRoot: freshWorkspace(t), ProjectRoot: t.TempDir()}
	cfg.CatalogTypes.Capabilities = []api.Capability{{Name: "notes", Attach: &api.ShapeFilter{Types: []string{"*"}},
		Files: map[string]api.NamedFile{
			"thread": {Path: "notes/${id}.yaml", Owned: true},
			"page":   {Path: "page.md"},
		}}}
	cfg.CatalogTypes.Objects = []api.ObjectType{{Name: "note"}}
	svc, err := New(ctx, cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	if _, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "billing", Object: api.Object{ID: "billing.n1", Type: "note", Name: "n1"}}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"thread", "page"} {
		if _, err := svc.ObjectNamedFileWrite(ctx, api.ObjectNamedFileWriteInput{ID: "billing.n1", Capability: "notes", Name: name, Content: []byte(name)}); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	root, _, _ := svc.objectFileRoot("billing.n1")
	modules := filepath.Dir(root)

	mv, err := svc.ObjectMove(ctx, api.ObjectMoveInput{ID: "billing.n1", NewModule: "shipping", Reason: "belongs there"})
	if err != nil {
		t.Fatalf("move: %v", err)
	}
	got, err := svc.ObjectNamedFileRead(ctx, api.ObjectNamedFileReadInput{ID: mv.ID, Capability: "notes", Name: "thread"})
	if err != nil || string(got.Content) != "thread" {
		t.Fatalf("the owned file after the move: %q %v", got.Content, err)
	}
	if _, err := os.Stat(filepath.Join(modules, "billing", "notes", "billing.n1.yaml")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the owned file stayed behind: %v", err)
	}
	if _, err := os.Stat(filepath.Join(modules, "billing", "page.md")); err != nil {
		t.Errorf("a file the object only names was carried: %v", err)
	}

	if _, err := svc.ObjectDelete(ctx, api.ObjectDeleteInput{Module: "shipping", ID: mv.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(modules, "shipping", "notes", mv.ID+".yaml")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the owned file outlived its object: %v", err)
	}
	if _, err := os.Stat(filepath.Join(modules, "billing", "page.md")); err != nil {
		t.Errorf("a file the object only names was deleted: %v", err)
	}
}
