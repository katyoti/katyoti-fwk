// SPDX-License-Identifier: Apache-2.0

package sidecarkit

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
)

// writeHome lays out a .katyoti config root with a Univers.yaml and (optionally)
// an override file, returning the home path. Resets the manifest cache so each
// case re-reads its own manifest.
func writeHome(t *testing.T, allow bool, overrideRel, overrideBody string) string {
	t.Helper()
	cenv.ResetUniverseManifestCache()
	home := t.TempDir()
	cfgDir := filepath.Join(home, "config")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := "name: testverse\n"
	if allow {
		manifest += "allow_overwrites: true\n"
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "Univers.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if overrideRel != "" {
		full := filepath.Join(cenv.OverwritesDir(home, "ktw-compose"), overrideRel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(overrideBody), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(cenv.ResetUniverseManifestCache)
	return home
}

// embedBase fakes a sidecar's embedded tree (keys mirror embed/<type>/<name>).
func embedBase() loader.Loader {
	return loader.NewMapLoader(map[string]string{
		"embed/skills/greet.md": "---\nname: greet\n---\nEMBED body",
	})
}

// TestBuildOverlay_GateOffIgnoresOverride: default (no allow_overwrites) serves
// embed even when an override file exists (gate c — prod-safe default).
func TestBuildOverlay_GateOffIgnoresOverride(t *testing.T) {
	home := writeHome(t, false, "embed/skills/greet.md", "---\nname: greet\n---\nOVERRIDE")
	ldr := buildOverlay(embedBase(), home, "ktw-compose")

	got, _ := ldr.ReadFile("embed/skills/greet.md")
	if string(got) != "---\nname: greet\n---\nEMBED body" {
		t.Fatalf("gate off must serve embed; got %q", got)
	}
}

// TestBuildOverlay_GateOnShadows: with allow_overwrites and an override tree, the
// override shadows the embedded resource (gate a).
func TestBuildOverlay_GateOnShadows(t *testing.T) {
	home := writeHome(t, true, "embed/skills/greet.md", "---\nname: greet\n---\nOVERRIDE body")
	ldr := buildOverlay(embedBase(), home, "ktw-compose")

	got, _ := ldr.ReadFile("embed/skills/greet.md")
	if string(got) != "---\nname: greet\n---\nOVERRIDE body" {
		t.Fatalf("gate on must shadow embed; got %q", got)
	}
}

// TestBuildOverlay_GateOnNoTree: gate on but the sidecar has no override dir →
// embed only (no crash).
func TestBuildOverlay_GateOnNoTree(t *testing.T) {
	home := writeHome(t, true, "", "")
	ldr := buildOverlay(embedBase(), home, "ktw-compose")

	got, _ := ldr.ReadFile("embed/skills/greet.md")
	if string(got) != "---\nname: greet\n---\nEMBED body" {
		t.Fatalf("no override tree must serve embed; got %q", got)
	}
}

// TestBuildOverlay_MalformedFallsBack: gate on, but a malformed override (no
// frontmatter) falls back to the embedded copy (gate c robustness).
func TestBuildOverlay_MalformedFallsBack(t *testing.T) {
	home := writeHome(t, true, "embed/skills/greet.md", "garbage, no frontmatter")
	ldr := buildOverlay(embedBase(), home, "ktw-compose")

	got, _ := ldr.ReadFile("embed/skills/greet.md")
	if string(got) != "---\nname: greet\n---\nEMBED body" {
		t.Fatalf("malformed override must fall back to embed; got %q", got)
	}
}
