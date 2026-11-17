// SPDX-License-Identifier: Apache-2.0

package census

import (
	"strings"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
)

// The layer guard names a media type that contradicts the kind's declaration
// and a layer name carried two ways, and nothing else: an undeclared media
// type is not checked, JSON where YAML is declared conforms, an undeclared
// kind is another guard's.
func TestLayerDeclarationGuard_NamesMismatchesAndAmbiguityOnly(t *testing.T) {
	kind := declaredKind(t, contribapi.KindDeclaration{Name: "skill", Schema: "ktw-test:/data-type/skill", Validate: "warn",
		Layers: map[string]contribapi.LayerDecl{
			"data":         {MediaType: "application/json", Required: true},
			"instructions": {MediaType: "text/markdown", Required: true, Default: true},
			"source":       {},
		},
		Meta: &contribapi.MetaDecl{Budget: 1}})
	dt := declaredKind(t, contribapi.KindDeclaration{Name: "schema-like", Schema: "ktw-test:/data-type/schema-like", Validate: "warn",
		Layers: map[string]contribapi.LayerDecl{"source": {MediaType: "application/yaml", Required: true, Default: true}},
		Meta:   &contribapi.MetaDecl{Budget: 1}})
	kinds := contribapi.KindsFrom([]contribapi.Contribution{kind, dt})

	good := contribapi.Contribution{Provider: "ktw-test", Type: "skill", ID: "good"}
	good.SetData([]byte(`{}`))
	good.Layers["instructions"] = contribapi.LayerRef{Hash: "sha256:i", Size: 1, MediaType: "text/markdown; charset=utf-8"}
	good.Layers["source"] = contribapi.LayerRef{Hash: "sha256:s", Size: 1, MediaType: "text/anything"} // undeclared media type: not checked

	wrong := contribapi.Contribution{Provider: "ktw-test", Type: "skill", ID: "wrong"}
	wrong.SetData([]byte(`{}`))
	wrong.Layers["instructions"] = contribapi.LayerRef{Hash: "sha256:i", Size: 1, MediaType: "text/html"}

	twoWays := contribapi.Contribution{Provider: "ktw-test", Type: "skill", ID: "two-ways"}
	twoWays.SetData([]byte(`{}`))
	twoWays.Layers["instructions"] = contribapi.LayerRef{Hash: "sha256:i", Size: 1, MediaType: "text/markdown"}
	twoWays.Layers["Instructions"] = contribapi.LayerRef{Hash: "sha256:j", Size: 1, MediaType: "text/markdown"}

	jsonForYAML := contribapi.Contribution{Provider: "ktw-test", Type: "schema-like", ID: "j"}
	jsonForYAML.SetSource([]byte(`{"name":"j"}`), "application/json")

	stranger := contribapi.Contribution{Provider: "ktw-test", Type: "ghost", ID: "g"}
	stranger.Layers = map[string]contribapi.LayerRef{"source": {MediaType: "x/y"}}

	got := layerDeclarationMismatches(kinds, []contribapi.Contribution{good, wrong, twoWays, jsonForYAML, stranger})
	joined := strings.Join(got, "\n")
	if len(got) != 3 {
		t.Fatalf("findings = %d, want 3 (wrong media type; Instructions not lower-case; one name two ways):\n%s", len(got), joined)
	}
	for _, want := range []string{
		`ktw-test:/skill/wrong: layer "instructions" has media type "text/html", kind "skill" declares "text/markdown"`,
		`ktw-test:/skill/two-ways carries layer "Instructions": layer names are lower-case`,
		`ktw-test:/skill/two-ways carries layers`,
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing finding %q in:\n%s", want, joined)
		}
	}
}
