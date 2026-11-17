// SPDX-License-Identifier: Apache-2.0

package render

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	drawioapi "github.com/katyoti/katyoti-fwk/internal/extensions/drawio/api"
)

// checkFixture runs the layout check on render/testdata/check/<name>.
func checkFixture(t *testing.T, name string) []drawioapi.Finding {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "check", name))
	if err != nil {
		t.Fatal(err)
	}
	findings, err := CheckLayout(data)
	if err != nil {
		t.Fatal(err)
	}
	return findings
}

// pairs lists the findings of a kind as "a/b" cell-id pairs, sorted.
func pairs(findings []drawioapi.Finding, kind string) []string {
	var out []string
	for _, f := range findings {
		if f.Kind == kind {
			out = append(out, strings.Join(f.CellIDs, "/"))
		}
	}
	sort.Strings(out)
	return out
}

func gated(findings []drawioapi.Finding) int {
	n := 0
	for _, f := range findings {
		if f.Gated {
			n++
		}
	}
	return n
}

// TestCheckLayout_OverlapFails: two wrapped titles painted over each other
// are a gated text-overlap.
func TestCheckLayout_OverlapFails(t *testing.T) {
	f := checkFixture(t, "overlap.drawio")
	if got := pairs(f, drawioapi.FindingTextOverlap); len(got) != 1 || got[0] != "a/b" {
		t.Fatalf("want the a/b text-overlap, got %v (%+v)", got, f)
	}
	if gated(f) != 1 {
		t.Fatalf("want 1 gated finding, got %+v", f)
	}
}

// TestCheckLayout_LayeringIsNotAFinding: deliberate layering — text on a
// card, a chip on it, a ring under its arc with a KPI inside, a bar's fill
// on its track, a hidden label — gives no finding; a label cell on an edge
// is listed as not measured.
func TestCheckLayout_LayeringIsNotAFinding(t *testing.T) {
	f := checkFixture(t, "layering.drawio")
	if len(f) != 1 || f[0].Kind != drawioapi.FindingNotMeasured || f[0].CellIDs[0] != "link-label" {
		t.Fatalf("want only link-label not measured, got %+v", f)
	}
}

// TestCheckLayout_Tolerance: labels meeting by 3.9 px are not counted, by
// 4.1 px they are, across and down.
func TestCheckLayout_Tolerance(t *testing.T) {
	f := checkFixture(t, "tolerance.drawio")
	got := pairs(f, drawioapi.FindingTextOverlap)
	if want := []string{"x41a/x41b", "y41a/y41b"}; strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("want %v, got %v (%+v)", want, got, f)
	}
	if len(f) != 2 {
		t.Fatalf("want no other finding, got %+v", f)
	}
}

// TestCheckLayout_Portfolio runs the check on frozen copies of the
// portfolio's drawings: the snapshot is clean; the roadmap fails on its
// stacked status labels (two texts in one box) and its titles running
// into each other; the dependencies fail on their cascaded cards.
func TestCheckLayout_Portfolio(t *testing.T) {
	if f := checkFixture(t, "portfolio-snapshot.drawio"); gated(f) != 0 {
		t.Errorf("snapshot: want no gated finding, got %+v", f)
	}

	road := checkFixture(t, "portfolio-roadmap.drawio")
	overlaps := strings.Join(pairs(road, drawioapi.FindingTextOverlap), ",")
	for _, want := range []string{"c27/c33", "c28/c34", "c46/c52", "c30/c43"} {
		if !strings.Contains(overlaps, want) {
			t.Errorf("roadmap: want the %s text-overlap, got %s", want, overlaps)
		}
	}
	if d := pairs(road, drawioapi.FindingDuplicate); len(d) != 1 || d[0] != "c47/c53" {
		t.Errorf("roadmap: want the c47/c53 duplicate (\"Planifié\" twice), got %v", d)
	}
	if o := pairs(road, drawioapi.FindingTextOverflow); !contains(o, "c96") {
		t.Errorf("roadmap: want c96's three lines out of its 40 px box, got %v", o)
	}
	if c := pairs(road, drawioapi.FindingBoxCover); !contains(c, "c27/c32") {
		t.Errorf("roadmap: want \"83\" (c27) hidden under the white disc c32, got %v", c)
	}

	dep := checkFixture(t, "portfolio-dependencies.drawio")
	if gated(dep) == 0 || !contains(pairs(dep, drawioapi.FindingTextOverlap), "c10/c16") {
		t.Errorf("dependencies: want the cascaded cards' labels (c10/c16) gated, got %+v", dep)
	}
	if !contains(pairs(dep, drawioapi.FindingBoxCover), "c10/c16") {
		t.Errorf("dependencies: want the cascaded cards covering each other")
	}
}

// TestCheckLayout_UnreadableIsAnError: a file that is not a drawing is an
// error, never a pass.
func TestCheckLayout_UnreadableIsAnError(t *testing.T) {
	if _, err := CheckLayout([]byte("not a drawing")); err == nil {
		t.Fatal("want an error")
	}
}

// TestScore_CountsLayoutFindings: the quality report carries the counts.
func TestScore_CountsLayoutFindings(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "check", "portfolio-roadmap.drawio"))
	if err != nil {
		t.Fatal(err)
	}
	q, err := Score(data)
	if err != nil {
		t.Fatal(err)
	}
	if q.TextOverlaps < 4 || q.Duplicates != 1 || q.TextOverflows == 0 || q.BoxCovers == 0 {
		t.Fatalf("want the roadmap's layout counts, got %+v", q)
	}
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
