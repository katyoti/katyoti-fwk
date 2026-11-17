// SPDX-License-Identifier: Apache-2.0

package storage

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// TestLoadLayeredMethodPacks pins the type vocabulary the packs ship
// together (CWM's own, ktw-agents, the method packs, a sidecar's): the generic ladder,
// the code-specific and the compose types, the containment the composition
// surfaces rely on, the method's edges, and the types no pack ships any more.
// privatePacks are the sidecar packs of the private build loaded with the others
// (methodpack_private_test.go); the public build has none.
var privatePacks []string

func TestLoadLayeredMethodPacks(t *testing.T) {
	up := filepath.Join("..", "..", "..", "..", "..")
	dirs := []string{
		filepath.Join(up, "internal", "extensions", "cwm", "contrib", "embed", "cwm"), // CWM's own types: project
		filepath.Join(up, "app", "ktw-agents", "embed", "cwm"),
		filepath.Join(up, "app", "ktw-methods", "embed", "cwm"),
		filepath.Join(up, "app", "ktw-code", "embed", "cwm"),
		filepath.Join(up, "app", "ktw-compose", "embed", "cwm"),
	}
	for _, pack := range privatePacks {
		dirs = append(dirs, filepath.Join(up, "app", pack, "embed", "cwm"))
	}
	var sources []fs.FS
	for _, d := range dirs {
		if _, err := os.Stat(d); err != nil {
			t.Fatalf("embed dir not reachable from test cwd: %s: %v", d, err)
		}
		sources = append(sources, os.DirFS(d))
	}
	types, err := LoadObjectTypesLayered(sources...)
	if err != nil {
		t.Fatalf("LoadObjectTypesLayered: %v", err)
	}
	byName := map[string]api.ObjectType{}
	for _, ot := range types {
		byName[ot.Name] = ot
	}
	for _, name := range []string{"software-project", "feature", "documentation", "composition", "compose-artifact",
		"use-case", "acceptance-criterion", "test-case", "charter", "test-run"} {
		if _, ok := byName[name]; !ok {
			t.Errorf("object type %q not shipped", name)
		}
	}
	// Retired: work is a request (its steps the scheduler's to-dos); what is
	// delivered is a use case; an audit is a review with its findings; nothing
	// writes a spawn record; an action run is journaled on its object; durable
	// data is objects of their own types, never a generic record / records /
	// shared-state, and nothing creates a workspace or runbook object; a
	// dossier is a project of its own type; the bare module and project types
	// are retired.
	for _, name := range []string{"task", "deliverable", "audit-bundle", "audit-finding", "spawn-record", "action-invocation",
		"record", "records", "workspace", "shared-state", "runbook", "dossier", "module", "code-module", "project"} {
		if _, ok := byName[name]; ok {
			t.Errorf("object type %q is shipped; it was retired", name)
		}
	}

	// The method's want node is `feature`; `user-story` is no type.
	if _, ok := byName["user-story"]; ok {
		t.Error("object type user-story is shipped; the want node is feature")
	}

	comp := byName["composition"]
	for _, kind := range []string{"compose-artifact", "feature", "goal", "use-case", "test-case"} {
		if !contains(comp.AllowedChildren, kind) {
			t.Errorf("composition allowed_children missing %q: %v", kind, comp.AllowedChildren)
		}
	}
	if agentText(comp) == "" {
		t.Error("composition's agent system text is empty — it frames every chat on the object")
	}
	foundAction := false
	for _, a := range comp.Actions {
		if strings.Contains(a.URI, "ktw-compose:tasks/compose-artifact") {
			foundAction = true
		}
	}
	if !foundAction {
		t.Errorf("composition.actions = %+v; want ktw-compose:tasks/compose-artifact", comp.Actions)
	}

	ca := byName["compose-artifact"]
	if !contains(ca.AcceptParents, "composition") {
		t.Errorf("compose-artifact accept_parents = %v; want [composition]", ca.AcceptParents)
	}

	// Edge kinds: generic in methods, code-specific in code.
	reg, err := LoadTypesLayered(sources...)
	if err != nil {
		t.Fatalf("LoadTypesLayered: %v", err)
	}
	for _, name := range []string{"verifies", "deferred-by", "closes", "serves", "wants"} {
		if _, ok := reg.EdgeType(api.EdgeType(name)); !ok {
			t.Errorf("EdgeType(%q) not registered", name)
		}
	}
	// Retired: what waits on what is depends-on; what a request concerns is about.
	for _, name := range []string{"addresses", "impacts", "dependencies"} {
		if _, ok := reg.EdgeType(api.EdgeType(name)); ok {
			t.Errorf("EdgeType(%q) is registered; it was retired", name)
		}
	}
	w, _ := reg.EdgeType(api.EdgeType("wants"))
	if w.From != "feature" || w.To != "use-case" {
		t.Errorf("wants edge = %q→%q want feature→use-case", w.From, w.To)
	}
	if sv, _ := reg.EdgeType(api.EdgeType("serves")); sv.From != "feature" {
		t.Errorf("serves edge from %q want feature", sv.From)
	}
}

func contains(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}
