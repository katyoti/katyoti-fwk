// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/catalog"
)

// UpdateUniverse + AddExistingUniverse were ported from ktw-gui's vendored
// orchestrator/write.go so the GUI can drop its direct writers and reach these as
// KTSP client calls (gui-pure-ktsp-client). The universe owner is the sole writer.

// UpdateUniverse edits a universe's mutable fields. The slug Name is immutable.
func (s *Service) UpdateUniverse(ctx context.Context, in api.UpdateUniverseInput) (api.UniverseInfo, error) {
	if !validUniverseName(in.Name) {
		return api.UniverseInfo{}, fmt.Errorf("invalid universe name %q", in.Name)
	}
	if strings.TrimSpace(in.DisplayName) == "" {
		return api.UniverseInfo{}, fmt.Errorf("display name is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	entry, err := s.registry.Find(in.Name)
	if err != nil {
		return api.UniverseInfo{}, err
	}
	// Self-heal a pre-2026-05-15 manifest that omitted `name` rather than refusing.
	old, err := catalog.LoadManifestOrRepair(entry.HomePath, entry.Name)
	if err != nil {
		return api.UniverseInfo{}, fmt.Errorf("read existing Univers.yaml: %w", err)
	}

	sidecars := mergeSidecarSelection(old.Sidecars, in.Sidecars)
	manifest := &catalog.Manifest{
		Version:     old.Version, // preserve
		Name:        in.Name,     // immutable
		DisplayName: strings.TrimSpace(in.DisplayName),
		Description: strings.TrimSpace(in.Description),
		Icon:        firstNonEmpty(in.Icon, old.Icon),
		Color:       firstNonEmpty(in.Color, old.Color),
		Sidecars:    sidecars,
		GUI:         old.GUI, // the editor does not show it: kept
		Tags:        old.Tags,
		FirstGuide:  old.FirstGuide,
		Solution:    old.Solution,
		NoWelcome:   old.NoWelcome,
	}
	if err := catalog.SaveManifest(entry.HomePath, manifest); err != nil {
		return api.UniverseInfo{}, err
	}
	notifyUniversesChanged(ctx, "updated", manifest.Name)
	return manifestInfo(manifest, entry.HomePath, entry.Protected), nil
}

// mergeSidecarSelection turns the editor's selection into the manifest's
// membership WITHOUT rewriting members that were already there.
//
// A selection is a name plus, at best, a binary path the inventory happened to
// know. Rebuilding every member from that lost whatever the selection cannot
// carry: a content member's host + folder + env, and the version-pinned store
// path of any member the inventory could not see (on a machine built from the
// channel, the scan saw nothing but the control plane) — which then fell back
// to the BARE NAME and a hub that could not exec it.
//
// So: a name already in the manifest keeps its spec verbatim (repointing a
// member is `universe wire`'s job, not the editor's); a new name gets a fresh
// spec from its selected binary; a name no longer selected is dropped.
func mergeSidecarSelection(old []api.SidecarSpec, sel []api.CreateSidecarSelection) []api.SidecarSpec {
	existing := make(map[string]api.SidecarSpec, len(old))
	for _, sc := range old {
		existing[sc.Name] = sc
	}
	out := make([]api.SidecarSpec, 0, len(sel))
	for _, s := range sel {
		if sc, ok := existing[s.Name]; ok {
			out = append(out, sc)
			continue
		}
		out = append(out, api.SidecarSpec{
			Name:      s.Name,
			Transport: "nats", // canonical bus-split convention (DO.3)
			Binary:    firstNonEmpty(s.BinaryPath, s.Name),
			Args:      []string{"sc", "ktsp", "nats"},
			ConfigDir: filepath.Join("sidecars", s.Name),
		})
	}
	return out
}

// AddExistingUniverse registers a pre-initialized universe folder (already carrying
// .katyoti/config/Univers.yaml) into the registry. No filesystem mutation.
// Idempotent when the name+path already match; conflicts on a different path.
func (s *Service) AddExistingUniverse(ctx context.Context, in api.AddExistingUniverseInput) (api.UniverseInfo, error) {
	path := filepath.Clean(in.Path)
	info, err := os.Stat(path)
	if err != nil {
		return api.UniverseInfo{}, fmt.Errorf("universe path %q: %w", path, err)
	}
	if !info.IsDir() {
		return api.UniverseInfo{}, fmt.Errorf("universe path %q is not a directory", path)
	}
	manifest, err := catalog.LoadManifest(path)
	if err != nil {
		return api.UniverseInfo{}, fmt.Errorf(
			"%s is not a universe — no .katyoti/config/Univers.yaml found (%v)", path, err)
	}
	if manifest.Name == "" {
		return api.UniverseInfo{}, fmt.Errorf("Univers.yaml at %s is missing the 'name' field", path)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if existing, err := s.registry.Find(manifest.Name); err == nil {
		if existing.HomePath != path {
			return api.UniverseInfo{}, fmt.Errorf(
				"universe %q is already registered at %s — pick a different folder or rename it in Univers.yaml",
				manifest.Name, existing.HomePath)
		}
		return manifestInfo(manifest, path, existing.Protected), nil // idempotent
	}
	if err := s.registry.Add(catalog.RegistryEntry{Name: manifest.Name, HomePath: path}); err != nil {
		return api.UniverseInfo{}, err
	}
	notifyUniversesChanged(ctx, "attached", manifest.Name)
	return manifestInfo(manifest, path, false), nil
}

// manifestInfo builds a UniverseInfo from a manifest + registry facts.
func manifestInfo(m *catalog.Manifest, homePath string, protected bool) api.UniverseInfo {
	return api.UniverseInfo{
		Summary: api.UniverseSummary{
			Version:     m.Version,
			Name:        m.Name,
			DisplayName: m.DisplayName,
			Description: m.Description,
			Icon:        m.Icon,
			Color:       m.Color,
			HomePath:    homePath,
			Protected:   protected,
		},
		Sidecars: m.Sidecars,
	}
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
