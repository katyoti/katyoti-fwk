// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
	"github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/catalog"
)

// SetUniverseSolution gives a universe made before solutions - one whose
// Univers.yaml names none - the solution it belongs to, so a
// project whose type needs that solution opens in it. Only an
// installed solution is taken: one whose install made a universe of this
// machine, or the installation itself. A name alone adopts that solution's
// installed version - refused when several are installed, the version to name.
// A universe that has a solution is refused: moving it to another version is
// the solution's update (repin), never this verb. Every other field of
// Univers.yaml is kept.
func (s *Service) SetUniverseSolution(ctx context.Context, in api.SetUniverseSolutionInput) (api.UniverseInfo, error) {
	want := strings.TrimSpace(in.Solution)
	name, version, _ := strings.Cut(want, "@")
	if name == "" {
		return api.UniverseInfo{}, fmt.Errorf("name the solution: <solution> or <solution>@<version>")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	entry, err := s.registry.Find(in.Name)
	if err != nil {
		return api.UniverseInfo{}, err
	}
	m, err := catalog.LoadManifestOrRepair(entry.HomePath, entry.Name)
	if err != nil {
		return api.UniverseInfo{}, fmt.Errorf("read Univers.yaml: %w", err)
	}
	if m.Solution != "" {
		return api.UniverseInfo{}, fmt.Errorf("universe %q already has the solution %s: move it to another version with the solution's update (katyoti solution update), not this verb", in.Name, m.Solution)
	}

	installed := s.installedSolutionsLocked()[name]
	label := ""
	switch {
	case len(installed) == 0:
		return api.UniverseInfo{}, fmt.Errorf("the solution %s is not installed on this machine: install it first (katyoti solution install %s)", name, name)
	case version != "":
		for _, v := range installed {
			if v == version {
				label = name + "@" + version
			}
		}
		if label == "" {
			return api.UniverseInfo{}, fmt.Errorf("the solution %s@%s is not installed on this machine; installed: %s", name, version, versionsOf(name, installed))
		}
	case len(installed) > 1:
		return api.UniverseInfo{}, fmt.Errorf("the solution %s is installed in several versions (%s): name one", name, versionsOf(name, installed))
	default:
		label = name + "@" + installed[0]
	}

	m.Solution = label
	if err := catalog.SaveManifest(entry.HomePath, m); err != nil {
		return api.UniverseInfo{}, err
	}
	notifyUniversesChanged(ctx, "updated", m.Name)
	return m.ToUniverseInfo(entry.Name, entry.HomePath, entry.Protected, nil), nil
}

// installedSolutionsLocked answers each installed solution's versions: those
// recorded by the universes of the registry (a solution's install writes
// `solution: <name>@<version>` into every universe it makes) and the
// installation's own. The caller holds s.mu.
func (s *Service) installedSolutionsLocked() map[string][]string {
	seen := map[string]map[string]bool{}
	add := func(label string) {
		n, v, ok := strings.Cut(strings.TrimSpace(label), "@")
		if !ok || n == "" || v == "" {
			return
		}
		if seen[n] == nil {
			seen[n] = map[string]bool{}
		}
		seen[n][v] = true
	}
	for _, e := range s.registry.List() {
		if m, err := catalog.LoadManifest(e.HomePath); err == nil {
			add(m.Solution)
		}
	}
	if inst := cenv.CurrentInstallation(); inst != nil {
		add(inst.Solution)
	}
	out := make(map[string][]string, len(seen))
	for n, vs := range seen {
		for v := range vs {
			out[n] = append(out[n], v)
		}
		sort.Strings(out[n])
	}
	return out
}

func versionsOf(name string, versions []string) string {
	labels := make([]string, len(versions))
	for i, v := range versions {
		labels[i] = name + "@" + v
	}
	return strings.Join(labels, ", ")
}
