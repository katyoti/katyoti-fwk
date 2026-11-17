// SPDX-License-Identifier: Apache-2.0

package main

import (
	"path/filepath"
	"testing"
)

// TestOnlyAProjectFolderIsOpened: the panel's « Ouvrir le dossier » opens one
// of the universe's project folders and nothing else.
func TestOnlyAProjectFolderIsOpened(t *testing.T) {
	a := filepath.Join(t.TempDir(), "alpha")
	b := filepath.Join(t.TempDir(), "beta")
	roots := []string{a, b}

	if got, ok := projectRootMatch("", roots); !ok || got != a {
		t.Fatalf("no folder named: want the first project %q, got %q (%v)", a, got, ok)
	}
	if got, ok := projectRootMatch(b+string(filepath.Separator), roots); !ok || got != b {
		t.Fatalf("a named project: want %q, got %q (%v)", b, got, ok)
	}
	for _, other := range []string{filepath.Join(a, ".."), filepath.Join(a, "sub"), filepath.Join(t.TempDir(), "elsewhere")} {
		if got, ok := projectRootMatch(other, roots); ok {
			t.Errorf("%q is no project of the universe, yet %q would be opened", other, got)
		}
	}
	if _, ok := projectRootMatch("", nil); ok {
		t.Error("a universe without a folder opened something")
	}
}
