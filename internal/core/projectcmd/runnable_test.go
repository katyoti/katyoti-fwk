// SPDX-License-Identifier: Apache-2.0

package projectcmd

import (
	"os"
	"path/filepath"
	"testing"
)

// A check runs a program, a project script or a shell statement; a document
// path, a card name, a sentence or a word that cannot fail is refused before
// anything runs.
func TestRunnableRefusesWhatIsNoCommand(t *testing.T) {
	dir := t.TempDir()
	doc := filepath.Join(dir, "manual-tests.md")
	script := filepath.Join(dir, "probe.sh")
	spaced := filepath.Join(dir, "my tools", "go.exe")
	bare := filepath.Join(dir, "bin", "probe.exe")
	for _, d := range []string{filepath.Dir(spaced), filepath.Dir(bare)} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, f := range []string{doc, script, spaced, bare} {
		if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for cmd, ok := range map[string]bool{
		"go test ./internal/... -run X":            true,
		"if ($true) { exit 0 }":                    true,
		"cd app/ktw-gui/frontend; bun test":        true,
		"$n=1; exit 0":                             true,
		"exit 1":                                   true,
		script + " --fast":                         true,
		`"` + spaced + `" test ./...`:              true,
		filepath.Join(dir, "bin", "probe") + " -x": true,
		doc + " §6, §10":                           false,
		"ms-demo-install/dm03":                     false,
		"the GUI shows the request in its new tab": false,
		"echo the GUI shows it":                    false,
		"true":                                     false,
		"":                                         false,
	} {
		if err := Runnable(cmd); (err == nil) != ok {
			t.Errorf("Runnable(%q) = %v; want runnable %v", cmd, err, ok)
		}
	}
}
