// SPDX-License-Identifier: Apache-2.0

package core

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// shippedPack is one CWM pack of this repository: its name and its folder.
type shippedPack struct {
	name, dir string
}

// shippedPacks lists the CWM packs the repository ships - a sidecar's
// `app/<name>/embed/cwm` and an extension's
// `internal/extensions/<name>/contrib/embed/cwm` - named by their folder. It
// is the one list of packs the tests read: the corpus is judged against
// them (realCorpusConfig) and a migration step names one of them.
func shippedPacks(repo string) []shippedPack {
	var out []shippedPack
	for _, glob := range [][]string{
		{"app", "*", "embed", "cwm"},
		{"internal", "extensions", "*", "contrib", "embed", "cwm"},
	} {
		dirs, _ := filepath.Glob(filepath.Join(append([]string{repo}, glob...)...))
		for _, d := range dirs {
			rel, err := filepath.Rel(repo, d)
			if err != nil {
				continue
			}
			// The pack's name is the folder the glob's star matched.
			out = append(out, shippedPack{name: strings.Split(filepath.ToSlash(rel), "/")[slices.Index(glob, "*")], dir: d})
		}
	}
	return out
}

// stepsWithoutAKnownPack names each step whose pack is empty or not one of
// known.
func stepsWithoutAKnownPack(steps []MigrationStep, known map[string]bool) []string {
	var bad []string
	for i, s := range steps {
		switch {
		case s.Pack == "":
			bad = append(bad, fmt.Sprintf("step %d (%q) names no pack", i+1, s.Title))
		case !known[s.Pack]:
			bad = append(bad, fmt.Sprintf("step %d (%q) names the pack %q, which nothing ships", i+1, s.Title, s.Pack))
		}
	}
	return bad
}

// privatePacks are the packs of the private build the steps name
// (migrate_private_test.go); the public build has none.
var privatePacks []string

// : every data migration step names the pack that owns the shape
// it follows, one of the packs the repository ships - so a step can follow
// its pack when the pack leaves for a repository of its own.
func TestEveryMigrationStepNamesAKnownPack(t *testing.T) {
	packs := shippedPacks(filepath.Join("..", "..", "..", "..", ".."))
	if len(packs) < 6 {
		t.Fatalf("found only %d packs - the globs are wrong, not the steps", len(packs))
	}
	known := map[string]bool{}
	for _, p := range packs {
		known[p.name] = true
	}
	for _, want := range append([]string{"cwm", "planification"}, privatePacks...) {
		if !known[want] {
			t.Fatalf("the pack %q is not found among %v - the pack names are read wrong", want, known)
		}
	}
	if bad := stepsWithoutAKnownPack(migrationSteps, known); len(bad) > 0 {
		t.Fatalf("a migration step names the pack that owns it, one the repository ships (migrate.go):\n  %s", strings.Join(bad, "\n  "))
	}
	// The guard on its own: an empty and an unknown pack are refused.
	probe := []MigrationStep{{Pack: "cwm", Title: "kept"}, {Title: "no pack"}, {Pack: "no-such-pack", Title: "unknown"}}
	if bad := stepsWithoutAKnownPack(probe, known); len(bad) != 2 {
		t.Fatalf("an empty and an unknown pack must both be refused, got %v", bad)
	}
}
