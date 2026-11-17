// SPDX-License-Identifier: Apache-2.0

package configset

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/marshal"
)

// MembersFile is the OPTIONAL member list a config-set may carry (§10).
//
// A config-set has always described a universe's CONFIG. With this it can also
// describe its MEMBERSHIP, so one artifact answers both halves of "what is this
// universe": which sidecars, and how they are configured.
//
// OPTIONAL IS LOAD-BEARING. A bundle without it, or with an empty list, applies
// exactly as it always did — templates and keys, membership untouched. Every
// config-set published before this existed keeps working unchanged, which is
// the only reason this can be added to a shipped format.
const MembersFile = "members.yaml"

// LegacyMembersFile is the member list's former name: read, for bundles
// published before the rename; refused at publish.
const LegacyMembersFile = "solution.yaml"

// Members is the config-set's declared membership.
type Members struct {
	// Sidecars are member NAMES, never paths or versions.
	//
	// NO VERSIONS, deliberately. A config-set says WHICH sidecars a universe
	// needs, not which build of them — apply resolves the latest and, crucially,
	// leaves a member that is already wired exactly as it is. Pinning belongs to
	// a `kind: solution`, which exists to be installed byte for byte; a config-set
	// is something you APPLY to a universe you are already running, possibly
	// alongside other config-sets.
	Sidecars []string `yaml:"sidecars"`
}

// LoadMembers reads a bundle's members.yaml (or, published before the rename,
// its solution.yaml).
//
// A MISSING FILE IS NOT AN ERROR — it is the common case and it means "this
// config-set has nothing to say about membership".
func LoadMembers(folder string) (Members, error) {
	var sol Members
	data, err := os.ReadFile(filepath.Join(folder, MembersFile))
	if os.IsNotExist(err) {
		data, err = os.ReadFile(filepath.Join(folder, LegacyMembersFile))
	}
	if os.IsNotExist(err) {
		return sol, nil
	}
	if err != nil {
		return sol, fmt.Errorf("read %s: %w", MembersFile, err)
	}
	if err := marshal.Unmarshal(data, marshal.YAML, &sol); err != nil {
		return sol, fmt.Errorf("decode %s: %w", MembersFile, err)
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(sol.Sidecars))
	for _, name := range sol.Sidecars {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		// A name carrying a version is a category error worth naming rather
		// than silently trimming: the author expected a pin this format does
		// not give, and apply would have quietly resolved something else.
		if strings.Contains(name, "@") {
			return Members{}, fmt.Errorf(
				"%s: sidecar %q carries a version — a config-set declares WHICH sidecars, "+
					"not which build; pin versions in a solution instead", MembersFile, name)
		}
		if seen[name] {
			continue // listed twice is harmless; the union is a set
		}
		seen[name] = true
		out = append(out, name)
	}
	sol.Sidecars = out
	return sol, nil
}
