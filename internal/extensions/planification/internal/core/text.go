// SPDX-License-Identifier: Apache-2.0

package core

import (
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding/charmap"
)

// STORED TEXT — what a worker hands planification (a rescope's wording, a
// report's more: a delta, follow-ups, a choice) is stored as UTF-8 with LF line
// ends, whatever the shell it came through did to it: a Windows command line
// brings CRLF, and a code-page hop reads UTF-8 bytes as Windows-1252 (« turns
// into Â«, — into â€”).

// cleanText is s with LF line ends and its doubly encoded UTF-8 repaired.
func cleanText(s string) string {
	return repairMojibake(strings.ReplaceAll(s, "\r\n", "\n"))
}

// cleanValue is v - decoded JSON: maps, lists, strings - with every string,
// key included, through cleanText.
func cleanValue(v any) any {
	switch x := v.(type) {
	case string:
		return cleanText(x)
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, e := range x {
			out[cleanText(k)] = cleanValue(e)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = cleanValue(e)
		}
		return out
	}
	return v
}

// repairMojibake is s decoded back when the whole of it is UTF-8 read as
// Windows-1252 (or Latin-1): every rune of s is one byte in Windows-1252, those
// bytes are valid UTF-8, and they hold at least two characters beyond ASCII.
// Anything else is kept as is - the repair is never made one character at a
// time, so a correct text whose accent happens to precede a byte-like
// character (« le café » with its no-break space, «ÉTÉ», „Gruß“) is never
// rewritten; and one lone sequence (Â©2026, CAFÉ followed by a no-break space)
// is too little to tell a code-page hop from text that was meant.
func repairMojibake(s string) string {
	if !strings.ContainsFunc(s, func(r rune) bool { return r >= 0xC2 && r <= 0xF4 }) {
		return s
	}
	buf := make([]byte, 0, len(s))
	for _, r := range s {
		if r < utf8.RuneSelf {
			buf = append(buf, byte(r))
			continue
		}
		b, ok := cp1252Byte(r)
		if !ok {
			return s
		}
		buf = append(buf, b)
	}
	if !utf8.Valid(buf) {
		return s
	}
	wide := 0
	for _, r := range string(buf) {
		if r >= utf8.RuneSelf {
			wide++
		}
	}
	if wide < 2 {
		return s
	}
	return string(buf)
}

// cp1252Byte is the byte r is in Windows-1252 (Latin-1 for the bytes it leaves
// undefined).
func cp1252Byte(r rune) (byte, bool) {
	if b, ok := charmap.Windows1252.EncodeRune(r); ok {
		return b, true
	}
	if r < 0x100 {
		return byte(r), true
	}
	return 0, false
}
