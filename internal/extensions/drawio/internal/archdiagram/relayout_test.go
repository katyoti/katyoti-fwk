// SPDX-License-Identifier: Apache-2.0

package archdiagram

import (
	"strings"
	"testing"

	drawioapi "github.com/katyoti/katyoti-fwk/internal/extensions/drawio/api"
)

const relayoutDoc = `<mxGraphModel><root>
  <mxCell id="0"/>
  <mxCell id="1" parent="0"/>
  <UserObject id="g" label="G" ktw-role="zone"><mxCell style="swimlane;" vertex="1" parent="1"><mxGeometry x="10" y="10" width="300" height="200" as="geometry"><mxRectangle width="80" height="20" as="alternateBounds"/></mxGeometry></mxCell></UserObject>
  <mxCell id="a" value="A" style="rounded=1;" vertex="1" parent="g"><mxGeometry y="40" width="80" height="40" as="geometry"/></mxCell>
  <mxCell id="b" value="B" vertex="1" parent="1"><mxGeometry x="400" y="50" width="80" height="40" as="geometry"/></mxCell>
  <mxCell id="e" edge="1" parent="1" source="a" target="b"><mxGeometry relative="1" as="geometry"/></mxCell>
</root></mxGraphModel>`

func TestScanGeometry_FindsEachCellsGeometry(t *testing.T) {
	spans, err := scanGeometry([]byte(relayoutDoc))
	if err != nil {
		t.Fatal(err)
	}
	for id, want := range map[string]string{
		"g": `<mxGeometry x="10" y="10" width="300" height="200" as="geometry"><mxRectangle width="80" height="20" as="alternateBounds"/></mxGeometry>`,
		"a": `<mxGeometry y="40" width="80" height="40" as="geometry"/>`,
		"e": `<mxGeometry relative="1" as="geometry"/>`,
	} {
		s, ok := spans[id]
		if !ok {
			t.Fatalf("no geometry for %q", id)
		}
		if got := relayoutDoc[s.start:s.end]; got != want {
			t.Errorf("%s: got %q", id, got)
		}
	}
}

func TestSetBox_KeepsTheRestOfTheElement(t *testing.T) {
	got := string(setBox([]byte(`<mxGeometry y="40" width="80" height="40" as="geometry"/>`), 12.4, 7.6, 1, 1, false))
	if got != `<mxGeometry x="12" y="8" width="80" height="40" as="geometry"/>` {
		t.Errorf("leaf: %s", got)
	}
	got = string(setBox([]byte(`<mxGeometry x="10" y="10" width="300" height="200" as="geometry"><mxRectangle as="alternateBounds"/></mxGeometry>`), 0, 5, 320, 210, true))
	if got != `<mxGeometry x="0" y="5" width="320" height="210" as="geometry"><mxRectangle as="alternateBounds"/></mxGeometry>` {
		t.Errorf("group: %s", got)
	}
}

func TestSetPoints(t *testing.T) {
	pts := []drawioapi.Point{{X: 1, Y: 2}}
	got := string(setPoints([]byte(`<mxGeometry relative="1" as="geometry"/>`), pts, nil))
	if got != `<mxGeometry relative="1" as="geometry"><Array as="points"><mxPoint x="1" y="2" /></Array></mxGeometry>` {
		t.Errorf("insert: %s", got)
	}
	got = string(setPoints([]byte(`<mxGeometry relative="1" as="geometry"><Array as="points"><mxPoint x="9" y="9"/></Array><mxPoint as="sourcePoint"/></mxGeometry>`), nil, nil))
	if got != `<mxGeometry relative="1" as="geometry"><mxPoint as="sourcePoint"/></mxGeometry>` {
		t.Errorf("clear: %s", got)
	}
}

func TestPlan_GroupsAndDrawnContainment(t *testing.T) {
	doc := strings.Replace(relayoutDoc, `<mxCell id="e"`, `<mxCell id="inside" value="I" vertex="1" parent="1"><mxGeometry x="420" y="55" width="20" height="20" as="geometry"/></mxCell><mxCell id="e"`, 1)
	cells, err := parseCells([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	p := newPlan(cells)
	if p.parent["a"] != "g" {
		t.Errorf("a's layout parent: %q", p.parent["a"])
	}
	if p.parent["inside"] != "b" {
		t.Errorf("a box drawn inside b is b's content, got %q", p.parent["inside"])
	}
	if !p.isGroup["g"] {
		t.Error("g holds a, which an edge reaches: a group laid out inside")
	}
	if p.isGroup["b"] {
		t.Error("no edge reaches inside b: laid out as one box")
	}
	if got := p.oldAbs["a"]; got.X != 10 || got.Y != 50 {
		t.Errorf("a's page box: %+v", got)
	}
}
