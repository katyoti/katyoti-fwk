// SPDX-License-Identifier: Apache-2.0

package render

import (
	"os"
	"path/filepath"
	"testing"

	drawioapi "github.com/katyoti/katyoti-fwk/internal/extensions/drawio/api"
)

// onePage is a page with a single labelled box: nothing to find.
const onePage = `<mxfile><diagram name="x"><mxGraphModel><root><mxCell id="0"/><mxCell id="1" parent="0"/>` +
	`<mxCell id="n" value="Alone" style="rounded=0;whiteSpace=wrap;" vertex="1" parent="1"><mxGeometry x="40" y="40" width="160" height="60" as="geometry"/></mxCell>` +
	`</root></mxGraphModel></diagram></mxfile>`

// A drawing of several pages: assembled from named pages, each with its
// name slugged as its id; the layout check runs on every page and names the
// page of each finding — a text overlap on the second page is found there.
func TestPagesAreAssembledAndEachIsChecked(t *testing.T) {
	overlap, err := os.ReadFile(filepath.Join("testdata", "check", "overlap.drawio"))
	if err != nil {
		t.Fatal(err)
	}
	file, err := Assemble(NamedPage{Name: "Snapshot", XML: []byte(onePage)}, NamedPage{Name: "Road map", XML: overlap})
	if err != nil {
		t.Fatal(err)
	}
	pages, err := Pages(file)
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) != 2 || pages[0].ID != "snapshot" || pages[1].ID != "road-map" || pages[1].Name != "Road map" {
		t.Fatalf("pages: %+v; want snapshot then road-map", pages)
	}
	findings, err := CheckLayout(file)
	if err != nil {
		t.Fatal(err)
	}
	got := pairs(findings, drawioapi.FindingTextOverlap)
	if len(got) != 1 || got[0] != "a/b" {
		t.Fatalf("the text overlap of page 2 not found: %+v", findings)
	}
	for _, f := range findings {
		if f.Kind == drawioapi.FindingTextOverlap && f.Page != "Road map" {
			t.Errorf("the overlap names page %q; want Road map", f.Page)
		}
	}
	scores, err := ScorePages(file)
	if err != nil || len(scores) != 2 || scores[1].Quality.TextOverlaps != 1 || scores[0].Quality.TextOverlaps != 0 {
		t.Fatalf("scores per page: %+v, %v; want page 2's overlap only", scores, err)
	}
	// One page answers as before: no page named.
	if one, _ := CheckLayout(overlap); len(one) == 0 || one[0].Page != "" {
		t.Errorf("a one-page file's findings: %+v; want them unnamed", one)
	}
	if _, err := Assemble(NamedPage{Name: "Map", XML: []byte(onePage)}, NamedPage{Name: "map", XML: []byte(onePage)}); err == nil {
		t.Error("two pages with the same id were assembled")
	}
}
