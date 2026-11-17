// SPDX-License-Identifier: Apache-2.0

package storeset

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/internal/core"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/internal/storage"
)

// projectStore opens a store on one project folder, with a workspace of its
// own (its runtime objects), and mints its project object named name.
func projectStore(t *testing.T, folder, name string) *core.Service {
	t.Helper()
	ws := t.TempDir()
	if err := storage.WriteWorkspaceRaw(ws, []byte("version: \"1\"\n")); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	svc, err := core.New(context.Background(), &core.CWMServiceConfig{WorkspaceRoot: ws, ProjectRoot: folder})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	if name != "" {
		if _, err := svc.ProjectInit(context.Background(), api.ProjectInitInput{Name: name}); err != nil {
			t.Fatalf("ProjectInit %s: %v", name, err)
		}
	}
	return svc
}

func note(id string) api.Object {
	o := api.Object{ID: id, Type: "note"}
	o.Name = id
	return o
}

// objectFiles are the object files under a project folder, folder-relative.
func objectFiles(t *testing.T, folder string) []string {
	t.Helper()
	var ids []string
	err := filepath.WalkDir(folder, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		if strings.HasSuffix(d.Name(), ".cwm.yaml") {
			rel, _ := filepath.Rel(folder, p)
			ids = append(ids, filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(ids)
	return ids
}

func located(objs []Located) []string {
	out := make([]string, len(objs))
	for i, l := range objs {
		out[i] = l.Project + ":" + l.Object.ID
	}
	return out
}

// TestTwoProjectStoresOfOneRepositoryServeOneIndex: two project
// folders of one repository, each served by a store of its own, read as one -
// listed and searched across both - and written each in its own: every write
// lands in the project holding the object or its module, each project's files
// hold only its own objects, and a bare id or module both hold is refused.
func TestTwoProjectStoresOfOneRepositoryServeOneIndex(t *testing.T) {
	ctx := context.Background()
	repo := t.TempDir()
	alphaDir, betaDir := filepath.Join(repo, "alpha"), filepath.Join(repo, "beta")
	set, err := New(projectStore(t, alphaDir, "alpha"), projectStore(t, betaDir, "beta"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if got := set.Keys(); len(got) != 2 || got[0] != "alpha" || got[1] != "beta" {
		t.Fatalf("keys %v, want [alpha beta]", got)
	}

	// The same id in a module of the same name, in each project, named.
	for _, p := range []string{"alpha", "beta"} {
		if key, _, err := set.ObjectCreate(ctx, p, api.ObjectCreateInput{Module: "inbox", Object: note("inbox.n1")}); err != nil || key != p {
			t.Fatalf("create inbox.n1 in %s: key %q, %v", p, key, err)
		}
	}
	if _, _, err := set.ObjectCreate(ctx, "alpha", api.ObjectCreateInput{Module: "billing", Object: note("billing.spec")}); err != nil {
		t.Fatalf("create billing.spec: %v", err)
	}
	// A module only one project has: the write goes there unnamed.
	if key, _, err := set.ObjectCreate(ctx, "", api.ObjectCreateInput{Module: "billing", Object: note("billing.second")}); err != nil || key != "alpha" {
		t.Fatalf("create billing.second unnamed: key %q, %v - want alpha", key, err)
	}
	// A module both have, unnamed: refused, naming both.
	if _, _, err := set.ObjectCreate(ctx, "", api.ObjectCreateInput{Module: "inbox", Object: note("inbox.n2")}); !errors.Is(err, api.ErrInvalidInput) || !strings.Contains(err.Error(), "alpha, beta") {
		t.Fatalf("create in a module both hold, unnamed: %v - want a refusal naming alpha, beta", err)
	}
	// A module neither has, unnamed: nowhere to write.
	if _, _, err := set.ObjectCreate(ctx, "", api.ObjectCreateInput{Module: "nowhere", Object: note("nowhere.x")}); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("create in a module no project has: %v - want not found", err)
	}

	// Reads across both stores.
	list, err := set.ObjectList(ctx, api.ObjectListInput{Module: "inbox"})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Join(located(list.Objects), " "), "alpha:inbox.n1 beta:inbox.n1"; got != want || list.Total != 2 {
		t.Fatalf("list inbox: %s (total %d), want %s", got, list.Total, want)
	}
	all, err := set.ObjectList(ctx, api.ObjectListInput{Limit: 3})
	if err != nil {
		t.Fatal(err)
	}
	// alpha, beta (the project objects), billing.second, billing.spec, inbox.n1 x2.
	if all.Total != 6 || !all.Truncated || len(all.Objects) != 3 {
		t.Fatalf("list all limit 3: %v total %d truncated %v - want 3 of 6", located(all.Objects), all.Total, all.Truncated)
	}
	found, err := set.ObjectSearch(ctx, api.ObjectSearchInput{Query: "n1"})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Join(located(found.Objects), " "), "alpha:inbox.n1 beta:inbox.n1"; got != want {
		t.Fatalf("search n1: %s, want %s", got, want)
	}
	found, err = set.ObjectSearch(ctx, api.ObjectSearchInput{Query: "billing", Mode: api.SearchFullText})
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range found.Objects {
		if l.Project != "alpha" || l.Hit == nil {
			t.Fatalf("fulltext billing: %v - want alpha's objects with their hit", located(found.Objects))
		}
	}
	if len(found.Objects) != 2 {
		t.Fatalf("fulltext billing: %v - want alpha's two billing objects", located(found.Objects))
	}

	// One object: routed to the store holding it.
	if key, got, err := set.ObjectGet(ctx, "", api.ObjectGetInput{Module: "billing", ID: "billing.spec"}); err != nil || key != "alpha" || got.Object.ID != "billing.spec" {
		t.Fatalf("get billing.spec unnamed: %q %v", key, err)
	}
	if _, _, err := set.ObjectGet(ctx, "", api.ObjectGetInput{Module: "inbox", ID: "inbox.n1"}); !errors.Is(err, api.ErrInvalidInput) {
		t.Fatalf("get inbox.n1 unnamed: %v - want a refusal (both projects hold it)", err)
	}
	key, got, err := set.ObjectGet(ctx, "beta", api.ObjectGetInput{Module: "inbox", ID: "inbox.n1"})
	if err != nil || key != "beta" {
		t.Fatalf("get beta's inbox.n1: %q %v", key, err)
	}
	upd := got.Object
	upd.Description = "beta's own"
	if key, _, err := set.ObjectUpdate(ctx, "beta", api.ObjectUpdateInput{Module: "inbox", Object: upd, Revision: got.Revision}); err != nil || key != "beta" {
		t.Fatalf("update beta's inbox.n1: %q %v", key, err)
	}
	if _, a, err := set.ObjectGet(ctx, "alpha", api.ObjectGetInput{Module: "inbox", ID: "inbox.n1"}); err != nil || a.Object.Description != "" {
		t.Fatalf("alpha's inbox.n1 after beta's update: %q %v - want it untouched", a.Object.Description, err)
	}
	if _, _, err := set.ObjectGet(ctx, "gamma", api.ObjectGetInput{ID: "inbox.n1"}); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("get from a project not served: %v - want not found", err)
	}

	// Each project's files hold only its own objects.
	if got, want := strings.Join(objectFiles(t, alphaDir), " "), "modules/billing/objects/second.cwm.yaml modules/billing/objects/spec.cwm.yaml modules/inbox/objects/n1.cwm.yaml project.cwm.yaml"; got != want {
		t.Fatalf("alpha's files: %s, want %s", got, want)
	}
	if got, want := strings.Join(objectFiles(t, betaDir), " "), "modules/inbox/objects/n1.cwm.yaml project.cwm.yaml"; got != want {
		t.Fatalf("beta's files: %s, want %s", got, want)
	}
}

// TestAStoreSetRefusesAStoreWithoutAProjectOfItsOwn: a store is addressed by
// its project's key - one without a project object, or a second store of the
// same project, has none of its own.
func TestAStoreSetRefusesAStoreWithoutAProjectOfItsOwn(t *testing.T) {
	repo := t.TempDir()
	alpha := projectStore(t, filepath.Join(repo, "alpha"), "alpha")
	if _, err := New(alpha, projectStore(t, filepath.Join(repo, "bare"), "")); !errors.Is(err, api.ErrInvalidInput) {
		t.Fatalf("a store with no project object: %v - want refused", err)
	}
	if _, err := New(alpha, projectStore(t, filepath.Join(repo, "alpha"), "")); !errors.Is(err, api.ErrInvalidInput) {
		t.Fatalf("two stores of one project: %v - want refused", err)
	}
}
