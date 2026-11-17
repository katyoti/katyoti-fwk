// SPDX-License-Identifier: Apache-2.0

package contrib

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/contribution/datatypegen"
	"github.com/katyoti/katyoti-fwk/internal/core/projectfile"
	ktwreflect "github.com/katyoti/katyoti-fwk/internal/core/reflect"
	schemaapi "github.com/katyoti/katyoti-fwk/internal/core/schema/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/projectcontributions/api"
)

// The `contributions` capability's field-group, rendered from the Go type its
// one reader decodes it with (projectfile.Contributions).
var capContributions = datatypegen.Source{
	Name:    "cap-contributions",
	Version: "1.0.0",
	Root:    "CapContributions",
	Origin:  "projectfile.Contributions (internal/core/projectfile/projectfile.go)",
	Description: "Field-group for the `contributions` capability of a project: the provider\n" +
		"token its contributions are published under and their folder.\n",
	AST: func() (*schemaapi.Type, error) { return ktwreflect.TypeAST[projectfile.Contributions]() },
}

// The `contributes` capability's field-group, rendered from the Go type its
// one reader decodes it with (api.Contributes).
var capContributes = datatypegen.Source{
	Name:    "cap-contributes",
	Version: "1.0.0",
	Root:    "CapContributes",
	Origin:  "api.Contributes (internal/extensions/projectcontributions/api/contributes.go)",
	Description: "Field-group for the `contributes` capability: the object published as a\n" +
		"contribution entry - its kind, its id, where each layer comes from and the\n" +
		"entry it overrides.\n",
	AST: func() (*schemaapi.Type, error) { return ktwreflect.TypeAST[api.Contributes]() },
}

// TestTheFieldGroupIsCurrent refuses a data-type that no longer says what its
// Go type says. KTW_DATATYPE_UPDATE=1 rewrites it.
func TestTheFieldGroupIsCurrent(t *testing.T) {
	for _, src := range []datatypegen.Source{capContributions, capContributes} {
		t.Run(src.Name, func(t *testing.T) { fieldGroupIsCurrent(t, src) })
	}
}

func fieldGroupIsCurrent(t *testing.T, src datatypegen.Source) {
	file := filepath.Join("embed", "cwm", "data-types", src.Name+".dsl.yaml")
	want, err := datatypegen.RenderAs(src, datatypegen.Header{
		Tool:       "the projectcontributions contrib test",
		Regenerate: "`KTW_DATATYPE_UPDATE=1 go test ./internal/extensions/projectcontributions/contrib/`",
		URI:        "ktw-agents:/data-type/" + src.Name,
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
		t.Errorf("%s is stale: %s changed — run with KTW_DATATYPE_UPDATE=1", file, src.Origin)
	}
}
