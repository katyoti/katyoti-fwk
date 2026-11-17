// SPDX-License-Identifier: Apache-2.0

package arch

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// sqliteDriverHome is the one package that imports the SQLite driver: the
// scheduler's store adapter.
const sqliteDriverHome = "internal/extensions/scheduler/internal/sqlstore/"

// sqliteImportRE is an import line of the driver, aliased or blank or not.
var sqliteImportRE = regexp.MustCompile(`^\s*(import\s+)?([\w.]+\s+)?"modernc\.org/sqlite`)

// TestTheSQLiteDriverStaysInTheStoreAdapter: no Go file - test files included -
// imports modernc.org/sqlite outside the scheduler's SQLite store, so the
// driver (and its 4.7 MB) is linked only where that store is.
func TestTheSQLiteDriverStaysInTheStoreAdapter(t *testing.T) {
	root := repoRoot(t)
	var problems []string
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
			r := rel(root, p)
			if !strings.HasSuffix(p, ".go") || strings.HasPrefix(r, sqliteDriverHome) {
				return nil
			}
			b, err := os.ReadFile(p)
			if err != nil {
				return nil
			}
			for i, line := range strings.Split(string(b), "\n") {
				if sqliteImportRE.MatchString(line) {
					problems = append(problems, r+":"+strconv.Itoa(i+1)+": "+strings.TrimSpace(line))
				}
			}
			return nil
		})
	}
	if len(problems) > 0 {
		t.Errorf("only %s imports the SQLite driver:\n  %s", sqliteDriverHome, strings.Join(problems, "\n  "))
	}
}
