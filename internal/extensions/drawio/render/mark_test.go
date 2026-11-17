// SPDX-License-Identifier: Apache-2.0

package render

import (
	"regexp"
	"strconv"
	"strings"
	"testing"

	docsapi "github.com/katyoti/katyoti-fwk/internal/extensions/docs/api"
	drawioapi "github.com/katyoti/katyoti-fwk/internal/extensions/drawio/api"
)

var markGeometry = regexp.MustCompile(`id="production-mark"[\s\S]*?<mxGeometry x="(-?[\d.]+)" y="(-?[\d.]+)"`)

// TestEmit_CarriesTheProductionMark: a drawn diagram carries the one
// production mark (docs) under everything drawn, which a read-back skips; a
// diagram whose mark is off carries none.
func TestEmit_CarriesTheProductionMark(t *testing.T) {
	needBun(t)
	ctx := ctxWithAppFlux(t)
	diagram := func(mark string) *drawioapi.LogicalDiagram {
		return &drawioapi.LogicalDiagram{
			Viewpoint: "drawio-test:/viewpoint/app-flux",
			Title:     "Order intake",
			Mark:      mark,
			Nodes: []drawioapi.LogicalNode{
				{ID: "billing", Label: "Billing", Kind: "Application"},
				{ID: "ledger", Label: "Ledger", Kind: "Application"},
			},
			Edges: []drawioapi.LogicalEdge{{ID: "f1", Source: "billing", Target: "ledger", Kind: "Flow"}},
		}
	}
	out, err := Emit(ctx, diagram(""), false)
	if err != nil {
		t.Fatal(err)
	}
	mustContain(t, out.XML, `ktw-role="mark"`, docsapi.DefaultMarkText+" · ")

	read, err := Read([]byte(out.XML))
	if err != nil {
		t.Fatal(err)
	}
	if len(read.Diagram.Nodes) != 2 {
		t.Errorf("the mark is read back as a node: %+v", read.Diagram.Nodes)
	}
	bottom := 0.0
	for _, b := range read.Geometry {
		bottom = max(bottom, b.Y+b.Height)
	}
	m := markGeometry.FindStringSubmatch(out.XML)
	if m == nil {
		t.Fatal("no geometry for the production mark")
	}
	if y, _ := strconv.ParseFloat(m[2], 64); y <= bottom {
		t.Errorf("the mark (y=%v) is not under the drawing (bottom %v)", y, bottom)
	}

	off, err := Emit(ctx, diagram("off"), false)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(off.XML, `ktw-role="mark"`) {
		t.Error("a diagram whose mark is off carries one")
	}
}
