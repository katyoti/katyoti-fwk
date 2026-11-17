// SPDX-License-Identifier: Apache-2.0

package wire

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
)

// A solution's universe carries its tags and its ordered priority tags
// (gui.first): installing ArchiFactory gives a universe tagged archifactory
// with them. Installing again without tags keeps them.
func TestUniverseTagsWrittenByASolution(t *testing.T) {
	t.Cleanup(cenv.ResetUniverseManifestCache)
	home := filepath.Join(t.TempDir(), "archi")
	tags := []string{"archifactory"}
	gui := &cenv.UniverseGUI{First: []string{"etude-existant", "archifactory"}}
	if _, err := CreateUniverse(nil, Display{DisplayName: "ArchiFactory", GUI: gui, Tags: tags}, home); err != nil {
		t.Fatal(err)
	}
	read := func() *cenv.UniverseManifest {
		cenv.ResetUniverseManifestCache()
		m, err := cenv.LoadUniverseManifest(application.EnvOf(home).HomeDir())
		if err != nil {
			t.Fatal(err)
		}
		return m
	}
	if m := read(); !reflect.DeepEqual(m.Tags, tags) || !reflect.DeepEqual(m.GUI.First, gui.First) {
		t.Fatalf("Univers.yaml tags %v first %v; want %v and %v", m.Tags, m.GUI.First, tags, gui.First)
	}
	if _, err := CreateUniverse(nil, Display{}, home); err != nil {
		t.Fatal(err)
	}
	if m := read(); !reflect.DeepEqual(m.Tags, tags) {
		t.Fatalf("installing again without tags dropped them: %v", m.Tags)
	}
}
