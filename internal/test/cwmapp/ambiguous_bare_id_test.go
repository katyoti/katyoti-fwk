// SPDX-License-Identifier: Apache-2.0

package cwmapp

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// projectFolder makes a project folder of key inside the repository folder
// repo, holding the module folders given.
func projectFolder(t *testing.T, repo, key string, modules ...string) string {
	t.Helper()
	dir := filepath.Join(repo, key)
	for _, m := range modules {
		if err := os.MkdirAll(filepath.Join(dir, "modules", m), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, cwmapi.ProjectFileName), []byte("id: "+key+"\ntype: project\nname: "+key+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// A BARE ID SEVERAL PROJECTS HOLD IS REFUSED, NAMING EACH CANDIDATE
// . Two projects of one repository both have the module crm and
// both hold crm.N-1. New input naming crm.N-1 bare as the object a verb acts
// on - read, update, delete, an edge's source, a tree's root, a resolve with
// no container - is refused as invalid input
// naming alpha:crm.N-1 and beta:crm.N-1, never served from the first project
// holding it; a create in the bare module crm is refused naming alpha:crm and
// beta:crm. Written qualified, each is accepted and reaches its project (a
// list's parent and a search's within narrow to it); bare, those two filters
// match in each project, each row naming its project; an id one project
// alone holds still goes bare. The check is the CWM service's, so
// the CLI, the GUI bindings and an agent's tools meet the same refusal.
func TestABareIDSeveralProjectsHoldIsRefusedNamingEachCandidate(t *testing.T) {
	root := workspaceDir(t)
	repo := filepath.Join(root, "repo")
	alpha := projectFolder(t, repo, "alpha", "crm")
	beta := projectFolder(t, repo, "beta", "crm")
	ctx, svc := Members(t, root, []string{alpha, beta})

	note := func(id, desc, parent string) cwmapi.Object {
		o := cwmapi.Object{ID: id, Type: "note", Parent: parent}
		o.Name, o.Description = id, desc
		return o
	}
	for _, c := range []struct {
		module string
		o      cwmapi.Object
	}{
		{"alpha:crm", note("crm.N-1", "alpha's", "")},
		{"beta:crm", note("crm.N-1", "beta's", "")},
		{"beta:crm", note("crm.N-2", "beta's child", "crm.N-1")},
		{"beta:crm", note("crm.only", "beta alone", "")},
	} {
		if _, err := svc.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: c.module, Object: c.o}); err != nil {
			t.Fatalf("create %s in %s: %v", c.o.ID, c.module, err)
		}
	}

	refused := func(what string, err error, cands ...string) {
		t.Helper()
		if !errors.Is(err, cwmapi.ErrInvalidInput) {
			t.Errorf("%s: %v - want refused as invalid input", what, err)
			return
		}
		for _, c := range cands {
			if !strings.Contains(err.Error(), c) {
				t.Errorf("%s: %v - want the refusal naming %s", what, err, c)
			}
		}
	}
	ids := []string{"alpha:crm.N-1", "beta:crm.N-1"}

	// Bare: refused by every verb, naming both candidates.
	_, err := svc.ObjectGet(ctx, cwmapi.ObjectGetInput{ID: "crm.N-1"})
	refused("get crm.N-1", err, ids...)
	upd := note("crm.N-1", "changed", "")
	_, err = svc.ObjectUpdate(ctx, cwmapi.ObjectUpdateInput{Object: upd})
	refused("update crm.N-1", err, ids...)
	_, err = svc.ObjectDelete(ctx, cwmapi.ObjectDeleteInput{ID: "crm.N-1"})
	refused("delete crm.N-1", err, ids...)
	_, err = svc.EdgeList(ctx, cwmapi.EdgeListInput{ObjectID: "crm.N-1"})
	refused("edges of crm.N-1", err, ids...)
	_, err = svc.EdgeCreate(ctx, cwmapi.EdgeCreateInput{From: "crm.N-1", To: "crm.only", Kind: "depends-on"})
	refused("edge from crm.N-1", err, ids...)
	_, err = svc.ObjectTree(ctx, cwmapi.ObjectTreeInput{Root: "crm.N-1"})
	refused("tree under crm.N-1", err, ids...)
	_, err = svc.ObjectResolve(ctx, cwmapi.ObjectResolveInput{Ref: "crm.N-1"})
	refused("resolve crm.N-1", err, ids...)
	_, err = svc.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "crm", Object: note("crm.N-3", "", "")})
	refused("create in crm", err, "alpha:crm", "beta:crm")

	// Qualified: accepted, in its project.
	got, err := svc.ObjectGet(ctx, cwmapi.ObjectGetInput{ID: "beta:crm.N-1"})
	if err != nil || got.Project != "beta" || got.Object.Description != "beta's" {
		t.Fatalf("get beta:crm.N-1 = %q %q, %v - want beta's", got.Project, got.Object.Description, err)
	}
	got, err = svc.ObjectGet(ctx, cwmapi.ObjectGetInput{ID: "alpha:crm.N-1"})
	if err != nil || got.Project != "alpha" || got.Object.Description != "alpha's" {
		t.Fatalf("get alpha:crm.N-1 = %q %q, %v - want alpha's", got.Project, got.Object.Description, err)
	}
	if _, err := svc.EdgeList(ctx, cwmapi.EdgeListInput{ObjectID: "alpha:crm.N-1"}); err != nil {
		t.Errorf("edges of alpha:crm.N-1: %v", err)
	}
	if _, err := svc.ObjectTree(ctx, cwmapi.ObjectTreeInput{Root: "beta:crm.N-1"}); err != nil {
		t.Errorf("tree under beta:crm.N-1: %v", err)
	}
	list, err := svc.ObjectList(ctx, cwmapi.ObjectListInput{Parent: "beta:crm.N-1"})
	if err != nil || len(list.Objects) != 1 || list.Objects[0].ID != "crm.N-2" || list.Projects[0] != "beta" {
		t.Errorf("list under beta:crm.N-1 = %+v, %v - want beta's crm.N-2 alone", list, err)
	}
	if list, err := svc.ObjectList(ctx, cwmapi.ObjectListInput{Parent: "alpha:crm.N-1"}); err != nil || len(list.Objects) != 0 {
		t.Errorf("list under alpha:crm.N-1 = %+v, %v - want none", list, err)
	}
	found, err := svc.ObjectSearch(ctx, cwmapi.ObjectSearchInput{Query: "crm", Within: "beta:crm.N-1"})
	if err != nil || len(found.Objects) == 0 {
		t.Errorf("search within beta:crm.N-1 = %+v, %v - want beta's objects", found, err)
	}
	for i, p := range found.Projects {
		if p != "beta" {
			t.Errorf("search within beta:crm.N-1 found %s of %q - want beta's only", found.Objects[i].ID, p)
		}
	}
	if res, err := svc.ObjectResolve(ctx, cwmapi.ObjectResolveInput{Ref: "alpha:crm.N-1"}); err != nil || !strings.HasPrefix(filepath.Clean(res.Path), filepath.Clean(alpha)) {
		t.Errorf("resolve alpha:crm.N-1 = %+v, %v - want in %s", res, err, alpha)
	}
	if _, err := svc.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "alpha:crm", Object: note("crm.N-3", "", "alpha:crm.N-1")}); err != nil {
		t.Errorf("create crm.N-3 under alpha:crm.N-1: %v", err)
	}
	if _, err := svc.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "beta:crm", Object: note("crm.N-4", "", "alpha:crm.N-1")}); !errors.Is(err, cwmapi.ErrInvalidInput) {
		t.Errorf("create in beta:crm under alpha:crm.N-1: %v - want refused (a write never crosses projects)", err)
	}

	// A filter of a joined answer is no object to act on: a bare parent or
	// within matches in each project holding it, each row naming its project.
	list, err = svc.ObjectList(ctx, cwmapi.ObjectListInput{Parent: "crm.N-1"})
	if err != nil || len(list.Objects) != 2 {
		t.Errorf("list under bare crm.N-1 = %+v, %v - want both projects' children (crm.N-2 of beta, crm.N-3 of alpha)", list, err)
	}
	found, err = svc.ObjectSearch(ctx, cwmapi.ObjectSearchInput{Query: "crm", Within: "crm.N-1"})
	if err != nil || len(found.Projects) == 0 || !slices.Contains(found.Projects, "alpha") || !slices.Contains(found.Projects, "beta") {
		t.Errorf("search within bare crm.N-1 = %v, %v - want both projects' rows, each named", found.Projects, err)
	}

	// An id one project alone holds goes bare.
	if got, err := svc.ObjectGet(ctx, cwmapi.ObjectGetInput{ID: "crm.only"}); err != nil || got.Project != "beta" {
		t.Errorf("get crm.only = %q, %v - want beta's", got.Project, err)
	}
}
