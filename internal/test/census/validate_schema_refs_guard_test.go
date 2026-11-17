// SPDX-License-Identifier: Apache-2.0

package census

import (
	"encoding/json"
	"strings"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
)

// Falsifying assertSchemaReferencesShip: a panel page whose table names a
// data-type of its own provider that is not published is refused, naming the
// entry and the URI; a shipped one, another provider's one and a schema
// written inline are not.
func TestSchemaReferenceGuard_NamesAnUnshippedOwnDataTypeOnly(t *testing.T) {
	panel := contribapi.Contribution{Provider: "ktw-x", Type: "object-panel", ID: "composition"}
	panel.SetData(json.RawMessage(`{"name":"Composition","content":{"kind":"page","dslTree":{"kind":"vbox","children":[
		{"kind":"table","schema":"ktw-x:/data-type/use-case"},
		{"kind":"table","schema":"ktw-x:/data-type/deliverable"},
		{"kind":"table","schema":"ktw-other:/data-type/thing"},
		{"kind":"table","schema":"system:/data-type/gone"},
		{"kind":"form","schema":{"root":"T","types":{}}}]}}}`))
	panel.URI = panel.ResolvedURI()
	registered := map[string]bool{"ktw-x:/data-type/use-case": true}

	got := strings.Join(unshippedSchemaReferences("ktw-x", []contribapi.Contribution{panel}, registered), "\n")
	for _, want := range []string{
		"ktw-x:/object-panel/composition: names schema ktw-x:/data-type/deliverable",
		"ktw-x:/object-panel/composition: names schema system:/data-type/gone",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("findings lack %q:\n%s", want, got)
		}
	}
	if strings.Count(got, "\n")+1 != 2 {
		t.Errorf("want exactly two findings, got:\n%s", got)
	}

	registered["ktw-x:/data-type/deliverable"] = true
	registered["system:/data-type/gone"] = true
	if got := unshippedSchemaReferences("ktw-x", []contribapi.Contribution{panel}, registered); len(got) != 0 {
		t.Errorf("every reference ships, yet: %v", got)
	}
}
