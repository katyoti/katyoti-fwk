// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"testing/fstest"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/internal/sqlstore"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// : one store serves every project of the universe; a to-do
// its claim and its module locks are keyed by (project, module).

const (
	alphaUUID = "0193a0d3-a5ed-71e3-9f71-b810b046623d"
	betaUUID  = "0193a0d3-a5ed-71e3-9f71-c921c157734e"
)

// twoProjects is a scheduler over one SQLite store whose module references
// resolve as two projects' would: alpha:planification and beta:planification,
// each project's planning space its own folder.
func twoProjects(t *testing.T) (context.Context, *Service, map[string]string) {
	t.Helper()
	ctx, _ := cwmapp.New(t, cwmapp.Pack(fstest.MapFS{"object-types/module.yaml": {Data: []byte("name: module\ncapabilities: [module]\n")}}))
	repos := map[string]string{"alpha:planification": alphaUUID, "beta:planification": betaUUID}
	repoOf := func(_ context.Context, m string) (string, error) {
		if r, ok := repos[m]; ok {
			return r, nil
		}
		return "", fmt.Errorf("%w: module %s", cwmapi.ErrNotFound, m)
	}
	st, err := sqlstore.Open(t.TempDir(), sqlstore.Options{NewID: idOf, Now: now, RepoOf: repoOf})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	svc := NewOver(st)
	svc.repoOf = repoOf
	spaces := map[string]string{"alpha:planification": t.TempDir(), "beta:planification": t.TempDir()}
	svc.planned = func(context.Context) ([]plannedModule, error) {
		return []plannedModule{{ref: "alpha:planification", space: spaces["alpha:planification"]}, {ref: "beta:planification", space: spaces["beta:planification"]}}, nil
	}
	return ctx, svc, spaces
}

// TestTwoProjectsWithOneModuleNameShareNoLockNorQueue: two projects each hold
// planification. Each project's queue lists its own to-dos only; a writer
// holding one project's planification does not hold the other's, while a
// second writer of its own project waits on it; a module switched off in one
// project leaves the other's running; a to-do id stays <module>.TD-…, the
// store's primary key refusing a second row of one id.
func TestTwoProjectsWithOneModuleNameShareNoLockNorQueue(t *testing.T) {
	ctx, svc, spaces := twoProjects(t)
	put := func(module, title string) api.Todo {
		t.Helper()
		out, err := svc.Put(ctx, api.PutInput{Todo: api.Todo{Module: module, Target: "planification", Title: title, Owner: "test",
			For: api.ForAgent, Start: api.StartAuto, Priority: "P1", State: api.StateReady, Writes: []string{"planification"}}})
		if err != nil {
			t.Fatalf("put %s: %v", title, err)
		}
		return out.Todo
	}
	a := put("alpha:planification", "alpha's step")
	b := put("beta:planification", "beta's step")
	for _, c := range []struct {
		td   api.Todo
		repo string
	}{{a, alphaUUID}, {b, betaUUID}} {
		if c.td.Module != "planification" || c.td.Repository != c.repo || !strings.HasPrefix(c.td.ID, "planification.TD-") {
			t.Fatalf("%s: module %q, repository %q, id %s - want planification of %s, its id planification.TD-…", c.td.Title, c.td.Module, c.td.Repository, c.td.ID, c.repo)
		}
	}

	ids := func(f api.Filter) []string {
		t.Helper()
		q, err := svc.Queue(ctx, f)
		if err != nil {
			t.Fatalf("queue %+v: %v", f, err)
		}
		var out []string
		for _, e := range q.Queue {
			out = append(out, e.Todo.ID+"/"+fmt.Sprint(e.Takeable))
		}
		return out
	}
	// Each project's queue: its own to-do only, takeable.
	for _, c := range []struct {
		f    api.Filter
		want string
	}{
		{api.Filter{Project: alphaUUID}, a.ID + "/true"},
		{api.Filter{Project: betaUUID}, b.ID + "/true"},
		{api.Filter{Module: "beta:planification"}, b.ID + "/true"},
		{api.Filter{Module: "alpha:planification", Project: alphaUUID}, a.ID + "/true"},
	} {
		if got := ids(c.f); len(got) != 1 || got[0] != c.want {
			t.Fatalf("queue %+v = %v, want only %s", c.f, got, c.want)
		}
	}
	if _, err := svc.Queue(ctx, api.Filter{Module: "alpha:planification", Project: betaUUID}); err == nil {
		t.Fatal("alpha's module asked in beta's project: answered, want refused")
	}

	// A writer holding alpha's planification does not hold beta's.
	pa, err := svc.Pull(ctx, api.PullInput{By: alice, Filter: api.Filter{Project: alphaUUID}})
	if err != nil || pa.Todo == nil || pa.Todo.ID != a.ID {
		t.Fatalf("alice pulls in alpha: %+v (%v), want %s", pa, err, a.ID)
	}
	if got := ids(api.Filter{Project: betaUUID}); len(got) != 1 || got[0] != b.ID+"/true" {
		t.Fatalf("beta's queue while alpha's planification is written = %v, want %s takeable", got, b.ID)
	}
	pb, err := svc.Pull(ctx, api.PullInput{By: bob, Filter: api.Filter{Module: "beta:planification"}})
	if err != nil || pb.Todo == nil || pb.Todo.ID != b.ID {
		t.Fatalf("bob pulls beta's planification while alpha's is written: %+v (%v), want %s", pb, err, b.ID)
	}
	// A second writer of alpha's planification waits on alice's.
	a2 := put("alpha:planification", "alpha's second step")
	q, err := svc.Queue(ctx, api.Filter{Project: alphaUUID})
	if err != nil || len(q.Queue) != 1 || q.Queue[0].Todo.ID != a2.ID || q.Queue[0].Takeable || q.Queue[0].Wait == nil || q.Queue[0].Wait.HolderTodo != a.ID {
		t.Fatalf("alpha's queue while alice writes its planification = %+v (%v), want %s waiting on %s", q.Queue, err, a2.ID, a.ID)
	}

	// A module switched off in beta leaves alpha's running.
	b2 := put("beta:planification", "beta's second step")
	if err := writeSettings(spaces["beta:planification"], api.Settings{Scheduling: api.ModeOff}); err != nil {
		t.Fatal(err)
	}
	all := strings.Join(ids(api.Filter{}), " ")
	if !strings.Contains(all, a2.ID) || strings.Contains(all, b2.ID) {
		t.Fatalf("queue with beta's planification off = %s, want %s and not %s", all, a2.ID, b2.ID)
	}

	// One store mints every id: a second row of one id is refused.
	fixed := func(module, kind string) string { return module + "." + kind + "-20260101-000000000000-0000" }
	st, err := sqlstore.Open(t.TempDir(), sqlstore.Options{NewID: fixed, Now: now, RepoOf: svc.repoOf})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if _, err := st.Create(ctx, api.Todo{Module: "alpha:planification", Title: "x", State: api.StateReady}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Create(ctx, api.Todo{Module: "beta:planification", Title: "y", State: api.StateReady}); err == nil {
		t.Fatal("beta's to-do minted alpha's id: stored, want refused by the store's key")
	}
}

// TestAProjectIsNamedByItsKeyOrItsUUID: a filter's project is read as the
// uuid its to-dos carry - its key resolves through the CWM to the project
// object's uuid, a uuid is kept, and a key no project carries is refused.
func TestAProjectIsNamedByItsKeyOrItsUUID(t *testing.T) {
	ctx, cwm := cwmapp.New(t, cwmapp.Pack(fstest.MapFS{
		"capabilities/container.yaml": {Data: []byte("name: container\n")},
		"object-types/project.yaml":   {Data: []byte("name: project\ncapabilities: [project, container]\n")},
	}))
	if _, err := cwm.ProjectInit(ctx, cwmapi.ProjectInitInput{Name: "acme"}); err != nil {
		t.Fatalf("ProjectInit: %v", err)
	}
	if _, err := cwm.ObjectScan(ctx, cwmapi.ObjectScanInput{}); err != nil {
		t.Fatal(err)
	}
	svc := NewOver(openSQL(t, t.TempDir(), ""))
	f, err := svc.scopeOf(ctx, api.Filter{Project: "acme"})
	if err != nil || !looksLikeUUID(f.Project) {
		t.Fatalf("project acme = %q (%v), want its uuid", f.Project, err)
	}
	if again, err := svc.scopeOf(ctx, api.Filter{Project: f.Project}); err != nil || again.Project != f.Project {
		t.Fatalf("project %s = %q (%v), want it kept", f.Project, again.Project, err)
	}
	if _, err := svc.scopeOf(ctx, api.Filter{Project: "nosuch"}); err == nil || !strings.Contains(err.Error(), "nosuch") {
		t.Fatalf("project nosuch = %v, want refused naming it", err)
	}
}
