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

// serviceCalls are the application and publish generics whose first type
// argument is a service type: the container keys a service by that type's
// bare name (config.TypeName), so it names one service only.
var serviceCalls = map[string]bool{
	"PublishService": true, "PublishSimpleService": true,
	"DeclareService": true, "DeclareSimpleService": true,
	"PutService": true, "UseService": true, "AppUseService": true,
}

// separateBinaries are names shared by packages that never run in one process:
// two connectors' API clients mirror each other's API, each in its own binary.
var separateBinaries = map[string]bool{"APIInvoker": true, "AuthService": true, "DiscoveryService": true}

// TestServiceTypeNamesAreUnique refuses two service types of the same bare
// name in different packages. Services are keyed by the type's name, so two
// interfaces both called `Service` resolve to whichever registered first: the
// other's slot then serves the wrong methods (universe-view and
// project-contributions answered comms' ask/answer/await). It reads the Go
// sources (tests excluded) under internal/ and app/ and collects the first
// type argument of every call in serviceCalls, resolved to its package (a
// type alias to its target: it is the same type).
func TestServiceTypeNamesAreUnique(t *testing.T) {
	root := repoRoot(t)
	const module = "github.com/katyoti/katyoti-fwk"
	byName := map[string]map[string][]string{} // name → package → where
	aliases := map[string]string{}             // "pkg.Name" → "target-pkg.Name"
	for _, top := range []string{"internal", "app"} {
		_ = filepath.WalkDir(filepath.Join(root, top), func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			fset := token.NewFileSet()
			f, perr := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
			if perr != nil {
				return nil
			}
			imports := map[string]string{}
			for _, im := range f.Imports {
				p, _ := strconv.Unquote(im.Path.Value)
				name := filepath.Base(p)
				if im.Name != nil {
					name = im.Name.Name
				}
				imports[name] = p
			}
			dir, _ := filepath.Rel(root, filepath.Dir(path))
			self := module + "/" + filepath.ToSlash(dir)
			ast.Inspect(f, func(n ast.Node) bool {
				if ts, ok := n.(*ast.TypeSpec); ok && ts.Assign.IsValid() {
					if sel, ok := ts.Type.(*ast.SelectorExpr); ok {
						if pkg, ok := sel.X.(*ast.Ident); ok && imports[pkg.Name] != "" {
							aliases[self+"."+ts.Name.Name] = imports[pkg.Name] + "." + sel.Sel.Name
						}
					}
					return true
				}
				var fun, arg ast.Expr
				switch x := n.(type) {
				case *ast.IndexExpr:
					fun, arg = x.X, x.Index
				case *ast.IndexListExpr:
					fun, arg = x.X, x.Indices[0]
				default:
					return true
				}
				callee := fun
				if sel, ok := fun.(*ast.SelectorExpr); ok {
					callee = sel.Sel
				}
				if id, ok := callee.(*ast.Ident); !ok || !serviceCalls[id.Name] {
					return true
				}
				sel, ok := arg.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				pkg, ok := sel.X.(*ast.Ident)
				if !ok || imports[pkg.Name] == "" {
					return true
				}
				name := sel.Sel.Name
				if byName[name] == nil {
					byName[name] = map[string][]string{}
				}
				rel, _ := filepath.Rel(root, path)
				byName[name][imports[pkg.Name]] = append(byName[name][imports[pkg.Name]], filepath.ToSlash(rel))
				return true
			})
			return nil
		})
	}
	for name, pkgs := range byName {
		for p := range pkgs {
			if target, ok := aliases[p+"."+name]; ok && strings.HasSuffix(target, "."+name) {
				tp := strings.TrimSuffix(target, "."+name)
				pkgs[tp] = append(pkgs[tp], pkgs[p]...)
				delete(pkgs, p)
			}
		}
	}
	var bad []string
	for name, pkgs := range byName {
		if len(pkgs) < 2 || separateBinaries[name] {
			continue
		}
		var list []string
		for p, where := range pkgs {
			list = append(list, p+" ("+where[0]+")")
		}
		sort.Strings(list)
		bad = append(bad, name+": "+strings.Join(list, ", "))
	}
	sort.Strings(bad)
	for _, b := range bad {
		t.Errorf("service type name used by several packages - name each for itself: %s", b)
	}
	if len(byName) < 20 {
		t.Fatalf("only %d service types found: the scan is broken", len(byName))
	}
}
