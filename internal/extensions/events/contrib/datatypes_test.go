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
	"github.com/katyoti/katyoti-fwk/internal/extensions/events/api"
)

// The pack's field-group data-types, each rendered from the Go type its
// service decodes it with — the one source of the contract.
var dataTypes = []datatypegen.Source{
	{
		Name:        "cap-timed",
		Version:     "1.0.0",
		Root:        "CapTimed",
		Origin:      "api.TimedFields (internal/extensions/events/api/sources.go)",
		Description: "Field-group for the `timed` capability: an object's recurring timers, each raising timer.elapsed on it when due.\n",
		AST:         func() (*schemaapi.Type, error) { return ktwreflect.TypeAST[api.TimedFields]() },
	},
}

// TestDataTypesAreCurrent refuses a data-type that no longer says what its Go
// type says. KTW_DATATYPE_UPDATE=1 rewrites them.
func TestDataTypesAreCurrent(t *testing.T) {
	for _, src := range dataTypes {
		file := "embed/cwm/data-types/" + src.Name + ".dsl.yaml"
		want, err := datatypegen.RenderAs(src, datatypegen.Header{
			Tool:       "the events contrib test",
			Regenerate: "`KTW_DATATYPE_UPDATE=1 go test ./internal/extensions/events/contrib/`",
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
