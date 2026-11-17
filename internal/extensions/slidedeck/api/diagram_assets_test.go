// SPDX-License-Identifier: Apache-2.0

package api

import (
	"reflect"
	"testing"
)

// A draw.io diagram beside the deck is one of its files: published with the
// others, and named by the lint when it is missing.
func TestDrawioDiagramIsAFileBesideTheDeck(t *testing.T) {
	src := "---\ntitle: T\n---\n\n## Architecture\n![Context](diagrams/context.drawio)\n"
	if got := LocalAssets(src); !reflect.DeepEqual(got, []string{"diagrams/context.drawio"}) {
		t.Fatalf("LocalAssets = %v", got)
	}
	fs := LintAssets(src, func(string) bool { return false })
	if len(fs) != 1 || fs[0].Rule != "asset-missing" {
		t.Fatalf("LintAssets = %+v", fs)
	}
}

// A deck object names a file of its module from the root — a dossier's
// diagram where it lives — listed under _module/; never above the module.
func TestModulePathsNameAFileOfTheModule(t *testing.T) {
	for target, want := range map[string]string{
		"module:/dossiers/d1/travail/flux.drawio": "_module/dossiers/d1/travail/flux.drawio",
		"module:dossiers/d1/./a.png?v=2":          "_module/dossiers/d1/a.png",
		"module:/dossiers/../../etc/passwd":       "",
		"module:/":                                "",
	} {
		got, ok := LocalAsset(target)
		if !ok || got != want {
			t.Errorf("LocalAsset(%q) = %q, %v; want %q", target, got, ok, want)
		}
	}
	src := "---\ntitle: T\n---\n\n## Flux\n![Flux](module:/dossiers/d1/travail/flux.drawio)\n"
	if got := LocalAssets(src); !reflect.DeepEqual(got, []string{"_module/dossiers/d1/travail/flux.drawio"}) {
		t.Fatalf("LocalAssets = %v", got)
	}
}
