// SPDX-License-Identifier: Apache-2.0

package arch

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// ONE READER FOR A LIST OF DECLARATIONS. Two copies of the same loop over the
// catalog — List every manifest, GetBody each entry, decode it — is how they
// came to disagree (an `aspect` kind was once dropped by one of two copies).
// The shared readers are mbapi.QueryTyped (any kind, decoded once) and
// cwmapi.DeclarationsFromCatalog (what CWM itself reads); no other code hand-
// rolls the loop.

// catalogLoopOwners may walk the catalog by hand: the manifest extension (the
// catalog and its readers) and cwm/api (DeclarationsFromCatalog).
var catalogLoopOwners = []string{
	"internal/extensions/manifest/",
	"internal/extensions/cwm/api/",
}

// TestNoHandRolledCatalogDeclarationLoop refuses a function outside the
// owners that calls `.List(`, `.GetBody(` and a json `Unmarshal` — the
// hand-rolled declaration loop.
func TestNoHandRolledCatalogDeclarationLoop(t *testing.T) {
	root := repoRoot(t)
	var offenders []string
	scanned := 0
	for _, top := range []string{"internal", "app"} {
		err := filepath.WalkDir(filepath.Join(root, top), func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if n := d.Name(); n == "node_modules" || n == "testdata" || strings.HasPrefix(n, ".") || strings.HasPrefix(n, "_") {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
				return nil
			}
			rel := filepath.ToSlash(strings.TrimPrefix(p, root+string(filepath.Separator)))
			for _, owner := range catalogLoopOwners {
				if strings.HasPrefix(rel, owner) {
					return nil
				}
			}
			src, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			scanned++
			offenders = append(offenders, handRolledLoops(t, rel, src)...)
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if scanned < 500 {
		t.Fatalf("only %d files scanned — the walk is wrong, not the tree", scanned)
	}
	sort.Strings(offenders)
	for _, o := range offenders {
		t.Errorf("%s hand-rolls the catalog declaration loop (List → GetBody → Unmarshal): read through mbapi.QueryTyped or cwmapi.DeclarationsFromCatalog", o)
	}
}

// handRolledLoops names each function of src calling all three.
func handRolledLoops(t *testing.T, rel string, src []byte) []string {
	t.Helper()
	if !strings.Contains(string(src), "GetBody(") {
		return nil
	}
	f, err := parser.ParseFile(token.NewFileSet(), rel, src, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", rel, err)
	}
	var out []string
	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		seen := map[string]bool{}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
				switch sel.Sel.Name {
				case "List", "GetBody":
					seen[sel.Sel.Name] = true
				case "Unmarshal":
					if x, ok := sel.X.(*ast.Ident); ok && x.Name == "json" {
						seen["Unmarshal"] = true
					}
				}
			}
			return true
		})
		if seen["List"] && seen["GetBody"] && seen["Unmarshal"] {
			out = append(out, rel+": "+fn.Name.Name)
		}
	}
	return out
}
