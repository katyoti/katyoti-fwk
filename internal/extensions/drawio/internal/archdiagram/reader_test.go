// SPDX-License-Identifier: Apache-2.0

package archdiagram

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	drawioapi "github.com/katyoti/katyoti-fwk/internal/extensions/drawio/api"
	drawiotesting "github.com/katyoti/katyoti-fwk/internal/extensions/drawio/testing"
)

// TestRead_RoundTrip asserts model → emit → read gives back the model —
// ids, kinds, labels, attributes, zones and positions — for a sample of
// every shipped viewpoint.
func TestRead_RoundTrip(t *testing.T) {
	for vp, d := range sampleDiagrams() {
		d := d
		t.Run(vp, func(t *testing.T) {
			out := renderShipped(t, vp, d)
			res, err := Read(out)
			if err != nil {
				t.Fatalf("read: %v", err)
			}
			if res.Foreign {
				t.Fatal("an emitted diagram must not read as foreign")
			}
			if res.Diagram.Viewpoint != vp || res.Diagram.Title != d.Title {
				t.Errorf("diagram identity: got (%q, %q), want (%q, %q)", res.Diagram.Viewpoint, res.Diagram.Title, vp, d.Title)
			}
			if len(res.Diagram.Nodes) != len(d.Nodes) {
				t.Fatalf("nodes: got %d, want %d (report %v)", len(res.Diagram.Nodes), len(d.Nodes), res.Report)
			}
			got := map[string]drawioapi.LogicalNode{}
			for _, n := range res.Diagram.Nodes {
				got[n.ID] = n
			}
			for _, n := range d.Nodes {
				g, ok := got[n.ID]
				if !ok {
					t.Errorf("node %q lost", n.ID)
					continue
				}
				if g.Kind != n.Type || g.Label != n.Label || g.Zone != n.Zone {
					t.Errorf("node %q: got (%q, %q, %q), want (%q, %q, %q)", n.ID, g.Kind, g.Label, g.Zone, n.Type, n.Label, n.Zone)
				}
				if !sameJSON(g.Attributes, n.Data) {
					t.Errorf("node %q attributes: got %v, want %v", n.ID, g.Attributes, n.Data)
				}
				box := res.Geometry[n.ID]
				if box.X != n.X || box.Y != n.Y || box.Width != n.Width {
					t.Errorf("node %q box: got %+v, want (%v, %v, %v)", n.ID, box, n.X, n.Y, n.Width)
				}
			}
			var wantEdges, gotEdges []string
			for _, e := range d.Edges {
				wantEdges = append(wantEdges, e.ID+":"+e.Source+">"+e.Target+":"+e.EdgeStyle+":"+e.Label)
			}
			for _, e := range res.Diagram.Edges {
				gotEdges = append(gotEdges, e.ID+":"+e.Source+">"+e.Target+":"+e.Kind+":"+e.Label)
			}
			sort.Strings(wantEdges)
			sort.Strings(gotEdges)
			if !reflect.DeepEqual(wantEdges, gotEdges) {
				t.Errorf("edges: got %v, want %v", gotEdges, wantEdges)
			}
		})
	}
}

// TestRead_HandEdits asserts what a person does in draw.io comes back: an
// attribute changed in Edit Data, a box added by hand, a cell locked.
func TestRead_HandEdits(t *testing.T) {
	d := &ArchDiagram{
		Nodes: []*Node{
			{ID: "a", Label: "A", Type: "Application", Data: map[string]any{"domain": "reference", "codes": []any{"x"}}, X: 10, Y: 20},
			{ID: "b", Label: "B", Type: "Application", X: 300, Y: 20},
		},
		Edges: []*Edge{{ID: "e1", Source: "a", Target: "b", EdgeStyle: "Flow"}},
	}
	xml := string(renderShipped(t, "drawio-test:/viewpoint/app-flux", d))
	xml = strings.Replace(xml, `domain="reference"`, `domain="invoicing"`, 1)
	xml = strings.Replace(xml, `id="n-b">`, `id="n-b">`, 1)
	xml = strings.Replace(xml, `        <mxCell id="1" parent="0"/>`+"\n",
		`        <mxCell id="1" parent="0"/>`+"\n"+`        <mxCell id="hand-1" value="&lt;b&gt;New&lt;/b&gt; box" style="rounded=1;locked=1;" vertex="1" parent="1"><mxGeometry x="5" y="400" width="80" height="40" as="geometry"/></mxCell>`+"\n", 1)
	res, err := Read([]byte(xml))
	if err != nil {
		t.Fatal(err)
	}
	var a, hand *drawioapi.LogicalNode
	for i := range res.Diagram.Nodes {
		switch res.Diagram.Nodes[i].ID {
		case "a":
			a = &res.Diagram.Nodes[i]
		case "hand-1":
			hand = &res.Diagram.Nodes[i]
		}
	}
	if a == nil || a.Attributes["domain"] != "invoicing" {
		t.Errorf("an Edit Data change must win over the recorded attributes, got %+v", a)
	}
	if a != nil && !reflect.DeepEqual(a.Attributes["codes"], []any{"x"}) {
		t.Errorf("non-scalar attributes survive from the JSON copy, got %v", a.Attributes["codes"])
	}
	if hand == nil || hand.Kind != drawioapi.KindUnknown || hand.Label != "New box" {
		t.Fatalf("hand-added box: got %+v", hand)
	}
	if !reflect.DeepEqual(res.Pinned, []string{"hand-1"}) {
		t.Errorf("pinned: got %v", res.Pinned)
	}
	if len(res.Report) == 0 {
		t.Error("the hand-added cell must be reported")
	}
}

// TestRead_Foreign reads a diagram drawn by hand in draw.io.
func TestRead_Foreign(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "foreign", "infra-sidecars.drawio"))
	if err != nil {
		t.Fatal(err)
	}
	res, err := Read(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Foreign {
		t.Error("a hand-drawn diagram must read as foreign")
	}
	if len(res.Diagram.Nodes) == 0 {
		t.Fatal("no nodes read")
	}
	for _, n := range res.Diagram.Nodes {
		if n.Kind != drawioapi.KindUnknown {
			t.Errorf("node %q: kind %q, want %q", n.ID, n.Kind, drawioapi.KindUnknown)
		}
		if _, ok := res.Geometry[n.ID]; !ok {
			t.Errorf("node %q has no box", n.ID)
		}
	}
	t.Logf("read %d nodes, %d edges, %d zones; report: %v", len(res.Diagram.Nodes), len(res.Diagram.Edges), len(res.Diagram.Zones), res.Report)
}

// TestRead_NotADiagram rejects a file that is not a draw.io diagram.
func TestRead_NotADiagram(t *testing.T) {
	if _, err := Read([]byte(`<svg/>`)); err == nil {
		t.Error("want an error")
	}
}

func sameJSON(a, b map[string]any) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	ja, _ := json.Marshal(a)
	jb, _ := json.Marshal(b)
	return string(ja) == string(jb)
}

// TestRead_Compressed reads a deflate-compressed <diagram>, the form
// draw.io writes by default.
func TestRead_Compressed(t *testing.T) {
	res, err := Read(drawiotesting.BuildMxfileXML(true))
	if err != nil {
		t.Fatalf("read compressed: %v", err)
	}
	if !res.Foreign || len(res.Diagram.Nodes) != 0 {
		t.Errorf("want an empty foreign diagram, got %+v", res.Diagram)
	}
}
