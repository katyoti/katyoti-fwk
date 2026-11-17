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

// THE MODULE PLANNING SPACE: a module's plans live IN it, in `.ktw/`, the one
// hidden folder the index admits — and only directly under the module.

func planningFixture(t *testing.T) (*Service, string) {
	t.Helper()
	project := t.TempDir()
	cfg := &CWMServiceConfig{WorkspaceRoot: freshWorkspace(t), ProjectRoot: project,
		ObjectTypeValidation: api.TypeValidationError}
	cfg.CatalogTypes.Objects = []api.ObjectType{
		{Name: "plan", Folder: ".ktw/plans", Capabilities: []string{"container"}},
		{Name: "note", Capabilities: []string{"container"}},
	}
	cfg.CatalogTypes.Capabilities = []api.Capability{{Name: "container"}}
	svc, err := New(stdctx.Background(), cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	return svc, project
}

func stray(t *testing.T, path, id string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	body := "id: " + id + "\ntype: note\nname: " + id + "\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestPlanningSpaceIsFiledAndIndexed: a type filed in `.ktw/plans` writes its
// objects there, and the index finds them again after a full rescan.
func TestPlanningSpaceIsFiledAndIndexed(t *testing.T) {
	svc, project := planningFixture(t)
	ctx := stdctx.Background()
	o := api.Object{ID: "billing.pl-1", Type: "plan"}
	o.Name = "Ship invoicing"
	out, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "billing", Object: o})
	if err != nil {
		t.Fatalf("ObjectCreate: %v", err)
	}
	if filepath.ToSlash(filepath.Dir(out.Path)) != "modules/billing/.ktw/plans" {
		t.Fatalf("filed at %q, want modules/billing/.ktw/plans/", out.Path)
	}
	if _, err := os.Stat(filepath.Join(project, filepath.FromSlash(out.Path))); err != nil {
		t.Fatalf("file not on disk: %v", err)
	}
	if _, err := svc.ObjectScan(ctx, api.ObjectScanInput{}); err != nil {
		t.Fatalf("ObjectScan: %v", err)
	}
	got, err := svc.ObjectList(ctx, api.ObjectListInput{Module: "billing", Type: "plan"})
	if err != nil || len(got.Objects) != 1 || got.Objects[0].ID != "billing.pl-1" {
		t.Fatalf("after a rescan, plans = %+v (%v), want billing.pl-1", got.Objects, err)
	}
}

// TestOnlyTheModulesPlanningSpaceIsAdmitted: every other hidden folder stays
// invisible — another dot folder, and a `.ktw` that is not directly under the
// module.
func TestOnlyTheModulesPlanningSpaceIsAdmitted(t *testing.T) {
	svc, project := planningFixture(t)
	ctx := stdctx.Background()
	mod := filepath.Join(project, "modules", "billing")
	stray(t, filepath.Join(mod, ".ktw", "notes", "billing.seen.cwm.yaml"), "billing.seen")
	stray(t, filepath.Join(mod, ".secret", "billing.hidden.cwm.yaml"), "billing.hidden")
	stray(t, filepath.Join(mod, "sub", ".ktw", "billing.nested.cwm.yaml"), "billing.nested")
	if _, err := svc.ObjectScan(ctx, api.ObjectScanInput{}); err != nil {
		t.Fatalf("ObjectScan: %v", err)
	}
	got, err := svc.ObjectList(ctx, api.ObjectListInput{Module: "billing"})
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]bool{}
	for _, o := range got.Objects {
		ids[o.ID] = true
	}
	if !ids["billing.seen"] {
		t.Errorf("an object in the module's .ktw is not indexed: %v", ids)
	}
	for _, hidden := range []string{"billing.hidden", "billing.nested"} {
		if ids[hidden] {
			t.Errorf("%s is indexed; only the module's own .ktw is admitted", hidden)
		}
	}
}

// TestHiddenFoldersAreRefusedAtDeclaration: a type that would file its objects
// where the index never looks is refused when it is declared.
func TestHiddenFoldersAreRefusedAtDeclaration(t *testing.T) {
	for _, f := range []string{".secret", ".ktw/_drafts", ".ktw/.x", ".ktw/a/b", "_generated", "a/b"} {
		err := api.ObjectType{Name: "x", Folder: f}.ValidateFolder()
		if !errors.Is(err, api.ErrInvalidInput) {
			t.Errorf("folder %q accepted; want it refused", f)
		}
	}
	for _, f := range []string{"plans", ".ktw", ".ktw/plans"} {
		if err := (api.ObjectType{Name: "x", Folder: f}).ValidateFolder(); err != nil {
			t.Errorf("folder %q refused: %v", f, err)
		}
	}
}
