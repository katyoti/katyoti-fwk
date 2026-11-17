// SPDX-License-Identifier: Apache-2.0

package long

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

// THE LIST IS EXACT: every test that calls long.Test has its
// row in Tests, and every row names a test that calls it - the list a person
// reviews is the list the quick lane skips.
func TestEveryLongTestIsListedAndEveryRowIsCalled(t *testing.T) {
	calls := callSites(t, repoRoot(t))
	var unlisted, uncalled []string
	for k := range calls {
		if _, ok := Tests[k]; !ok {
			unlisted = append(unlisted, k)
		}
	}
	for k, why := range Tests {
		if !calls[k] {
			uncalled = append(uncalled, k)
		}
		if strings.TrimSpace(why) == "" {
			t.Errorf("Tests[%q] says nothing of what the test costs", k)
		}
	}
	sort.Strings(unlisted)
	sort.Strings(uncalled)
	if len(unlisted) > 0 {
		t.Errorf("tests calling long.Test with no row in internal/test/long.Tests:\n  %s", strings.Join(unlisted, "\n  "))
	}
	if len(uncalled) > 0 {
		t.Errorf("rows of internal/test/long.Tests whose test does not call long.Test:\n  %s", strings.Join(uncalled, "\n  "))
	}
	if len(calls) == 0 {
		t.Error("no test calls long.Test: the quick lane skips nothing")
	}
}

// The key a test computes at run time is the key the guard reads from the
// source: the package folder from the module root, the top-level test name.
func TestTheKeyIsThePackageFolderAndTheTopLevelTest(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	if got := Key(PackageDir(filepath.Dir(file)), t.Name()+"/sub"); got != "internal/test/long:TestTheKeyIsThePackageFolderAndTheTopLevelTest" {
		t.Errorf("key = %q", got)
	}
	if got := PackageDir("github.com/katyoti/katyoti-fwk/internal/x"); got != "internal/x" {
		t.Errorf("a -trimpath folder: %q", got)
	}
}

// callSites are the "<package folder>:<TestName>" of every test function of the
// repository whose body calls long.Test.
func callSites(t *testing.T, root string) map[string]bool {
	t.Helper()
	calls := map[string]bool{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "node_modules", "vendor", ".katyoti", "dist", "bin", "modules", "test_files", "third_party":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, "_test.go") {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil || !bytes.Contains(src, []byte("long.Test(")) {
			return nil
		}
		f, err := parser.ParseFile(token.NewFileSet(), path, src, 0)
		if err != nil {
			t.Errorf("parse %s: %v", path, err)
			return nil
		}
		rel, _ := filepath.Rel(root, filepath.Dir(path))
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Test" {
					if id, ok := sel.X.(*ast.Ident); ok && id.Name == "long" {
						calls[filepath.ToSlash(rel)+":"+fn.Name.Name] = true
					}
				}
				return true
			})
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return calls
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, here, _, _ := runtime.Caller(0)
	return filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(here))))
}
