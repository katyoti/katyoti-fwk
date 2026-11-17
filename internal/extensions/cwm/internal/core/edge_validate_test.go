// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// EDGE KINDS ARE DECLARED IN CAPABILITIES AND CHECKED ON EVERY WRITE (7e).

func edgeKindsFixture(t *testing.T, mode string) (*Service, string) {
	t.Helper()
	root := freshWorkspace(t)
	project := t.TempDir()
	cfg := &CWMServiceConfig{WorkspaceRoot: root, ProjectRoot: project, ObjectTypeValidation: mode}
	cfg.CatalogTypes.Objects = []api.ObjectType{
		{Name: "module", Capabilities: []string{"container", "module"}, Tree: api.TreeNode},
		{Name: "goal", Capabilities: []string{"container", "want"}, Tree: api.TreeNode},
		{Name: "note", Capabilities: []string{"content"}},
	}
	cfg.CatalogTypes.Capabilities = []api.Capability{{Name: "container"}, {Name: "content"}, {Name: "module"}, {Name: "want"}}
	cfg.CatalogTypes.Edges = []api.EdgeTypeRegistration{
		{Name: "serves", FromCapabilities: []string{"want"}, ToCapabilities: []string{"want"}, Cardinality: "M:N", InverseName: "served-by"},
		{Name: "realizes", FromCapabilities: []string{"container"}, Target: api.EdgeTargetPath, Cardinality: "M:N"},
		{Name: "owner", FromCapabilities: []string{"container"}, Cardinality: "N:1"},
		{Name: "parent", ObjectStorage: api.EdgeStorageField, Field: "left.container.parent"},
	}
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
	mk(m)
	for _, id := range []string{"m.g1", "m.g2"} {
		g := api.Object{ID: id, Type: "goal"}
		g.Name = id
		g.Parent = "m"
		g.Name = id
		_ = g.SetCapability("want", map[string]any{"reachability": "closable"})
		mk(g)
	}
	n := api.Object{ID: "m.n1", Type: "note"}
	n.Name = "n1"
	n.Parent = "m"
	mk(n)
	return svc, project
}

func withEdge(svc *Service, t *testing.T, id, kind, to string) error {
	t.Helper()
	cur, err := svc.ObjectGet(stdctx.Background(), api.ObjectGetInput{Module: "m", ID: id})
	if err != nil {
		t.Fatal(err)
	}
	o := cur.Object
	o.Edges = append(o.Edges, api.ObjectEdge{Kind: kind, To: to})
	_, err = svc.ObjectUpdate(stdctx.Background(), api.ObjectUpdateInput{Module: "m", Object: o})
	return err
}

func TestEdgeKindMustBeDeclared(t *testing.T) {
	svc, _ := edgeKindsFixture(t, api.TypeValidationError)
	if err := withEdge(svc, t, "m.g1", "nobody-declares-this", "m.g2"); !errors.Is(err, api.ErrInvalidInput) || !strings.Contains(err.Error(), "nobody-declares-this") {
		t.Errorf("undeclared kind: %v — must refuse naming the kind", err)
	}
	if err := withEdge(svc, t, "m.g1", "serves", "m.g2"); err != nil {
		t.Errorf("a declared, satisfied edge refused: %v", err)
	}
	warn, _ := edgeKindsFixture(t, api.TypeValidationWarn)
	if err := withEdge(warn, t, "m.g1", "nobody-declares-this", "m.g2"); err != nil {
		t.Errorf("warn mode must accept an undeclared kind: %v", err)
	}
}

func TestEdgeFromCapabilitiesAreCheckedOnTheEffectiveSet(t *testing.T) {
	svc, _ := edgeKindsFixture(t, api.TypeValidationError)
	// A note has no `want`: it may not serve.
	if err := withEdge(svc, t, "m.n1", "serves", "m.g1"); !errors.Is(err, api.ErrInvalidInput) || !strings.Contains(err.Error(), `"want"`) {
		t.Errorf("source without want: %v — must refuse naming the capability", err)
	}
	// The TARGET's capabilities are checked when it is in the index: a note
	// (no want) cannot be served.
	if err := withEdge(svc, t, "m.g1", "serves", "m.n1"); !errors.Is(err, api.ErrInvalidInput) || !strings.Contains(err.Error(), "the target m.n1 lacks") {
		t.Errorf("target without want: %v — must refuse naming the target", err)
	}
	// A link names an object of the project: a target that does not exist is
	// refused at write (a file landing before its target is reported on read).
	if err := withEdge(svc, t, "m.g1", "serves", "m.not-yet-written"); !errors.Is(err, api.ErrInvalidInput) || !strings.Contains(err.Error(), "m.not-yet-written does not exist") {
		t.Errorf("a dangling target: %v — must refuse naming it", err)
	}
	// Give the note `want` ON THE INSTANCE: now it may serve, and be served.
	cur, _ := svc.ObjectGet(stdctx.Background(), api.ObjectGetInput{Module: "m", ID: "m.n1"})
	o := cur.Object
	o.Name = "n1 wants"
	_ = o.SetCapability("want", map[string]any{"reachability": "closable"})
	o.Edges = []api.ObjectEdge{{Kind: "serves", To: "m.g1"}}
	if _, err := svc.ObjectUpdate(stdctx.Background(), api.ObjectUpdateInput{Module: "m", Object: o}); err != nil {
		t.Errorf("an instance-added want must satisfy from_capabilities: %v", err)
	}
	if err := withEdge(svc, t, "m.g2", "serves", "m.n1"); err != nil {
		t.Errorf("a target with an instance-added want must satisfy to_capabilities: %v", err)
	}
}

func TestEdgeNeverCrossesAProjectAndFieldKindsStayOut(t *testing.T) {
	for _, mode := range []string{api.TypeValidationWarn, api.TypeValidationError} {
		svc, _ := edgeKindsFixture(t, mode)
		full := api.FullName("00000000-0000-7000-8000-000000000001", "cwm.g1")
		if err := withEdge(svc, t, "m.g1", "serves", full); !errors.Is(err, api.ErrInvalidInput) || !strings.Contains(err.Error(), "crosses a project") {
			t.Errorf("mode %s: an edge to another project: %v — refused in EVERY mode", mode, err)
		}
		if err := withEdge(svc, t, "m.g1", "parent", "m"); !errors.Is(err, api.ErrInvalidInput) || !strings.Contains(err.Error(), "stored as the field") {
			t.Errorf("mode %s: a field-stored kind in the edges list: %v — refused in EVERY mode", mode, err)
		}
	}
}

func TestEdgeTargetPathAndCardinality(t *testing.T) {
	svc, project := edgeKindsFixture(t, api.TypeValidationError)
	if err := withEdge(svc, t, "m", "realizes", "internal/does/not/exist"); !errors.Is(err, api.ErrInvalidInput) {
		t.Errorf("realizes a missing path: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(project, "internal", "billing"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := withEdge(svc, t, "m", "realizes", "internal/billing"); err != nil {
		t.Errorf("realizes an existing path: %v", err)
	}
	if err := withEdge(svc, t, "m.g1", "owner", "m.g2"); err != nil {
		t.Fatalf("first owner: %v", err)
	}
	if err := withEdge(svc, t, "m.g1", "owner", "m"); !errors.Is(err, api.ErrInvalidInput) || !strings.Contains(err.Error(), "N:1") {
		t.Errorf("a second N:1 edge: %v — must refuse naming the cardinality", err)
	}
}

// A BARE `to: g1` IN A HAND-WRITTEN FILE reads as `m.g1`: qualified from the
// source's id on load, written qualified by the verbs, and listed with the
// declaration's inverse name.
func TestBareLocalTargetIsQualifiedAndInverseNamed(t *testing.T) {
	svc, project := edgeKindsFixture(t, api.TypeValidationWarn)
	ctx := stdctx.Background()
	path := filepath.Join(project, "modules", "m", ObjectsDirName, "g3"+ObjectFileSuffix)
	body := "id: m.g3\ntype: goal\nname: g3\nparent: m\ncapabilities:\n  want: {reachability: closable}\nedges:\n  - kind: serves\n    to: g1\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ObjectScan(ctx, api.ObjectScanInput{Force: true}); err != nil {
		t.Fatal(err)
	}
	in, err := svc.EdgeList(ctx, api.EdgeListInput{ObjectID: "m.g1", Direction: api.DirectionIn})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range in.Edges {
		if e.From == "m.g3" && e.To == "m.g1" && e.Kind == "serves" {
			found = true
			if e.Inverse != "served-by" {
				t.Errorf("inverse = %q, want served-by from the declaration", e.Inverse)
			}
		}
	}
	if !found {
		t.Errorf("the bare `to: g1` was not read as m.g1: %v", in.Edges)
	}
	// And the verb writes it qualified.
	if _, err := svc.EdgeCreate(ctx, api.EdgeCreateInput{From: "m.g2", To: "g1", Kind: "serves"}); err != nil {
		t.Fatal(err)
	}
	got, _ := ReadObjectFile(filepath.Join(project, "modules", "m", ObjectsDirName, "g2"+ObjectFileSuffix))
	if len(got.Edges) != 1 || got.Edges[0].To != "m.g1" {
		t.Errorf("EdgeCreate wrote %v, want to: m.g1", got.Edges)
	}
}
