// SPDX-License-Identifier: Apache-2.0

package render

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html"
	"image"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	drawioapi "github.com/katyoti/katyoti-fwk/internal/extensions/drawio/api"
)

// textMetricsFile holds what draw.io desktop painted for the calibration
// labels: each label, its style and cell, and the box its ink covers in the
// exported PNG. TestTextBoxesMatchExport checks the estimates against it on
// every run; KTW_REGEN_TEXTMETRICS=1 exports the calibration drawing again
// through draw.io desktop and rewrites it.
var textMetricsFile = filepath.Join("testdata", "textmetrics.json")

type textSample struct {
	Name  string        `json:"name"`
	Label string        `json:"label"`
	Style string        `json:"style"`
	Size  float64       `json:"size"` // the label's largest font size
	Cell  drawioapi.Box `json:"cell"`
	Ink   drawioapi.Box `json:"ink"`
}

// calibrationSamples are the labels measured: sizes 10 to 46, regular and
// bold, capitals, accents, digits, HTML runs, wrapping, alignments.
func calibrationSamples() []textSample {
	const text = "text;html=1;strokeColor=none;fillColor=none;fontFamily=Helvetica;"
	var out []textSample
	add := func(name, label, style string, size, w, h float64) {
		out = append(out, textSample{Name: name, Label: label, Style: style, Size: size, Cell: drawioapi.Box{Width: w, Height: h}})
	}
	words := []string{"Planifié en bonne voie", "AOÛT SEPT. DÉC. JANV.", "0123456789 % 41", "Wq,|Éjgy (ÀÇ) – «»"}
	for _, size := range []float64{10, 11, 12, 13, 14, 16, 19, 22, 26, 34, 46} {
		for _, bold := range []bool{false, true} {
			for i, w := range words {
				style := fmt.Sprintf("%salign=center;verticalAlign=middle;fontSize=%g;", text, size)
				name := fmt.Sprintf("s%g-w%d", size, i)
				label := html.EscapeString(w)
				if bold {
					style += "fontStyle=1;"
					name += "-b"
				}
				add(name, label, style, size, size*16, math.Round(size*2.4))
			}
		}
	}
	wrap := text + "whiteSpace=wrap;fontSize=13;align=center;"
	add("wrap-bottom", "<b>Un référentiel d'objets unique et fiable</b>", wrap+"verticalAlign=bottom;", 13, 174.5, 44)
	add("wrap-top-3", "<b>Le dossier d'architecture, de sa création à sa publication</b>", wrap+"verticalAlign=top;", 13, 174.5, 40)
	add("wrap-middle", "Un historique de session unique, quel que soit le terminal", wrap+"verticalAlign=middle;", 13, 120, 70)
	add("runs-kpi", `<b style="font-size:46px">41<font style="font-size:22px">%</font></b><br><font style="font-size:10px">AVANCEMENT</font>`,
		text+"whiteSpace=wrap;fontSize=12;align=center;verticalAlign=middle;", 46, 156, 110)
	add("div-lines", "<div>Première ligne</div><div><b>Deuxième</b> ligne</div>", text+"fontSize=12;align=left;verticalAlign=top;", 12, 200, 60)
	add("align-right", "Aligné à droite", text+"fontSize=13;align=right;verticalAlign=bottom;", 13, 200, 40)
	add("spacing-left", "Carte avec marge", "rounded=1;html=1;whiteSpace=wrap;align=left;verticalAlign=top;spacingLeft=26;spacingTop=8;spacingRight=10;fillColor=none;strokeColor=none;fontSize=13;fontFamily=Helvetica;", 13, 220, 60)
	add("plain-lines", "Deux lignes\nsans HTML", "text;strokeColor=none;fillColor=none;fontSize=12;align=center;verticalAlign=middle;", 12, 150, 50)
	return out
}

// calibrationDrawing lays the samples out one per row, far apart, between
// two registration marks: a 4 px square at the origin and one at the
// bottom right, so the PNG's pixels are page pixels.
func calibrationDrawing(samples []textSample) ([]textSample, string, drawioapi.Point) {
	var b strings.Builder
	b.WriteString(`<mxfile><diagram id="cal" name="cal"><mxGraphModel><root><mxCell id="0"/><mxCell id="1" parent="0"/>`)
	b.WriteString(`<mxCell id="m0" value="" style="fillColor=#000000;strokeColor=none;" vertex="1" parent="1"><mxGeometry x="0" y="0" width="4" height="4" as="geometry"/></mxCell>`)
	y, right := 40.0, 0.0
	for i := range samples {
		s := &samples[i]
		s.Cell.X, s.Cell.Y = 80, y
		right = max(right, s.Cell.X+s.Cell.Width)
		fmt.Fprintf(&b, `<mxCell id="t%d" value="%s" style="%s" vertex="1" parent="1"><mxGeometry x="%g" y="%g" width="%g" height="%g" as="geometry"/></mxCell>`,
			i, strings.ReplaceAll(html.EscapeString(s.Label), "\n", "&#xa;"), html.EscapeString(s.Style), s.Cell.X, s.Cell.Y, s.Cell.Width, s.Cell.Height)
		y += s.Cell.Height + 60
	}
	far := drawioapi.Point{X: right + 120, Y: y}
	fmt.Fprintf(&b, `<mxCell id="m1" value="" style="fillColor=#000000;strokeColor=none;" vertex="1" parent="1"><mxGeometry x="%g" y="%g" width="4" height="4" as="geometry"/></mxCell>`, far.X, far.Y)
	b.WriteString(`</root></mxGraphModel></diagram></mxfile>`)
	return samples, b.String(), far
}

// inkBox is the box of the pixels painted (alpha ≥ 96) inside region.
func inkBox(img image.Image, region drawioapi.Box) (drawioapi.Box, bool) {
	x0, y0 := int(region.X), int(region.Y)
	x1, y1 := int(region.X+region.Width), int(region.Y+region.Height)
	minX, minY, maxX, maxY := x1, y1, -1, -1
	for y := max(y0, 0); y < min(y1, img.Bounds().Max.Y); y++ {
		for x := max(x0, 0); x < min(x1, img.Bounds().Max.X); x++ {
			if _, _, _, a := img.At(x, y).RGBA(); a>>8 >= 96 {
				minX, minY, maxX, maxY = min(minX, x), min(minY, y), max(maxX, x), max(maxY, y)
			}
		}
	}
	if maxX < 0 {
		return drawioapi.Box{}, false
	}
	return drawioapi.Box{X: float64(minX), Y: float64(minY), Width: float64(maxX - minX + 1), Height: float64(maxY - minY + 1)}, true
}

// recordTextMetrics exports the calibration drawing through draw.io
// desktop and measures each label's ink.
func recordTextMetrics(t *testing.T) []textSample {
	samples, doc, far := calibrationDrawing(calibrationSamples())
	src := filepath.Join(t.TempDir(), "calibration.drawio")
	if err := os.WriteFile(src, []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}
	data, err := Export(src, "png")
	if err != nil {
		t.Fatalf("export through draw.io desktop: %v", err)
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	// The marks pin the PNG to the page: one at the origin, the far one
	// where it was put (no scale, no offset).
	if m, ok := inkBox(img, drawioapi.Box{Width: 10, Height: 10}); !ok || m.X != 0 || m.Y != 0 {
		t.Fatalf("origin mark not at (0,0): %+v", m)
	}
	if m, ok := inkBox(img, drawioapi.Box{X: far.X - 6, Y: far.Y - 6, Width: 16, Height: 16}); !ok || math.Abs(m.X-far.X) > 1 || math.Abs(m.Y-far.Y) > 1 {
		t.Fatalf("far mark not at %+v: %+v (the PNG is scaled or offset)", far, m)
	}
	for i := range samples {
		s := &samples[i]
		region := drawioapi.Box{X: 4, Y: s.Cell.Y - 28, Width: far.X - 8, Height: s.Cell.Height + 56}
		ink, ok := inkBox(img, region)
		if !ok {
			t.Fatalf("%s: nothing painted", s.Name)
		}
		s.Ink = ink
	}
	return samples
}

// TestTextBoxesMatchExport checks the label estimates against what draw.io
// desktop painted: the nominal box (the one the text-overlap check uses)
// holds every label's ink, and is no more than a little wider and taller
// than it. It runs on the recorded measurements; KTW_REGEN_TEXTMETRICS=1
// records them again (needs draw.io desktop).
func TestTextBoxesMatchExport(t *testing.T) {
	var samples []textSample
	if os.Getenv("KTW_REGEN_TEXTMETRICS") == "1" {
		samples = recordTextMetrics(t)
		out, _ := json.MarshalIndent(samples, "", "  ")
		if err := os.WriteFile(textMetricsFile, append(out, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
	} else {
		raw, err := os.ReadFile(textMetricsFile)
		if err != nil {
			t.Fatalf("%v: record it with KTW_REGEN_TEXTMETRICS=1 (needs draw.io desktop)", err)
		}
		if err := json.Unmarshal(raw, &samples); err != nil {
			t.Fatal(err)
		}
	}
	if len(samples) < 90 {
		t.Fatalf("only %d recorded samples", len(samples))
	}
	const edge = 1.0 // anti-aliasing: a pixel either side
	for _, s := range samples {
		est, lines := drawioapi.TextBox(s.Label, s.Style, s.Cell, 1)
		ink := s.Ink
		if ink.X < est.X-edge || ink.Y < est.Y-edge || ink.X+ink.Width > est.X+est.Width+edge || ink.Y+ink.Height > est.Y+est.Height+edge {
			t.Errorf("%s: the estimate %+v does not hold the ink %+v", s.Name, est, ink)
			continue
		}
		// Side bearings and the lines' leading are all the estimate adds.
		if limit := ink.Width*1.10 + 2; est.Width > limit {
			t.Errorf("%s: estimate %.1f px wide for %.1f px of ink (limit %.1f)", s.Name, est.Width, ink.Width, limit)
		}
		if limit := ink.Height + 0.5*s.Size*float64(lines) + 2; est.Height > limit {
			t.Errorf("%s: estimate %.1f px tall for %.1f px of ink (limit %.1f)", s.Name, est.Height, ink.Height, limit)
		}
	}
}
