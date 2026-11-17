// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// The detail fixture: a small project written through ObjectCreate — a
// project with two modules, a note, two records and a decision — and the
// object types those objects follow.

const fixtureModule = "live"

func qualify(id string) string { return api.QualifyObjectID(fixtureModule, id) }

func unqualify(id string) string { return strings.TrimPrefix(id, fixtureModule+".") }

// detailFix is the fixture: the service and each object's local id by key.
type detailFix struct {
	svc   *Service
	byKey map[string]string
}

// fixtureObjectTypes are the fixture's types: project → module / note,
// module ← project, note under anything, and the record / decision kinds.
func fixtureObjectTypes() []api.ObjectType {
	return []api.ObjectType{
		{Name: "project", Label: "Project", Tree: api.TreeNode, AllowedChildren: []string{"module", "note", "decision"}},
		{Name: "module", Label: "Module", Tree: api.TreeNode, AcceptParents: []string{"project"},
			AllowedChildren: []string{"record"}, Capabilities: []string{"checkable"}},
		{Name: "note", Label: "Note", Tree: api.TreeNode, AcceptParents: []string{"*"}},
		{Name: "record", Description: "A record", Sections: map[string]map[string]any{
			"context": {"propagation": "local", "resolution_strategy": "local-only"}}},
		{Name: "decision", Description: "A decision", Sections: map[string]map[string]any{
			"context": {"propagation": "inherit-from-ancestors", "resolution_strategy": "closest-wins", "override_key": "topic"}}},
	}
}

func detailFixture(t *testing.T) *detailFix {
	t.Helper()
	cfg := &CWMServiceConfig{WorkspaceRoot: freshWorkspace(t), ProjectRoot: t.TempDir()}
	// `run: check` is read from the capability declaration, the way
	// ktw-methods ships checkable.
	cfg.CatalogTypes.Capabilities = []api.Capability{
		{Name: "container"}, {Name: "content"},
		{Name: "checkable", Hooks: api.Hooks{api.HookVerify: {{URI: "x:acceptance/verify"}}}},
		// The type-level capability the fixture's `context:` sections set.
		{Name: "context", TypeSchema: "x:/data-type/type-context"},
	}
	cfg.CatalogTypes.Objects = fixtureObjectTypes()
	cfg.CatalogTypes.Edges = []api.EdgeTypeRegistration{{Name: "depends-on", From: "any", To: "any", Cardinality: "M:N"}}
	svc, err := New(stdctx.Background(), cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = svc.Close() })

	f := &detailFix{svc: svc, byKey: map[string]string{}}
	mk := func(key, local, typ, parent string, set func(*api.Object)) {
		o := api.Object{ID: qualify(local), Type: typ}
		if parent != "" {
			o.Parent = qualify(f.byKey[parent])
		}
		if set != nil {
			set(&o)
		}
		if _, err := svc.ObjectCreate(stdctx.Background(), api.ObjectCreateInput{Module: fixtureModule, Object: o}); err != nil {
			t.Fatalf("ObjectCreate %s: %v", key, err)
		}
		f.byKey[key] = local
	}
	named := func(name, desc string) func(*api.Object) {
		return func(o *api.Object) {
			o.Name, o.Description = name, desc
		}
	}
	checkable := func(name, status string) func(*api.Object) {
		return func(o *api.Object) {
			named(name, "")(o)
			_ = o.SetCapability("checkable", map[string]any{"probe": "status == 'done'", "status": status})
		}
	}
	record := func(title, body string) func(*api.Object) {
		return func(o *api.Object) { o.Name = title; o.SetFieldsJSON(json.RawMessage(body)) }
	}
	mk("P", "p", "project", "", named("Acme", "the project"))
	mk("M1", "billing", "module", "P", checkable("billing", "done"))
	mk("M2", "shipping", "module", "P", checkable("shipping", "todo"))
	mk("N", "reminder", "note", "M1", named("reminder", ""))
	mk("r1", "r1", "record", "M1", record("invoice run", `{"amount":12}`))
	mk("r2", "r2", "record", "M1", record("refund run", `{"amount":3}`))
	mk("d1", "d1", "decision", "P", record("use stripe", `{"topic":"payments"}`))
	return f
}
