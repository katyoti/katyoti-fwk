// SPDX-License-Identifier: Apache-2.0

package core

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/marshal"
)

// The inspect verb is generic by construction, and these guards keep it so
// (ms-contribution-ext e22): no lint library, no kind-owning package, no kind
// named in code. Inspection is not lint — CLAUDE.md "Content lint, three
// places, one library" — and a verb that learned one kind would be the first
// step to a kind switch.

const modulePrefix = "github.com/katyoti/katyoti-fwk/"

// lintBaseline is every `*/lint` package the compose extension imported before
// the inspect verb existed: the content-lint library behind the lint verbs.
var lintBaseline = map[string]bool{
	modulePrefix + "internal/extensions/manifest/lint": true,
}

// inspectFiles are the files the verb is made of, relative to the extension.
var inspectFiles = []string{"api/inspect.go", "internal/core/inspect.go", "local/inspect_command.go"}

// inspectAllowed is everything the verb may import beyond the standard
// library: the contribution contract, the consumer surface, its own extension,
// the application, and its rendering.
var inspectAllowed = map[string]bool{
	modulePrefix + "internal/core/application":                 true,
	modulePrefix + "internal/core/contribution/api":            true,
	modulePrefix + "internal/extensions/manifest/api":          true,
	modulePrefix + "internal/extensions/compose/api":           true,
	modulePrefix + "internal/extensions/compose/internal/core": true,
	"github.com/spf13/cobra":                                   true,
	"gopkg.in/yaml.v3":                                         true,
}

func composeRoot(t *testing.T) string {
	t.Helper()
	return filepath.Join(repoRootDir(t), "internal", "extensions", "compose")
}

func importsOf(t *testing.T, path string) (*ast.File, []string) {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly|parser.ParseComments)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	var out []string
	for _, s := range f.Imports {
		p, _ := strconv.Unquote(s.Path.Value)
		out = append(out, p)
	}
	return f, out
}

func isLintPkg(p string) bool { return strings.HasSuffix(p, "/lint") || strings.Contains(p, "/lint/") }

// TestInspect_ImportsNoLintBeyondTheBaseline: across the whole extension, the
// only lint package is the one the lint verbs already used — and the verb's own
// files import none.
func TestInspect_ImportsNoLintBeyondTheBaseline(t *testing.T) {
	root := composeRoot(t)
	scanned := 0
	var extra []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return err
		}
		scanned++
		_, imps := importsOf(t, p)
		for _, imp := range imps {
			if isLintPkg(imp) && !lintBaseline[imp] {
				rel, _ := filepath.Rel(root, p)
				extra = append(extra, filepath.ToSlash(rel)+" imports "+imp)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if scanned < 10 {
		t.Fatalf("scanned %d files under %s — the walk is not looking at the extension", scanned, root)
	}
	if len(extra) > 0 {
		t.Errorf("a lint package beyond the lint verbs' baseline:\n  %s", strings.Join(extra, "\n  "))
	}
}

// TestInspect_VerbIsKindAgnostic: the verb's files import only the contract
// and the consumer surface (no kind-owning extension, no lint), and name no
// kind the framework declares in a string literal.
func TestInspect_VerbIsKindAgnostic(t *testing.T) {
	root := composeRoot(t)
	kinds := declaredKindNames(t)
	var bad []string
	for _, rel := range inspectFiles {
		_, imps := importsOf(t, filepath.Join(root, filepath.FromSlash(rel)))
		for _, imp := range imps {
			std := !strings.Contains(strings.SplitN(imp, "/", 2)[0], ".")
			if !std && !inspectAllowed[imp] {
				bad = append(bad, rel+" imports "+imp+" (not the contribution contract or the consumer surface)")
			}
			if isLintPkg(imp) {
				bad = append(bad, rel+" imports lint package "+imp)
			}
		}
		full, err := parser.ParseFile(token.NewFileSet(), filepath.Join(root, filepath.FromSlash(rel)), nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(full, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			if s, err := strconv.Unquote(lit.Value); err == nil && kinds[s] {
				bad = append(bad, rel+" names kind "+strconv.Quote(s)+" in code")
			}
			return true
		})
	}
	if len(bad) > 0 {
		sort.Strings(bad)
		t.Errorf("the inspect verb must answer every kind through contribapi.Kinds:\n  %s", strings.Join(bad, "\n  "))
	}
}

// declaredKindNames reads the name of every kind the framework ships.
func declaredKindNames(t *testing.T) map[string]bool {
	t.Helper()
	dir := filepath.Join(repoRootDir(t), "internal", "core", "contribution", "embed", "contribution-type")
	files, err := filepath.Glob(filepath.Join(dir, "*.yaml"))
	if err != nil || len(files) < 10 {
		t.Fatalf("found %d kind declarations under %s (%v)", len(files), dir, err)
	}
	out := map[string]bool{}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		var decl struct {
			Name string `yaml:"name"`
		}
		if err := marshal.Unmarshal(b, marshal.YAML, &decl); err != nil || decl.Name == "" {
			t.Fatalf("%s: no kind name (%v)", f, err)
		}
		out[decl.Name] = true
	}
	return out
}
