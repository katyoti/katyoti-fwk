// SPDX-License-Identifier: Apache-2.0

package contribution

import (
	"context"
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	schemaapi "github.com/katyoti/katyoti-fwk/internal/core/schema/api"
	"github.com/katyoti/katyoti-fwk/internal/core/schema/parser"
)

// Every translatable text a system kind declares is a field of the kind's own
// data-type: the list (or field) at the root, and — for a list — its key and
// text fields on the items. A path naming a field the schema lacks would be a
// translation nothing can carry; the lint would refuse every translation of it
// as unknown while the kind promised it.
func TestEveryTranslatablePathNamesASchemaField(t *testing.T) {
	kinds, err := SystemKinds()
	if err != nil {
		t.Fatal(err)
	}
	checked := 0
	for _, name := range kinds.Names() {
		decl, _ := kinds.Get(name)
		if len(decl.Translatable) == 0 {
			continue
		}
		schemaURI := decl.Schema
		if !strings.HasPrefix(schemaURI, "system:/data-type/") {
			continue
		}
		file := filepath.Join("embed", "data-type", strings.TrimPrefix(schemaURI, "system:/data-type/")+".yaml")
		out, err := parser.ParseFile(context.Background(), schemaapi.ParseFileInput{Path: file})
		if err != nil {
			t.Errorf("%s: schema %s: %v", name, file, err)
			continue
		}
		root := out.Schema.Types[out.Schema.Root]
		if root == nil {
			t.Errorf("%s: schema has no root type", name)
			continue
		}
		for _, p := range decl.TranslatablePaths() {
			checked++
			// Walk the path down the schema: a field of the current object, the
			// items of a keyed list (which carry the key field), the values of
			// a map. An untyped position (an object described in prose, a
			// list of untyped entries) holds nothing to check further.
			cur := root
			for _, seg := range p.Segments {
				if cur == nil || (len(cur.Fields) == 0 && cur.Catchall == nil) {
					break
				}
				if seg.Any {
					cur = resolve(out.Schema, cur.Catchall)
					continue
				}
				f, ok := cur.Fields[seg.Field]
				if !ok && (cur.Catchall != nil || (cur == root && capabilitySections[name][seg.Field])) {
					// An open object, or a capability's section on an object
					// type: its shape is the capability's type_schema, shipped
					// by the capability's own pack.
					break
				}
				if !ok {
					t.Errorf("%s: translatable %q: the data-type has no field %q", name, p, seg.Field)
					break
				}
				cur = f
				if seg.Key == "" {
					continue
				}
				cur = resolve(out.Schema, f.Items)
				if cur != nil && len(cur.Fields) > 0 {
					if _, ok := cur.Fields[seg.Key]; !ok {
						t.Errorf("%s: translatable %q: the items of %q have no field %q", name, p, seg.Field, seg.Key)
					}
				}
			}
		}
	}
	if checked == 0 {
		t.Fatal("no system kind declares a translatable text — the guide recipe should")
	}
}

// capabilitySections are the capability sections a kind's translatable paths
// step into: top-level keys of an object type the kind's own schema leaves
// open (`pane:` is objectpane's type-pane, holding the type's badges).
var capabilitySections = map[string]map[string]bool{
	"object-type": {"pane": true},
}

// resolve is the type a reference names: inline, or a named type of the schema.
func resolve(s *schemaapi.Schema, r *schemaapi.TypeRef) *schemaapi.Type {
	if r == nil {
		return nil
	}
	if r.Inline != nil {
		return r.Inline
	}
	return s.Types[r.Name]
}

// The data-type kind's own translatable paths: a field's options (list form,
// keyed by value) give their label and description; a type's display extensions
// give their menu, slots.actions, group (with an id) and axis texts; a group
// without an id and a text that is only one `${...}` template are left out,
// a text mixing words and `${...}` stays; two extensions with one name share
// one key.
func TestDataTypeKind_ExtensionTextsTranslatable(t *testing.T) {
	kinds, err := SystemKinds()
	if err != nil {
		t.Fatal(err)
	}
	decl, _ := kinds.Get("data-type")
	paths, errs := contribapi.ParseTranslatablePaths(decl.Translatable)
	if len(errs) > 0 || len(paths) != 13 {
		t.Fatalf("data-type translatable: %d paths, %v", len(paths), errs)
	}
	data := json.RawMessage(`{"types":{
	  "IncrementRow":{"fields":{"a":{"label":"A"},"k":{"content":{"options":[{"value":"svc","label":"Services","description":"Ds"},{"value":"host","label":"Hosts"}]}},"m":{"content":{"options":{"x":"Short form"}}}},"extensions":[
	    {"name":"default","menu":[{"id":"take-out","label":"Take out","prompt":"Why?","confirm":"Take it out?"},{"label":"no id"}]},
	    {"name":"default","menu":[{"id":"hold","label":"Hold"}]}]},
	  "ReleaseRows":{"extensions":[
	    {"name":"header","slots":{"actions":[{"id":"next","label":"${item.next_label}"},{"id":"finish","label":"Finish ${n}"},{"id":"unfreeze","label":"Unfreeze"}]}},
	    {"name":"default","groups":[{"id":"ships","title":"Ships in this release","description":"D"},{"title":"No id"}]}]},
	  "StreamLane":{"extensions":[{"name":"default","axisLabel":"Stream · release mode"}]}}}`)
	want := map[string]string{
		"types.IncrementRow.fields.a.label":                                "A",
		"types.IncrementRow.fields.k.content.options.svc.label":            "Services",
		"types.IncrementRow.fields.k.content.options.svc.description":      "Ds",
		"types.IncrementRow.fields.k.content.options.host.label":           "Hosts",
		"types.IncrementRow.extensions.default.menu.take-out.label":        "Take out",
		"types.IncrementRow.extensions.default.menu.take-out.prompt":       "Why?",
		"types.IncrementRow.extensions.default.menu.take-out.confirm":      "Take it out?",
		"types.IncrementRow.extensions.default.menu.hold.label":            "Hold",
		"types.ReleaseRows.extensions.header.slots.actions.finish.label":   "Finish ${n}",
		"types.ReleaseRows.extensions.header.slots.actions.unfreeze.label": "Unfreeze",
		"types.ReleaseRows.extensions.default.groups.ships.title":          "Ships in this release",
		"types.ReleaseRows.extensions.default.groups.ships.description":    "D",
		"types.StreamLane.extensions.default.axisLabel":                    "Stream · release mode",
	}
	if got := contribapi.SourceTexts(paths, data); !reflect.DeepEqual(got, want) {
		t.Fatalf("source texts:\n got %v\nwant %v", got, want)
	}

	tr := []byte(`{"fr":{"types":{
	  "IncrementRow":{"fields":{"k":{"content":{"options":{"svc":{"label":"Les services"},"host":{"label":"Les hôtes"}}}}},"extensions":{"default":{"menu":{"take-out":{"label":"Retirer","confirm":"Le retirer ?"},"hold":{"label":"Suspendre"}}}}},
	  "ReleaseRows":{"extensions":{"header":{"slots":{"actions":{"unfreeze":{"label":"Dégeler"}}}},"default":{"groups":{"ships":{"title":"Livré dans cette version"}}}}},
	  "StreamLane":{"extensions":{"default":{"axisLabel":"Flux · mode de livraison"}}}}}}`)
	got := contribapi.BundleTexts("p:/data-type/delivery", paths, tr, "fr")
	wantFr := map[string]string{
		"p:/data-type/delivery#types.IncrementRow.fields.k.content.options.svc.label":            "Les services",
		"p:/data-type/delivery#types.IncrementRow.fields.k.content.options.host.label":           "Les hôtes",
		"p:/data-type/delivery#types.IncrementRow.extensions.default.menu.take-out.label":        "Retirer",
		"p:/data-type/delivery#types.IncrementRow.extensions.default.menu.take-out.confirm":      "Le retirer ?",
		"p:/data-type/delivery#types.IncrementRow.extensions.default.menu.hold.label":            "Suspendre",
		"p:/data-type/delivery#types.ReleaseRows.extensions.header.slots.actions.unfreeze.label": "Dégeler",
		"p:/data-type/delivery#types.ReleaseRows.extensions.default.groups.ships.title":          "Livré dans cette version",
		"p:/data-type/delivery#types.StreamLane.extensions.default.axisLabel":                    "Flux · mode de livraison",
	}
	if !reflect.DeepEqual(got, wantFr) {
		t.Fatalf("bundle:\n got %v\nwant %v", got, wantFr)
	}
	var tree map[string]any
	_ = json.Unmarshal(tr, &tree)
	if _, unknown := contribapi.FlattenTranslation(paths, tree["fr"].(map[string]any)); len(unknown) != 0 {
		t.Errorf("unknown: %v", unknown)
	}
	// An option text the kind does not declare stays refused.
	var bad map[string]any
	_ = json.Unmarshal([]byte(`{"types":{"T":{"fields":{"k":{"content":{"options":{"svc":{"label":"S","tip":"?"}}}}}}}}`), &bad)
	if _, unknown := contribapi.FlattenTranslation(paths, bad); strings.Join(unknown, ",") != "types.T.fields.k.content.options.svc.tip" {
		t.Errorf("unknown option text: %v", unknown)
	}
}
