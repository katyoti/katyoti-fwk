// SPDX-License-Identifier: Apache-2.0

package api

import (
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// SlugMax is the longest slug Slug answers.
const SlugMax = 48

// slugLetters spells the letters NFD does not decompose into a base letter
// and a mark.
var slugLetters = map[rune]string{
	'œ': "oe", 'æ': "ae", 'ß': "ss", 'ø': "o", 'ł': "l", 'đ': "d", 'ð': "d", 'þ': "th", 'ı': "i",
}

// Slug turns a title into a readable kebab slug - the one spelling CWM gives
// a new object's file name and a module's id: letters transliterated to ASCII
// (« Études été » → etudes-ete, œ → oe), lowercase letters, digits and single
// hyphens, at most SlugMax characters cut at a word boundary (a single word
// longer than that is cut where the limit falls); empty when nothing survives.
func Slug(title string) string {
	var b strings.Builder
	lastDash := true
	for _, r := range norm.NFD.String(strings.ToLower(title)) {
		if unicode.Is(unicode.Mn, r) {
			continue
		}
		s := string(r)
		if t, ok := slugLetters[r]; ok {
			s = t
		}
		for _, c := range s {
			if c >= 'a' && c <= 'z' || c >= '0' && c <= '9' {
				b.WriteRune(c)
				lastDash = false
			} else if !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	slug := strings.Trim(b.String(), "-")
	if len(slug) <= SlugMax {
		return slug
	}
	// The word straddling the limit goes: keep up to the last dash that
	// fits, or - when the limit falls just before a dash - the whole word.
	if slug[SlugMax] == '-' {
		return slug[:SlugMax]
	}
	if i := strings.LastIndexByte(slug[:SlugMax], '-'); i > 0 {
		return slug[:i]
	}
	return slug[:SlugMax]
}
