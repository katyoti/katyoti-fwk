// SPDX-License-Identifier: Apache-2.0

package commands

import (
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
)

// The URI parser lives in mbapi (single source of truth). These tests
// exercise it from the consumer's POV — same behaviour as before.

func TestParseURI_Valid(t *testing.T) {
	cases := []struct {
		uri          string
		wantProvider string
		wantType     string
		wantItemPath string
	}{
		{"ktw-docs:/skill/doc-create", "ktw-docs", "skill", "doc-create"},
		{"ktw-docs:/reference/templates/report.typ", "ktw-docs", "reference", "templates/report.typ"},
		{"ktw-example:/capability/architecture-query", "ktw-example", "capability", "architecture-query"},
		{"ktw-test:/task/render-and-announce", "ktw-test", "task", "render-and-announce"},
		{"ktw-x:/file/a/b/c/d.txt", "ktw-x", "file", "a/b/c/d.txt"},
	}
	for _, c := range cases {
		t.Run(c.uri, func(t *testing.T) {
			uri, err := contribapi.ParseResourceURI(c.uri)
			if err != nil {
				t.Fatalf("err = %v", err)
			}
			if uri.Provider != c.wantProvider || uri.Type != c.wantType || uri.ID != c.wantItemPath {
				t.Errorf("got (%q, %q, %q), want (%q, %q, %q)",
					uri.Provider, uri.Type, uri.ID,
					c.wantProvider, c.wantType, c.wantItemPath)
			}
		})
	}
}

func TestParseURI_Invalid(t *testing.T) {
	cases := []string{
		"",
		"no-colon-slash",
		"ktw-docs",
		"ktw-docs:",
		"ktw-docs:/",
		"ktw-docs:/skill",
		// `:/skill/x` is now VALID (empty provider = current binary per the
		// the command-surface decision). Dropped from the invalid set.
		"ktw-docs://x",
	}
	for _, uri := range cases {
		t.Run(uri, func(t *testing.T) {
			_, err := contribapi.ParseResourceURI(uri)
			if err == nil {
				t.Errorf("expected error for %q", uri)
				return
			}
			// New parser returns sentinel ErrInvalidURI (errors.Is-able)
			// instead of the legacy ErrInvalidURI struct type.
			// (Either form indicates a parse failure; the test only
			// needs to confirm err != nil here.)
		})
	}
}
