// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
	api "github.com/katyoti/katyoti-fwk/internal/extensions/compose/api"
)

// TestCreateSkill_OverwriteTarget: target=overwrite writes into the workspace
// overlay (<home>/overwrites/<sidecar>/embed/skills/), NOT the repo, and needs
// no go.mod/app anchoring.
func TestCreateSkill_OverwriteTarget(t *testing.T) {
	s, _ := fixtureService(t)
	home := t.TempDir()
	s.homeOverride = home // gate-off note path; resolution doesn't read the gate

	in := apiSkill("ktw-target", "live-skill")
	in.Target = "overwrite"
	out, err := s.CreateSkill(context.Background(), in)
	if err != nil {
		t.Fatalf("CreateSkill overwrite: %v", err)
	}

	wantDir := filepath.Join(cenv.OverwritesDir(home, "ktw-target"), "embed", "skills")
	if filepath.Dir(out.Path) != wantDir {
		t.Fatalf("overwrite target wrote to %s, want under %s", out.Path, wantDir)
	}
	if _, err := os.Stat(out.Path); err != nil {
		t.Fatalf("overlay file not written: %v", err)
	}
	if !strings.Contains(out.Note, "promote") {
		t.Errorf("overwrite note must mention promote; got %q", out.Note)
	}
}

// TestCreateSkill_InvalidTarget rejects an unknown target.
func TestCreateSkill_InvalidTarget(t *testing.T) {
	s, _ := fixtureService(t)
	in := apiSkill("ktw-target", "x")
	in.Target = "bogus"
	if _, err := s.CreateSkill(context.Background(), in); err == nil || !strings.Contains(err.Error(), "invalid target") {
		t.Fatalf("expected invalid-target error, got %v", err)
	}
}

// TestPromote_OverlayToSource: promote copies the overlay file into the repo
// source tree and removes the overlay (overlay-wins → must be deleted).
func TestPromote_OverlayToSource(t *testing.T) {
	s, root := fixtureService(t)
	home := t.TempDir()
	s.homeOverride = home

	// Seed an overlay skill (as create --target overwrite would).
	in := apiSkill("ktw-target", "promote-me")
	in.Target = "overwrite"
	created, err := s.CreateSkill(context.Background(), in)
	if err != nil {
		t.Fatalf("seed overlay: %v", err)
	}

	out, err := s.Promote(context.Background(), api.PromoteInput{Sidecar: "ktw-target", Path: "skills/promote-me.md"})
	if err != nil {
		t.Fatalf("Promote: %v", err)
	}

	// Source written under the repo.
	wantSrc := filepath.Join(root, "app", "ktw-target", "embed", "skills", "promote-me.md")
	if out.Source != wantSrc {
		t.Fatalf("promoted source = %s, want %s", out.Source, wantSrc)
	}
	if _, err := os.Stat(wantSrc); err != nil {
		t.Fatalf("source not written: %v", err)
	}
	// Overlay removed.
	if _, err := os.Stat(created.Path); !os.IsNotExist(err) {
		t.Fatalf("overlay must be removed after promote; stat err=%v", err)
	}
	if !strings.Contains(out.Note, "PR") {
		t.Errorf("promote note should mention opening a PR; got %q", out.Note)
	}
}

// TestPromote_MissingOverlay errors clearly when there's nothing to promote.
func TestPromote_MissingOverlay(t *testing.T) {
	s, _ := fixtureService(t)
	s.homeOverride = t.TempDir()
	if _, err := s.Promote(context.Background(), api.PromoteInput{Sidecar: "ktw-target", Path: "skills/nope.md"}); err == nil ||
		!strings.Contains(err.Error(), "no overlay artifact") {
		t.Fatalf("expected missing-overlay error, got %v", err)
	}
}
