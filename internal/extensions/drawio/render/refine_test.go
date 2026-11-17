// SPDX-License-Identifier: Apache-2.0

package render

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	drawioapi "github.com/katyoti/katyoti-fwk/internal/extensions/drawio/api"
	drawiotesting "github.com/katyoti/katyoti-fwk/internal/extensions/drawio/testing"
)

func corpusModel(t *testing.T, name string) *drawioapi.LogicalDiagram {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(corpusDir, name+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var ld drawioapi.LogicalDiagram
	if err := json.Unmarshal(raw, &ld); err != nil {
		t.Fatal(err)
	}
	return &ld
}

// TestRefine_UnchangedModelDoesNotMove: the same model drawn over its own
// drawing comes back where it was.
func TestRefine_UnchangedModelDoesNotMove(t *testing.T) {
	needBun(t)
	ctx := drawiotesting.WithCatalog(t, drawiotesting.TestCatalog(t))
	for _, name := range []string{"archimate-order-intake", "landscape", "uml-order-model"} {
		t.Run(name, func(t *testing.T) {
			ld := corpusModel(t, name)
			v1, err := Emit(ctx, ld, false)
			if err != nil {
				t.Fatal(err)
			}
			base := v1.XML
			v2, err := Refine(ctx, ld, []byte(base))
			if err != nil {
				t.Fatal(err)
			}
			a, _ := Read([]byte(base))
			b, _ := Read([]byte(v2.XML))
			if m := share(a, b); m > 0 {
				t.Errorf("an unchanged model moved %.0f%% of its boxes", m*100)
			}
		})
	}
}

// TestRefine_AddedNodeAndPin: a node added to the model is placed beside
// its neighbour without overlapping anything; no node the person has
// already seen moves, the locked one included.
func TestRefine_AddedNodeAndPin(t *testing.T) {
	needBun(t)
	ctx := drawiotesting.WithCatalog(t, drawiotesting.TestCatalog(t))
	for _, name := range []string{"archimate-order-intake", "landscape"} {
		t.Run(name, func(t *testing.T) {
			ld := corpusModel(t, name)
			v1, err := Emit(ctx, ld, false)
			if err != nil {
				t.Fatal(err)
			}
			base := v1.XML
			locked := ld.Nodes[len(ld.Nodes)-1].ID
			edited := regexp.MustCompile(`(ktw-id="`+locked+`"[^>]*>\s*<mxCell [^>]*style=")`).ReplaceAllString(base, "${1}locked=1;")
			before, _ := Read([]byte(edited))

			next := *ld
			added := ld.Nodes[0]
			added.ID, added.Label = "added", "Added"
			added.Attributes = nil
			next.Nodes = append(append([]drawioapi.LogicalNode(nil), ld.Nodes...), added)
			e := ld.Edges[0]
			e.ID, e.Source, e.Target = "e-added", ld.Nodes[0].ID, "added"
			next.Edges = append(append([]drawioapi.LogicalEdge(nil), ld.Edges...), e)

			v2, err := Refine(ctx, &next, []byte(edited))
			if err != nil {
				t.Fatal(err)
			}
			if len(v2.Errors) > 0 {
				t.Fatalf("errors: %v", v2.Errors)
			}
			after, err := Read([]byte(v2.XML))
			if err != nil {
				t.Fatal(err)
			}
			if after.Geometry[locked] != before.Geometry[locked] {
				t.Errorf("the locked node moved: %+v → %+v", before.Geometry[locked], after.Geometry[locked])
			}
			if _, ok := after.Geometry["added"]; !ok {
				t.Fatal("the added node is not drawn")
			}
			if q := v2.Quality; q == nil || q.Overlaps != 0 || q.ZoneOverlaps != 0 {
				t.Errorf("quality: %+v", q)
			}
			for _, n := range ld.Nodes {
				if after.Geometry[n.ID] != before.Geometry[n.ID] {
					t.Errorf("node %s moved: %+v → %+v", n.ID, before.Geometry[n.ID], after.Geometry[n.ID])
				}
			}
		})
	}
}

// TestRefine_ZoneGrowthKeepsOutsiders: a new member widens its zone and a
// new outside node linked into the zone does not end up inside it; the
// draft's quality warning does not survive into the result.
func TestRefine_ZoneGrowthKeepsOutsiders(t *testing.T) {
	needBun(t)
	ctx := drawiotesting.WithCatalog(t, drawiotesting.TestCatalog(t))
	ld := corpusModel(t, "c4-internet-banking")
	v1, err := Emit(ctx, ld, false)
	if err != nil {
		t.Fatal(err)
	}
	next := *ld
	next.Nodes = append(append([]drawioapi.LogicalNode(nil), ld.Nodes...),
		drawioapi.LogicalNode{ID: "mobile", Label: "Mobile App", Kind: "Container", Zone: "ibs"},
		drawioapi.LogicalNode{ID: "fraud", Label: "Fraud Check", Kind: "ExternalSoftwareSystem"})
	next.Edges = append(append([]drawioapi.LogicalEdge(nil), ld.Edges...),
		drawioapi.LogicalEdge{ID: "m1", Source: "mobile", Target: "api", Kind: "uses"},
		drawioapi.LogicalEdge{ID: "m2", Source: "api", Target: "fraud", Kind: "uses"})
	v2, err := Refine(ctx, &next, []byte(v1.XML))
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range v2.Warnings {
		if strings.HasPrefix(w, "quality: ") {
			t.Errorf("a draft's quality warning survived: %s", w)
		}
	}
	if q := v2.Quality; q == nil || q.Overlaps != 0 || q.ZoneIntrusions != 0 {
		t.Errorf("quality: %+v", q)
	}
	before, _ := Read([]byte(v1.XML))
	after, _ := Read([]byte(v2.XML))
	for _, n := range ld.Nodes {
		if after.Geometry[n.ID] != before.Geometry[n.ID] {
			t.Errorf("node %s moved", n.ID)
		}
	}
}

// TestRefine_LanesKeepTheirBands: a task added to a lane joins the lane's
// row — beside the task it follows, sized as drawn — and no pool or lane
// climbs over its neighbour. The new tasks first landed below the drawing,
// the pool re-padded itself over the lane above, and the lanes overlapped.
func TestRefine_LanesKeepTheirBands(t *testing.T) {
	needBun(t)
	ctx := drawiotesting.WithCatalog(t, drawiotesting.TestCatalog(t))
	ld := corpusModel(t, "bpmn-order-fulfilment")
	v1, err := Emit(ctx, ld, false)
	if err != nil {
		t.Fatal(err)
	}
	// A task after the customer's order, and one after the sales check.
	for i := range ld.Edges {
		if ld.Edges[i].Source == "c-order" && ld.Edges[i].Kind == "sequenceFlow" {
			old := ld.Edges[i].Target
			ld.Edges[i].Target = "c-voucher"
			ld.Edges = append(ld.Edges, drawioapi.LogicalEdge{ID: "e-voucher", Kind: "sequenceFlow", Source: "c-voucher", Target: old})
			break
		}
	}
	ld.Nodes = append(ld.Nodes,
		drawioapi.LogicalNode{ID: "c-voucher", Label: "Apply Voucher", Kind: "Task", Zone: "customer"},
		drawioapi.LogicalNode{ID: "s-note", Label: "Print Delivery Note", Kind: "Task", Zone: "sales"})
	ld.Edges = append(ld.Edges, drawioapi.LogicalEdge{ID: "e-note", Kind: "sequenceFlow", Source: "s-check", Target: "s-note"})
	v2, err := Refine(ctx, ld, []byte(v1.XML))
	if err != nil {
		t.Fatal(err)
	}
	q := v2.Quality
	if q == nil || q.Overlaps > 0 || q.ZoneOverlaps > 0 || q.ZoneIntrusions > 0 || q.ZoneEscapes > 0 {
		t.Fatalf("quality after refine: %+v", q)
	}
	a, _ := Read([]byte(v1.XML))
	b, _ := Read([]byte(v2.XML))
	for _, z := range []string{"customer", "shop", "sales", "warehouse"} {
		if a.Geometry[z].Y != b.Geometry[z].Y {
			t.Errorf("zone %s moved from y=%v to y=%v", z, a.Geometry[z].Y, b.Geometry[z].Y)
		}
	}
	order, voucher := b.Geometry["c-order"], b.Geometry["c-voucher"]
	if voucher.Y+voucher.Height/2 != order.Y+order.Height/2 || voucher.X <= order.X {
		t.Errorf("the new task %+v is not in the row after %+v", voucher, order)
	}
}
