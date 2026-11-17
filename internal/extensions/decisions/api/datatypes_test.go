// SPDX-License-Identifier: Apache-2.0

package api_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/contribution/datatypegen"
	ktwreflect "github.com/katyoti/katyoti-fwk/internal/core/reflect"
	schemaapi "github.com/katyoti/katyoti-fwk/internal/core/schema/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/decisions/api"
)

const regen = "run `KTW_REGEN_DATATYPES=1 go test ./internal/extensions/decisions/api/ -run TestDecisionsDataTypes`"

// decisionsDataTypes are rendered from the Go types into the ktw-methods pack
// (app/ktw-methods/embed/cwm/data-types).
var decisionsDataTypes = []datatypegen.Source{
	{
		Name: "evaluation-grid", Version: "1.0.0", Root: "EvaluationGrid",
		Origin:      "api.EvaluationGrid (internal/extensions/decisions/api/evaluation.go)",
		Description: "What alternatives are judged on: weighted criteria and the scale\nthey are scored on.\n",
		AST:         func() (*schemaapi.Type, error) { return ktwreflect.TypeAST[api.EvaluationGrid]() },
	},
	{
		Name: "solution-evaluation", Version: "1.0.0", Root: "SolutionEvaluation",
		Origin:      "api.SolutionEvaluation (internal/extensions/decisions/api/evaluation.go)",
		Description: "One alternative scored on a grid: a score per criterion with its\njustification and evidence, and the computed total.\n",
		AST:         func() (*schemaapi.Type, error) { return ktwreflect.TypeAST[api.SolutionEvaluation]() },
	},
}

// TestDecisionsDataTypesAreCurrent refuses a stale pack data-type.
func TestDecisionsDataTypesAreCurrent(t *testing.T) {
	dir := filepath.Join("..", "..", "..", "..", "app", "ktw-methods", "embed", "cwm", "data-types")
	for _, src := range decisionsDataTypes {
		want, err := datatypegen.RenderAs(src, datatypegen.Header{
			Tool: "TestDecisionsDataTypesAreCurrent", Regenerate: regen, URI: "ktw-methods:/data-type/" + src.Name,
		})
		if err != nil {
			t.Fatalf("%s: %v", src.Name, err)
		}
		p := filepath.Join(dir, src.Name+".dsl.yaml")
		if os.Getenv("KTW_REGEN_DATATYPES") == "1" {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(p, want, 0o644); err != nil {
				t.Fatal(err)
			}
			continue
		}
		got, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("%v (%s)", err, regen)
		}
		if strings.ReplaceAll(string(got), "\r\n", "\n") != string(want) {
			t.Errorf("%s is stale — the Go type is the source; %s", p, regen)
		}
	}
}
