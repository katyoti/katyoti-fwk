// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"
	"go/ast"
	"go/doc"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/extensions/code/api"
)

// codeService implements api.CodeService.
type codeService struct {
	cfg CodeServiceConfig
}

func NewCodeService(_ context.Context, cfg *CodeServiceConfig) (api.CodeService, error) {
	return &codeService{cfg: *cfg}, nil
}

func (s *codeService) AnalyzeFolder(_ context.Context, input api.AnalyzeFolderInput) (api.AnalyzeFolderOutput, error) {
	root := input.Path
	if root == "" {
		return api.AnalyzeFolderOutput{}, fmt.Errorf("path is required")
	}

	exts := input.Extensions
	if len(exts) == 0 {
		exts = s.cfg.DefaultExtensions
	}
	extSet := make(map[string]bool, len(exts))
	for _, e := range exts {
		extSet[e] = true
	}

	langMap := make(map[string]*api.Language)
	dirMap := make(map[string]*api.DirSummary)
	totalFiles := 0
	totalLines := 0

	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip errors
		}
		if info.IsDir() {
			name := info.Name()
			for _, excl := range s.cfg.ExcludeDirs {
				if name == excl {
					return filepath.SkipDir
				}
			}
			if input.MaxDepth > 0 {
				rel, _ := filepath.Rel(root, path)
				depth := strings.Count(filepath.ToSlash(rel), "/") + 1
				if depth > input.MaxDepth {
					return filepath.SkipDir
				}
			}
			return nil
		}

		ext := filepath.Ext(path)
		if len(extSet) > 0 && !extSet[ext] {
			return nil
		}
		if info.Size() > int64(s.cfg.MaxFileSize) {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		lines := strings.Count(string(data), "\n") + 1
		totalFiles++
		totalLines += lines

		// Language stats
		lang := langFromExt(ext)
		if l, ok := langMap[lang]; ok {
			l.FileCount++
			l.LineCount += lines
		} else {
			langMap[lang] = &api.Language{Name: lang, Extension: ext, FileCount: 1, LineCount: lines}
		}

		// Directory stats
		dir := filepath.ToSlash(filepath.Dir(path))
		rel, _ := filepath.Rel(root, dir)
		if rel == "" || rel == "." {
			rel = "."
		}
		rel = filepath.ToSlash(rel)
		if d, ok := dirMap[rel]; ok {
			d.FileCount++
		} else {
			dirMap[rel] = &api.DirSummary{Path: rel, FileCount: 1}
		}

		return nil
	})

	langs := make([]api.Language, 0, len(langMap))
	for _, l := range langMap {
		langs = append(langs, *l)
	}
	dirs := make([]api.DirSummary, 0, len(dirMap))
	for _, d := range dirMap {
		dirs = append(dirs, *d)
	}

	return api.AnalyzeFolderOutput{
		RootPath:    root,
		TotalFiles:  totalFiles,
		TotalLines:  totalLines,
		Languages:   langs,
		Directories: dirs,
	}, nil
}

func (s *codeService) ExtractStructure(_ context.Context, input api.ExtractStructureInput) (api.ExtractStructureOutput, error) {
	path := input.Path
	if path == "" {
		return api.ExtractStructureOutput{}, fmt.Errorf("path is required")
	}

	info, err := os.Stat(path)
	if err != nil {
		return api.ExtractStructureOutput{}, fmt.Errorf("stat %s: %w", path, err)
	}

	var packages []api.PackageInfo
	if info.IsDir() {
		packages, err = extractGoPackages(path, input.Recursive)
	} else {
		pkg, e := extractGoFile(path)
		if e != nil {
			return api.ExtractStructureOutput{}, e
		}
		packages = []api.PackageInfo{pkg}
		err = nil
	}
	if err != nil {
		return api.ExtractStructureOutput{}, err
	}

	return api.ExtractStructureOutput{Packages: packages}, nil
}

func (s *codeService) ExtractDependencies(_ context.Context, input api.ExtractDependenciesInput) (api.ExtractDependenciesOutput, error) {
	path := input.Path
	if path == "" {
		return api.ExtractDependenciesOutput{}, fmt.Errorf("path is required")
	}

	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, path, nil, parser.ImportsOnly)
	if err != nil {
		return api.ExtractDependenciesOutput{}, fmt.Errorf("parse %s: %w", path, err)
	}

	importCount := make(map[string]int)
	for _, pkg := range pkgs {
		for _, file := range pkg.Files {
			for _, imp := range file.Imports {
				p := strings.Trim(imp.Path.Value, `"`)
				importCount[p]++
			}
		}
	}

	imports := make([]api.ImportInfo, 0, len(importCount))
	for pkg, count := range importCount {
		imports = append(imports, api.ImportInfo{Package: pkg, Count: count})
	}

	return api.ExtractDependenciesOutput{Imports: imports}, nil
}

// --- Go AST helpers ---

func extractGoPackages(dir string, recursive bool) ([]api.PackageInfo, error) {
	var results []api.PackageInfo

	process := func(d string) error {
		fset := token.NewFileSet()
		pkgs, err := parser.ParseDir(fset, d, nil, parser.ParseComments)
		if err != nil {
			return nil // skip unparseable dirs
		}
		for name, pkg := range pkgs {
			if strings.HasSuffix(name, "_test") {
				continue
			}
			info := goPackageToInfo(d, pkg, fset)
			results = append(results, info)
		}
		return nil
	}

	if !recursive {
		return results, process(dir)
	}

	return results, filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || !info.IsDir() {
			return nil
		}
		name := info.Name()
		if name == "vendor" || name == "node_modules" || name == ".git" || name == "testdata" {
			return filepath.SkipDir
		}
		return process(path)
	})
}

func goPackageToInfo(dir string, pkg *ast.Package, fset *token.FileSet) api.PackageInfo {
	dp := doc.New(pkg, dir, doc.AllDecls)

	info := api.PackageInfo{
		Name: dp.Name,
		Path: filepath.ToSlash(dir),
		Doc:  strings.TrimSpace(dp.Doc),
	}

	for _, t := range dp.Types {
		ti := api.TypeInfo{Name: t.Name, Doc: strings.TrimSpace(t.Doc)}
		info.Types = append(info.Types, ti)

		for _, m := range t.Methods {
			info.Functions = append(info.Functions, api.FuncInfo{
				Name:      m.Name,
				Doc:       firstLine(m.Doc),
				Signature: formatFuncDecl(fset, m.Decl),
				Receiver:  t.Name,
			})
		}

		for _, f := range t.Funcs {
			info.Functions = append(info.Functions, api.FuncInfo{
				Name:      f.Name,
				Doc:       firstLine(f.Doc),
				Signature: formatFuncDecl(fset, f.Decl),
			})
		}
	}

	for _, f := range dp.Funcs {
		info.Functions = append(info.Functions, api.FuncInfo{
			Name:      f.Name,
			Doc:       firstLine(f.Doc),
			Signature: formatFuncDecl(fset, f.Decl),
		})
	}

	return info
}

func extractGoFile(path string) (api.PackageInfo, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		return api.PackageInfo{}, fmt.Errorf("parse %s: %w", path, err)
	}

	info := api.PackageInfo{
		Name: file.Name.Name,
		Path: filepath.ToSlash(path),
	}

	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			fi := api.FuncInfo{
				Name:      d.Name.Name,
				Signature: formatFuncDecl(fset, d),
			}
			if d.Recv != nil && len(d.Recv.List) > 0 {
				fi.Receiver = formatExpr(d.Recv.List[0].Type)
			}
			if d.Doc != nil {
				fi.Doc = firstLine(d.Doc.Text())
			}
			info.Functions = append(info.Functions, fi)
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				if ts, ok := spec.(*ast.TypeSpec); ok {
					ti := api.TypeInfo{Name: ts.Name.Name}
					if d.Doc != nil {
						ti.Doc = firstLine(d.Doc.Text())
					}
					if iface, ok := ts.Type.(*ast.InterfaceType); ok {
						ii := api.IfaceInfo{Name: ts.Name.Name, Doc: ti.Doc}
						for _, m := range iface.Methods.List {
							if len(m.Names) > 0 {
								ii.Methods = append(ii.Methods, m.Names[0].Name)
							}
						}
						info.Interfaces = append(info.Interfaces, ii)
					} else {
						info.Types = append(info.Types, ti)
					}
				}
			}
		}
	}

	return info, nil
}

func formatFuncDecl(fset *token.FileSet, decl *ast.FuncDecl) string {
	if decl == nil || decl.Type == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("func ")
	if decl.Recv != nil && len(decl.Recv.List) > 0 {
		b.WriteString("(")
		b.WriteString(formatExpr(decl.Recv.List[0].Type))
		b.WriteString(") ")
	}
	b.WriteString(decl.Name.Name)
	b.WriteString("(")
	if decl.Type.Params != nil {
		b.WriteString(formatFieldList(decl.Type.Params))
	}
	b.WriteString(")")
	if decl.Type.Results != nil && len(decl.Type.Results.List) > 0 {
		results := formatFieldList(decl.Type.Results)
		if len(decl.Type.Results.List) > 1 || len(decl.Type.Results.List[0].Names) > 0 {
			b.WriteString(" (")
			b.WriteString(results)
			b.WriteString(")")
		} else {
			b.WriteString(" ")
			b.WriteString(results)
		}
	}
	return b.String()
}

func formatFieldList(fl *ast.FieldList) string {
	var parts []string
	for _, f := range fl.List {
		typ := formatExpr(f.Type)
		if len(f.Names) == 0 {
			parts = append(parts, typ)
		} else {
			for _, n := range f.Names {
				parts = append(parts, n.Name+" "+typ)
			}
		}
	}
	return strings.Join(parts, ", ")
}

func formatExpr(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.StarExpr:
		return "*" + formatExpr(e.X)
	case *ast.SelectorExpr:
		return formatExpr(e.X) + "." + e.Sel.Name
	case *ast.ArrayType:
		return "[]" + formatExpr(e.Elt)
	case *ast.MapType:
		return "map[" + formatExpr(e.Key) + "]" + formatExpr(e.Value)
	case *ast.InterfaceType:
		return "interface{}"
	case *ast.Ellipsis:
		return "..." + formatExpr(e.Elt)
	case *ast.FuncType:
		return "func(...)"
	case *ast.ChanType:
		return "chan " + formatExpr(e.Value)
	default:
		return "any"
	}
}

func langFromExt(ext string) string {
	switch ext {
	case ".go":
		return "Go"
	case ".ts", ".tsx":
		return "TypeScript"
	case ".js", ".jsx":
		return "JavaScript"
	case ".py":
		return "Python"
	case ".rs":
		return "Rust"
	case ".java":
		return "Java"
	case ".md":
		return "Markdown"
	case ".yaml", ".yml":
		return "YAML"
	case ".json":
		return "JSON"
	case ".sh", ".bash":
		return "Shell"
	default:
		return ext
	}
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
