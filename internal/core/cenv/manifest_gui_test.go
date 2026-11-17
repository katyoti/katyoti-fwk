// SPDX-License-Identifier: Apache-2.0

package cenv

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// A universe declares in Univers.yaml what its GUI hides and which task
// hashtags come first; a universe without the section declares nothing (its
// GUI shows everything).
func TestUniverseGUIIsReadFromUniversYaml(t *testing.T) {
	t.Cleanup(ResetUniverseManifestCache)
	write := func(body string) string {
		home := t.TempDir()
		if err := os.MkdirAll(filepath.Join(home, "config"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(home, "config", "Univers.yaml"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return home
	}

	m, err := LoadUniverseManifest(write("name: archi\ngui:\n  hide: [panel:planning-deck, runs, debug]\n  first: [archifactory]\n"))
	if err != nil {
		t.Fatal(err)
	}
	want := &UniverseGUI{Hide: []string{"panel:planning-deck", "runs", "debug"}, First: []string{"archifactory"}}
	if !reflect.DeepEqual(m.GUI, want) {
		t.Fatalf("gui = %+v, want %+v", m.GUI, want)
	}

	m, err = LoadUniverseManifest(write("name: plain\n"))
	if err != nil {
		t.Fatal(err)
	}
	if m.GUI != nil {
		t.Fatalf("a universe without a gui section declares %+v", m.GUI)
	}
}
