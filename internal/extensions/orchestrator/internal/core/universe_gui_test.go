// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
	"github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/api"
)

// The hub's universe summary carries the gui section of Univers.yaml (what the
// GUI hides, which task hashtags come first), the one the GUI reads; editing
// the universe keeps it.
func TestUniverseGUIIsCarriedByTheSummary(t *testing.T) {
	tempDir := t.TempDir()
	s := withRegistryAt(t, filepath.Join(tempDir, "universes.yaml"))
	home := filepath.Join(tempDir, "homes", "archi")
	ctx := context.Background()
	if _, err := s.CreateUniverse(ctx, api.CreateUniverseInput{Name: "archi", DisplayName: "Archi", HomePath: home}); err != nil {
		t.Fatal(err)
	}
	// The person (or the solution that installed it) declares the section.
	manifest := filepath.Join(application.EnvOf(home).ConfigDir(), "Univers.yaml")
	body, err := os.ReadFile(manifest)
	if err != nil {
		t.Fatal(err)
	}
	body = append(body, []byte("gui:\n  hide: [panel:planning-deck, runs]\n  first: [archifactory]\n")...)
	if err := os.WriteFile(manifest, body, 0o644); err != nil {
		t.Fatal(err)
	}
	want := &cenv.UniverseGUI{Hide: []string{"panel:planning-deck", "runs"}, First: []string{"archifactory"}}

	out, err := s.ListUniverses(ctx, api.ListUniversesInput{})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Universes) != 1 || !reflect.DeepEqual(out.Universes[0].GUI, want) {
		t.Fatalf("the summary does not carry the gui section: %+v", out.Universes)
	}
	info, err := s.GetUniverse(ctx, api.GetUniverseInput{Name: "archi"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(info.Summary.GUI, want) {
		t.Fatalf("get-universe does not carry the gui section: %+v", info.Summary.GUI)
	}

	if _, err := s.UpdateUniverse(ctx, api.UpdateUniverseInput{Name: "archi", DisplayName: "Architecture"}); err != nil {
		t.Fatal(err)
	}
	info, err = s.GetUniverse(ctx, api.GetUniverseInput{Name: "archi"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(info.Summary.GUI, want) {
		t.Fatalf("editing the universe lost its gui section: %+v", info.Summary.GUI)
	}
}
