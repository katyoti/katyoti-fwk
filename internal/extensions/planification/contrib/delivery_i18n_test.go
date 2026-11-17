// SPDX-License-Identifier: Apache-2.0

package contrib

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/contribution"
	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	"github.com/katyoti/katyoti-fwk/internal/core/marshal"
)

// THE DELIVERY DATA TYPES HAVE THEIR FRENCH TEXTS:
// every translatable text of the delivery data types - each field's
// description, the rows' menus, the release header's and the lane header's
// actions, the detail panes' groups (each with its stable id) and the
// timeline's axis label - has its French in the <name>.dsl.i18n.yaml beside
// its DSL, and none of those files holds a key the data-type kind does not
// translate.
func TestDeliveryDataTypesHaveTheirFrenchTexts(t *testing.T) {
	kinds, err := contribution.SystemKinds()
	if err != nil {
		t.Fatal(err)
	}
	decl, ok := kinds.Get("data-type")
	if !ok {
		t.Fatal("no data-type kind")
	}
	paths, errs := contribapi.ParseTranslatablePaths(decl.Translatable)
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	dir := filepath.Join("embed", "cwm", "data-types")
	asJSON := func(file string) (json.RawMessage, map[string]any) {
		t.Helper()
		b, err := os.ReadFile(filepath.Join(dir, file))
		if err != nil {
			t.Fatal(err)
		}
		var tree map[string]any
		if err := config.DecodeBytes(b, marshal.YAML, &tree); err != nil {
			t.Fatalf("%s: %v", file, err)
		}
		raw, err := json.Marshal(tree)
		if err != nil {
			t.Fatal(err)
		}
		return raw, tree
	}
	for _, name := range []string{"stream-lane", "release-rows", "increment-row", "increment-detail"} {
		src, _ := asJSON(name + ".dsl.yaml")
		texts := contribapi.SourceTexts(paths, src)
		raw, tree := asJSON(name + ".dsl.i18n.yaml")
		fr, _ := tree["fr"].(map[string]any)
		if fr == nil {
			t.Errorf("%s.dsl.i18n.yaml has no fr", name)
			continue
		}
		if _, unknown := contribapi.FlattenTranslation(paths, fr); len(unknown) > 0 {
			t.Errorf("%s.dsl.i18n.yaml: keys the data-type kind does not translate: %v", name, unknown)
		}
		got := contribapi.BundleTexts("u", paths, raw, "fr")
		var missing []string
		for p, en := range texts {
			if s := got[contribapi.TranslationKey("u", p)]; s == "" || s == en {
				missing = append(missing, p)
			}
		}
		sort.Strings(missing)
		if len(missing) > 0 {
			t.Errorf("%s: %d texts without their French:\n  %s", name, len(missing), strings.Join(missing, "\n  "))
		}
		// Every group with a title is translatable: it has its id.
		titled := false
		for p := range texts {
			titled = titled || (strings.Contains(p, ".groups.") && strings.HasSuffix(p, ".title"))
		}
		if !titled && (name == "release-rows" || name == "increment-detail") {
			t.Errorf("%s: no group title is translatable - its groups have no id", name)
		}
	}
	// The texts the screen shows, by name.
	_, tree := asJSON("release-rows.dsl.i18n.yaml")
	b, _ := json.Marshal(tree)
	for _, want := range []string{`"ships":{"title":"Livrés dans cette version"}`, `"unfreeze":{"confirm":"La dégeler pour changer ce qu'elle livre ?","label":"Dégeler"}`,
		`"take-out":{"confirm":"Le retirer de sa version ?","label":"Retirer"}`} {
		if !strings.Contains(string(b), want) {
			t.Errorf("release-rows.dsl.i18n.yaml lacks %s", want)
		}
	}
	_, lane := asJSON("stream-lane.dsl.i18n.yaml")
	if b, _ := json.Marshal(lane); !strings.Contains(string(b), `"axisLabel":"Flux · mode de livraison"`) {
		t.Errorf("stream-lane.dsl.i18n.yaml: the timeline's axis label is not French: %s", b)
	}
}
