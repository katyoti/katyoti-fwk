// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"context"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/contribution"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	"github.com/katyoti/katyoti-fwk/internal/core/marshal"
	"github.com/katyoti/katyoti-fwk/internal/core/panel"
	manifestcontrib "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/contrib"
)

// A pack adds a panel type with a file and no code: its
// panel-types/<name>.yaml is read as the system kind declares - a
// <pack>:/panel-type/<name> entry - and its tree is a valid panel type.
func TestAPackAddsAPanelTypeWithNoCode(t *testing.T) {
	kinds, err := contribution.SystemKinds()
	if err != nil {
		t.Fatal(err)
	}
	body := "label: Étapes\npage: ktw-test:/page-template/steps\ndataType: ktw-test:/data-type/steps\ninputs: [cancel]\nretention: 7d\nkeep: outcome\n"
	pack := loader.NewMapLoader(map[string]string{"panel-types/steps.yaml": body})
	var uris []string
	for _, p := range manifestcontrib.PackProviders("ktw-test", pack, ".", kinds) {
		if p.Type() != "panel-type" {
			continue
		}
		got, err := p.Enumerate(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range got {
			uris = append(uris, c.ResolvedURI())
		}
	}
	if len(uris) != 1 || uris[0] != "ktw-test:/panel-type/steps" {
		t.Fatalf("the pack's panel-types/ folder gives %v, want the one entry ktw-test:/panel-type/steps", uris)
	}
	var decl panel.TypeDecl
	if err := config.DecodeBytes([]byte(body), marshal.YAML, &decl); err != nil {
		t.Fatal(err)
	}
	if err := decl.Validate(); err != nil || decl.KeepOrDefault() != panel.KeepOutcome {
		t.Fatalf("the pack's type is no valid panel type: %v %+v", err, decl)
	}
}
