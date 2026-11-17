// SPDX-License-Identifier: Apache-2.0

package api_test

import (
	"strings"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
)

// declaring is a member publishing one object type with body.
func declaring(provider, body string) *mbapi.Manifest {
	return &mbapi.Manifest{Provider: provider, Contributions: []contribapi.Contribution{
		{Provider: provider, Type: "object-type", ID: "note", Layers: contribapi.InlineData([]byte(body))},
	}}
}

// TWO PROVIDERS NEVER COLLIDE SILENTLY. A name declared
// differently by a second provider is refused with an error finding naming
// both - the same one whichever order the catalog lists them in; the same body
// from several binaries (a pack linked into each) is one declaration. Every
// declaration read carries its URI and provider.
func TestTwoProvidersDeclaringOneNameDifferentlyAreRefused(t *testing.T) {
	a, b := declaring("ktw-a", `{"name":"note"}`), declaring("ktw-b", `{"name":"note","abstract":true}`)
	for _, order := range [][]*mbapi.Manifest{{a, b}, {b, a}} {
		d := cwmapi.DeclarationsFromManifests(order, nil)
		if len(d.Objects) != 1 || d.Objects[0].Provider != "ktw-a" || d.Objects[0].Abstract {
			t.Fatalf("objects = %+v, want ktw-a's note only", d.Objects)
		}
		src := d.Sources[cwmapi.DeclarationKey("object-type", "note")]
		if src.Provider != "ktw-a" || src.URI == "" {
			t.Errorf("source of note = %+v, want ktw-a's URI", src)
		}
		var clash []cwmapi.DeclarationFinding
		for _, f := range d.Findings {
			if f.Rule == cwmapi.FindingNameClash {
				clash = append(clash, f)
			}
		}
		if len(clash) != 1 || clash[0].Severity != cwmapi.FindingError || clash[0].Provider != "ktw-b" ||
			!strings.Contains(clash[0].Message, "ktw-a") {
			t.Errorf("clash findings = %+v, want one error on ktw-b naming ktw-a", clash)
		}
		if !d.Members["ktw-a"] || !d.Members["ktw-b"] || len(d.Named) != 2 {
			t.Errorf("members %v, named %v: want both providers and both URIs", d.Members, d.Named)
		}
	}

	same := cwmapi.DeclarationsFromManifests([]*mbapi.Manifest{a, declaring("ktw-c", `{ "name": "note" }`)}, nil)
	if len(same.Objects) != 1 || len(same.Findings) != 0 {
		t.Errorf("one body from two binaries: objects %d, findings %+v; want one, none", len(same.Objects), same.Findings)
	}
}
