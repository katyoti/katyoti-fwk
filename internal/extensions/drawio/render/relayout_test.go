// SPDX-License-Identifier: Apache-2.0

package render

import (
	"encoding/json"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	drawioapi "github.com/katyoti/katyoti-fwk/internal/extensions/drawio/api"
	drawiotesting "github.com/katyoti/katyoti-fwk/internal/extensions/drawio/testing"
	"github.com/katyoti/katyoti-fwk/internal/test/long"
)

var geometryRe = regexp.MustCompile(`(?s)<mxGeometry\b[^>]*?(?:/>|>.*?</mxGeometry>)`)

// stripGeometry is a document without its geometry: what a relayout must
// leave byte for byte.
func stripGeometry(doc string) string { return geometryRe.ReplaceAllString(doc, "<g/>") }

// needBun skips a test that runs the ELK layout (elkjs through bun) when bun
// is not on PATH: the published repository builds and tests with Go alone.
// Every test reaching Emit or Relayout with an ELK layout calls it.
func needBun(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("bun"); err != nil {
		t.Skip("bun not on PATH: the ELK layout runs elkjs through bun")
	}
}

// corpusDrawings renders every corpus model with layout (on the test catalog:
// the shipped notations and the testdata app-flux one) and adds every
// hand-drawn corpus file: the drawings relayout is tried on.
func corpusDrawings(t *testing.T) map[string]string {
	t.Helper()
	ctx := drawiotesting.WithCatalog(t, drawiotesting.TestCatalog(t))
	entries, err := os.ReadDir(corpusDir)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, e := range entries {
		data, err := os.ReadFile(filepath.Join(corpusDir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		name := strings.TrimSuffix(e.Name(), filepath.Ext(e.Name()))
		switch {
		case name == "ceilings":
		case strings.HasSuffix(e.Name(), ".json"):
			var ld drawioapi.LogicalDiagram
			if err := json.Unmarshal(data, &ld); err != nil {
				t.Fatal(err)
			}
			r, err := Emit(ctx, &ld, false)
			if err != nil {
				t.Fatal(err)
			}
			out[name] = r.XML
		case strings.HasSuffix(e.Name(), ".drawio"):
			out[name] = string(data)
		}
	}
	return out
}

// TestRelayout_KeepsEverythingButGeometry: a laid-out drawing reads back
// into the same model, differs only in geometry, and overlaps nothing.
func TestRelayout_KeepsEverythingButGeometry(t *testing.T) {
	long.Test(t)
	needBun(t)
	ctx := drawiotesting.WithCatalog(t, drawiotesting.TestCatalog(t))
	for name, doc := range corpusDrawings(t) {
		t.Run(name, func(t *testing.T) {
			res, err := Relayout(ctx, []byte(doc), nil)
			if err != nil {
				t.Fatal(err)
			}
			if stripGeometry(res.XML) != stripGeometry(doc) {
				t.Error("a relayout changed more than geometry")
			}
			before, _ := Read([]byte(doc))
			after, err := Read([]byte(res.XML))
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(before.Diagram, after.Diagram) {
				t.Error("the model changed")
			}
			if q := res.Quality; q == nil || q.Overlaps != 0 || q.ZoneEscapes != 0 || q.ZoneOverlaps != 0 {
				t.Errorf("quality after relayout: %+v", q)
			}
			// The layout keeps room for the edges' labels: a relayout
			// that forgot them left one label over another on most edges.
			if q := res.Quality; q != nil && q.LabelCollisions > 3+q.Edges/25 {
				t.Errorf("%d label collisions after relayout", q.LabelCollisions)
			}
			t.Logf("%s: moved %d, %+v", name, res.Moved, *res.Quality)
		})
	}
}

// TestRelayout_Stable: relayout reaches a fixpoint — a drawing it laid
// out, laid out again, does not move. (The first relayout of a drawing
// may still move things: the drawing's positions are its starting point.)
func TestRelayout_Stable(t *testing.T) {
	long.Test(t)
	needBun(t)
	ctx := drawiotesting.WithCatalog(t, drawiotesting.TestCatalog(t))
	for name, doc := range corpusDrawings(t) {
		t.Run(name, func(t *testing.T) {
			var runs []*drawioapi.ReadResult
			cur := doc
			for i := 0; i < 3; i++ {
				res, err := Relayout(ctx, []byte(cur), nil)
				if err != nil {
					t.Fatal(err)
				}
				cur = res.XML
				r, err := Read([]byte(cur))
				if err != nil {
					t.Fatal(err)
				}
				runs = append(runs, r)
			}
			t.Logf("%s: %.0f%% moved on the second run, %.0f%% on the third", name, share(runs[0], runs[1])*100, share(runs[1], runs[2])*100)
			if m := share(runs[1], runs[2]); m > 0 {
				t.Errorf("no fixpoint: the third relayout moved %.0f%% of the boxes", m*100)
			}
		})
	}
}

// share is the fraction of boxes that moved more than a pixel.
func share(a, b *drawioapi.ReadResult) float64 {
	moved, n := 0, 0
	for id, ba := range a.Geometry {
		bb, ok := b.Geometry[id]
		if !ok {
			continue
		}
		n++
		if math.Abs(ba.X-bb.X) > 1 || math.Abs(ba.Y-bb.Y) > 1 {
			moved++
		}
	}
	if n == 0 {
		return 0
	}
	return float64(moved) / float64(n)
}

// TestRelayout_EditedDrawing: a generated drawing a person edited in
// draw.io — a box added by hand and wired in, a node locked where they
// put it — is laid out around the locked node, and the added box is laid
// out with the rest.
func TestRelayout_EditedDrawing(t *testing.T) {
	needBun(t)
	ctx := drawiotesting.WithCatalog(t, drawiotesting.ShippedCatalog(t))
	raw, err := os.ReadFile(filepath.Join(corpusDir, "archimate-order-intake.json"))
	if err != nil {
		t.Fatal(err)
	}
	var ld drawioapi.LogicalDiagram
	if err := json.Unmarshal(raw, &ld); err != nil {
		t.Fatal(err)
	}
	out, err := Emit(ctx, &ld, false)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := Read([]byte(out.XML))
	locked := ld.Nodes[0].ID
	// Lock the first node where it is; add a box by hand, far off, wired
	// to the second node.
	doc := regexp.MustCompile(`(ktw-id="`+locked+`"[^>]*>\s*<mxCell [^>]*style=")`).ReplaceAllString(out.XML, "${1}locked=1;")
	doc = strings.Replace(doc, `<mxCell id="1" parent="0"/>`, `<mxCell id="1" parent="0"/>`+
		`<mxCell id="hand-1" value="Added" style="rounded=1;" vertex="1" parent="1"><mxGeometry x="900" y="900" width="120" height="60" as="geometry"/></mxCell>`+
		`<mxCell id="hand-e" edge="1" parent="1" source="hand-1" target="n-`+ld.Nodes[1].ID+`"><mxGeometry relative="1" as="geometry"/></mxCell>`, 1)
	if doc == out.XML || !strings.Contains(doc, "hand-1") {
		t.Fatal("could not edit the drawing")
	}
	pinnedAt := before.Geometry[locked]

	res, err := Relayout(ctx, []byte(doc), nil)
	if err != nil {
		t.Fatal(err)
	}
	after, err := Read([]byte(res.XML))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(res.Pinned, []string{"n-" + locked}) {
		t.Errorf("pinned: got %v", res.Pinned)
	}
	if got := after.Geometry[locked]; got != pinnedAt {
		t.Errorf("the locked node moved: %+v → %+v", pinnedAt, got)
	}
	if got := after.Geometry["hand-1"]; got.X == 900 && got.Y == 900 {
		t.Error("the box added by hand was not laid out")
	}
	if q := res.Quality; q == nil || q.Overlaps != 0 {
		t.Errorf("quality: %+v", q)
	}
}
