// SPDX-License-Identifier: Apache-2.0

package arch

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// confluenceSPIWriteRE is a request that writes in a version-specific
// provider of the restapi client.
var confluenceSPIWriteRE = regexp.MustCompile(`"(POST|PUT|DELETE)"|http\.Method(Post|Put|Delete)|uploadAttachmentDataV1\(`)

// restapiWriteGuardProblems checks the receipt guard inside the restapi
// client: the Client holds a *guardedProvider; every
// provider method that writes (its v1 or v2 implementation sends POST, PUT
// or DELETE) is declared by guardedProvider, checks the guard (allowPage or
// allowCreate) and records the write; and no file calls a write of the
// wrapped provider around it.
func restapiWriteGuardProblems(t *testing.T, dir string) []string {
	t.Helper()
	type method struct{ file, body string }
	methods := map[string]map[string]method{} // receiver type -> name -> body
	files := map[string]string{}
	clientProvider := ""
	fset := token.NewFileSet()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		n := e.Name()
		if !strings.HasSuffix(n, ".go") || strings.HasSuffix(n, "_test.go") {
			continue
		}
		src, err := os.ReadFile(filepath.Join(dir, n))
		if err != nil {
			t.Fatal(err)
		}
		files[n] = string(src)
		f, err := parser.ParseFile(fset, n, src, 0)
		if err != nil {
			t.Fatal(err)
		}
		text := func(a, b token.Pos) string { return string(src[fset.Position(a).Offset:fset.Position(b).Offset]) }
		ast.Inspect(f, func(node ast.Node) bool {
			switch d := node.(type) {
			case *ast.FuncDecl:
				if d.Recv == nil || len(d.Recv.List) == 0 || d.Body == nil {
					return false
				}
				recv := d.Recv.List[0].Type
				if star, ok := recv.(*ast.StarExpr); ok {
					recv = star.X
				}
				if id, ok := recv.(*ast.Ident); ok {
					if methods[id.Name] == nil {
						methods[id.Name] = map[string]method{}
					}
					methods[id.Name][d.Name.Name] = method{file: n, body: text(d.Body.Pos(), d.Body.End())}
				}
				return false
			case *ast.TypeSpec:
				if st, ok := d.Type.(*ast.StructType); ok && d.Name.Name == "Client" {
					for _, fl := range st.Fields.List {
						for _, nm := range fl.Names {
							if nm.Name == "provider" {
								clientProvider = text(fl.Type.Pos(), fl.Type.End())
							}
						}
					}
				}
			}
			return true
		})
	}
	var problems []string
	if clientProvider != "*guardedProvider" {
		problems = append(problems, "the Client's provider is "+clientProvider+", not *guardedProvider")
	}
	writes := map[string]bool{}
	for _, spi := range []string{"spiV1", "spiV2"} {
		for name, m := range methods[spi] {
			if confluenceSPIWriteRE.MatchString(m.body) {
				writes[name] = true
			}
		}
	}
	if len(writes) < 8 {
		problems = append(problems, "fewer writing provider methods found in spiV1/spiV2 than Confluence has - the check reads too little: "+strconv.Itoa(len(writes)))
	}
	for name := range writes {
		m, ok := methods["guardedProvider"][name]
		switch {
		case !ok:
			problems = append(problems, "guardedProvider does not guard the write "+name)
		case !strings.Contains(m.body, "p.allowPage(") && !strings.Contains(m.body, "p.allowCreate("):
			problems = append(problems, "guardedProvider."+name+" writes without checking the receipt guard")
		case !strings.Contains(m.body, "p.record("):
			problems = append(problems, "guardedProvider."+name+" writes without journaling it")
		}
		around := regexp.MustCompile(`\.Provider\.` + name + `\(`)
		for n, src := range files {
			for i, line := range strings.Split(src, "\n") {
				if !around.MatchString(line) {
					continue
				}
				if ok && m.file == n && strings.Contains(m.body, strings.TrimSpace(line)) {
					continue
				}
				problems = append(problems, n+":"+strconv.Itoa(i+1)+": calls the write "+name+" around the guard")
			}
		}
	}
	return problems
}
