// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/api"
)

// Post-cutover (2026-05-15): Univers.yaml stays canonical, lives at
// `<workspaceHome>/.katyoti/config/Univers.yaml`, and carries BOTH universe
// identity (name / displayName / icon / color / sidecars) AND sidecar
// membership — there is no separate extensions.yaml file.

func TestManifest_SaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	want := &Manifest{
		Name:        "acme",
		DisplayName: "Acme",
		Description: "Primary client",
		Icon:        "R",
		Color:       "violet",
		Sidecars: []api.SidecarSpec{
			{
				Name:      "ktw-example",
				Transport: "stdio",
				Binary:    "ktw-example",
				Args:      []string{"sc", "ktsp", "stdio"},
				ConfigDir: "sidecars/ktw-example",
			},
			{
				Name:       "ktw-display",
				Transport:  "local-http",
				Binary:     "bun",
				Args:       []string{"run", "${KTW_DISPLAY_BIN}/src/index.ts"},
				HealthzURL: "http://127.0.0.1:${ALLOC_PORT}/healthz",
				Env: map[string]string{
					"KTW_DISPLAY_PORT": "${ALLOC_PORT}",
				},
				ConfigDir: "sidecars/ktw-display",
			},
		},
	}
	if err := SaveManifest(dir, want); err != nil {
		t.Fatalf("SaveManifest: %v", err)
	}
	got, err := LoadManifest(dir)
	if err != nil {
		t.Fatalf("LoadManifest: %v", err)
	}
	if got.Name != want.Name || got.DisplayName != want.DisplayName || got.Color != want.Color {
		t.Fatalf("metadata mismatch: got %+v", got)
	}
	if len(got.Sidecars) != 2 {
		t.Fatalf("expected 2 sidecars, got %d", len(got.Sidecars))
	}
	if got.Sidecars[0].Name != "ktw-example" || got.Sidecars[0].Transport != "stdio" {
		t.Fatalf("sidecar[0]: got %+v", got.Sidecars[0])
	}
	if got.Sidecars[1].HealthzURL != want.Sidecars[1].HealthzURL {
		t.Fatalf("sidecar[1].HealthzURL: got %q want %q", got.Sidecars[1].HealthzURL, want.Sidecars[1].HealthzURL)
	}
}

func TestManifest_LoadMissingFile(t *testing.T) {
	dir := t.TempDir()
	if _, err := LoadManifest(dir); err == nil {
		t.Fatalf("LoadManifest: expected error for missing Univers.yaml, got nil")
	}
}

func TestManifest_SaveRequiresName(t *testing.T) {
	dir := t.TempDir()
	if err := SaveManifest(dir, &Manifest{}); err == nil {
		t.Fatalf("SaveManifest: expected error for empty Name")
	}
}

// TestManifest_VersionRoundTrip verifies the numeric version field per
// rule.
func TestManifest_VersionRoundTrip(t *testing.T) {
	cases := []struct {
		name string
		ver  int
	}{
		{"v1", 1},
		{"v2", 2},
		{"v10", 10},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			want := &Manifest{Version: c.ver, Name: "test"}
			if err := SaveManifest(dir, want); err != nil {
				t.Fatalf("SaveManifest: %v", err)
			}
			got, err := LoadManifest(dir)
			if err != nil {
				t.Fatalf("LoadManifest: %v", err)
			}
			if got.Version != c.ver {
				t.Errorf("Version round-trip: got %d, want %d", got.Version, c.ver)
			}
		})
	}
}
