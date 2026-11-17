// SPDX-License-Identifier: Apache-2.0

package contribution

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// Scope is the PROVIDER's identity (`system`, `local`, anything else is
// shared — contribution-types.md §5), never a field on a kind. A kind that
// declared `scope:` would put one fact in two places and require a lookup
// before every visibility filter (ms-contribution-debug d0; e15).
//
// Covers every system kind declaration and every pack-declared kind shipped
// in a binary's embed (app/*/embed/contribution-types/).
func TestNoKindDeclarationCarriesAScope(t *testing.T) {
	check := func(name string, raw []byte) {
		t.Helper()
		var decl map[string]any
		if err := yaml.Unmarshal(raw, &decl); err != nil {
			t.Fatalf("%s: yaml: %v", name, err)
		}
		if _, has := decl["scope"]; has {
			t.Errorf("%s declares `scope:` — scope is the provider's identity (use the `local` provider for binary-private content), not a kind field", name)
		}
	}

	seen := 0
	err := fs.WalkDir(systemFS, "embed/contribution-type", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".yaml") {
			return err
		}
		raw, rerr := fs.ReadFile(systemFS, p)
		if rerr != nil {
			return rerr
		}
		seen++
		check(p, raw)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if seen == 0 {
		t.Fatal("no system kind declaration found — the walk is wrong, not the rule satisfied")
	}

	packs, err := filepath.Glob(filepath.Join("..", "..", "..", "app", "*", "embed", "contribution-types", "*.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range packs {
		raw, rerr := os.ReadFile(p)
		if rerr != nil {
			t.Fatal(rerr)
		}
		check(filepath.ToSlash(p), raw)
	}
	t.Logf("checked %d system and %d pack kind declaration(s)", seen, len(packs))
}
