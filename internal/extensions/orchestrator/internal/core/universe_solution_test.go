// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/catalog"
)

// A universe made before solutions (its Univers.yaml names none) is given an
// installed solution - one a universe of the machine was installed from -
// every other field kept; refused for a solution or a version not installed,
// for a name installed in several versions, and for a universe that has one
func TestSetUniverseSolution(t *testing.T) {
	tempDir := t.TempDir()
	s := withRegistryAt(t, filepath.Join(tempDir, "universes.yaml"))
	ctx := context.Background()
	homeOf := func(name string) string { return filepath.Join(tempDir, "homes", name) }
	create := func(name, solution string) {
		t.Helper()
		if _, err := s.CreateUniverse(ctx, api.CreateUniverseInput{Name: name, DisplayName: strings.ToUpper(name),
			HomePath: homeOf(name), Description: "made by hand", Tags: []string{"archifactory"}}); err != nil {
			t.Fatal(err)
		}
		if solution == "" {
			return
		}
		manifest := filepath.Join(application.EnvOf(homeOf(name)).ConfigDir(), "Univers.yaml")
		body, err := os.ReadFile(manifest)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(manifest, append(body, []byte("solution: "+solution+"\n")...), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	set := func(name, solution string) (api.UniverseInfo, error) {
		return s.SetUniverseSolution(ctx, api.SetUniverseSolutionInput{Name: name, Solution: solution})
	}
	refused := func(name, solution, says string) {
		t.Helper()
		before, err := catalog.LoadManifest(homeOf(name))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := set(name, solution); err == nil || !strings.Contains(err.Error(), says) {
			t.Fatalf("set %s on %s: err %v; want a refusal saying %q", solution, name, err, says)
		}
		if m, err := catalog.LoadManifest(homeOf(name)); err != nil || m.Solution != before.Solution {
			t.Fatalf("a refusal changed Univers.yaml's solution: %+v, %v; was %q", m, err, before.Solution)
		}
	}

	create("archi", "archifactory@v2026.10-rc6") // installed from the solution
	create("acme", "")                           // made by hand, before solutions

	refused("acme", "weather", "not installed")
	refused("acme", "archifactory@v2026.09", "not installed")
	refused("acme", "", "name the solution")
	refused("archi", "archifactory", "already has the solution")

	info, err := set("acme", "archifactory")
	if err != nil {
		t.Fatal(err)
	}
	if info.Summary.Solution != "archifactory@v2026.10-rc6" {
		t.Fatalf("answered solution %q; want the installed version adopted", info.Summary.Solution)
	}
	m, err := catalog.LoadManifest(homeOf("acme"))
	if err != nil {
		t.Fatal(err)
	}
	if m.Solution != "archifactory@v2026.10-rc6" || m.DisplayName != "ACME" || m.Description != "made by hand" ||
		len(m.Tags) != 1 || m.Tags[0] != "archifactory" {
		t.Fatalf("Univers.yaml after the set = %+v; want the solution written and the other fields kept", m)
	}
	refused("acme", "archifactory", "already has the solution")

	// Two versions installed: a name alone is refused, a version named is taken.
	create("archi-next", "archifactory@v2026.11-rc1")
	create("lab", "")
	refused("lab", "archifactory", "several versions")
	if info, err := set("lab", "archifactory@v2026.11-rc1"); err != nil || info.Summary.Solution != "archifactory@v2026.11-rc1" {
		t.Fatalf("set a named version: %+v, %v", info.Summary, err)
	}
}
