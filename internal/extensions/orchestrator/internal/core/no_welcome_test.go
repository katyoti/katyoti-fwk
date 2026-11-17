// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/api"
)

// : a universe created with its welcome screens off writes
// no_welcome in Univers.yaml, its summary says so (what the GUI reads), and an
// edit keeps it; a universe created without it carries nothing - its welcome
// screens open as usual.
func TestAUniverseCanTurnItsWelcomeScreensOff(t *testing.T) {
	tempDir := t.TempDir()
	s := withRegistryAt(t, filepath.Join(tempDir, "universes.yaml"))
	ctx := context.Background()
	create := func(name string, off bool) {
		t.Helper()
		if _, err := s.CreateUniverse(ctx, api.CreateUniverseInput{Name: name, DisplayName: name,
			HomePath: filepath.Join(tempDir, "homes", name), NoWelcome: off}); err != nil {
			t.Fatal(err)
		}
	}
	file := func(name string) string {
		t.Helper()
		raw, err := os.ReadFile(filepath.Join(tempDir, "homes", name, ".katyoti", "config", "Univers.yaml"))
		if err != nil {
			t.Fatal(err)
		}
		return string(raw)
	}

	create("scn-case", true)
	if !strings.Contains(file("scn-case"), "no_welcome: true") {
		t.Fatalf("Univers.yaml of a universe created without welcome screens:\n%s", file("scn-case"))
	}
	info, err := s.GetUniverse(ctx, api.GetUniverseInput{Name: "scn-case"})
	if err != nil || !info.Summary.NoWelcome {
		t.Fatalf("its summary does not say the welcome screens are off: %+v, %v", info.Summary, err)
	}
	if _, err := s.UpdateUniverse(ctx, api.UpdateUniverseInput{Name: "scn-case", DisplayName: "Scenario case"}); err != nil {
		t.Fatal(err)
	}
	if info, _ := s.GetUniverse(ctx, api.GetUniverseInput{Name: "scn-case"}); !info.Summary.NoWelcome {
		t.Fatal("an edit turned the welcome screens back on")
	}

	create("everyday", false)
	if strings.Contains(file("everyday"), "no_welcome") {
		t.Fatalf("a universe created as usual carries the setting:\n%s", file("everyday"))
	}
	if info, _ := s.GetUniverse(ctx, api.GetUniverseInput{Name: "everyday"}); info.Summary.NoWelcome {
		t.Fatal("a universe created as usual reads its welcome screens off")
	}
}
