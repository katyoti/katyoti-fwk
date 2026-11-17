// SPDX-License-Identifier: Apache-2.0

package api

import (
	"math"
	"testing"
)

// TestGlyphTable_IsTheHelveticaMetric pins the generated table on Adobe's
// core-14 Helvetica AFM widths: whichever Arial-metric font it was read
// from, these advances must come out.
func TestGlyphTable_IsTheHelveticaMetric(t *testing.T) {
	for _, c := range []struct {
		r       rune
		regular int16
		bold    int16
	}{
		{' ', 278, 278}, {'a', 556, 556}, {'i', 222, 278}, {'m', 833, 889}, {'W', 944, 944},
		{'M', 833, 833}, {'0', 556, 556}, {'%', 889, 889}, {'.', 278, 278}, {'é', 556, 556}, {'–', 556, 556},
	} {
		if got := helveticaRegular[c.r]; got != c.regular {
			t.Errorf("%q regular: %d, want %d", c.r, got, c.regular)
		}
		if got := helveticaBold[c.r]; got != c.bold {
			t.Errorf("%q bold: %d, want %d", c.r, got, c.bold)
		}
	}
	// A letter outside the table takes its base letter's width.
	if got := glyphAdvance('ě', false, true); got != 556 {
		t.Errorf("ě: %v, want e's 556", got)
	}
	if got := glyphAdvance('漢', false, true); got != 1000 {
		t.Errorf("CJK: %v, want a full em", got)
	}
}

func near(a, b float64) bool { return math.Abs(a-b) < 0.01 }

// TestTextBox_Layout covers the style reading: widths, wrap, runs, align.
func TestTextBox_Layout(t *testing.T) {
	cell := Box{X: 100, Y: 100, Width: 200, Height: 40}
	// "MMMM" at 12 px: 4 × 833 / 1000 × 12 = 39.984, left-aligned at the
	// default 2 px spacing, centred vertically on one 14.4 px line.
	b, n := TextBox("MMMM", "text;html=1;align=left;fontSize=12;", cell, 1)
	if n != 1 || !near(b.X, 102) || !near(b.Width, 39.984) || !near(b.Height, 14.4) || !near(b.Y, 112.8) {
		t.Errorf("MMMM: %+v, %d lines", b, n)
	}
	// Bold is wider; the slack widens and adds a pixel.
	bb, _ := TextBox("<b>mmmm</b>", "text;html=1;align=left;fontSize=12;", cell, 1)
	if !near(bb.Width, 4*0.889*12) {
		t.Errorf("bold mmmm: %+v", bb)
	}
	ws, _ := TextBox("MMMM", "text;html=1;align=left;fontSize=12;", cell, OverflowSlack)
	if !near(ws.Width, 39.984*OverflowSlack+1) {
		t.Errorf("slack: %+v", ws)
	}
	// Wrap breaks on spaces at the cell's width minus spacing; without
	// wrap the line runs on.
	long := "MMMM MMMM MMMM MMMM MMMM MMMM"
	if _, n := TextBox(long, "text;html=1;whiteSpace=wrap;fontSize=12;", cell, 1); n != 2 {
		t.Errorf("wrapped: %d lines, want 2", n)
	}
	if b, n := TextBox(long, "text;html=1;fontSize=12;", cell, 1); n != 1 || b.Width < 200 {
		t.Errorf("unwrapped: %+v, %d lines", b, n)
	}
	// HTML runs: one line per <br>, each as tall as its largest run.
	b, n = TextBox(`<b style="font-size:46px">41<font style="font-size:22px">%</font></b><br><font style="font-size:10px">AVANCEMENT</font>`,
		"text;html=1;fontSize=12;", Box{Width: 156, Height: 156}, 1)
	if n != 2 || !near(b.Height, 46*1.2+10*1.2) {
		t.Errorf("runs: %+v, %d lines", b, n)
	}
	// Not drawn.
	if _, n := TextBox("x", "text;noLabel=1;", cell, 1); n != 0 {
		t.Error("noLabel draws nothing")
	}
	if _, n := TextBox("<br>", "text;html=1;", cell, 1); n != 0 {
		t.Error("an empty label draws nothing")
	}
	// Right and bottom alignment respect the spacing.
	b, _ = TextBox("MMMM", "text;html=1;align=right;verticalAlign=bottom;fontSize=12;spacingRight=8;", cell, 1)
	if !near(b.X+b.Width, 300-10) || !near(b.Y+b.Height, 140-2) {
		t.Errorf("right/bottom: %+v", b)
	}
}
