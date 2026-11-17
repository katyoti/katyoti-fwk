// SPDX-License-Identifier: Apache-2.0

package arch

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

const (
	cwmSourceDir  = "internal/extensions/cwm"
	cwmCoreImport = "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/internal/core"
	cwmStoreSet   = "internal/extensions/cwm/internal/storeset"
)

// cwmCoreImporters are the only packages, beside the core itself, that may
// import CWM's core: the store set (the one write seam of several projects),
// cwm/local (the opener: it opens the stores and hands them to the set), and
// the openers of a folder no hub serves, each with a scratch store of its own
// (cwm/folder, cwm/migrate).
var cwmCoreImporters = []string{
	cwmCoreDir,
	cwmStoreSet,
	"internal/extensions/cwm/local",
	"internal/extensions/cwm/folder",
	"internal/extensions/cwm/migrate",
}

// TestTheStoreSetIsCWMsOneWriteSeam (the start of E1's write
// port): storeset.Served embeds no core store - it declares every verb of
// api.CWMService itself, asserted at compile time, so a new verb does not
// compile until it is routed - and outside test files only the core, the
// store set, the opener and the unserved-folder openers import the core: no
// other package writes an object past the set.
func TestTheStoreSetIsCWMsOneWriteSeam(t *testing.T) {
	root := repoRoot(t)
	var problems []string

	// Served embeds no store and asserts the contract.
	fset := token.NewFileSet()
	foundServed, asserted := false, false
	dir := filepath.Join(root, filepath.FromSlash(cwmStoreSet))
	files, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range files {
		if strings.HasSuffix(p, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, p, nil, 0)
		if err != nil {
			t.Fatalf("%s: %v", p, err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.TypeSpec:
				st, ok := x.Type.(*ast.StructType)
				if !ok || x.Name.Name != "Served" {
					return true
				}
				foundServed = true
				for _, fld := range st.Fields.List {
					if len(fld.Names) == 0 {
						problems = append(problems, "storeset.Served embeds "+exprString(fld.Type)+": a verb it does not declare would fall silently to that store - declare and route every verb")
					}
				}
			case *ast.ValueSpec:
				if len(x.Names) == 1 && x.Names[0].Name == "_" && x.Type != nil && exprString(x.Type) == "api.CWMService" &&
					len(x.Values) == 1 && strings.Contains(conversionType(x.Values[0]), "Served") {
					asserted = true
				}
			}
			return true
		})
	}
	if !foundServed {
		problems = append(problems, "struct Served not found in "+cwmStoreSet+" - the guard no longer looks at the seam")
	}
	if !asserted {
		problems = append(problems, "storeset has no `var _ api.CWMService = (*Served)(nil)`: a verb left undeclared would not fail the build")
	}

	// Only the listed packages import the core.
	err = filepath.WalkDir(filepath.Join(root, filepath.FromSlash(cwmSourceDir)), func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return nil
		}
		rel, _ := filepath.Rel(root, filepath.Dir(p))
		rel = filepath.ToSlash(rel)
		if contains(cwmCoreImporters, rel) {
			return nil
		}
		f, err := parser.ParseFile(fset, p, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, im := range f.Imports {
			if path, _ := strconv.Unquote(im.Path.Value); path == cwmCoreImport {
				frel, _ := filepath.Rel(root, p)
				problems = append(problems, filepath.ToSlash(frel)+" imports CWM's core: write objects through the CWM service (the store set when several projects are served), never a core store")
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(problems) > 0 {
		sort.Strings(problems)
		t.Errorf("the store set is CWM's one write seam:\n  %s", strings.Join(problems, "\n  "))
	}
}

// conversionType renders the type named by `(*T)(nil)`-like expressions well enough to
// find T in it.
func conversionType(e ast.Expr) string {
	switch x := e.(type) {
	case *ast.CallExpr:
		return conversionType(x.Fun)
	case *ast.ParenExpr:
		return conversionType(x.X)
	case *ast.StarExpr:
		return "*" + conversionType(x.X)
	default:
		return exprString(e)
	}
}
