// SPDX-License-Identifier: Apache-2.0

package census

import (
	"strings"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
)

// Falsifying assertEveryBodyIsValidated: an entry with a body whose kind's
// schema nothing registers is refused, naming the entry, its kind and the
// schema; an entry whose schema is registered (by the binary or the declared
// universe), an entry with no data layer and an entry of a kind declaring no
// schema are not - the last is assertEveryKindIsComplete's.
func TestBodyGuard_NamesAnEntryWhoseSchemaNothingRegistersOnly(t *testing.T) {
	kinds := contribapi.KindsFrom([]contribapi.Contribution{
		declaredKind(t, contribapi.KindDeclaration{Name: "widget", Schema: "ktw-other:/data-type/widget", Validate: "error",
			Layers: map[string]contribapi.LayerDecl{"data": {MediaType: "application/json", Default: true}},
			Meta:   &contribapi.MetaDecl{Budget: 1}}),
		declaredKind(t, contribapi.KindDeclaration{Name: "gadget", Schema: "ktw-x:/data-type/gadget", Validate: "error",
			Layers: map[string]contribapi.LayerDecl{"data": {MediaType: "application/json", Default: true}},
			Meta:   &contribapi.MetaDecl{Budget: 1}}),
	})
	entry := func(kind, id string, body string) contribapi.Contribution {
		c := contribapi.Contribution{Provider: "ktw-x", Type: kind, ID: id}
		if body != "" {
			c.SetData([]byte(body))
		}
		c.URI = c.ResolvedURI()
		return c
	}
	cs := []contribapi.Contribution{
		entry("widget", "unchecked", `{"a":1}`),
		entry("widget", "empty", ""),
		entry("gadget", "checked", `{"a":1}`),
		entry("ghost", "no-schema", `{"a":1}`),
	}
	registered := map[string]bool{"ktw-x:/data-type/gadget": true}

	got := unvalidatedBodies(kinds, cs, registered)
	if len(got) != 1 || !strings.Contains(got[0], "ktw-x:/widget/unchecked (widget): schema ktw-other:/data-type/widget is not registered") {
		t.Fatalf("want exactly the unchecked widget, got:\n%s", strings.Join(got, "\n"))
	}

	registered["ktw-other:/data-type/widget"] = true // named through the declared universe
	if got := unvalidatedBodies(kinds, cs, registered); len(got) != 0 {
		t.Errorf("every schema registered, yet: %v", got)
	}
}
