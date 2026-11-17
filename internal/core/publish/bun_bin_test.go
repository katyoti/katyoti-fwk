// SPDX-License-Identifier: Apache-2.0

package publish

import (
	"path/filepath"
	"testing"
)

// bun add --global puts a package's commands in $BUN_INSTALL/bin, else in
// ~/.bun/bin: what the installer puts on PATH and the checks
// look in.
func TestBunGlobalBin_IsBunInstallsBinFolder(t *testing.T) {
	root := t.TempDir()
	t.Setenv("BUN_INSTALL", root)
	if got := BunGlobalBin(); got != filepath.Join(root, "bin") {
		t.Errorf("with BUN_INSTALL %s: %q, want its bin folder", root, got)
	}
	home := t.TempDir()
	t.Setenv("BUN_INSTALL", "")
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	if got := BunGlobalBin(); got != filepath.Join(home, ".bun", "bin") {
		t.Errorf("without BUN_INSTALL: %q, want %s", got, filepath.Join(home, ".bun", "bin"))
	}
}
