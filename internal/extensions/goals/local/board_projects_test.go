// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/goals/api"
	goalslocal "github.com/katyoti/katyoti-fwk/internal/extensions/goals/local"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// THE GOALS TAB KEEPS TO THE PROJECT HOLDING THE OBJECT (
// ). A universe serves alpha and beta, both holding module m and the
// bare goal, each with a sub-goal of its own; the GUI names them
// qualified. The board of alpha: lists alpha's sub-goal only, the board
// of alpha:m alpha's goals only - never beta's - and what serves a goal is
// counted in its own project.
func TestTheGoalsBoardOfAQualifiedIdIsItsProjects(t *testing.T) {
	root := t.TempDir()
	t.Setenv("KTW_KATYOTI_ROOT", filepath.Join(root, "installation"))
	application.ResetGlobals()
	t.Cleanup(application.ResetGlobals)

	var members []string
	for _, k := range []string{"alpha", "beta"} {
		dir := filepath.Join(root, "repo-"+k)
		if err := os.MkdirAll(filepath.Join(dir, "modules"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, cwmapi.ProjectFileName), []byte("id: "+k+"\ntype: project\nname: "+k+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		members = append(members, dir)
	}
	ctx, cwm := cwmapp.Members(t, root, members, goalslocal.Extension(), cwmapp.Pack(fstest.MapFS{
		"object-types/module.yaml": {Data: []byte("name: module\ntree: node\n")},
		"object-types/thing.yaml":  {Data: []byte("name: thing\ntree: node\n")},
		"object-types/goal.yaml":   {Data: []byte("name: goal\ncapabilities: [want]\n")},
	}))
	create := func(key string, o cwmapi.Object) {
		t.Helper()
		if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: key + ":m", Object: o}); err != nil {
			t.Fatalf("create %s in %s: %v", o.ID, key, err)
		}
	}
	for _, k := range []string{"alpha", "beta"} {
		create(k, cwmapi.NewContainerObject("m", "m", "module", "", "m", ""))
		create(k, cwmapi.Object{ID: "m.G-1", Type: api.GoalType, Name: k + " goal", Parent: "m"})
		create(k, cwmapi.Object{ID: "m.G-" + map[string]string{"alpha": "2", "beta": "3"}[k], Type: api.GoalType, Name: k + " sub-goal", Parent: "m.G-1"})
	}
	// alpha's sub-goal is served by a thing of alpha; beta's by nothing.
	create("alpha", cwmapi.Object{ID: "m.S-1", Type: "thing", Name: "alpha server", Parent: "m",
		Edges: []cwmapi.ObjectEdge{{Kind: api.EdgeServes, To: "m.G-2"}}})
	svc, err := application.UseService[api.GoalsService](ctx)
	if err != nil {
		t.Fatal(err)
	}

	board := func(id string) string {
		t.Helper()
		return boardJSON(t, ctx, svc, api.BoardInput{ID: id})
	}
	has := func(id, page string, want, not []string) {
		t.Helper()
		for _, w := range want {
			if !strings.Contains(page, w) {
				t.Errorf("the board of %s lacks %s", id, w)
			}
		}
		for _, n := range not {
			if strings.Contains(page, n) {
				t.Errorf("the board of %s lists %s", id, n)
			}
		}
	}
	// A row's own link is its name as "text":"<name>" (its parent's link reads "↳ <name>").
	text := func(names ...string) []string {
		out := make([]string, len(names))
		for i, n := range names {
			out[i] = `"text":"` + n + `"`
		}
		return out
	}
	has("alpha:m.G-1", board("alpha:m.G-1"), append(text("alpha sub-goal", "served by 1"), `"badge":1`),
		text("beta sub-goal", "beta goal", "alpha goal", "alpha server"))
	has("beta:m.G-1", board("beta:m.G-1"), append(text("beta sub-goal", "nothing serves it"), `"badge":1`),
		text("alpha sub-goal", "alpha goal", "beta goal"))
	has("alpha:m", board("alpha:m"), append(text("alpha goal", "alpha sub-goal"), `"badge":2`),
		text("beta goal", "beta sub-goal", "alpha server"))
	has("beta:m", board("beta:m"), append(text("beta goal", "beta sub-goal"), `"badge":2`),
		text("alpha goal", "alpha sub-goal"))
}
