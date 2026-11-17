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
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// CWM's own field-group data-types, each rendered from the Go type its readers
// decode it with — the one source of the contract.
var dataTypes = []datatypegen.Source{
	{
		Name:    "cap-project",
		Version: "2.2.0",
		Root:    "CapProject",
		Origin:  "projectfile.Project (internal/core/projectfile/projectfile.go)",
		Description: "Field-group for the `project` capability: the project's one computer id,\n" +
			"the folder that holds its modules, what the object scan leaves out and\n" +
			"the version of the shape of its data.\n" +
			"No path: a path is machine-local.\n",
		AST: func() (*schemaapi.Type, error) { return ktwreflect.TypeAST[projectfile.Project]() },
	},
	{
		Name:    "cap-system-module",
		Version: "1.0.0",
		Root:    "CapSystemModule",
		Origin:  "api.SystemModule (internal/extensions/cwm/api/system_module.go)",
		Description: "Field-group for the `system-module` capability: the capability whose\n" +
			"operational space the module is, when it operates one.\n",
		AST: func() (*schemaapi.Type, error) { return ktwreflect.TypeAST[api.SystemModule]() },
	},
}

// TestDataTypesAreCurrent refuses a data-type that no longer says what its Go
// type says. KTW_DATATYPE_UPDATE=1 rewrites them.
func TestDataTypesAreCurrent(t *testing.T) {
	for _, src := range dataTypes {
		file := "embed/cwm/data-types/" + src.Name + ".dsl.yaml"
		want, err := datatypegen.RenderAs(src, datatypegen.Header{
			Tool:       "the cwm contrib test",
			Regenerate: "`KTW_DATATYPE_UPDATE=1 go test ./internal/extensions/cwm/contrib/`",
			URI:        "ktw-agents:/data-type/" + src.Name,
		})
		if err != nil {
			t.Fatal(err)
		}
		if os.Getenv("KTW_DATATYPE_UPDATE") == "1" {
			if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(file, want, 0o644); err != nil {
				t.Fatal(err)
			}
			continue
		}
		got, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("%v — run with KTW_DATATYPE_UPDATE=1", err)
		}
		if !bytes.Equal(bytes.ReplaceAll(got, []byte("\r\n"), []byte("\n")), want) {
			t.Errorf("%s is stale: %s changed — run with KTW_DATATYPE_UPDATE=1", file, src.Origin)
		}
	}
}
