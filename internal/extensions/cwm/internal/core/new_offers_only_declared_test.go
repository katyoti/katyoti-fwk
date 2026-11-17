// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// « + NEW » OFFERS ONLY WHAT THE OBJECT CAN HOLD: over the types
// the packs ship, a software project and a project group offer exactly the
// children they declare - their modules' types - and a system module its own
// list, never every type that accepts any parent ("*").
func TestNewOffersOnlyTheDeclaredChildren(t *testing.T) {
	cfg, _ := realCorpusConfig(t)
	offers := func(t *testing.T, projectType, parent string) []string {
		t.Helper()
		c := *cfg
		c.WorkspaceRoot = freshWorkspace(t)
		c.ProjectRoot = t.TempDir()
		if err := os.WriteFile(filepath.Join(c.ProjectRoot, api.ProjectFileName), []byte("id: demo\ntype: "+projectType+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		writeTypedModule(t, c.ProjectRoot, "sys-planification", "system-module")
		svc, err := New(stdctx.Background(), &c)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = svc.Close() })
		out, err := svc.ObjectAllowedChildren(stdctx.Background(), api.ObjectAllowedChildrenInput{ParentID: parent})
		if err != nil {
			t.Fatal(err)
		}
		var names []string
		for _, ch := range out.Choices {
			names = append(names, ch.Name)
		}
		slices.Sort(names)
		return names
	}
	for _, typ := range []string{"software-project"} {
		t.Run(typ, func(t *testing.T) {
			if got := offers(t, typ, "demo"); !slices.Equal(got, []string{"documentation", "software-module", "system-module"}) {
				t.Errorf("%s offers %v; want its declared modules only", typ, got)
			}
		})
	}
	t.Run("system-module", func(t *testing.T) {
		if got := offers(t, "software-project", "sys-planification"); !slices.Equal(got, []string{"inbox", "journal-day", "publication", "release", "request", "stream"}) {
			t.Errorf("sys-planification offers %v; want its declared children only", got)
		}
	})
}

func writeTypedModule(t *testing.T, project, module, typ string) {
	t.Helper()
	dir := filepath.Join(project, "modules", module, ObjectsDirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	o := api.Object{ID: module, Type: typ, Capabilities: map[string]map[string]any{"planned": {}}}
	o.Name = module
	if err := WriteObjectFile(filepath.Join(dir, module+ObjectFileSuffix), o); err != nil {
		t.Fatal(err)
	}
}

// UNDER A SOFTWARE MODULE TOO: it offers the method objects
// requests, streams, findings, reviews, decks and portfolio it holds - never
// every type that accepts any parent.
func TestASoftwareModuleOffersOnlyWhatItHolds(t *testing.T) {
	cfg, _ := realCorpusConfig(t)
	c := *cfg
	c.WorkspaceRoot = freshWorkspace(t)
	c.ProjectRoot = t.TempDir()
	if err := os.WriteFile(filepath.Join(c.ProjectRoot, api.ProjectFileName), []byte("id: demo\ntype: software-project\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeTypedModule(t, c.ProjectRoot, "billing", "software-module")
	svc, err := New(stdctx.Background(), &c)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	out, err := svc.ObjectAllowedChildren(stdctx.Background(), api.ObjectAllowedChildrenInput{ParentID: "billing"})
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, ch := range out.Choices {
		got = append(got, ch.Name)
	}
	slices.Sort(got)
	want := []string{"acceptance-criterion", "decision", "document", "feature", "finding", "goal", "portfolio", "request", "review", "rule", "slide-deck", "slide-group", "stream", "use-case"}
	if !slices.Equal(got, want) {
		t.Errorf("a software module offers %v; want %v", got, want)
	}
}

// UNDER ANY OBJECT, WHAT NAMES IT: a goal or a feature, which
// declare no children, offer only the types naming their type or a level they
// carry - never a type accepting any parent (a finding, a review, an
// information source) nor one saying nothing.
func TestAnObjectDeclaringNoChildrenOffersOnlyWhatNamesIt(t *testing.T) {
	cfg, _ := realCorpusConfig(t)
	c := *cfg
	c.WorkspaceRoot = freshWorkspace(t)
	c.ProjectRoot = t.TempDir()
	if err := os.WriteFile(filepath.Join(c.ProjectRoot, api.ProjectFileName), []byte("id: demo\ntype: software-project\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeTypedModule(t, c.ProjectRoot, "billing", "software-module")
	svc, err := New(stdctx.Background(), &c)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	ctx := stdctx.Background()
	types := svc.objectTypes()
	for _, typ := range []string{"goal", "feature"} {
		o := api.Object{ID: "billing." + typ + "-1", Type: typ, Parent: "billing"}
		o.Name = typ
		if _, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "billing", Object: o}); err != nil {
			t.Fatalf("create the %s: %v", typ, err)
		}
		out, err := svc.ObjectAllowedChildren(ctx, api.ObjectAllowedChildrenInput{ParentID: o.ID})
		if err != nil {
			t.Fatal(err)
		}
		pt := types[typ]
		for _, ch := range out.Choices {
			ct := types[ch.Name]
			if !slices.Contains(pt.AllowedChildren, ch.Name) && !ct.NamesParentType(typ) && !namesALevel(ct, pt) {
				t.Errorf("under a %s « + New » offers %s, which does not name it (accept_parents %v)", typ, ch.Name, ct.AcceptParents)
			}
		}
		for _, any := range []string{"finding", "review", "information-source"} {
			if slices.ContainsFunc(out.Choices, func(ch api.TypeChoice) bool { return ch.Name == any }) {
				t.Errorf("under a %s « + New » offers %s, which accepts any parent", typ, any)
			}
		}
	}
}

// NO OBJECT OFFERS AN INCREMENT: an increment comes from
// a study's hand-over (k-study --decide), never from « + New » - neither
// sys-planification, a module nor the project offers one.
func TestNoObjectOffersAnIncrement(t *testing.T) {
	cfg, _ := realCorpusConfig(t)
	c := *cfg
	c.WorkspaceRoot = freshWorkspace(t)
	c.ProjectRoot = t.TempDir()
	if err := os.WriteFile(filepath.Join(c.ProjectRoot, api.ProjectFileName), []byte("id: demo\ntype: software-project\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeTypedModule(t, c.ProjectRoot, "sys-planification", "system-module")
	writeTypedModule(t, c.ProjectRoot, "billing", "software-module")
	svc, err := New(stdctx.Background(), &c)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	for _, parent := range []string{"sys-planification", "billing", "demo"} {
		out, err := svc.ObjectAllowedChildren(stdctx.Background(), api.ObjectAllowedChildrenInput{ParentID: parent})
		if err != nil {
			t.Fatal(err)
		}
		if slices.ContainsFunc(out.Choices, func(ch api.TypeChoice) bool { return ch.Name == "increment" }) {
			t.Errorf("%s offers an increment in « + New »; an increment comes from a study's hand-over", parent)
		}
	}
}
