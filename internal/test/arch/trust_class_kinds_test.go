// SPDX-License-Identifier: Apache-2.0

package arch

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/marshal"
)

// trustClassKinds are the kinds whose content runs, names something to run or
// widens rights (decision D9 of the contribution study
// widened by to object-type, display-engine and
// config-program):
// each must declare execution: executable, so the override gate refuses an
// untrusted project's override of one.
var trustClassKinds = []string{
	"task", "source-connector", "publication-connector",
	"hook", "workflow", "policy", "gate-requirement", "capability",
	"permission-profile", "skill",
	"object-type", "display-engine", "config-program",
}

// Every kind of the trust classes declares execution: executable in its one
// declaration (a contribution-type file, wherever its pack ships it).
func TestTrustClassKindsDeclareExecutable(t *testing.T) {
	root := repoRoot(t)
	declared := map[string][]string{} // kind -> execution of each declaration found
	for _, top := range []string{"internal", "app"} {
		err := filepath.WalkDir(filepath.Join(root, top), func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				switch d.Name() {
				case "node_modules", "testdata", "fixtures", "frontend":
					return filepath.SkipDir
				}
				return nil
			}
			dir := filepath.Base(filepath.Dir(p))
			if !strings.HasSuffix(p, ".yaml") || !(dir == "contribution-type" || dir == "contribution-types" || strings.HasSuffix(dir, "kind")) {
				return nil
			}
			raw, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			var decl struct {
				Name      string `yaml:"name"`
				Execution string `yaml:"execution"`
			}
			if err := config.DecodeBytes(raw, marshal.YAML, &decl); err != nil {
				t.Errorf("%s: %v", p, err)
				return nil
			}
			declared[decl.Name] = append(declared[decl.Name], decl.Execution)
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(declared) < 30 {
		t.Fatalf("found %d kind declarations: the walk is wrong", len(declared))
	}
	var wrong []string
	for _, kind := range trustClassKinds {
		got := declared[kind]
		switch {
		case len(got) == 0:
			wrong = append(wrong, kind+" (no declaration found)")
		case len(got) > 1:
			wrong = append(wrong, kind+" (declared more than once)")
		case got[0] != "executable":
			wrong = append(wrong, kind+" (execution: "+got[0]+")")
		}
	}
	sort.Strings(wrong)
	if len(wrong) > 0 {
		t.Errorf("trust-class kinds not declared executable: %s", strings.Join(wrong, ", "))
	}
}
