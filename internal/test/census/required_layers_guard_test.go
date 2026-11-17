// SPDX-License-Identifier: Apache-2.0

package census

import (
	"encoding/json"
	"strings"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
)

func declaredKind(t *testing.T, d contribapi.KindDeclaration) contribapi.Contribution {
	t.Helper()
	body, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	c := contribapi.Contribution{Provider: "ktw-test", Type: contribapi.ResourceTypeContributionType, ID: d.Name}
	c.SetData(body)
	return c
}

// A layer a kind declares `required: true` had no reader until 2026-09-22.
// The guard names every entry that lacks one, and nothing else: an optional
// layer absent is not a finding, an undeclared kind is another guard's.
func TestRequiredLayersGuard_NamesEveryMissingLayerAndOnlyThose(t *testing.T) {
	kind := declaredKind(t, contribapi.KindDeclaration{Name: "skill", Schema: "ktw-test:/data-type/skill", Validate: "warn",
		Layers: map[string]contribapi.LayerDecl{
			"data":         {Required: true},
			"instructions": {Required: true, Default: true},
			"source":       {Required: false},
		},
		Meta: &contribapi.MetaDecl{Budget: 1}})
	kinds := contribapi.KindsFrom([]contribapi.Contribution{kind})

	complete := contribapi.Contribution{Provider: "ktw-test", Type: "skill", ID: "complete"}
	complete.SetData([]byte(`{}`))
	complete.Layers["instructions"] = contribapi.LayerRef{Hash: "sha256:i", Size: 1, MediaType: "text/markdown"}
	// no source: optional, not a finding

	halved := contribapi.Contribution{Provider: "ktw-test", Type: "skill", ID: "halved"}
	halved.SetData([]byte(`{}`)) // lacks instructions

	stranger := contribapi.Contribution{Provider: "ktw-test", Type: "ghost", ID: "g"} // undeclared kind: not this guard's

	got := missingRequiredLayers(kinds, []contribapi.Contribution{complete, halved, stranger})
	if len(got) != 1 || !strings.Contains(got[0], `ktw-test:/skill/halved: kind "skill" requires layer "instructions"`) {
		t.Fatalf("missing = %v, want exactly the halved entry's instructions layer", got)
	}
}
