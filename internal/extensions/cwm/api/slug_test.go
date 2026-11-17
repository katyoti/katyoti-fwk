// SPDX-License-Identifier: Apache-2.0

package api

import "testing"

// : a title's accented letters are spelled in ASCII, never dropped.
func TestSlugTransliteratesAndCaps(t *testing.T) {
	cases := []struct{ title, want string }{
		{"Études été", "etudes-ete"},
		{"Cœur Ça va Straße", "coeur-ca-va-strasse"},
		{"Ångström Ærø", "angstrom-aero"},
		{"Short title", "short-title"},
		{"Une très longue étude des écarts entre les applications métier", "une-tres-longue-etude-des-ecarts-entre-les"},
		{"?!", ""},
	}
	for _, c := range cases {
		got := Slug(c.title)
		if got != c.want {
			t.Errorf("Slug(%q) = %q, want %q", c.title, got, c.want)
		}
		if len(got) > SlugMax {
			t.Errorf("Slug(%q) is %d long, over %d", c.title, len(got), SlugMax)
		}
	}
}
