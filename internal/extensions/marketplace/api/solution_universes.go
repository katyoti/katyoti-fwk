// SPDX-License-Identifier: Apache-2.0

package api

import (
	"encoding/json"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// A solution composes one or more universes, exactly one of them main
// (modules/katyoti/solutions.md §2). An older catalog carried one universe in
// `universe:`: it still loads, as the one main universe of `universes:`, and
// the version's configSets and seed are the main universe's, as they were.

func (v *ItemVersion) UnmarshalYAML(node *yaml.Node) error {
	type plain ItemVersion
	if err := node.Decode((*plain)(v)); err != nil {
		return err
	}
	v.liftUniverse()
	return nil
}

func (v *ItemVersion) UnmarshalJSON(b []byte) error {
	type plain ItemVersion
	if err := json.Unmarshal(b, (*plain)(v)); err != nil {
		return err
	}
	v.liftUniverse()
	return nil
}

// liftUniverse moves the older single `universe:` into Universes, as its main.
func (v *ItemVersion) liftUniverse() {
	if v.Universe == nil {
		return
	}
	if len(v.Universes) == 0 {
		u := *v.Universe
		u.Main = true
		v.Universes = []SolutionUniverse{u}
	}
	v.Universe = nil
}

// SolutionUniverses are the universes a solution version installs, the main
// one first, each with what it gets: the main universe the version's
// configSets (before its own) and seed (when it names none). A version that
// declares none is one main universe of every member.
func (v ItemVersion) SolutionUniverses() []SolutionUniverse {
	list := v.Universes
	if len(list) == 0 && v.Universe != nil {
		list = []SolutionUniverse{*v.Universe}
	}
	if len(list) == 0 {
		list = []SolutionUniverse{{}}
	}
	mainAt := 0
	for i, u := range list {
		if u.Main {
			mainAt = i
			break
		}
	}
	out := make([]SolutionUniverse, 0, len(list))
	for i, u := range list {
		u.Main = i == mainAt
		if u.Main {
			u.ConfigSets = append(append([]PinnedRef(nil), v.ConfigSets...), u.ConfigSets...)
			if u.Seed == nil {
				u.Seed = v.Seed
			}
		}
		if u.Main {
			out = append([]SolutionUniverse{u}, out...)
		} else {
			out = append(out, u)
		}
	}
	return out
}

// UniverseMembers are the pinned members a universe of this version gets: in
// a solution of one universe every member, else the members its sidecars name.
func (v ItemVersion) UniverseMembers(u SolutionUniverse) []PinnedRef {
	if len(v.SolutionUniverses()) == 1 || len(u.Sidecars) == 0 {
		return v.Includes
	}
	names := map[string]bool{}
	for _, sc := range u.Sidecars {
		names[sc.Name] = true
	}
	var out []PinnedRef
	for _, m := range v.Includes {
		if names[m.Name] {
			out = append(out, m)
		}
	}
	return out
}

// ValidateUniverses refuses a solution version whose universes cannot be
// installed: not exactly one main, a universe of several unnamed or named
// twice, a sidecar no member pins, a member no universe takes, two seeds
// for the main universe.
func (v ItemVersion) ValidateUniverses() error {
	list := v.Universes
	if len(list) == 0 && v.Universe != nil {
		list = []SolutionUniverse{*v.Universe}
	}
	if len(list) == 0 {
		return nil
	}
	mains := 0
	for _, u := range list {
		if u.Main {
			mains++
			if u.Seed != nil && v.Seed != nil {
				return fmt.Errorf("the main universe has two seeds (%s@%s and %s@%s): a universe serves one project",
					v.Seed.Name, v.Seed.Version, u.Seed.Name, u.Seed.Version)
			}
		}
	}
	if len(list) == 1 && mains <= 1 {
		return nil // one universe: the main one, marked or not
	}
	if mains != 1 {
		return fmt.Errorf("a solution has exactly one main universe, this one %d", mains)
	}
	pinned := map[string]bool{}
	for _, m := range v.Includes {
		pinned[m.Name] = true
	}
	taken := map[string]bool{}
	seen := map[string]bool{}
	for _, u := range list {
		name := strings.TrimSpace(u.Name)
		if name == "" {
			return fmt.Errorf("a solution of %d universes names each one", len(list))
		}
		if strings.ContainsAny(name, `/\:`) || name == "." || name == ".." {
			return fmt.Errorf("universe name %q: a name, not a path", name)
		}
		if seen[strings.ToLower(name)] {
			return fmt.Errorf("universe %q is named twice", name)
		}
		seen[strings.ToLower(name)] = true
		if len(u.Sidecars) == 0 {
			return fmt.Errorf("universe %q names no sidecar", name)
		}
		for _, sc := range u.Sidecars {
			if !pinned[sc.Name] {
				return fmt.Errorf("universe %q: sidecar %s is not a pinned member of the solution", name, sc.Name)
			}
			taken[sc.Name] = true
		}
	}
	for _, m := range v.Includes {
		if !taken[m.Name] {
			return fmt.Errorf("member %s@%s belongs to no universe", m.Name, m.Version)
		}
	}
	return nil
}
