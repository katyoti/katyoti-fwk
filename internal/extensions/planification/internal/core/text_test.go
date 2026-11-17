// SPDX-License-Identifier: Apache-2.0

package core

import (
	"testing"

	"golang.org/x/text/encoding/charmap"
)

// Stored text is UTF-8 with LF line ends: what a Windows command line or a
// code-page hop did to it is undone, and text that was right is never touched.
func TestStoredTextIsRepairedNeverDamaged(t *testing.T) {
	right := "« Une règle » — the person’s naïve façade, 1 € ✅"
	cp1252, err := charmap.Windows1252.NewDecoder().String(right)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ name, in, want string }{
		{"right text", right, right},
		{"its UTF-8 read as Windows-1252", cp1252, right},
		{"its UTF-8 read as Latin-1", "Â« statement Â»", "« statement »"},
		{"CRLF", "one\r\ntwo\r\n", "one\ntwo\n"},
		{"a lone lead", "Ã la carte; Â plain", "Ã la carte; Â plain"},
		{"le café before a no-break space", "« le café »", "« le café »"},
		{"voilà before a no-break space", "« voilà »", "« voilà »"},
		{"capitals before a no-break space", "CAFÉ !", "CAFÉ !"},
		{"capitals in guillemets", "«ÉTÉ»", "«ÉTÉ»"},
		{"German quotes", "„Gruß“", "„Gruß“"},
		{"one lone sequence", "Â©2026", "Â©2026"},
		{"a mixed text", "Â« repaired Â» and « kept »", "Â« repaired Â» and « kept »"},
	} {
		if got := cleanText(c.in); got != c.want {
			t.Errorf("%s: %q; want %q", c.name, got, c.want)
		}
	}
}
