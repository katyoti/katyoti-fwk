// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// PHASE 7b — the index holds metadata and capabilities; bodies load on demand;
// draw, load, aspects and panels read the ONE effective set (type ∪ instance).

var splitTypes = []api.ObjectType{
	// Authored types declare `tree:` themselves (the derivation default adds it
	// only to types DERIVED from node declarations).
	{Name: "module", Capabilities: []string{"container", "module"}, Tree: api.TreeNode},
	{Name: "goal", Capabilities: []string{"container", "want", "checkable"}, Tree: api.TreeNode},
	{Name: "note", Capabilities: []string{"content"}},
	{Name: "log", Capabilities: []string{"content"}, Tree: api.TreeNode, Load: LoadNeverEager},
}

var splitCaps = []api.Capability{
	{Name: "container"}, {Name: "content"}, {Name: "module"}, {Name: "want"},
	{Name: "checkable", Hooks: api.Hooks{api.HookVerify: {{URI: "x:acceptance/verify"}}}},
	{Name: "composition"},
}

func splitFixture(t *testing.T) (*Service, string) {
	t.Helper()
	root := freshWorkspace(t)
	project := t.TempDir()
	cfg := &CWMServiceConfig{WorkspaceRoot: root, ProjectRoot: project}
	cfg.CatalogTypes.Objects = splitTypes
	cfg.CatalogTypes.Capabilities = splitCaps
	cfg.CatalogTypes.Edges = []api.EdgeTypeRegistration{{Name: "serves", From: "any", To: "any", Cardinality: "M:N"}}
	svc, err := New(stdctx.Background(), cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	ctx := stdctx.Background()
	mk := func(o api.Object) {
		if _, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "m", Object: o}); err != nil {
			t.Fatalf("create %s: %v", o.ID, err)
		}
	}
	m := api.Object{ID: "m", Type: "module"}
	m.Name = "m"
	m.Description = "the billing module"
	mk(m)
	g := api.Object{ID: "m.g1", Type: "goal"}
	g.Name = "g1"
	g.Parent = "m"
	g.Description = "invoices out by day one"
	g.Name = "Invoices ship"
	_ = g.SetCapability("want", map[string]any{"reachability": "closable"})
	mk(g)
	n := api.Object{ID: "m.n1", Type: "note"}
	n.Name = "n1"
	n.Parent = "m"
	n.Name = "a long note"
	n.Tags = []string{"draft"}
	n.SetFieldsJSON(json.RawMessage(`{"text":"` + strings.Repeat("prose ", 200) + `"}`))
	mk(n)
	return svc, project
}

// THE BODY IS NOT RESIDENT: list and tree answer without it; get hydrates it.
func TestBodyIsNotResidentButGetHydratesIt(t *testing.T) {
	svc, _ := splitFixture(t)
	ctx := stdctx.Background()
	lst, err := svc.ObjectList(ctx, api.ObjectListInput{Type: "note"})
	if err != nil || len(lst.Objects) != 1 {
		t.Fatalf("list: %v %d", err, len(lst.Objects))
	}
	if c := lst.Objects[0]; len(c.FieldsJSON()) != 0 {
		t.Errorf("ObjectList returned a BODY (%d bytes) — the index must hold metadata only", len(c.FieldsJSON()))
	} else if c.Name != "a long note" || len(c.Tags) != 1 {
		t.Errorf("resident content lost its title / tags: %+v", c)
	}
	tree, _ := svc.ObjectTree(ctx, api.ObjectTreeInput{Root: "m"})
	for _, ch := range tree.Roots[0].Children {
		if c := ch.Object; len(c.FieldsJSON()) != 0 {
			t.Errorf("ObjectTree returned a body for %s", ch.Object.ID)
		}
	}
	got, err := svc.ObjectGet(ctx, api.ObjectGetInput{Module: "m", ID: "m.n1"})
	if err != nil {
		t.Fatal(err)
	}
	if c := got.Object; !strings.Contains(string(c.FieldsJSON()), "prose") {
		t.Errorf("ObjectGet did not hydrate the body: %q", string(c.FieldsJSON()))
	}
}

// AN EDGE EDIT MUST NOT DROP THE BODY it never loaded.
func TestEdgeEditKeepsTheBody(t *testing.T) {
	svc, _ := splitFixture(t)
	ctx := stdctx.Background()
	if _, err := svc.EdgeCreate(ctx, api.EdgeCreateInput{From: "m.n1", To: "m.g1", Kind: "serves"}); err != nil {
		t.Fatal(err)
	}
	got, _ := svc.ObjectGet(ctx, api.ObjectGetInput{Module: "m", ID: "m.n1"})
	if c := got.Object; !strings.Contains(string(c.FieldsJSON()), "prose") {
		t.Fatalf("the body was DROPPED by an edge edit — rewriteObject wrote the resident copy")
	}
	if len(got.Object.Edges) != 1 {
		t.Errorf("edges = %v", got.Object.Edges)
	}
}

// THE ONE EFFECTIVE SET: type ∪ instance, read by aspects, drawing and lists.
func TestEffectiveSetIsTypeUnionInstance(t *testing.T) {
	svc, _ := splitFixture(t)
	ctx := stdctx.Background()
	// The goal carries only `want` as a group; its type declares checkable too.
	eff, err := svc.ObjectEffectiveCapabilities(ctx, api.ObjectEffectiveCapabilitiesInput{ID: "m.g1"})
	if err != nil {
		t.Fatal(err)
	}
	src := map[string]string{}
	verify := map[string]bool{}
	for _, a := range eff.Capabilities {
		src[a.ID] = a.Source
		verify[a.ID] = a.Verify
	}
	// Who brings each: the type declares all three, whether or not the goal
	// fills a group for it.
	if src["want"] != "type" || src["checkable"] != "type" || src["container"] != "type" {
		t.Errorf("sources = %v, want every one from the type", src)
	}
	if !verify["checkable"] {
		t.Errorf("the verifiable list is not carried: checkable.Verify = false — the pane's Run check needs it")
	}
	// An instance-added group on a module: effective, listed by capability.
	cur, _ := svc.ObjectGet(ctx, api.ObjectGetInput{Module: "m", ID: "m"})
	o := cur.Object
	_ = o.SetCapability("composition", map[string]any{"members": []string{"m.g1"}})
	if _, err := svc.ObjectUpdate(ctx, api.ObjectUpdateInput{Module: "m", Object: o}); err != nil {
		t.Fatal(err)
	}
	lst, _ := svc.ObjectList(ctx, api.ObjectListInput{Capability: "composition"})
	if len(lst.Objects) != 1 || lst.Objects[0].ID != "m" {
		t.Errorf("ObjectList{Capability: composition} = %v, want [m]", ids(lst.Objects))
	}
	lst, _ = svc.ObjectList(ctx, api.ObjectListInput{Capability: "checkable"})
	if len(lst.Objects) != 1 || lst.Objects[0].ID != "m.g1" {
		t.Errorf("ObjectList{Capability: checkable} (type-declared, no group) = %v, want [m.g1]", ids(lst.Objects))
	}
}

// DRAWN BY THE TYPE: a note's type is not drawn, whatever the note carries —
// two objects of one type are never drawn differently. And `load: never-eager`
// never draws, even on a drawn type.
func TestTheTypeDrawsAndLoadOverrideWins(t *testing.T) {
	svc, _ := splitFixture(t)
	ctx := stdctx.Background()
	drawn := func() map[string]bool {
		tree, err := svc.ObjectTree(ctx, api.ObjectTreeInput{Root: "m", DrawnOnly: true})
		if err != nil {
			t.Fatal(err)
		}
		out := map[string]bool{}
		for _, ch := range tree.Roots[0].Children {
			out[ch.Object.ID] = true
		}
		return out
	}
	if d := drawn(); d["m.n1"] || !d["m.g1"] {
		t.Fatalf("before: drawn = %v, want the goal and not the note", d)
	}
	cur, _ := svc.ObjectGet(ctx, api.ObjectGetInput{Module: "m", ID: "m.n1"})
	o := cur.Object
	_ = o.SetCapability("tree-node", map[string]any{})
	if _, err := svc.ObjectUpdate(ctx, api.ObjectUpdateInput{Module: "m", Object: o}); err == nil {
		t.Errorf("an object took a `tree-node` group — drawing is no capability an object can add")
	}
	if d := drawn(); d["m.n1"] {
		t.Errorf("the note is drawn although its type is not: %v", d)
	}
	lg := api.Object{ID: "m.l1", Type: "log"}
	lg.Name = "l1"
	lg.Parent = "m"
	if _, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "m", Object: lg}); err != nil {
		t.Fatal(err)
	}
	if d := drawn(); d["m.l1"] {
		t.Errorf("a never-eager type is drawn although its load override says never: %v", d)
	}
	if p := svc.loadPolicy(lg); p != LoadNeverEager {
		t.Errorf("loadPolicy(log) = %q", p)
	}
}

// SEARCH FROM THE INDEX: names, titles, descriptions, tags, ids; never a body.
func TestObjectSearchReadsResidentTextOnly(t *testing.T) {
	svc, _ := splitFixture(t)
	ctx := stdctx.Background()
	cases := []struct {
		in   api.ObjectSearchInput
		want []string
	}{
		{api.ObjectSearchInput{Query: "invoices"}, []string{"m.g1"}},  // description
		{api.ObjectSearchInput{Query: "long note"}, []string{"m.n1"}}, // title
		{api.ObjectSearchInput{Query: "draft"}, []string{"m.n1"}},     // tag
		{api.ObjectSearchInput{Query: "billing"}, []string{"m"}},      // container description
		{api.ObjectSearchInput{Query: "prose"}, nil},                  // BODY: never
		{api.ObjectSearchInput{Types: []string{"goal"}}, []string{"m.g1"}},
		{api.ObjectSearchInput{Capabilities: []string{"checkable"}}, []string{"m.g1"}},
		{api.ObjectSearchInput{Modules: []string{"m"}, Limit: 2}, []string{"m", "m.g1"}},
	}
	for _, c := range cases {
		out, err := svc.ObjectSearch(ctx, c.in)
		if err != nil {
			t.Fatalf("%+v: %v", c.in, err)
		}
		if strings.Join(ids(out.Objects), ",") != strings.Join(c.want, ",") {
			t.Errorf("%+v → %v, want %v", c.in, ids(out.Objects), c.want)
		}
		if c.in.Limit > 0 && out.Total != 3 {
			t.Errorf("Total = %d before the limit, want 3", out.Total)
		}
	}
}

func ids(objs []api.Object) []string {
	var out []string
	for _, o := range objs {
		out = append(out, o.ID)
	}
	return out
}

var _ = filepath.Join
