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
	"github.com/katyoti/katyoti-fwk/internal/extensions/sources/api"
)

const regen = "run `KTW_REGEN_DATATYPES=1 go test ./internal/extensions/sources/api/ -run TestSourcesDataTypes`"

// sourcesDataTypes are rendered from the Go types into the sources pack
// (contrib/embed/cwm/data-types), published by the binary hosting it. The
// source-connector data-type is not one: the kind's leaf publishes it from
// connectordecl.ConnectorDecl (contrib/connectorkind).
var sourcesDataTypes = []datatypegen.Source{
	{
		Name: "information-source", Version: "2.0.0", Root: "InformationSourceFields",
		Origin:      "api.InformationSourceFields (internal/extensions/sources/api/connectors.go)",
		Description: "A place a module reads from: its connector, where and what it reads,\nwhen it is read again, and what the last sync brought.\n",
		AST:         func() (*schemaapi.Type, error) { return ktwreflect.TypeAST[api.InformationSourceFields]() },
	},
	{
		Name: "cap-knowledge", Version: "2.0.0", Root: "CapKnowledge",
		Origin:      "api.CapKnowledge (internal/extensions/sources/api/knowledge.go)",
		Description: "The knowledge capability's fields on its carrier: how its information\nsources read what they bring.\n",
		AST:         func() (*schemaapi.Type, error) { return ktwreflect.TypeAST[api.CapKnowledge]() },
	},
}

// TestSourcesDataTypesAreCurrent refuses a stale pack data-type.
func TestSourcesDataTypesAreCurrent(t *testing.T) {
	dir := filepath.Join("..", "contrib", "embed", "cwm", "data-types")
	for _, src := range sourcesDataTypes {
		want, err := datatypegen.RenderAs(src, datatypegen.Header{
			Tool: "TestSourcesDataTypesAreCurrent", Regenerate: regen, URI: "ktw-agents:/data-type/" + src.Name,
		})
		if err != nil {
			t.Fatalf("%s: %v", src.Name, err)
		}
		p := filepath.Join(dir, src.Name+".dsl.yaml")
		if os.Getenv("KTW_REGEN_DATATYPES") == "1" {
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
