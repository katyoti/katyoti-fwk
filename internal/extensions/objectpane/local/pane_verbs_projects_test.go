// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/objectpane/api"
)

// THE PANE'S OTHER VERBS KEEP TO THE PROJECT HOLDING THE OBJECT
// , as Detail does: a universe serves alpha
// and beta, both holding the bare id, and the GUI names it qualified.

// TestTheOptionsOfAQualifiedIdAreItsProjects: alpha: is still proposed
// beta: was accepted - each one's options are its own state's moves, each
// judged allowed by the dry-run on its own project's object.
func TestTheOptionsOfAQualifiedIdAreItsProjects(t *testing.T) {
	root := t.TempDir()
	t.Setenv("KTW_KATYOTI_ROOT", filepath.Join(root, "installation"))
	application.ResetGlobals()
	t.Cleanup(application.ResetGlobals)

	ctx, cwm, svc := paneMembersWith(t, root, map[string]string{
		"object-types/thing.yaml": "name: thing\ntree: node\nworkflow: ladder\n",
		"workflows/ladder.yaml": "name: ladder\ninitial: proposed\nstates: [{name: proposed}, {name: accepted}, {name: closed, terminal: true}]\n" +
			"transitions: [{name: accept, from: [proposed], to: accepted}, {name: close, from: [accepted], to: closed}]\n",
	}, "alpha", "beta")
	for _, k := range []string{"alpha", "beta"} {
		createIn(t, ctx, cwm, k, cwmapi.Object{ID: "m.F-1", Type: "thing", Name: k + " feature"})
	}
	if _, err := cwm.ObjectTransition(ctx, cwmapi.ObjectTransitionInput{ID: "beta:m.F-1", To: "accepted", By: cwmapi.Actor{ID: "p", Kind: "person"}}); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ id, state, move string }{
		{"alpha:m.F-1", "proposed", "accept"},
		{"beta:m.F-1", "accepted", "close"},
	} {
		out, err := svc.TransitionOptions(ctx, api.TransitionOptionsInput{ID: c.id, By: cwmapi.Actor{ID: "p", Kind: "person"}})
		if err != nil {
			t.Fatalf("%s: %v", c.id, err)
		}
		if len(out.Lifecycles) != 1 {
			t.Fatalf("%s: lifecycles %+v, want its own one", c.id, out.Lifecycles)
		}
		lo := out.Lifecycles[0]
		if lo.State != c.state || len(lo.Transitions) != 1 || lo.Transitions[0].Name != c.move || !lo.Transitions[0].Allowed {
			t.Errorf("%s: state %q, transitions %+v - want %q with %s allowed", c.id, lo.State, lo.Transitions, c.state, c.move)
		}
	}
}

// TestTheAttachedFilesOfAQualifiedIdAreItsProjects: each project's
// attaches a file of its own; the qualified id lists that project's file.
func TestTheAttachedFilesOfAQualifiedIdAreItsProjects(t *testing.T) {
	root := t.TempDir()
	t.Setenv("KTW_KATYOTI_ROOT", filepath.Join(root, "installation"))
	application.ResetGlobals()
	t.Cleanup(application.ResetGlobals)

	ctx, cwm, svc := paneMembersWith(t, root, map[string]string{
		"edge-types/attaches.yaml": "name: attaches\ncardinality: M:N\ntarget: path\nobject_storage: edges\n",
	}, "alpha", "beta")
	for _, k := range []string{"alpha", "beta"} {
		createIn(t, ctx, cwm, k, cwmapi.Object{ID: "m.F-1", Type: "thing", Name: k + " feature"})
		id := k + ":m.F-1"
		if _, err := cwm.ObjectFileWrite(ctx, cwmapi.ObjectFileWriteInput{ID: id, Path: k + ".md", Content: []byte(k)}); err != nil {
			t.Fatalf("write %s: %v", id, err)
		}
		if att, err := cwm.ObjectFileAttach(ctx, cwmapi.ObjectFileAttachInput{ID: id, Patterns: []string{k + ".md"}}); err != nil || len(att.Added) != 1 {
			t.Fatalf("attach %s: %+v %v", id, att, err)
		}
	}
	for _, k := range []string{"alpha", "beta"} {
		out, err := svc.AttachedFiles(ctx, api.AttachedFilesInput{ID: k + ":m.F-1"})
		if err != nil {
			t.Fatalf("%s: %v", k, err)
		}
		if len(out.Files) != 1 || out.Files[0].Name != k+".md" || out.Files[0].Missing || out.Files[0].Outside || out.Module != "m" {
			t.Errorf("%s:m.F-1 files %+v (module %q): want its own %s.md, read in module m", k, out.Files, out.Module, k)
		}
	}
}

// TestTheContentsOfAQualifiedIdAreItsProjects: holds a child of its own
// in each project; the contents of alpha: are alpha's child only, never
// itself, and the same for beta. A project object's contents are its own
// project's objects only.
func TestTheContentsOfAQualifiedIdAreItsProjects(t *testing.T) {
	root := t.TempDir()
	t.Setenv("KTW_KATYOTI_ROOT", filepath.Join(root, "installation"))
	application.ResetGlobals()
	t.Cleanup(application.ResetGlobals)

	ctx, cwm, svc := paneMembersWith(t, root, map[string]string{
		"capabilities/project.yaml": "name: project\n",
		"object-types/project.yaml": "name: project\ncapabilities: [project]\ntree: root\n",
	}, "alpha", "beta")
	for _, k := range []string{"alpha", "beta"} {
		createIn(t, ctx, cwm, k, cwmapi.Object{ID: "m.F-1", Type: "thing", Name: k + " feature"})
	}
	createIn(t, ctx, cwm, "alpha", cwmapi.Object{ID: "m.C-1", Type: "thing", Name: "alpha child", Parent: "m.F-1"})
	createIn(t, ctx, cwm, "beta", cwmapi.Object{ID: "m.C-2", Type: "thing", Name: "beta child", Parent: "m.F-1"})
	createIn(t, ctx, cwm, "beta", cwmapi.Object{ID: "m.U-2", Type: "thing", Name: "only beta's"})

	contents := func(id string) string {
		t.Helper()
		d, err := svc.Contents(ctx, api.ContentsInput{ID: id})
		if err != nil {
			t.Fatalf("contents of %s: %v", id, err)
		}
		var got []string
		for _, ids := range folds(t, d) {
			got = append(got, ids...)
		}
		sort.Strings(got)
		return strings.Join(got, ",")
	}
	// A row opens its object by the qualified id.
	if got := contents("alpha:m.F-1"); got != "alpha:m.C-1" {
		t.Errorf("contents of alpha:m.F-1 = %s, want alpha's child alpha:m.C-1 only", got)
	}
	if got := contents("beta:m.F-1"); got != "beta:m.C-2" {
		t.Errorf("contents of beta:m.F-1 = %s, want beta's child beta:m.C-2 only", got)
	}
	// A project's contents: its own project's objects, never another's.
	if got := contents("alpha:alpha"); !strings.Contains(got, "m.C-1") || strings.Contains(got, "m.C-2") || strings.Contains(got, "m.U-2") {
		t.Errorf("contents of project alpha = %s, want alpha's objects only", got)
	}
	if got := contents("beta:beta"); !strings.Contains(got, "m.U-2") || strings.Contains(got, "m.C-1") {
		t.Errorf("contents of project beta = %s, want beta's objects only", got)
	}
}
