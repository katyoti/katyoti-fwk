// SPDX-License-Identifier: Apache-2.0

package ask_user_data

import (
	"reflect"
	"testing"
)

// The submission of 2026-09-22 carried "Infolog\n" as a subject and the
// agent searched for it verbatim. Every string a form returns is trimmed and
// stripped of control characters; newlines inside multi-line text stay.
func TestCleanStrings_TrimsAndStripsControlCharactersEverywhere(t *testing.T) {
	in := map[string]any{
		"subject": "Infolog\n",
		"notes":   "  line one\nline two\t end \x00\x07 ",
		"sources": []any{"confluence\r", " leanix ", 3},
		"nested":  map[string]any{"owner": "\u200bChris\x1b[0m"},
		"count":   2,
		"flag":    true,
	}
	got := cleanStrings(in)
	want := map[string]any{
		"subject": "Infolog",
		"notes":   "line one\nline two\t end",
		"sources": []any{"confluence", "leanix", 3},
		"nested":  map[string]any{"owner": "\u200bChris[0m"},
		"count":   2,
		"flag":    true,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("cleanStrings =\n  %#v\nwant\n  %#v", got, want)
	}
	if in["subject"] != "Infolog\n" {
		t.Error("the input must not be mutated")
	}
	if cleanStrings(nil) != nil {
		t.Error("nil stays nil")
	}
}
