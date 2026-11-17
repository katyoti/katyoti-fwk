// SPDX-License-Identifier: Apache-2.0

package reflect

import "testing"

func TestToKebab(t *testing.T) {
	cases := map[string]string{
		// Simple PascalCase
		"":              "",
		"Run":           "run",
		"RunReport":     "run-report",
		"List":          "list",
		"GetSchema":     "get-schema",
		"InstallSchema": "install-schema",

		// Trailing acronym (the bug that landed displaybridge as
		// `get-widget-u-r-l` until 2026-05-03 — see gui-schema-runtime
		// Subtask 6 follow-up).
		"GetWidgetURL":       "get-widget-url",
		"GetSchemaAST":       "get-schema-ast",
		"GetTSXFile":         "get-tsx-file",
		"GenerateJSONSchema": "generate-json-schema",

		// Leading lowercase + acronym.
		"parseHTTPRequest": "parse-http-request",

		// Acronym at start.
		"URLPath":   "url-path",
		"JSONInput": "json-input",

		// Adjacent acronyms.
		"XMLToJSON": "xml-to-json",

		// Single-letter words at end (degenerate).
		"FooA": "foo-a",

		// Already lowercase.
		"already-kebab": "already-kebab",
	}
	for in, want := range cases {
		if got := ToKebab(in); got != want {
			t.Errorf("ToKebab(%q) = %q, want %q", in, got, want)
		}
	}
}
