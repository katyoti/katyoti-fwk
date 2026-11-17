// SPDX-License-Identifier: Apache-2.0

package binaries

import (
	"os"
	"path/filepath"
	"testing"
)

// The external folders join the judged universe: their sidecars are found
// by the same filter as bin/'s, a name bin/ already holds stays bin/'s, and
// a file that is no sidecar is left out. Nothing is run.
func TestExternalBinaries_AddsAnotherFoldersSidecarsOnly(t *testing.T) {
	dir := t.TempDir()
	for _, f := range []string{"ktw-example" + exeSuffix, "ktw-agents" + exeSuffix, "ktw-gui" + exeSuffix, "notes.txt", "other" + exeSuffix} {
		if err := os.WriteFile(filepath.Join(dir, f), nil, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv(externalBinariesEnv, dir)

	got := externalBinaries(t, map[string]string{"ktw-agents": "bin/ktw-agents"})
	if len(got) != 1 || got["ktw-example"] != filepath.Join(dir, "ktw-example"+exeSuffix) {
		t.Fatalf("external = %v, want only ktw-example from %s", got, dir)
	}

	t.Setenv(externalBinariesEnv, "")
	if got := externalBinaries(t, nil); len(got) != 0 {
		t.Errorf("no folder named, yet: %v", got)
	}
}
