// SPDX-License-Identifier: Apache-2.0

package arch

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// cwmCoreDir is CWM's core: one store, serving one project folder.
const cwmCoreDir = "internal/extensions/cwm/internal/core"

// cwmCoreRoots are the only folder fields the core's store keeps, by struct:
// one corpus root and one store root per store. Several projects
// are several stores, joined above the core by cwm/internal/storeset - never
// a second root passed into the core.
var cwmCoreRoots = map[string][]string{
	// WorkspaceRoot: the store's workspace (its runtime objects, its lock);
	// ProjectRoot: the one project folder it serves; ProjectsFolder: the
	// installation's projects, listed by ProjectsKnown, never served;
	// MinterDir: the installation's number minter; BackupDir: where a
	// migration copies the project files it removes (the universe's data
	// folder) - a destination, never a corpus the store serves.
	"CWMServiceConfig": {"WorkspaceRoot", "ProjectRoot", "ProjectsFolder", "MinterDir", "BackupDir"},
	// root: the corpus root; storeRoot: the store root.
	"objectIndex": {"root", "storeRoot"},
	"Service":     {},
}

// rootName is a field naming a folder, or several.
var rootName = regexp.MustCompile(`(?i)(root|dir|folder|path)s?$`)

// TestCWMCoreKeepsOneRootPerStore - CWM's core keeps one corpus root and one
// store root per store: its config, its service and its index hold no folder
// field beyond the ones listed, and nothing in the core holds several stores
// or several indexes. The join of several projects is the store set's
// (cwm/internal/storeset), above the core.
func TestCWMCoreKeepsOneRootPerStore(t *testing.T) {
	dir := filepath.Join(repoRoot(t), filepath.FromSlash(cwmCoreDir))
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	seen := map[string]bool{}
	var problems []string
	for _, e := range entries {
		n := e.Name()
		if e.IsDir() || !strings.HasSuffix(n, ".go") || strings.HasSuffix(n, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(dir, n), nil, 0)
		if err != nil {
			t.Fatalf("%s: %v", n, err)
		}
		ast.Inspect(f, func(node ast.Node) bool {
			ts, ok := node.(*ast.TypeSpec)
			if !ok {
				return true
			}
			st, ok := ts.Type.(*ast.StructType)
			if !ok {
				return true
			}
			allowed, guarded := cwmCoreRoots[ts.Name.Name]
			if guarded {
				seen[ts.Name.Name] = true
			}
			for _, fld := range st.Fields.List {
				typ := exprString(fld.Type)
				for _, name := range fld.Names {
					where := n + ": " + ts.Name.Name + "." + name.Name
					if guarded && rootName.MatchString(name.Name) && !contains(allowed, name.Name) {
						problems = append(problems, where+" ("+typ+") is a folder the core does not keep: a store serves one project folder")
					}
					if holdsSeveralStores(typ) {
						problems = append(problems, where+" ("+typ+") holds several stores or indexes: join them in cwm/internal/storeset")
					}
				}
			}
			return true
		})
	}
	for name := range cwmCoreRoots {
		if !seen[name] {
			problems = append(problems, "struct "+name+" not found in "+cwmCoreDir+" - the guard no longer looks at the core's store")
		}
	}
	if len(problems) > 0 {
		sort.Strings(problems)
		t.Errorf("CWM's core keeps one corpus root and one store root per store:\n  %s", strings.Join(problems, "\n  "))
	}
}

// holdsSeveralStores is a slice or map of the core's store, index or storage.
func holdsSeveralStores(typ string) bool {
	if !strings.HasPrefix(typ, "[]") && !strings.HasPrefix(typ, "map[") {
		return false
	}
	for _, s := range []string{"Service", "objectIndex", "storage.Storage", "CWMServiceConfig"} {
		if strings.HasSuffix(typ, "*"+s) || strings.HasSuffix(typ, "]"+s) {
			return true
		}
	}
	return false
}

// exprString renders a field type well enough to read it.
func exprString(e ast.Expr) string {
	switch x := e.(type) {
	case *ast.Ident:
		return x.Name
	case *ast.StarExpr:
		return "*" + exprString(x.X)
	case *ast.SelectorExpr:
		return exprString(x.X) + "." + x.Sel.Name
	case *ast.ArrayType:
		return "[]" + exprString(x.Elt)
	case *ast.MapType:
		return "map[" + exprString(x.Key) + "]" + exprString(x.Value)
	default:
		return "?"
	}
}
