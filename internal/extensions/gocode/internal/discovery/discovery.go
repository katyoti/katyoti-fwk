// SPDX-License-Identifier: Apache-2.0

// Package discovery implements the Go AST scanning primitives that
// power ktw-gocode's retro-engineering mode (modules/code-gocode/concepts.md §3).
// Pure data — no CWM writes, no networking, no subprocess shell-out.
//
// Given a root path, Discover walks every Go package under it and
// returns an Inventory: packages, exported symbols, test functions,
// imports, and layout hints. The wiring layer (gocode/internal/core/
// methods_discover.go) converts that inventory into CWM items.
//
// Implementation note: this package uses stdlib `go/ast` +
// `go/parser` + `go/token` directly rather than
// `golang.org/x/tools/go/packages`. The richer "packages" loader
// resolves types across module boundaries, which costs hundreds of
// ms per package and trips on missing dependencies. For
// retro-engineering we only need syntax-level inventory (signatures,
// not type identity), which stdlib parser handles cleanly + fast.
package discovery

import (
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Discover walks Go packages under root and returns an inventory.
// All output slices are sorted (packages by Path, symbols by
// Package then Name, tests by TestRef) so consecutive runs produce
// byte-identical results.
//
// Subdirectories matching the standard Go skip list (vendor,
// node_modules, .git, .katyoti) are ignored. Files
// matching `*_gen.go` / `*.pb.go` / files starting with `//
// Code generated …` are skipped unless opts.IncludeGenerated.
func Discover(ctx context.Context, root string, opts Options) (*Inventory, error) {
	root = filepath.Clean(root)
	info, err := os.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("discovery: stat %s: %w", root, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("discovery: %s is not a directory", root)
	}

	inv := &Inventory{}
	fset := token.NewFileSet()

	// Walk and aggregate per-package data. Map keyed by relative
	// package directory so siblings + tests of the same package
	// land in the same bucket.
	pkgs := map[string]*aggregator{}

	err = filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if d.IsDir() {
			if shouldSkipDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".go") {
			return nil
		}
		if !opts.IncludeGenerated && isGeneratedFileName(d.Name()) {
			return nil
		}

		// Size guard against huge generated files.
		fi, statErr := d.Info()
		if statErr != nil {
			return nil
		}
		if opts.MaxFileBytes > 0 && fi.Size() > opts.MaxFileBytes {
			return nil
		}

		isTestFile := strings.HasSuffix(d.Name(), "_test.go")
		if isTestFile && !opts.IncludeTests {
			// Still parse test files when IncludeTests=true (default).
			// Default behaviour SKIPS them. Most retro-engineering
			// callers DO want test data; the field is opt-in for
			// users who want the inventory to exclude tests.
			return nil
		}

		fileNode, parseErr := parser.ParseFile(fset, path, nil, parser.ParseComments|parser.SkipObjectResolution)
		if parseErr != nil {
			// Tolerate parse errors on individual files — log via
			// warning rather than aborting the whole scan. Callers
			// of Discover get a partial inventory rather than
			// nothing.
			return nil
		}

		if !opts.IncludeGenerated && isGeneratedHeader(fileNode) {
			return nil
		}

		dir := filepath.Dir(path)
		relDir, _ := filepath.Rel(root, dir)
		relDir = filepath.ToSlash(relDir)
		if relDir == "" {
			relDir = "."
		}

		agg, ok := pkgs[relDir]
		if !ok {
			agg = &aggregator{
				path:    relDir,
				name:    fileNode.Name.Name,
				imports: map[string]bool{},
			}
			pkgs[relDir] = agg
		}
		// First file's name wins; subsequent files (including _test.go
		// counterparts which carry the `<pkg>_test` form) just augment.
		if isTestFile {
			agg.hasTest = true
		} else {
			agg.hasSource = true
		}
		if fileNode.Name.Name == "main" {
			agg.isCmd = true
		}
		agg.fileBytes += fi.Size()

		// Imports.
		for _, imp := range fileNode.Imports {
			path := strings.Trim(imp.Path.Value, "\"")
			agg.imports[path] = true
		}

		// Symbols + tests.
		relFile, _ := filepath.Rel(root, path)
		relFile = filepath.ToSlash(relFile)
		for _, decl := range fileNode.Decls {
			extract(decl, fset, agg, relDir, relFile, isTestFile)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("discovery: walk %s: %w", root, err)
	}

	// Convert aggregator into the typed Inventory + sort.
	for _, agg := range pkgs {
		publicCount := 0
		for _, s := range agg.exports {
			if s.IsExported {
				publicCount++
			}
		}
		inv.Packages = append(inv.Packages, &PackageInfo{
			Path:          agg.path,
			Name:          agg.name,
			IsTest:        agg.hasTest && !agg.hasSource,
			IsCmd:         agg.isCmd,
			PublicExports: publicCount,
		})
		inv.PublicSymbols = append(inv.PublicSymbols, agg.exports...)
		inv.TestFunctions = append(inv.TestFunctions, agg.tests...)
		for imp := range agg.imports {
			inv.Imports = append(inv.Imports, &ImportInfo{
				Importer: agg.path,
				Imported: imp,
				IsLocal:  isLocalImport(imp, root),
			})
		}
	}

	sortInventory(inv)
	inv.LayoutHints = detectLayout(root)
	return inv, nil
}

// extract pulls symbols + tests from a single top-level decl.
func extract(decl ast.Decl, fset *token.FileSet, agg *aggregator, pkgPath, relFile string, isTestFile bool) {
	switch d := decl.(type) {
	case *ast.FuncDecl:
		extractFunc(d, fset, agg, pkgPath, relFile, isTestFile)
	case *ast.GenDecl:
		extractGenDecl(d, agg, pkgPath)
	}
}

// aggregator holds per-package data accumulated during the walk
// before the Inventory is assembled. Named (not aliased) so
// extract / helpers can declare *aggregator receivers.
type aggregator struct {
	path      string
	name      string
	isCmd     bool
	hasTest   bool
	hasSource bool
	exports   []*SymbolInfo
	tests     []*TestFunctionInfo
	imports   map[string]bool
	fileBytes int64
}

func extractFunc(fn *ast.FuncDecl, fset *token.FileSet, agg *aggregator, pkgPath, relFile string, isTestFile bool) {
	name := fn.Name.Name
	doc := strings.TrimSpace(fn.Doc.Text())

	// Test functions in *_test.go files. The Go test framework
	// recognises Test<Name>, Benchmark<Name>, Example<Name>, Fuzz<Name>
	// where the suffix is upper-case or empty.
	if isTestFile {
		if kind, ok := testKindOf(name); ok {
			agg.tests = append(agg.tests, &TestFunctionInfo{
				Name:    name,
				Package: pkgPath,
				File:    relFile,
				Kind:    kind,
				TestRef: relFile + "::" + name,
				Doc:     doc,
			})
			return
		}
	}

	if !fn.Name.IsExported() {
		return
	}
	var receiver string
	if fn.Recv != nil && len(fn.Recv.List) > 0 {
		receiver = exprToString(fn.Recv.List[0].Type)
	}
	agg.exports = append(agg.exports, &SymbolInfo{
		Name:       name,
		Kind:       SymbolKindFunc,
		Package:    pkgPath,
		Doc:        doc,
		Receiver:   receiver,
		Signature:  funcSignature(fn),
		IsExported: true,
	})
}

func extractGenDecl(d *ast.GenDecl, agg *aggregator, pkgPath string) {
	for _, spec := range d.Specs {
		switch s := spec.(type) {
		case *ast.TypeSpec:
			if !s.Name.IsExported() {
				continue
			}
			kind := SymbolKindType
			switch s.Type.(type) {
			case *ast.InterfaceType:
				kind = SymbolKindInterface
			case *ast.StructType:
				kind = SymbolKindStruct
			}
			agg.exports = append(agg.exports, &SymbolInfo{
				Name:       s.Name.Name,
				Kind:       kind,
				Package:    pkgPath,
				Doc:        strings.TrimSpace(s.Doc.Text() + d.Doc.Text()),
				IsExported: true,
			})
		case *ast.ValueSpec:
			for _, n := range s.Names {
				if !n.IsExported() {
					continue
				}
				kind := SymbolKindVar
				if d.Tok == token.CONST {
					kind = SymbolKindConst
				}
				agg.exports = append(agg.exports, &SymbolInfo{
					Name:       n.Name,
					Kind:       kind,
					Package:    pkgPath,
					Doc:        strings.TrimSpace(s.Doc.Text() + d.Doc.Text()),
					IsExported: true,
				})
			}
		}
	}
}

// ── Layout detection ───────────────────────────────────────────────────

func detectLayout(root string) *LayoutHints {
	h := &LayoutHints{}
	dirExists := func(rel string) bool {
		info, err := os.Stat(filepath.Join(root, rel))
		return err == nil && info.IsDir()
	}
	h.HasPublicPkg = dirExists("public")
	h.HasInternalPkg = dirExists("internal")
	h.HasLocalPkg = dirExists("local")

	// Detect cmd/ + main.go. The §2.2 rule: a module is `sidecar`
	// when both cmd/main.go AND an `app/<name>/` parent are present
	// in the path. We don't have walk-up access to the parent
	// project, so we approximate using just the cmd/ presence.
	cmdMain := filepath.Join(root, "cmd", "main.go")
	if _, err := os.Stat(cmdMain); err == nil {
		h.HasAppCmd = true
	}
	// Some sidecars put main.go at the module root.
	if !h.HasAppCmd {
		if _, err := os.Stat(filepath.Join(root, "main.go")); err == nil {
			h.HasAppCmd = true
		}
	}

	// AppFolder — when the root path itself contains an `app/`
	// segment, record everything from there.
	parts := strings.Split(filepath.ToSlash(root), "/")
	for i, p := range parts {
		if p == "app" && i+1 < len(parts) {
			h.AppFolder = strings.Join(parts[i:i+2], "/") + "/"
			break
		}
	}

	switch {
	case h.HasAppCmd && h.AppFolder != "":
		h.SuggestedKind = "sidecar"
	case h.HasAppCmd:
		h.SuggestedKind = "sidecar"
	case h.HasPublicPkg && h.HasInternalPkg:
		h.SuggestedKind = "library"
	default:
		h.SuggestedKind = "unclear"
	}
	return h
}

// ── Helpers ────────────────────────────────────────────────────────────

var skipDirs = map[string]bool{
	"vendor":       true,
	"node_modules": true,
	".git":         true,
	".katyoti":     true,
	".index":       true,
	"testdata":     true, // standard Go convention — testdata isn't scanned
}

func shouldSkipDir(name string) bool {
	if skipDirs[name] {
		return true
	}
	// Dot-prefixed hidden dirs.
	return strings.HasPrefix(name, ".") && name != "." && name != ".."
}

func isGeneratedFileName(name string) bool {
	return strings.HasSuffix(name, "_gen.go") ||
		strings.HasSuffix(name, ".pb.go") ||
		strings.HasSuffix(name, "_generated.go") ||
		strings.Contains(name, ".generated.")
}

// isGeneratedHeader inspects file comments for the canonical
// "// Code generated …" marker per https://go.dev/s/generatedcode.
func isGeneratedHeader(f *ast.File) bool {
	for _, cg := range f.Comments {
		for _, c := range cg.List {
			text := strings.TrimSpace(strings.TrimPrefix(c.Text, "//"))
			if strings.HasPrefix(text, "Code generated ") && strings.Contains(text, "DO NOT EDIT") {
				return true
			}
		}
		// Only check the first comment group (top of file).
		break
	}
	return false
}

func isLocalImport(importPath, root string) bool {
	// Heuristic: import paths that begin with the repository's
	// module path are local. We can detect the module path by
	// reading go.mod near the root.
	modulePath := readModulePath(root)
	if modulePath == "" {
		return false
	}
	return strings.HasPrefix(importPath, modulePath)
}

// readModulePath walks up from root looking for go.mod and parses
// its `module <path>` line. Empty when not found.
func readModulePath(root string) string {
	abs, err := filepath.Abs(root)
	if err != nil {
		return ""
	}
	dir := abs
	for {
		modPath := filepath.Join(dir, "go.mod")
		if data, err := os.ReadFile(modPath); err == nil {
			for _, line := range strings.Split(string(data), "\n") {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "module ") {
					return strings.TrimSpace(strings.TrimPrefix(line, "module"))
				}
			}
			return ""
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

func testKindOf(funcName string) (TestKind, bool) {
	switch {
	case strings.HasPrefix(funcName, "Test"):
		return TestKindTest, true
	case strings.HasPrefix(funcName, "Benchmark"):
		return TestKindBenchmark, true
	case strings.HasPrefix(funcName, "Example"):
		return TestKindExample, true
	case strings.HasPrefix(funcName, "Fuzz"):
		return TestKindFuzz, true
	}
	return "", false
}

// exprToString stringifies a receiver expression (T or *T).
func exprToString(e ast.Expr) string {
	switch v := e.(type) {
	case *ast.Ident:
		return v.Name
	case *ast.StarExpr:
		return "*" + exprToString(v.X)
	case *ast.SelectorExpr:
		return exprToString(v.X) + "." + v.Sel.Name
	}
	return ""
}

// funcSignature returns a compact signature like `Foo(a int, b string) error`.
func funcSignature(fn *ast.FuncDecl) string {
	var b strings.Builder
	b.WriteString(fn.Name.Name)
	b.WriteByte('(')
	if fn.Type.Params != nil {
		for i, p := range fn.Type.Params.List {
			if i > 0 {
				b.WriteString(", ")
			}
			for j, n := range p.Names {
				if j > 0 {
					b.WriteString(", ")
				}
				b.WriteString(n.Name)
			}
			if len(p.Names) > 0 {
				b.WriteByte(' ')
			}
			b.WriteString(exprToString(p.Type))
		}
	}
	b.WriteByte(')')
	if fn.Type.Results != nil {
		switch len(fn.Type.Results.List) {
		case 0:
			// no return
		case 1:
			r := fn.Type.Results.List[0]
			if len(r.Names) == 0 {
				b.WriteByte(' ')
				b.WriteString(exprToString(r.Type))
			} else {
				b.WriteString(" (")
				b.WriteString(exprToString(r.Type))
				b.WriteByte(')')
			}
		default:
			b.WriteString(" (")
			for i, r := range fn.Type.Results.List {
				if i > 0 {
					b.WriteString(", ")
				}
				b.WriteString(exprToString(r.Type))
			}
			b.WriteByte(')')
		}
	}
	return b.String()
}

func sortInventory(inv *Inventory) {
	sort.Slice(inv.Packages, func(i, j int) bool {
		return inv.Packages[i].Path < inv.Packages[j].Path
	})
	sort.Slice(inv.PublicSymbols, func(i, j int) bool {
		if inv.PublicSymbols[i].Package != inv.PublicSymbols[j].Package {
			return inv.PublicSymbols[i].Package < inv.PublicSymbols[j].Package
		}
		return inv.PublicSymbols[i].Name < inv.PublicSymbols[j].Name
	})
	sort.Slice(inv.TestFunctions, func(i, j int) bool {
		return inv.TestFunctions[i].TestRef < inv.TestFunctions[j].TestRef
	})
	sort.Slice(inv.Imports, func(i, j int) bool {
		if inv.Imports[i].Importer != inv.Imports[j].Importer {
			return inv.Imports[i].Importer < inv.Imports[j].Importer
		}
		return inv.Imports[i].Imported < inv.Imports[j].Imported
	})
}
