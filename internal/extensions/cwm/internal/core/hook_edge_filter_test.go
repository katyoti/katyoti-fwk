// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// A HOOK'S FILTER BY EDGE: a `hook` contribution with `incoming: [kind]` is in
// the table of an object an edge of that kind points at, and only there; an
// entry's `outgoing:` filter is listed not applying, naming the missing kind,
// on an object without such an edge. A write sees the edges it is writing.
func TestAHookFilterSelectsByEdge(t *testing.T) {
	cfg := &CWMServiceConfig{WorkspaceRoot: freshWorkspace(t), ProjectRoot: t.TempDir()}
	cfg.CatalogTypes.Edges = []api.EdgeTypeRegistration{{Name: "challenges", From: "any", To: "any", Cardinality: "M:N", Target: "object", ObjectStorage: "edges"}}
	cfg.CatalogTypes.Objects = []api.ObjectType{{Name: "thing",
		Hooks: api.Hooks{api.HookLint: {{URI: "x:lint/challenger", Filter: &api.ObjectFilter{Outgoing: []string{"challenges"}}}}}}}
	cfg.CatalogTypes.Hooks = []api.Hook{
		{Name: "challenged", Provider: "ktw-x", Type: api.HookLint, URI: "x:lint/challenged", Filter: api.ObjectFilter{Incoming: []string{"challenges"}}},
	}
	svc, err := New(stdctx.Background(), cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	for _, o := range []api.Object{
		{ID: "m.target", Type: "thing"},
		{ID: "m.lone", Type: "thing"},
		{ID: "m.c", Type: "thing", Edges: []api.ObjectEdge{{Kind: "challenges", To: "m.target"}}},
	} {
		o.Name = o.ID
		if _, err := svc.ObjectCreate(stdctx.Background(), api.ObjectCreateInput{Module: "m", Object: o}); err != nil {
			t.Fatalf("create %s: %v", o.ID, err)
		}
	}

	if _, ok := table(t, svc, "m.target", api.HookLint)["x:lint/challenged"]; !ok {
		t.Error("the challenged object must carry the incoming-filtered hook")
	}
	for _, id := range []string{"m.lone", "m.c"} {
		if h, ok := table(t, svc, id, api.HookLint)["x:lint/challenged"]; ok {
			t.Errorf("%s has no incoming challenges edge: the hook must not be listed, got %+v", id, h)
		}
	}
	if h := table(t, svc, "m.c", api.HookLint)["x:lint/challenger"]; !h.Applies {
		t.Errorf("the challenger's outgoing-filtered entry must apply: %+v", h)
	}
	h := table(t, svc, "m.target", api.HookLint)["x:lint/challenger"]
	if h.Applies || !strings.Contains(h.Reason, "no outgoing edge of kind [challenges]") {
		t.Errorf("on the target the entry must not apply, naming the missing kind: %+v", h)
	}

	// Before it is stored, an object being created is asked about with the
	// edges it declares.
	fresh := api.Object{ID: "m.new", Type: "thing", Edges: []api.ObjectEdge{{Kind: "challenges", To: "m.lone"}}}
	applies := false
	for _, h := range svc.applyingHooks(fresh, api.HookLint, nil) {
		applies = applies || h.URI == "x:lint/challenger"
	}
	if !applies {
		t.Error("a write must see the edges it writes: the outgoing-filtered entry does not apply to the new object")
	}
}
