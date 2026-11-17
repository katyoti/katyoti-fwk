// SPDX-License-Identifier: Apache-2.0

package sidecarkit

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/contribution/builtin"
)

// TestKitBranchesOnNoKind guards the kit's shape: the sidecar shell wires no
// kind's runtime by a switch. A kind's runtime (the skills runtime, …) is the
// binary's own wiring, passed with WithDomain; the kit neither imports a
// kind's extension package nor offers a With<Kind>/Without<Kind> option nor
// keeps a <kind>Enabled flag. The kinds are the ones the binary declares
// (builtin.Kinds), so a kind added later is covered without editing the test.
func TestKitBranchesOnNoKind(t *testing.T) {
	kinds, err := builtin.Kinds()
	if err != nil {
		t.Fatalf("built-in kinds: %v", err)
	}
	if kinds.Len() == 0 {
		t.Fatal("built-in kinds: none declared, the guard would prove nothing")
	}
	forms := map[string]string{} // a name form → its kind
	for _, k := range kinds.Names() {
		base := strings.ReplaceAll(k, "-", "")
		forms[base] = k
		forms[base+"s"] = k
	}

	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	parsed := 0
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		f, err := parser.ParseFile(fset, name, src, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		parsed++
		for _, imp := range f.Imports {
			p, _ := strconv.Unquote(imp.Path.Value)
			_, rest, ok := strings.Cut(p, "/internal/extensions/")
			if !ok {
				continue
			}
			first, _, _ := strings.Cut(rest, "/")
			if k, hit := forms[strings.ReplaceAll(first, "-", "")]; hit {
				t.Errorf("%s imports %s: the kit wires the runtime of kind %q; the binary passes it with WithDomain", fset.Position(imp.Pos()), p, k)
			}
		}
		ast.Inspect(f, func(n ast.Node) bool {
			id, ok := n.(*ast.Ident)
			if !ok {
				return true
			}
			low := strings.ToLower(id.Name)
			for _, cut := range []string{"without", "with"} {
				if rest, ok := strings.CutPrefix(low, cut); ok {
					if k, hit := forms[rest]; hit {
						t.Errorf("%s: %s switches the runtime of kind %q on or off in the kit", fset.Position(id.Pos()), id.Name, k)
					}
					break
				}
			}
			if rest, ok := strings.CutSuffix(low, "enabled"); ok {
				if k, hit := forms[rest]; hit {
					t.Errorf("%s: %s is a switch on the runtime of kind %q in the kit", fset.Position(id.Pos()), id.Name, k)
				}
			}
			return true
		})
	}
	if parsed == 0 {
		t.Fatal("no source file of the kit parsed: the guard would prove nothing")
	}
}
