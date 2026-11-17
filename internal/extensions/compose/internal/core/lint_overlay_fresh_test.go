// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
	api "github.com/katyoti/katyoti-fwk/internal/extensions/compose/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/methods/verify"
)

func hasRule(res verify.LintResult, rule string) bool {
	for _, f := range res.Findings {
		if f.Rule == rule {
			return true
		}
	}
	return false
}

// gateOnHome writes a .katyoti home with allow_overwrites: true and returns it.
func gateOnHome(t *testing.T) string {
	t.Helper()
	cenv.ResetUniverseManifestCache()
	t.Cleanup(cenv.ResetUniverseManifestCache)
	home := t.TempDir()
	cfg := filepath.Join(home, "config")
	if err := os.MkdirAll(cfg, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfg, "Univers.yaml"), []byte("name: tv\nallow_overwrites: true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return home
}

const srcBody = "---\nname: foo\ndescription: src\n---\nOLD source body\n"
const overlayBody = "---\nname: foo\ndescription: new\n---\nNEW overlay body\n"

// writeSource lays out app/<sidecar>/embed/skills/foo.md under a repo root.
func writeSource(t *testing.T, root, sidecar, body string) string {
	t.Helper()
	p := filepath.Join(root, "app", sidecar, "embed", "skills", "foo.md")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// TestLintSkill_StaleWhenNoOverlay: served diverges from source, no overlay →
// stale-embedded-skill (the genuine stale-build case still fires).
func TestLintSkill_StaleWhenNoOverlay(t *testing.T) {
	s, _ := fixtureService(t)
	s.homeOverride = t.TempDir() // gate off (no Univers.yaml)
	src := writeSource(t, t.TempDir(), "ktw-foo", srcBody)

	res, err := s.LintSkill(context.Background(), api.LintSkillInput{
		File: src, Skill: "ktw-foo:/skill/foo", Served: overlayBody,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !hasRule(res, "stale-embedded-skill") {
		t.Fatalf("expected stale-embedded-skill with no overlay; got %+v", res.Findings)
	}
}

// TestLintSkill_OverlayActiveNotStale: with the gate on and an overlay twin that
// matches the served body, the divergence from source is EXPECTED — no stale
// finding, an info served-from-overlay note instead.
func TestLintSkill_OverlayActiveNotStale(t *testing.T) {
	s, _ := fixtureService(t)
	home := gateOnHome(t)
	s.homeOverride = home

	src := writeSource(t, t.TempDir(), "ktw-foo", srcBody)
	// overlay twin mirrors embed/skills/foo.md and matches the served body.
	twin := filepath.Join(cenv.OverwritesDir(home, "ktw-foo"), "embed", "skills", "foo.md")
	if err := os.MkdirAll(filepath.Dir(twin), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(twin, []byte(overlayBody), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := s.LintSkill(context.Background(), api.LintSkillInput{
		File: src, Skill: "ktw-foo:/skill/foo", Served: overlayBody,
	})
	if err != nil {
		t.Fatal(err)
	}
	if hasRule(res, "stale-embedded-skill") {
		t.Fatalf("overlay-served body must NOT be flagged stale; got %+v", res.Findings)
	}
	if !hasRule(res, "served-from-overlay") {
		t.Fatalf("expected served-from-overlay info note; got %+v", res.Findings)
	}
	if res.Status == "fail" {
		t.Fatalf("overlay-served must not fail the lint; status=%s", res.Status)
	}
}
