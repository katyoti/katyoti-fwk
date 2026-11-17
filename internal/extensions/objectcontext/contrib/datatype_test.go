// SPDX-License-Identifier: Apache-2.0

package contrib

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/contribution/datatypegen"
	ktwreflect "github.com/katyoti/katyoti-fwk/internal/core/reflect"
	schemaapi "github.com/katyoti/katyoti-fwk/internal/core/schema/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/objectcontext/api"
)

// typeContext is the `context` capability's type_schema, rendered from
// api.ContextSettings — the one source of the contract.
var typeContext = datatypegen.Source{
	Name:    "type-context",
	Version: "1.0.0",
	Root:    "TypeContext",
	Origin:  "api.ContextSettings (internal/extensions/objectcontext/api/context_section.go)",
	Description: "The settings a type writes in its `context:` section: how objects of the\n" +
		"type are gathered across the tree and briefed, and what every object of the\n" +
		"type carries. Read by object-context.\n",
	AST: func() (*schemaapi.Type, error) { return ktwreflect.TypeAST[api.ContextSettings]() },
}

// TestTypeContextIsCurrent refuses a data-type that no longer says what
// ContextSettings says. KTW_DATATYPE_UPDATE=1 rewrites it.
func TestTypeContextIsCurrent(t *testing.T) {
	file := filepath.Join("embed", "cwm", "data-types", "type-context.dsl.yaml")
	want, err := datatypegen.RenderAs(typeContext, datatypegen.Header{
		Tool:       "the objectcontext contrib test",
		Regenerate: "`KTW_DATATYPE_UPDATE=1 go test ./internal/extensions/objectcontext/contrib/`",
		URI:        "ktw-agents:/data-type/type-context",
	})
	if err != nil {
		t.Fatal(err)
	}
	if os.Getenv("KTW_DATATYPE_UPDATE") == "1" {
		if err := os.WriteFile(file, want, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	got, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("%v — run with KTW_DATATYPE_UPDATE=1", err)
	}
	if !bytes.Equal(bytes.ReplaceAll(got, []byte("\r\n"), []byte("\n")), want) {
		t.Errorf("%s is stale: %s changed — run with KTW_DATATYPE_UPDATE=1", file, typeContext.Origin)
	}
}
