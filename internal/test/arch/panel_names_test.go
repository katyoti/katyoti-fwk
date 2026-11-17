// SPDX-License-Identifier: Apache-2.0

package arch

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// An object's panels are tabs a person picks by their name, so no two shipped
// panels may carry the same one: ktw-agents' file-overview and ktw-methods'
// composition panel were both "Overview", two tabs nobody could tell apart.
func TestShippedObjectPanelNamesAreUnique(t *testing.T) {
	files, err := filepath.Glob(filepath.Join(repoRoot(t), "app", "*", "embed", "cwm", "object-panels", "*.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]string{}
	for _, f := range files {
		if strings.HasSuffix(f, ".i18n.yaml") {
			continue
		}
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		var decl struct {
			Name string `yaml:"name"`
		}
		if err := yaml.Unmarshal(raw, &decl); err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		if decl.Name == "" {
			continue
		}
		if other, dup := seen[decl.Name]; dup {
			t.Errorf("panel name %q is shipped twice: %s and %s", decl.Name, other, f)
		}
		seen[decl.Name] = f
	}
	if len(seen) == 0 {
		t.Fatal("no shipped object panel found: the glob is wrong")
	}
}

// A panel's stem (its file name) is its tab's id, `tab:<stem>`:
// no two packs may ship the same one - the census reads one binary and cannot
// see two packs collide; the object pane would keep the first and skip the
// other.
func TestShippedObjectPanelStemsAreUnique(t *testing.T) {
	root := repoRoot(t)
	var files []string
	for _, pattern := range []string{
		filepath.Join(root, "app", "*", "embed", "cwm", "object-panels", "*.yaml"),
		filepath.Join(root, "internal", "extensions", "*", "contrib", "embed", "cwm", "object-panels", "*.yaml"),
	} {
		got, err := filepath.Glob(pattern)
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, got...)
	}
	if len(files) == 0 {
		t.Fatal("no shipped object panel found")
	}
	seen := map[string]string{}
	for _, f := range files {
		if strings.HasSuffix(f, ".i18n.yaml") {
			continue
		}
		stem := strings.TrimSuffix(filepath.Base(f), ".yaml")
		if other, dup := seen[stem]; dup {
			t.Errorf("panel stem %q (tab:%s) is shipped twice: %s and %s", stem, stem, other, f)
			continue
		}
		seen[stem] = f
	}
}
