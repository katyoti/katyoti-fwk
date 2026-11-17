// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"
	"strings"

	mktapi "github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/internal/wire"
	orchcatalog "github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/catalog"
)

// RepinSolution moves the universes a solution installed to another of its
// versions: the version's pinned members installed (a version
// already placed is kept), then every universe of the solution - the main one
// and each beside it - rewired to exactly that version's members and recording
// `solution: <name>@<version>`. What an install applies once (config-sets,
// keys, the seed) is not applied again. It is the update path's repin, and its
// rollback with the previous version; a locked installation is not refused -
// installing a release is how its versions move.
func (s *service) RepinSolution(ctx context.Context, in mktapi.RepinSolutionInput) (mktapi.RepinSolutionOutput, error) {
	var out mktapi.RepinSolutionOutput
	if strings.TrimSpace(in.UniverseHome) == "" {
		return out, fmt.Errorf("repin-solution: name the solution's main universe (--universe)")
	}
	home := universeTarget(in.UniverseHome)
	m, err := orchcatalog.LoadManifest(home)
	if err != nil {
		return out, fmt.Errorf("repin-solution: %w", err)
	}
	name, from, ok := strings.Cut(m.Solution, "@")
	if !ok || name == "" {
		return out, fmt.Errorf("repin-solution: the universe at %s was not installed from a solution (its Univers.yaml names none)", home)
	}
	out.Solution, out.From = name, from
	it, _, _, err := s.findItem(ctx, name, "")
	if err != nil {
		return out, err
	}
	if it.Kind != mktapi.KindSolution {
		return out, fmt.Errorf("repin-solution: %q is a %s item, not a solution", name, it.Kind)
	}
	_, iv, err := pickVersion(it, in.Version)
	if err != nil {
		return out, err
	}
	if err := iv.ValidateUniverses(); err != nil {
		return out, fmt.Errorf("solution %s@%s: %w", name, iv.Version, err)
	}
	out.To = iv.Version
	out.Members = memberMoves(it, from, iv)
	universes := iv.SolutionUniverses() // the main one first
	for _, u := range universes {
		out.Universes = append(out.Universes, solutionUniverseHome(home, u))
	}
	if in.DryRun || out.To == out.From {
		return out, nil
	}

	installed := map[string]mktapi.Installed{}
	for _, mb := range iv.Includes {
		ins, err := s.Install(ctx, mktapi.ItemRef{Name: mb.Name, Version: mb.Version})
		if err != nil {
			return out, fmt.Errorf("solution %s@%s: install member %s@%s: %w", name, iv.Version, mb.Name, mb.Version, err)
		}
		installed[mb.Name] = ins
	}
	label := name + "@" + iv.Version
	for _, u := range universes {
		var members []mktapi.Installed
		for _, mb := range iv.UniverseMembers(u) {
			members = append(members, installed[mb.Name])
		}
		// The universe keeps its display name; what the version declares for
		// its GUI, tags and first guide is taken.
		disp := wire.Display{Icon: u.Icon, GUI: u.GUI, Tags: u.Tags, FirstGuide: u.FirstGuide, Solution: label}
		if _, err := wire.CreateUniverse(members, disp, solutionUniverseHome(home, u)); err != nil {
			return out, fmt.Errorf("solution %s: repin %s: %w", label, u.Name, err)
		}
	}
	return out, nil
}

// memberMoves lists each member whose version differs between the solution's
// version from (when the catalog still holds it) and iv.
func memberMoves(it mktapi.Item, from string, iv mktapi.ItemVersion) []mktapi.MemberMove {
	was := map[string]string{}
	if _, old, err := pickVersion(it, from); err == nil && from != "" {
		for _, mb := range old.Includes {
			was[mb.Name] = mb.Version
		}
	}
	var out []mktapi.MemberMove
	seen := map[string]bool{}
	for _, mb := range iv.Includes {
		seen[mb.Name] = true
		if was[mb.Name] != mb.Version {
			out = append(out, mktapi.MemberMove{Name: mb.Name, From: was[mb.Name], To: mb.Version})
		}
	}
	for n, v := range was {
		if !seen[n] {
			out = append(out, mktapi.MemberMove{Name: n, From: v})
		}
	}
	return out
}
