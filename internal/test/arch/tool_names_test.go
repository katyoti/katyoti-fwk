// SPDX-License-Identifier: Apache-2.0

package arch

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// kebabToolName is the one shape of an agent tool name (rule):
// lowercase words joined by single hyphens. The name an LLM provider puts on
// the wire is the provider adapter's business, never a reason to register
// another spelling.
var kebabToolName = regexp.MustCompile(`^[a-z][a-z0-9]*(-[a-z0-9]+)*$`)

// TestEveryRegisteredToolNameIsKebab lists every agent tool name the code
// registers and refuses one that is not kebab-case. It reads the Go sources
// (tests excluded) under internal/ and app/ and collects:
//
//   - the name given to a tool constructor — the first argument of any call
//     whose function name ends in "Tool" (toolsapi.NewFunctionTool and the
//     per-extension wrappers: cwmTool, gatesTool, …), and the group name of
//     NewToolGroup (its methods become "<group>-<method-kebab>");
//   - the Name of every ToolDef composite literal (plugins that build their
//     ToolDefs by hand: delegate, task-status, the orchestration tools, …).
//
// A name given through a constant resolves when the constant is declared in
// the same package. A tool named from a runtime value (an MCP or KTSP proxy)
// is not a registration in source and is not listed.
func TestEveryRegisteredToolNameIsKebab(t *testing.T) {
	root := repoRoot(t)
	names := map[string][]string{} // name → where
	for _, top := range []string{"internal", "app"} {
		pkgs := map[string][]*ast.File{}
		fset := token.NewFileSet()
		_ = filepath.WalkDir(filepath.Join(root, top), func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				switch d.Name() {
				case "node_modules", "frontend", "testdata", ".git":
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
				return nil
			}
			f, err := parser.ParseFile(fset, p, nil, 0)
			if err != nil {
				return nil
			}
			pkgs[filepath.Dir(p)] = append(pkgs[filepath.Dir(p)], f)
			return nil
		})
		for _, files := range pkgs {
			consts := packageStringConsts(files)
			str := func(e ast.Expr) (string, bool) {
				switch v := e.(type) {
				case *ast.BasicLit:
					if v.Kind == token.STRING {
						s, err := strconv.Unquote(v.Value)
						return s, err == nil
					}
				case *ast.Ident:
					s, ok := consts[v.Name]
					return s, ok
				}
				return "", false
			}
			for _, f := range files {
				ast.Inspect(f, func(n ast.Node) bool {
					switch n := n.(type) {
					case *ast.CallExpr:
						fn := calleeName(n.Fun)
						if (strings.HasSuffix(fn, "Tool") || fn == "NewToolGroup") && len(n.Args) > 0 {
							if s, ok := str(n.Args[0]); ok {
								rel, _ := filepath.Rel(root, fset.Position(n.Pos()).String())
								names[s] = append(names[s], filepath.ToSlash(rel))
							}
						}
					case *ast.CompositeLit:
						// A ToolDef{…}, or the elided elements of a []ToolDef{{…}}.
						defs := []*ast.CompositeLit{}
						if typeName(n.Type) == "ToolDef" {
							defs = append(defs, n)
						} else if at, ok := n.Type.(*ast.ArrayType); ok && typeName(at.Elt) == "ToolDef" {
							for _, el := range n.Elts {
								if cl, ok := el.(*ast.CompositeLit); ok && cl.Type == nil {
									defs = append(defs, cl)
								}
							}
						}
						for _, def := range defs {
							for _, el := range def.Elts {
								kv, ok := el.(*ast.KeyValueExpr)
								if !ok {
									continue
								}
								if k, ok := kv.Key.(*ast.Ident); ok && k.Name == "Name" {
									if s, ok := str(kv.Value); ok {
										rel, _ := filepath.Rel(root, fset.Position(def.Pos()).String())
										names[s] = append(names[s], filepath.ToSlash(rel))
									}
								}
							}
						}
					}
					return true
				})
			}
		}
	}
	// The scan must actually see the tools: the cwm toolset alone is 26.
	if len(names) < 60 {
		t.Fatalf("only %d tool names found — the scan is not working", len(names))
	}
	var bad []string
	for name, where := range names {
		if !kebabToolName.MatchString(name) {
			bad = append(bad, strconv.Quote(name)+" ← "+where[0])
		}
	}
	sort.Strings(bad)
	if len(bad) > 0 {
		t.Errorf("tool names must be kebab-case:\n  %s", strings.Join(bad, "\n  "))
	}
}

func packageStringConsts(files []*ast.File) map[string]string {
	out := map[string]string{}
	for _, f := range files {
		for _, d := range f.Decls {
			gd, ok := d.(*ast.GenDecl)
			if !ok || gd.Tok != token.CONST {
				continue
			}
			for _, sp := range gd.Specs {
				vs := sp.(*ast.ValueSpec)
				for i, n := range vs.Names {
					if i < len(vs.Values) {
						if bl, ok := vs.Values[i].(*ast.BasicLit); ok && bl.Kind == token.STRING {
							if s, err := strconv.Unquote(bl.Value); err == nil {
								out[n.Name] = s
							}
						}
					}
				}
			}
		}
	}
	return out
}

func calleeName(e ast.Expr) string {
	switch v := e.(type) {
	case *ast.Ident:
		return v.Name
	case *ast.SelectorExpr:
		return v.Sel.Name
	case *ast.IndexExpr:
		return calleeName(v.X)
	case *ast.IndexListExpr:
		return calleeName(v.X)
	}
	return ""
}

func typeName(e ast.Expr) string {
	switch v := e.(type) {
	case *ast.Ident:
		return v.Name
	case *ast.SelectorExpr:
		return v.Sel.Name
	}
	return ""
}
