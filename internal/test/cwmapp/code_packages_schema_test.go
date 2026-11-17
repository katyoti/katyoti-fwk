// SPDX-License-Identifier: Apache-2.0

package cwmapp

import (
	"bytes"
	"context"
	"testing"

	schemaparser "github.com/katyoti/katyoti-fwk/internal/core/schema/parser"
	gcapi "github.com/katyoti/katyoti-fwk/internal/extensions/code/api"
	"github.com/santhosh-tekuri/jsonschema/v5"
)

// code_packages is DECLARED: the shipped cap-code schema (: a
// module's code is the code capability's), compiled as CWM
// compiles it, accepts a list of package paths and refuses any other shape. A
// field read by the manifest and verify that the schema did not hold would
// drift unseen, which is how `facets` came to be read and never written.
func TestCodePackagesIsHeldByTheCapCodeSchema(t *testing.T) {
	compiled, err := schemaparser.CompileJSONSchema(context.Background(),
		ShippedFile(t, "app/ktw-code/embed/cwm/data-types/cap-code.dsl.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	c := jsonschema.NewCompiler()
	if err := c.AddResource("cap-code", bytes.NewReader(compiled)); err != nil {
		t.Fatal(err)
	}
	sch, err := c.Compile("cap-code")
	if err != nil {
		t.Fatal(err)
	}
	if err := sch.Validate(map[string]any{gcapi.CodePackagesField: []any{"internal/extensions/m/"}}); err != nil {
		t.Errorf("a list of packages is refused: %v", err)
	}
	if err := sch.Validate(map[string]any{gcapi.CodePackagesField: "internal/extensions/m/"}); err == nil {
		t.Error("a code_packages that is a string, not a list, is accepted: the schema does not hold the field")
	}
}
