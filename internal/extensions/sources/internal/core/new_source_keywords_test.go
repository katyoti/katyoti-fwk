// SPDX-License-Identifier: Apache-2.0

package core

import (
	"reflect"
	"testing"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// The create page's proposing note names the holder's keywords
// (mock-up v6 view 11): read from a `keywords` list of any of
// its capabilities, each once, in the order kept; none when it carries none.
func TestKeywordsOfReadsTheHoldersCapabilityKeywords(t *testing.T) {
	o := cwmapi.Object{Capabilities: map[string]map[string]any{
		"asis-study": {"keywords": []any{" facturation ", "Acme", "facturation", ""}},
		"other":      {"title": "x"},
		"zz":         {"keywords": []string{"Acme", "SAP"}},
	}}
	if got, want := keywordsOf(o), []string{"facturation", "Acme", "SAP"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("keywordsOf = %v, want %v", got, want)
	}
	if got := keywordsOf(cwmapi.Object{}); len(got) != 0 {
		t.Fatalf("an object with no keywords has none: %v", got)
	}
}
