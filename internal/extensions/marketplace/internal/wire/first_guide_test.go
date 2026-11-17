// SPDX-License-Identifier: Apache-2.0

package wire

import (
	"path/filepath"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
)

// A solution names its universe's first guide, and installing it writes it in
// Univers.yaml; installing again without one keeps it.
func TestInstallingASolutionSetsItsFirstGuide(t *testing.T) {
	t.Cleanup(cenv.ResetUniverseManifestCache)
	home := filepath.Join(t.TempDir(), "archi")
	const guide = "ktw-acme:/guide-recipe/welcome"
	read := func() string {
		cenv.ResetUniverseManifestCache()
		m, err := cenv.LoadUniverseManifest(application.EnvOf(home).HomeDir())
		if err != nil {
			t.Fatal(err)
		}
		return m.FirstGuide
	}
	if _, err := CreateUniverse(nil, Display{DisplayName: "ArchiFactory", FirstGuide: guide}, home); err != nil {
		t.Fatal(err)
	}
	if got := read(); got != guide {
		t.Fatalf("Univers.yaml first guide = %q, want %q", got, guide)
	}
	if _, err := CreateUniverse(nil, Display{}, home); err != nil {
		t.Fatal(err)
	}
	if got := read(); got != guide {
		t.Fatalf("installing again without a first guide dropped it: %q", got)
	}
}
