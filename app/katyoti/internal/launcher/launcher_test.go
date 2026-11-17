// SPDX-License-Identifier: Apache-2.0

package launcher

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/app/katyoti/internal/startup"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/catalog"
)

// TestResolveGUI_FromStore: a recorded ktw-gui in sidecars.yaml resolves.
func TestResolveGUI_FromStore(t *testing.T) {
	root := t.TempDir()
	t.Setenv("KTW_KATYOTI_ROOT", root)
	application.ResetGlobals()
	t.Cleanup(application.ResetGlobals)

	// a real file to stand in for the ktw-gui binary
	fake := filepath.Join(root, "ktw-gui-fake"+exeSuffix())
	if err := os.WriteFile(fake, []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	// record it in the global sidecars.yaml
	sy := "version: 1\nsidecars:\n  - name: ktw-gui\n    binary_path: " + filepath.ToSlash(fake) + "\n"
	if err := os.WriteFile(filepath.Join(root, "sidecars.yaml"), []byte(sy), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := ResolveGUI()
	if err != nil {
		t.Fatalf("ResolveGUI: %v", err)
	}
	// compare slash-normalized: the recorded path is stored slash-forward
	if filepath.ToSlash(got) != filepath.ToSlash(fake) {
		t.Fatalf("ResolveGUI = %q, want the recorded store path %q", got, fake)
	}
}

// TestResolveGUI_NotFound: no record, empty PATH, no ktw-gui beside the test
// binary → an actionable error.
func TestResolveGUI_NotFound(t *testing.T) {
	root := t.TempDir()
	t.Setenv("KTW_KATYOTI_ROOT", root)
	t.Setenv("PATH", t.TempDir()) // an empty dir so LookPath finds nothing
	application.ResetGlobals()
	t.Cleanup(application.ResetGlobals)

	if got, err := ResolveGUI(); err == nil {
		t.Fatalf("expected not-found error, got %q", got)
	}
}

// TestResolveLaunchUniverse: the explicit flag wins; an empty/blank flag falls
// back to the startup config's default_universe.
func TestResolveLaunchUniverse(t *testing.T) {
	root := t.TempDir()
	t.Setenv("KTW_KATYOTI_ROOT", root)
	application.ResetGlobals()
	t.Cleanup(application.ResetGlobals)

	if _, err := startup.Set("default_universe", "from-config"); err != nil {
		t.Fatalf("seed default_universe: %v", err)
	}

	if got := resolveLaunchUniverse("from-flag"); got != "from-flag" {
		t.Errorf("flag should win: got %q, want from-flag", got)
	}
	if got := resolveLaunchUniverse(""); got != "from-config" {
		t.Errorf("empty flag → config default: got %q, want from-config", got)
	}
	if got := resolveLaunchUniverse("   "); got != "from-config" {
		t.Errorf("blank flag → config default: got %q, want from-config", got)
	}
}

// TestUniverseArgs: a registered name yields the pass-through flag; an
// unregistered name is dropped with a warning; empty yields no args.
func TestUniverseArgs(t *testing.T) {
	root := t.TempDir()
	t.Setenv("KTW_KATYOTI_ROOT", root)
	application.ResetGlobals()
	t.Cleanup(application.ResetGlobals)

	regPath, err := catalog.DefaultRegistryPath()
	if err != nil {
		t.Fatal(err)
	}
	reg, err := catalog.LoadRegistry(regPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := reg.Add(catalog.RegistryEntry{Name: "known", HomePath: filepath.Join(root, "known")}); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if got := universeArgs(&buf, ""); got != nil {
		t.Errorf("empty name → nil args, got %v", got)
	}
	if got := universeArgs(&buf, "known"); len(got) != 2 || got[0] != "--universe" || got[1] != "known" {
		t.Errorf("registered → [--universe known], got %v", got)
	}

	buf.Reset()
	if got := universeArgs(&buf, "ghost"); got != nil {
		t.Errorf("unregistered → nil args, got %v", got)
	}
	if !strings.Contains(buf.String(), "ghost") {
		t.Errorf("expected a warning mentioning the unknown universe, got %q", buf.String())
	}
}

// `katyoti gui --open <address>` opens the universe the address names
// - an address, or the katyoti:// link a click hands over.
func TestAnAddressNamesTheUniverseToOpen(t *testing.T) {
	for addr, want := range map[string]string{
		"dev/object/demo.F-023/links":             "dev",
		"katyoti://open/archi/screen/marketplace": "archi",
		"": "",
	} {
		if got := universeOfAddress(addr); got != want {
			t.Errorf("universeOfAddress(%q) = %q; want %q", addr, got, want)
		}
	}
}
