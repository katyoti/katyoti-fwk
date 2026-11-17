// SPDX-License-Identifier: Apache-2.0

package cwmapp

import (
	"bytes"
	"context"
	"testing"

	schemaparser "github.com/katyoti/katyoti-fwk/internal/core/schema/parser"
	"github.com/santhosh-tekuri/jsonschema/v5"
)

// The project's `contributions` field-group (cap-contributions, generated from
// projectfile.Contributions) holds its two fields: a provider token and a
// folder pass, either may be left to its default, a value of the wrong shape
// is refused.
func TestContributionsIsHeldByItsSchema(t *testing.T) {
	compiled, err := schemaparser.CompileJSONSchema(context.Background(),
		ShippedFile(t, "internal/extensions/projectcontributions/contrib/embed/cwm/data-types/cap-contributions.dsl.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	c := jsonschema.NewCompiler()
	if err := c.AddResource("cap-contributions", bytes.NewReader(compiled)); err != nil {
		t.Fatal(err)
	}
	sch, err := c.Compile("cap-contributions")
	if err != nil {
		t.Fatal(err)
	}
	for name, v := range map[string]map[string]any{
		"both":     {"provider": "acme", "root": "packs/acme"},
		"defaults": {},
	} {
		if err := sch.Validate(v); err != nil {
			t.Errorf("%s is refused: %v", name, err)
		}
	}
	if err := sch.Validate(map[string]any{"provider": 3}); err == nil {
		t.Error("a provider that is a number is accepted: the schema does not hold the field")
	}
}

// The `system-module` field-group (cap-system-module, generated from
// cwmapi.SystemModule) holds what a system module operates: a capability's
// name, or nothing; a value of the wrong shape is refused. That the name is a
// declared capability is CWM's check (TestASystemModuleOperatesADeclaredCapability).
func TestSystemModuleIsHeldByItsSchema(t *testing.T) {
	compiled, err := schemaparser.CompileJSONSchema(context.Background(),
		ShippedFile(t, "internal/extensions/cwm/contrib/embed/cwm/data-types/cap-system-module.dsl.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	c := jsonschema.NewCompiler()
	if err := c.AddResource("cap-system-module", bytes.NewReader(compiled)); err != nil {
		t.Fatal(err)
	}
	sch, err := c.Compile("cap-system-module")
	if err != nil {
		t.Fatal(err)
	}
	for name, v := range map[string]map[string]any{
		"operates": {"operates": "contributions"},
		"a tag":    {},
	} {
		if err := sch.Validate(v); err != nil {
			t.Errorf("%s is refused: %v", name, err)
		}
	}
	if err := sch.Validate(map[string]any{"operates": []any{"contributions"}}); err == nil {
		t.Error("operates as a list is accepted: the schema does not hold the field")
	}
}
