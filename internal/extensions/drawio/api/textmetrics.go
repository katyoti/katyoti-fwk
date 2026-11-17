// SPDX-License-Identifier: Apache-2.0

package api

import (
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

//go:generate go run ../internal/gentextmetrics -o textmetrics_gen.go

// Glyph widths. draw.io's default font is Helvetica, which the browser
// draws with Arial-metric glyphs; the generated tables carry those
// advances per 1000 em. A glyph outside them takes its base letter's width
// (an accent adds none), else a pessimistic average; a wide glyph (CJK,
// emoji) a full em.
const (
	unknownRegular = 620
	unknownBold    = 680
	wideGlyph      = 1000
)

// metricFamilies are the font families the tables measure; any other family
// is measured with the unknown-glyph averages.
var metricFamilies = map[string]bool{
	"": true, "helvetica": true, "arial": true, "liberation sans": true, "arimo": true,
	"sans-serif": true, "helvetica neue": true,
}

// glyphAdvance is a rune's advance per 1000 em.
func glyphAdvance(r rune, bold, metric bool) float64 {
	if isWide(r) {
		return wideGlyph
	}
	if !metric {
		if r == ' ' || r == ' ' {
			return 278
		}
		if bold {
			return unknownBold
		}
		return unknownRegular
	}
	table := helveticaRegular
	if bold {
		table = helveticaBold
	}
	if w, ok := table[r]; ok {
		return float64(w)
	}
	// An accented letter is as wide as its base letter.
	if d := norm.NFD.String(string(r)); d != string(r) {
		if base := []rune(d)[0]; base != r {
			if w, ok := table[base]; ok {
				return float64(w)
			}
		}
	}
	if unicode.Is(unicode.Mn, r) {
		return 0
	}
	if bold {
		return unknownBold
	}
	return unknownRegular
}

// isWide reports a glyph a browser draws a full em wide: CJK, Hangul,
// full-width forms and emoji.
func isWide(r rune) bool {
	switch {
	case r >= 0x1100 && r <= 0x115F, r >= 0x2E80 && r <= 0xA4CF, r >= 0xAC00 && r <= 0xD7A3,
		r >= 0xF900 && r <= 0xFAFF, r >= 0xFE30 && r <= 0xFE4F, r >= 0xFF00 && r <= 0xFF60,
		r >= 0xFFE0 && r <= 0xFFE6, r >= 0x1F300 && r <= 0x1FAFF, r >= 0x2600 && r <= 0x27BF:
		return true
	}
	return false
}

// textWidth is the advance of s at size px.
func textWidth(s string, size float64, bold, metric bool) float64 {
	var w float64
	for _, r := range s {
		w += glyphAdvance(r, bold, metric)
	}
	return w * size / 1000
}

// metricFamily reports whether a style's fontFamily is measured by the
// tables (Helvetica and its Arial-metric equals).
func metricFamily(family string) bool {
	f := strings.ToLower(strings.TrimSpace(family))
	if i := strings.IndexByte(f, ','); i >= 0 {
		f = strings.TrimSpace(f[:i])
	}
	return metricFamilies[strings.Trim(f, `"'`)]
}
