// SPDX-License-Identifier: Apache-2.0

package api

import (
	"errors"
	"testing"
)

// TestParseQualifiedID: <key>:<module>.<local> splits into the
// project key and the module-qualified id; a bare id is not qualified but
// reads as a reference relative to its container; what is neither is refused,
// never guessed at.
func TestParseQualifiedID(t *testing.T) {
	for _, c := range []struct{ in, key, id string }{
		{"katyoti:billing.F-091", "katyoti", "billing.F-091"},
		{"archi-factory:shop.RQ-050", "archi-factory", "shop.RQ-050"},
		{"katyoti:billing", "katyoti", "billing"}, // a module object
		{" katyoti:billing.F-091 ", "katyoti", "billing.F-091"},
	} {
		key, id, err := ParseQualifiedID(c.in)
		if err != nil || key != c.key || id != c.id {
			t.Errorf("ParseQualifiedID(%q) = %q, %q, %v - want %q, %q", c.in, key, id, err, c.key, c.id)
		}
		if got := QualifiedID(key, id); got != QualifiedID(c.key, c.id) || !IsQualifiedID(got) {
			t.Errorf("QualifiedID(%q, %q) = %q does not parse back", key, id, got)
		}
	}
	for _, bad := range []string{
		"billing.F-091",            // bare: not qualified
		":billing.F-091",           // no key
		"katyoti:",                 // no id
		"kat.yoti:billing.F-091",   // a dot in the key
		"a:b:billing.F-091",        // two keys
		"ktw-methods:goals/add",    // a tool path, not an object id
		"ktw-compose:/skill/x",     // a resource URI
		"my project:billing.F-091", // a space in the key
	} {
		if _, _, err := ParseQualifiedID(bad); !errors.Is(err, ErrInvalidInput) {
			t.Errorf("ParseQualifiedID(%q): %v - want invalid input", bad, err)
		}
	}

	// A reference as written: bare relative to its container, or qualified.
	if key, id, err := SplitReference("billing.F-091"); err != nil || key != "" || id != "billing.F-091" {
		t.Errorf("SplitReference(bare) = %q, %q, %v", key, id, err)
	}
	if key, id, err := SplitReference("katyoti:billing.F-091"); err != nil || key != "katyoti" || id != "billing.F-091" {
		t.Errorf("SplitReference(qualified) = %q, %q, %v", key, id, err)
	}
	if _, _, err := SplitReference("a:b:billing.F-091"); err == nil {
		t.Error("SplitReference read a malformed qualified id as a reference")
	}

	// The writer: bare inside one project, qualified across.
	if got := Reference("katyoti", "katyoti", "billing.F-091"); got != "billing.F-091" {
		t.Errorf("Reference inside a project = %q, want bare", got)
	}
	if got := Reference("katyoti", "", "billing.F-091"); got != "billing.F-091" {
		t.Errorf("Reference to no named project = %q, want bare", got)
	}
	if got := Reference("archi-factory", "katyoti", "billing.F-091"); got != "katyoti:billing.F-091" {
		t.Errorf("Reference across projects = %q, want katyoti:billing.F-091", got)
	}
}
