// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// CONTAINMENT NAMES WHAT EXISTS: over the types the packs ship
// every allowed_children entry is a declared type, and every accept_parents
// entry a declared type, a declared capability (a level: module, project,
// system-module) or '*'. A name that is neither - an old type renamed away,
// the bare module - matches nothing, and the type silently stops being offered.
func TestContainmentNamesOnlyDeclaredTypesOrCapabilities(t *testing.T) {
	cfg, _ := realCorpusConfig(t)
	types, caps := map[string]bool{}, map[string]bool{}
	for _, ot := range cfg.CatalogTypes.Objects {
		types[ot.Name] = true
	}
	for _, c := range cfg.CatalogTypes.Capabilities {
		caps[c.Name] = true
	}
	if len(types) < 20 || len(caps) < 10 {
		t.Fatalf("read %d types and %d capabilities - the packs were not loaded", len(types), len(caps))
	}
	var bad []string
	for _, ot := range cfg.CatalogTypes.Objects {
		for _, c := range ot.AllowedChildren {
			if !types[c] {
				bad = append(bad, ot.Name+": allowed_children names "+c+", no declared type")
			}
		}
		for _, p := range ot.AcceptParents {
			if p != "*" && !types[p] && !caps[p] {
				bad = append(bad, ot.Name+": accept_parents names "+p+", neither a declared type nor a capability")
			}
		}
	}
	sort.Strings(bad)
	bad = slices.Compact(bad)
	for _, b := range bad {
		t.Error(b)
	}
}

// A STREAM UNDER A MODULE AGAIN: stream accepts the module level
// (accept_parents [module, system-module]) - the capability a software module's
// type carries, not a type named module - so « + New » under a software module
// offers it and its write passes, as under sys-planification.
func TestAStreamCanBeCreatedUnderASoftwareModuleAndSysPlanification(t *testing.T) {
	cfg, _ := realCorpusConfig(t)
	c := *cfg
	c.WorkspaceRoot = freshWorkspace(t)
	c.ProjectRoot = t.TempDir()
	if err := os.WriteFile(filepath.Join(c.ProjectRoot, api.ProjectFileName), []byte("id: demo\ntype: software-project\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeTypedModule(t, c.ProjectRoot, "billing", "software-module")
	writeTypedModule(t, c.ProjectRoot, "sys-planification", "system-module")
	svc, err := New(stdctx.Background(), &c)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	for _, parent := range []string{"billing", "sys-planification"} {
		out, err := svc.ObjectAllowedChildren(stdctx.Background(), api.ObjectAllowedChildrenInput{ParentID: parent})
		if err != nil {
			t.Fatal(err)
		}
		if !slices.ContainsFunc(out.Choices, func(ch api.TypeChoice) bool { return ch.Name == "stream" }) {
			t.Errorf("« + New » under %s does not offer a stream", parent)
		}
		if err := svc.checkObjectParentChild(api.Object{ID: parent + ".SM-001", Type: "stream", Parent: parent}, "create"); err != nil {
			t.Errorf("a stream under %s is refused: %v", parent, err)
		}
	}
}
