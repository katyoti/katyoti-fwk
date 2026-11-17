// SPDX-License-Identifier: Apache-2.0

package census

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	"github.com/katyoti/katyoti-fwk/internal/core/marshal"
)

// validate_described.go — every kind is a described list.
//
// A person walks the catalog kind by kind: each kind says what it is (its
// declaration's `description`), and each entry of a kind is listed with the
// description its producer wrote once, read from the index (meta) — never by
// opening a layer. Which kinds list described entries is not a list of kind
// names (rule: a kind is declared once and read through Kinds): it is read from
// the kind's own schema. A kind whose schema gives its root a top-level
// `description` field carries a description on every entry, so the kind must
// derive it into meta (`meta.from` with `description` among `meta.fields`).

// schemaDocFunc returns the DSL document of a data-type URI, or false.
type schemaDocFunc func(uri string) ([]byte, bool)

// schemaFromEntries answers a data-type's DSL from the data-type entries of cs.
func schemaFromEntries(cs []contribapi.Contribution) schemaDocFunc {
	docs := map[string][]byte{}
	for _, c := range cs {
		if c.Type == contribapi.ResourceTypeDataType && len(c.Source()) > 0 {
			docs[c.ResolvedURI()] = c.Source()
		}
	}
	return func(uri string) ([]byte, bool) {
		d, ok := docs[uri]
		return d, ok
	}
}

// rootHasDescription reports whether a DSL document's root type declares a
// top-level `description` field.
func rootHasDescription(doc []byte) bool {
	var dsl struct {
		Root  string `yaml:"root"`
		Types map[string]struct {
			Fields map[string]any `yaml:"fields"`
		} `yaml:"types"`
	}
	// Content, not configuration: decoded with no variable expansion.
	if err := marshal.Unmarshal(doc, marshal.YAML, &dsl); err != nil {
		return false
	}
	root, ok := dsl.Types[dsl.Root]
	if !ok {
		return false
	}
	_, has := root.Fields["description"]
	return has
}

// undescribedKinds names, one finding per kind: a kind whose declaration
// carries no description, and a kind whose schema's root has a top-level
// `description` but which does not derive description into meta. A kind whose
// schema the census cannot read is judged on its own description only.
func undescribedKinds(kinds contribapi.Kinds, schemaOf schemaDocFunc) []string {
	var out []string
	for _, name := range kinds.Names() {
		d, _ := kinds.Get(name)
		if strings.TrimSpace(d.Description) == "" {
			out = append(out, fmt.Sprintf("kind %q (%s): declares no description", name, kinds.DeclaredBy(name)))
		}
		doc, ok := schemaOf(kinds.SchemaFor(name))
		if !ok || !rootHasDescription(doc) {
			continue
		}
		_, fields := kinds.MetaFrom(name)
		derived := false
		for _, f := range fields {
			if f == "description" {
				derived = true
			}
		}
		if !derived {
			out = append(out, fmt.Sprintf("kind %q (%s): its schema %s gives every entry a description, but the kind does not derive it into meta (meta.from and description among meta.fields)",
				name, kinds.DeclaredBy(name), kinds.SchemaFor(name)))
		}
	}
	sort.Strings(out)
	return out
}

// assertEveryKindIsDescribed: every kind a binary sees is described, and every
// kind whose entries carry a description lists it from the index.
func assertEveryKindIsDescribed(t *testing.T, name string, kinds contribapi.Kinds, cs []contribapi.Contribution) {
	t.Helper()
	bad := undescribedKinds(kinds, schemaFromEntries(cs))
	if len(bad) == 0 {
		return
	}
	t.Errorf("census[%s]: %d kind(s) are not described lists:\n  %s\n\n"+
		"A kind declares a description; a kind whose schema gives its entries a description derives it into meta, so a listing reads it from the index.",
		name, len(bad), strings.Join(bad, "\n  "))
}
