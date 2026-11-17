// SPDX-License-Identifier: Apache-2.0

package api

import "testing"

func TestParseSkillSelector(t *testing.T) {
	cases := []struct {
		entry        string
		wantProvider string
		wantName     string
		wantBulk     bool
	}{
		{"", "", "*", true},
		{"*", "", "*", true},
		{"doc-create", "", "doc-create", false},
		{"cwm-*", "", "cwm-*", true},
		// Singular resource-URI form (the ONLY provider-scoped form) — one named
		// skill, exact. This is the form that broke compose-ui-live-demo.
		{"ktw-methods:/skill/author-schema", "ktw-methods", "author-schema", false},
		// Provider-scoped glob + all, both on the singular /skill/ grammar.
		{"ktw-methods:/skill/author-*", "ktw-methods", "author-*", true},
		{"ktw-weather:/skill/*", "ktw-weather", "*", true},
		// The plural /skills/ form is NOT special — falls through to a bare name
		// (it must never be authored).
		{"ktw-agents:/skills/cwm-*", "", "ktw-agents:/skills/cwm-*", true},
	}
	for _, c := range cases {
		got := ParseSkillSelector(c.entry)
		if got.Provider != c.wantProvider || got.Name != c.wantName {
			t.Errorf("ParseSkillSelector(%q) = {Provider:%q Name:%q}, want {Provider:%q Name:%q}",
				c.entry, got.Provider, got.Name, c.wantProvider, c.wantName)
		}
		if got.Bulk() != c.wantBulk {
			t.Errorf("ParseSkillSelector(%q).Bulk() = %v, want %v", c.entry, got.Bulk(), c.wantBulk)
		}
	}
}
