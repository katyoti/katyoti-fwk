// SPDX-License-Identifier: Apache-2.0

package contribution

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"path"
	"strings"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	"gopkg.in/yaml.v3"
)

// Every shipped system kind declaration decodes STRICTLY into the Go
// declaration and is complete.
//
// The system loader parses these files as a generic tree and never notices a
// misspelt key: `layer:` for `layers:` would ship a kind that looks
// descriptor-shaped, and the first symptom would be a data-type opening with
// no default layer somewhere in the GUI. This test is the author-side check
// (concepts §11, "authoring / test") for the one kind that governs all others.
//
// Completeness is the rule d3 introduced: a kind declares its layers and its
// severity, or it is not a declaration. Descriptor kinds (metadata IS the
// resource) are allowed to declare no layers, but must say so by name here so
// the exemption is visible rather than accidental.
func TestEverySystemKindDeclarationIsStrictAndComplete(t *testing.T) {
	descriptorKinds := map[string]bool{} // none today: every system kind carries a data layer

	seen := 0
	err := fs.WalkDir(systemFS, "embed/contribution-type", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(p, ".yaml") {
			return nil
		}
		seen++
		raw, err := fs.ReadFile(systemFS, p)
		if err != nil {
			t.Fatalf("%s: read: %v", p, err)
		}
		var tree any
		if err := yaml.Unmarshal(raw, &tree); err != nil {
			t.Fatalf("%s: yaml: %v", p, err)
		}
		body, err := json.Marshal(tree)
		if err != nil {
			t.Fatalf("%s: json: %v", p, err)
		}
		dec := json.NewDecoder(bytes.NewReader(body))
		dec.DisallowUnknownFields()
		var decl contribapi.KindDeclaration
		if err := dec.Decode(&decl); err != nil {
			t.Errorf("%s: does not decode strictly into KindDeclaration (misspelt or unknown key?): %v", p, err)
			return nil
		}

		id := strings.TrimSuffix(path.Base(p), ".yaml")
		if decl.Name != id {
			t.Errorf("%s: name %q must equal the file basename %q — the URI is derived from the file, the discriminator from the name, and they must agree", p, decl.Name, id)
		}
		if decl.Schema == "" {
			t.Errorf("%s: declares no schema", p)
		}
		if decl.Validate == "" {
			t.Errorf("%s: declares no validate severity", p)
		} else if decl.Validate != contribapi.ValidateError {
			t.Errorf("%s: validate %q - every system kind validates its entries as an error", p, decl.Validate)
		}
		if decl.Budget() <= 0 {
			t.Errorf("%s: declares no meta budget", p)
		}
		if len(decl.Layers) == 0 {
			if !descriptorKinds[id] {
				t.Errorf("%s: declares no layers and is not a listed descriptor kind", p)
			}
			return nil
		}
		defaults := 0
		for name, l := range decl.Layers {
			if l.Default {
				defaults++
			}
			switch name {
			case contribapi.LayerData, contribapi.LayerSource, contribapi.LayerInstructions, contribapi.LayerWidget, contribapi.LayerTranslations:
			default:
				t.Errorf("%s: layer %q is not a layer the framework names (data, source, instructions, widget, translations) — a new layer name needs a consumer first", p, name)
			}
		}
		if defaults != 1 {
			t.Errorf("%s: %d default layers, want exactly 1", p, defaults)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	if seen == 0 {
		t.Fatal("no kind declarations found under embed/contribution-type")
	}
}
