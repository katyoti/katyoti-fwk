// SPDX-License-Identifier: Apache-2.0

package core

import (
	"go/ast"
	"go/parser"
	"go/token"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/api"
)

// Every reason placeProject answers - the code a form says in its language -
// is in CreateProjectOutput.Reason's enum and named in its description
// : a reason the contract does not list reaches the form as
// the hub's raw English sentence. The reasons are read from placeProject's
// own return statements, so a new one cannot be added without the contract,
// and the enum lists nothing placeProject does not answer.
func TestEveryCreateProjectReasonIsInTheContract(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "project.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var reasons []string
	ast.Inspect(file, func(n ast.Node) bool {
		fn, ok := n.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "placeProject" {
			return true
		}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			ret, ok := n.(*ast.ReturnStmt)
			if !ok || len(ret.Results) != 3 {
				return true
			}
			if lit, ok := ret.Results[1].(*ast.BasicLit); ok && lit.Kind == token.STRING {
				if r, _ := strconv.Unquote(lit.Value); r != "" {
					reasons = append(reasons, r)
				}
			}
			return true
		})
		return false
	})
	if len(reasons) < 4 {
		t.Fatalf("placeProject's reasons read: %v; want at least the four it answers", reasons)
	}
	field, _ := reflect.TypeOf(api.CreateProjectOutput{}).FieldByName("Reason")
	enum := strings.Split(field.Tag.Get("enum"), ",")
	desc := field.Tag.Get("desc")
	for _, r := range reasons {
		if !slices.Contains(enum, r) {
			t.Errorf("placeProject answers reason %q, missing from CreateProjectOutput.Reason's enum %v", r, enum)
		}
		if !strings.Contains(desc, r) {
			t.Errorf("placeProject answers reason %q, not named in CreateProjectOutput.Reason's description %q", r, desc)
		}
	}
	// And no more: a code the form translates but placeProject never answers
	// is dead contract (dropped inside-project so).
	for _, r := range enum {
		if !slices.Contains(reasons, r) {
			t.Errorf("CreateProjectOutput.Reason's enum lists %q, which placeProject never answers", r)
		}
	}
}
