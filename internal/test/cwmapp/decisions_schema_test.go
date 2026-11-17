// SPDX-License-Identifier: Apache-2.0

package cwmapp

import (
	"bytes"
	"context"
	"testing"

	schemaparser "github.com/katyoti/katyoti-fwk/internal/core/schema/parser"
	"github.com/santhosh-tekuri/jsonschema/v5"
)

// decisions is DECLARED on the module capability: a module names its open
// decisions (one id or a list) or says `none`. The schema keeps `none` and a
// list valid and refuses any other shape, so silence (no field) and "none"
// stay two distinct answers.
func TestDecisionsIsHeldByTheCapModuleSchema(t *testing.T) {
	compiled, err := schemaparser.CompileJSONSchema(context.Background(),
		ShippedFile(t, "app/ktw-methods/embed/cwm/data-types/cap-module.dsl.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	c := jsonschema.NewCompiler()
	if err := c.AddResource("cap-module", bytes.NewReader(compiled)); err != nil {
		t.Fatal(err)
	}
	sch, err := c.Compile("cap-module")
	if err != nil {
		t.Fatal(err)
	}
	for name, v := range map[string]any{
		"none":   "none",
		"one id": "OD-003",
		"a list": []any{"OD-003", "OD-004"},
	} {
		if err := sch.Validate(map[string]any{"decisions": v}); err != nil {
			t.Errorf("decisions as %s is refused: %v", name, err)
		}
	}
	if err := sch.Validate(map[string]any{}); err != nil {
		t.Errorf("a module that says nothing about its decisions is refused: %v", err)
	}
	if err := sch.Validate(map[string]any{"decisions": 3}); err == nil {
		t.Error("a decisions that is a number is accepted: the schema does not hold the field")
	}
}
