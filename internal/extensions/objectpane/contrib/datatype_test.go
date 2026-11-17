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
	"github.com/katyoti/katyoti-fwk/internal/extensions/objectpane/api"
)

// typePane is the `pane` capability's type_schema, rendered from
// api.PaneSettings — the one source of the contract.
var typePane = datatypegen.Source{
	Name:    "type-pane",
	Version: "2.0.0",
	Root:    "TypePane",
	Origin:  "api.PaneSettings (internal/extensions/objectpane/api/panel.go)",
	Description: "The settings a type writes in its `pane:` section: the view of its\n" +
		"Information tab, the type's own header badges and its main tabs. Read by\n" +
		"the object pane.\n",
	AST: func() (*schemaapi.Type, error) { return ktwreflect.TypeAST[api.PaneSettings]() },
}

// TestTypePaneIsCurrent refuses a data-type that no longer says what
// PaneSettings says. KTW_DATATYPE_UPDATE=1 rewrites it.
func TestTypePaneIsCurrent(t *testing.T) {
	file := filepath.Join("embed", "cwm", "data-types", "type-pane.dsl.yaml")
	want, err := datatypegen.RenderAs(typePane, datatypegen.Header{
		Tool:       "the objectpane contrib test",
		Regenerate: "`KTW_DATATYPE_UPDATE=1 go test ./internal/extensions/objectpane/contrib/`",
		URI:        "ktw-agents:/data-type/type-pane",
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
		t.Errorf("%s is stale: %s changed — run with KTW_DATATYPE_UPDATE=1", file, typePane.Origin)
	}
}
