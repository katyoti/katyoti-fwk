// SPDX-License-Identifier: Apache-2.0

package contribution

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// packOnlyKind is the kind declared by the files-only fixture
// (internal/test/fixtures/contributions/files-only-pack). It exists ONLY there.
const packOnlyKind = "recipe"

// kindAsCode matches a string literal that USES the kind: the bare kind name,
// or a URI segment naming it (`<p>:/recipe/…`, `…/contribution-type/recipe`,
// `…/data-type/recipe`). Prose that merely contains the English word — a
// comment, `ktw-my-recipes` — is not a use of the kind.
var kindAsCode = regexp.MustCompile(`(^|[:/])` + packOnlyKind + `($|/)`)

// TestPackKindNeedsNoFrameworkCode is e19's no-framework-diff guard: the
// files-only fixture's kind is listed, opened and validated (app/ktw-host/cmd
// TestHostServesAKindItHasNoCodeFor, internal/test/catalog
// TestFilesOnlyKindThroughTheUniverse) while no non-test Go file under
// internal/ or app/ names it in code. If this fails, the proof that "a new kind
// is a declaration plus a provider" has been bought with framework code.
func TestPackKindNeedsNoFrameworkCode(t *testing.T) {
	_, here, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(here), "..", "..", "..")
	var hits []string
	scanned := 0
	for _, top := range []string{"internal", "app"} {
		err := filepath.WalkDir(filepath.Join(root, top), func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				switch d.Name() {
				case "node_modules", "frontend", ".git":
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
				return nil
			}
			scanned++
			hits = append(hits, kindLiteralsIn(t, root, p)...)
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", top, err)
		}
	}
	if scanned < 500 {
		t.Fatalf("scanned only %d Go files — the walk is not looking at the tree", scanned)
	}
	if len(hits) > 0 {
		t.Errorf("the pack-only kind %q is named in framework code (%d place(s)):\n  %s\n"+
			"a kind a pack declares must need no Go code — serve it through the generic provider",
			packOnlyKind, len(hits), strings.Join(hits, "\n  "))
	}
}

func kindLiteralsIn(t *testing.T, root, path string) []string {
	t.Helper()
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if !strings.Contains(string(src), packOnlyKind) {
		return nil // cheap pre-filter: most files never mention the word
	}
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, src, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	var out []string
	ast.Inspect(f, func(n ast.Node) bool {
		lit, ok := n.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		v, err := strconv.Unquote(lit.Value)
		if err != nil {
			v = lit.Value
		}
		if kindAsCode.MatchString(v) {
			rel, _ := filepath.Rel(root, path)
			out = append(out, filepath.ToSlash(rel)+":"+strconv.Itoa(fset.Position(lit.Pos()).Line)+" "+lit.Value)
		}
		return true
	})
	return out
}
