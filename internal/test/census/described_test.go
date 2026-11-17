// SPDX-License-Identifier: Apache-2.0

package census

import (
	"io/fs"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/contribution"
	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	"github.com/katyoti/katyoti-fwk/internal/core/contribution/builtin"
)

// shippedDeclarations is every kind declaration the shipped binaries carry,
// with the schemas they name: the system set, the pack leaves compiled in,
// and every embedded pack folder of the repository that declares kinds
// (`contribution-types/` beside its `data-types/`).
func shippedDeclarations(t *testing.T) ([]contribapi.Contribution, schemaDocFunc) {
	t.Helper()
	sys, err := contribution.SystemKindEntries()
	if err != nil {
		t.Fatal(err)
	}
	decls := append([]contribapi.Contribution(nil), sys...)
	decls = append(decls, LeafDeclarationsOf(t, builtin.ReaderProvider, builtin.Leaves()...)...)
	root := repoRoot(t)
	var packs []string
	for _, pattern := range []string{"app/*/embed/contribution-types", "internal/extensions/*/contrib/embed/contribution-types"} {
		found, _ := filepath.Glob(filepath.Join(root, filepath.FromSlash(pattern)))
		packs = append(packs, found...)
	}
	if len(packs) == 0 {
		t.Fatal("census: no pack folder declares kinds: the check would read the system set only")
	}
	sort.Strings(packs)
	for _, p := range packs {
		dir, _ := filepath.Rel(root, filepath.Dir(p))
		provider := strings.ReplaceAll(filepath.ToSlash(dir), "/", "-")
		decls = append(decls, DeclarationsOf(t, provider, dir)...)
	}
	fromEntries := schemaFromEntries(decls)
	sysFS := contribution.SystemFS()
	return decls, func(uri string) ([]byte, bool) {
		if doc, ok := fromEntries(uri); ok {
			return doc, true
		}
		u, err := contribapi.ParseResourceURI(uri)
		if err != nil || u.Provider != contribapi.SystemProvider {
			return nil, false
		}
		doc, err := fs.ReadFile(sysFS, path.Join("embed", "data-type", u.ID+".yaml"))
		return doc, err == nil
	}
}

// TestEveryKindIsDescribedAndListsDescribedEntries: every kind the shipped
// binaries declare carries a description, and every kind whose schema gives
// its entries a description derives it into meta, so a listing reads it from
// the index. Each binary's census runs the same check on its own manifest
// (AssertValidWithin); this one reads every declaration at once.
func TestEveryKindIsDescribedAndListsDescribedEntries(t *testing.T) {
	decls, schemaOf := shippedDeclarations(t)
	kinds := contribapi.KindsFrom(decls)
	if kinds.Len() < 20 {
		t.Fatalf("census: read %d kinds; the shipped set is larger - the check would prove little", kinds.Len())
	}
	derives := 0
	for _, name := range kinds.Names() {
		if doc, ok := schemaOf(kinds.SchemaFor(name)); ok && rootHasDescription(doc) {
			derives++
		}
	}
	if derives == 0 {
		t.Fatal("census: no kind schema read with a top-level description: the schema reader finds nothing")
	}
	for _, f := range undescribedKinds(kinds, schemaOf) {
		t.Error(f)
	}
}

// The check itself, on made-up kinds: a blank description and a described
// schema not derived are named; a derived one and one without a description
// field are not.
func TestUndescribedKindsNamesBlankAndUnderivedKindsOnly(t *testing.T) {
	kind := func(name, desc, schema, meta string) contribapi.Contribution {
		body := `{"name":"` + name + `","description":"` + desc + `","schema":"` + schema + `","layers":{"data":{"mediaType":"application/json","default":true}}` + meta + `}`
		return contribapi.Contribution{Provider: "ktw-example", Type: contribapi.ResourceTypeContributionType, ID: name,
			Layers: map[string]contribapi.LayerRef{contribapi.LayerData: {Inline: []byte(body), MediaType: "application/json"}}}
	}
	schema := func(id string, withDescription bool) contribapi.Contribution {
		f := "name: {type: string}"
		if withDescription {
			f += "\n      description: {type: string}"
		}
		doc := "name: " + id + "\nroot: R\ntypes:\n  R:\n    fields:\n      " + f + "\n"
		return contribapi.Contribution{Provider: "ktw-example", Type: contribapi.ResourceTypeDataType, ID: id,
			Layers: map[string]contribapi.LayerRef{contribapi.LayerSource: {Inline: []byte(doc), MediaType: "application/yaml"}}}
	}
	derived := `,"meta":{"from":"data","fields":["description"]}`
	cs := []contribapi.Contribution{
		kind("listed", "a listed kind", "ktw-example:/data-type/listed", derived), schema("listed", true),
		kind("plain", "a kind with no description field", "ktw-example:/data-type/plain", ""), schema("plain", false),
		kind("blank", "", "ktw-example:/data-type/plain", ""),
		kind("underived", "a described schema", "ktw-example:/data-type/described", ""), schema("described", true),
	}
	got := undescribedKinds(contribapi.KindsFrom(cs), schemaFromEntries(cs))
	if len(got) != 2 || !strings.Contains(got[0], `"blank"`) || !strings.Contains(got[1], `"underived"`) {
		t.Fatalf("findings = %q, want the blank and the underived kinds only", got)
	}
}
