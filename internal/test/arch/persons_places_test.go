// SPDX-License-Identifier: Apache-2.0

package arch

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// No test reaches the person's installations, registries or universes
// . One place resolves them - internal/core/cenv, whose
// KeepTestsOut moves every such root of a test process under a root of its
// own - so no other Go code composes them: the global root under a config
// home (`".config", "katyoti"`), the registry of installations under the user
// config folder (os.UserConfigDir), the installations' parent
// (`"Programs", "Katyoti"`). The exceptions, each with why:
var personsPlacesAllowed = map[string]string{
	"app/katyoti/internal/instance/instance.go":                       "an installation's default folder; answers through cenv.KeepTestsOut",
	"internal/appcore/controlplane/installations.go":                  "the registry of installations; a test process gets cenv.TestInstancesRegistry in its place",
	"internal/extensions/marketplace/internal/testworld/testworld.go": "the guard of the marketplace's tests: it stats the person's files, never writes them",
	"internal/extensions/scenario/internal/machine/machine.go":        "a sandbox machine's own LOCALAPPDATA (its environment), not the person's",
}

var personsPlaceRE = regexp.MustCompile(`"\.config",\s*"katyoti"|os\.UserConfigDir\(\)|"Programs",\s*"Katyoti"`)

// TestOnlyCenvResolvesThePersonsPlaces: no Go code outside cenv and the
// exceptions composes a path of the person's installation.
func TestOnlyCenvResolvesThePersonsPlaces(t *testing.T) {
	root := repoRoot(t)
	var problems []string
	seen := map[string]bool{}
	for _, top := range []string{"app", "internal", "tools"} {
		_ = filepath.WalkDir(filepath.Join(root, top), func(p string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				if n := d.Name(); n == "node_modules" || n == "testdata" || n == "frontend" {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
				return nil
			}
			rel, _ := filepath.Rel(root, p)
			rel = filepath.ToSlash(rel)
			if strings.HasPrefix(rel, "internal/core/cenv/") {
				return nil
			}
			b, err := os.ReadFile(p)
			if err != nil || !personsPlaceRE.Match(b) {
				return nil
			}
			seen[rel] = true
			if _, ok := personsPlacesAllowed[rel]; !ok {
				problems = append(problems, rel+": composes a path of the person's installation - resolve it through internal/core/cenv (KeepTestsOut keeps a test out of it)")
			}
			return nil
		})
	}
	for f := range personsPlacesAllowed {
		if !seen[f] {
			problems = append(problems, f+": allowed but no longer names a person's place - drop it from personsPlacesAllowed")
		}
	}
	b, err := os.ReadFile(filepath.Join(root, "app", "katyoti", "internal", "instance", "instance.go"))
	if err != nil || !strings.Contains(string(b), "cenv.KeepTestsOut(") {
		problems = append(problems, "app/katyoti/internal/instance/instance.go: an installation's default folder does not answer through cenv.KeepTestsOut")
	}
	sort.Strings(problems)
	for _, p := range problems {
		t.Error(p)
	}
}
