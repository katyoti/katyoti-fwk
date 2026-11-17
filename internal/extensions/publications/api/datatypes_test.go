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
	"github.com/katyoti/katyoti-fwk/internal/extensions/publications/api"
)

const regen = "run `KTW_REGEN_DATATYPES=1 go test ./internal/extensions/publications/api/ -run TestPublicationsDataTypes`"

// publicationsDataTypes are rendered from the Go types into the publications
// pack (contrib/embed/cwm/data-types), published by the binary hosting it.
// The publication-connector data-type is not one: the kind's leaf publishes
// it from api.ConnectorDecl (contrib/publisherkind).
var publicationsDataTypes = []datatypegen.Source{
	{
		Name: "cap-publication", Version: "2.0.0", Root: "CapPublication",
		Origin:      "api.PublicationFields (internal/extensions/publications/api/publication.go)",
		Description: "Field-group for the `publication` capability: what it publishes - a\nmanagement view or a document-tree source object - its target - a publication\nconnector or a view's converter - with its settings, and the last outcome.\n",
		AST:         func() (*schemaapi.Type, error) { return ktwreflect.TypeAST[api.PublicationFields]() },
	},
	{
		Name: "document-tree", Version: "1.0.0", Root: "DocumentTree",
		Origin:      "api.DocumentTree (internal/extensions/publications/api/publications.go)",
		Description: "What a publisher publishes: markdown documents and groups in order,\nand what the tree is.\n",
		AST:         func() (*schemaapi.Type, error) { return ktwreflect.TypeAST[api.DocumentTree]() },
		Named: []datatypegen.NamedType{
			{Name: "Document", Description: "One markdown document of a tree.",
				AST: func() (*schemaapi.Type, error) { return ktwreflect.TypeAST[api.Document]() }},
			{Name: "Asset", Description: "A file a document shows or attaches.",
				AST: func() (*schemaapi.Type, error) { return ktwreflect.TypeAST[api.Asset]() }},
		},
	},
}

// TestPublicationsDataTypesAreCurrent refuses a stale pack data-type.
func TestPublicationsDataTypesAreCurrent(t *testing.T) {
	dir := filepath.Join("..", "contrib", "embed", "cwm", "data-types")
	for _, src := range publicationsDataTypes {
		want, err := datatypegen.RenderAs(src, datatypegen.Header{
			Tool: "TestPublicationsDataTypesAreCurrent", Regenerate: regen, URI: "ktw-agents:/data-type/" + src.Name,
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
