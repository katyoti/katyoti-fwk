// SPDX-License-Identifier: Apache-2.0

package wire

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
)

// A universe a solution install creates is a whole universe: beside
// Univers.yaml, its .env and Settings.yaml — without the latter ktw-agents
// refuses to serve it (« not configured yet ») and the GUI cannot read a single
// object. A Settings.yaml already there is kept.
func TestCreateUniverseWritesTheWholeSkeleton(t *testing.T) {
	home := filepath.Join(t.TempDir(), "demo")
	if _, err := CreateUniverse(nil, Display{DisplayName: "Démonstration"}, home); err != nil {
		t.Fatal(err)
	}
	env := application.EnvOf(home)
	for _, p := range []string{
		filepath.Join(env.ConfigDir(), "Univers.yaml"),
		filepath.Join(env.ConfigDir(), "Settings.yaml"),
		filepath.Join(env.HomeDir(), ".env"),
	} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("%s: %v", filepath.Base(p), err)
		}
	}
	settings := filepath.Join(env.ConfigDir(), "Settings.yaml")
	if err := os.WriteFile(settings, []byte("max_turns: 7\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := CreateUniverse(nil, Display{}, home); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(settings); string(b) != "max_turns: 7\n" {
		t.Fatalf("installing again replaced the universe's Settings.yaml: %q", b)
	}
}
