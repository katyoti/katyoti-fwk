// SPDX-License-Identifier: Apache-2.0

package arch

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// A generated document is read like any other, marked « IA »:
// the exclusion that left it out - a carrier's or a source's include_generated,
// a sync dropping generated items, a CQL narrowed to non-generated pages - was
// deleted by and does not come back.
var generatedExclusion = regexp.MustCompile(`include_generated|IncludeGenerated|notGeneratedClause|withoutGenerated|left_out|LeftOut`)

// generatedExclusionRoots are the folders read for it; the private connectors'
// folders are added in private_test.go.
var generatedExclusionRoots = []string{"internal/extensions/sources"}

func TestTheGeneratedExclusionStaysDeleted(t *testing.T) {
	root := repoRoot(t)
	checked := 0
	for _, dir := range generatedExclusionRoots {
		err := filepath.WalkDir(filepath.Join(root, dir), func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			if !strings.HasSuffix(p, ".go") && !strings.HasSuffix(p, ".yaml") && !strings.HasSuffix(p, ".md") {
				return nil
			}
			raw, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			checked++
			if m := generatedExclusion.FindString(string(raw)); m != "" {
				rel, _ := filepath.Rel(root, p)
				t.Errorf("%s holds %q - the generated-document exclusion was deleted", filepath.ToSlash(rel), m)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if checked < 20 {
		t.Fatalf("checked %d files: the roots are wrong", checked)
	}
}
