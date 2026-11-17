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

// The universe summary carries the universe's tags and the per-universe level
// adjustments of its gui section; editing the universe keeps both.
func TestUniverseTagsAreCarriedByTheSummaryAndKept(t *testing.T) {
	tempDir := t.TempDir()
	s := withRegistryAt(t, filepath.Join(tempDir, "universes.yaml"))
	home := filepath.Join(tempDir, "homes", "archi")
	ctx := context.Background()
	if _, err := s.CreateUniverse(ctx, api.CreateUniverseInput{Name: "archi", DisplayName: "Archi", HomePath: home}); err != nil {
		t.Fatal(err)
	}
	manifest := filepath.Join(application.EnvOf(home).ConfigDir(), "Univers.yaml")
	body, err := os.ReadFile(manifest)
	if err != nil {
		t.Fatal(err)
	}
	body = append(body, []byte("tags: [archifactory]\ngui:\n  levels: {drawio: expert}\n")...)
	if err := os.WriteFile(manifest, body, 0o644); err != nil {
		t.Fatal(err)
	}
	wantTags := []string{"archifactory"}
	wantGUI := &cenv.UniverseGUI{Levels: map[string]string{"drawio": "expert"}}

	check := func(when string) {
		t.Helper()
		info, err := s.GetUniverse(ctx, api.GetUniverseInput{Name: "archi"})
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(info.Summary.Tags, wantTags) || !reflect.DeepEqual(info.Summary.GUI, wantGUI) {
			t.Fatalf("%s: tags %v gui %+v; want %v and %+v", when, info.Summary.Tags, info.Summary.GUI, wantTags, wantGUI)
		}
	}
	check("read")
	if _, err := s.UpdateUniverse(ctx, api.UpdateUniverseInput{Name: "archi", DisplayName: "Architecture"}); err != nil {
		t.Fatal(err)
	}
	check("after an edit")
}
