// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/features/api"
	featureslocal "github.com/katyoti/katyoti-fwk/internal/extensions/features/local"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// THE FEATURES BOARD KEEPS TO THE PROJECT HOLDING THE OBJECT
// . A universe serves alpha and beta, both
// holding module m with the bare ids m.f1 (a feature) and m.g (the goal it
// serves), each with its own names and cost of delay. The board of alpha:m - and of project
// alpha - lists alpha's feature only, with its own cost of delay, its goal named and opened as alpha's;
// the same for beta. The bare homonym m is refused (naming the project),
// never both merged.
func TestTheFeaturesBoardOfAQualifiedIdIsItsProjects(t *testing.T) {
	root := t.TempDir()
	t.Setenv("KTW_KATYOTI_ROOT", filepath.Join(root, "installation"))
	application.ResetGlobals()
	t.Cleanup(application.ResetGlobals)

	keys := []string{"alpha", "beta"}
	var members []string
	for _, k := range keys {
		dir := filepath.Join(root, "repo-"+k)
		if err := os.MkdirAll(filepath.Join(dir, "modules"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, cwmapi.ProjectFileName), []byte("id: "+k+"\ntype: project\nname: "+k+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		members = append(members, dir)
	}
	withModule := cwmapp.Pack(fstest.MapFS{"object-types/module.yaml": {Data: []byte("name: module\ntree: node\n")}})
	ctx, cwm := cwmapp.Members(t, root, members, featureslocal.Extension(), pack(), withModule)
	cods := map[string]int{"alpha": 3, "beta": 7}
	for _, k := range keys {
		module := k + ":m"
		mk := func(o cwmapi.Object) {
			t.Helper()
			if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: module, Object: o}); err != nil {
				t.Fatalf("create %s in %s: %v", o.ID, module, err)
			}
		}
		mk(cwmapi.NewContainerObject("m", "m", "module", "", "m", ""))
		mk(cwmapi.Object{ID: "m.g", Type: "goal", Name: k + " goal", Parent: "m"})
		mk(cwmapi.Object{ID: "m.f1", Type: "feature", Name: k + " feature", Parent: "m",
			Capabilities: map[string]map[string]any{"feature": {"cod": cods[k]}},
			Edges:        []cwmapi.ObjectEdge{{Kind: "serves", To: "m.g"}}})
	}
	svc, err := application.UseService[api.FeaturesService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ id, own, other string }{
		{"alpha:m", "alpha", "beta"}, {"beta:m", "beta", "alpha"},
		// A project object's board: its own project's features only.
		{"alpha:alpha", "alpha", "beta"}, {"beta:beta", "beta", "alpha"},
	} {
		page := board(t, ctx, svc, api.BoardInput{ID: c.id})
		for _, want := range []string{c.own + " feature", c.own + " goal", `"navigate":{"object":"` + c.own + `:m.g"}`, `"badge":1`,
			fmt.Sprintf(`"text":"CoD %d"`, cods[c.own])} {
			if !strings.Contains(page, want) {
				t.Errorf("the board of %s lacks %s", c.id, want)
			}
		}
		for _, not := range []string{c.other + " feature", c.other + " goal"} {
			if strings.Contains(page, not) {
				t.Errorf("the board of %s shows %s, another project's", c.id, not)
			}
		}
	}
	if _, err := svc.Board(ctx, api.BoardInput{ID: "m"}); err == nil {
		t.Error("the board of the bare homonym m answers, merging both projects - want it refused")
	}
}
