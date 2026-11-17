// SPDX-License-Identifier: Apache-2.0

package core

import (
	"slices"
	"strings"
	"testing"
)

// updateSegments is the updates of a page's Dernière mise à jour line, each
// with its day (the day a segment opens carried to those that follow).
func updateSegments(md string) []string {
	i := strings.Index(md, updatesLabel)
	if i < 0 {
		return nil
	}
	line := md[i+len(updatesLabel):]
	line = line[:strings.Index(line, "\n")]
	var out []string
	day := ""
	for _, seg := range strings.Split(line, " ; ") {
		if m := updateDated.FindStringSubmatch(seg); m != nil {
			day, seg = m[1]+"/"+m[2]+"/"+m[3], m[4]
		}
		out = append(out, day+" "+seg)
	}
	return out
}

// AN UPDATE NEVER CARRIES THE LINE'S SEPARATOR - the Dernière mise à jour
// line separates its updates with " ; ": a title or an update holding one is
// written with a comma there, so the line read again gives each update whole,
// on its own day.
func TestAnUpdateNeverCarriesTheLinesSeparator(t *testing.T) {
	head := "# 2026.10\n\n    " + updatesLabel + markUpdates + "\n\n"
	entries := []pageEntry{
		{Number: 611, Title: "Deux documents : projets ; univers", Domain: "Guides", Value: "notable", Maturity: "Construit et testé", ForWhom: "tous", Text: "Le texte.", Date: "2026-10-15"},
		{Number: 612, Title: "Une autre", Domain: "Guides", Value: "fix", Maturity: "Construit et testé", ForWhom: "tous", Text: "Le texte.", Date: "2026-10-08", Update: "une correction ; puis une autre"},
	}
	md := renderPage(head, "", entries)
	segs := updateSegments(md)
	if len(segs) != 2 {
		t.Fatalf("%d updates read back from %q; want 2", len(segs), segs)
	}
	for _, want := range []string{"15/10/2026 changement 611 : deux documents : projets, univers", "08/10/2026 changement 612 : une correction, puis une autre"} {
		if !slices.Contains(segs, want) {
			t.Errorf("updates read back %q; want %q", segs, want)
		}
	}
}
