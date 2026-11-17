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
	docsapi "github.com/katyoti/katyoti-fwk/internal/extensions/docs/api"
)

const regen = "run `KTW_REGEN_DATATYPES=1 go test ./internal/extensions/docs/api/ -run TestDocsDataTypes`"

// docsDataTypes are rendered from the Go types into the ktw-docs pack
// (app/ktw-docs/embed/data-types).
var docsDataTypes = []datatypegen.Source{
	{
		Name: "content-template", Version: "1.0.0", Root: "ContentTemplate",
		Origin:      "docsapi.ContentTemplate (internal/extensions/docs/api/content_template.go)",
		Description: "What a deliverable must say whatever its format: ordered sections with\ntheir intent and sources and guidance. A partial template is a section group\nother templates include.\n",
		AST:         func() (*schemaapi.Type, error) { return ktwreflect.TypeAST[docsapi.ContentTemplate]() },
	},
	{
		Name: "cap-deliverable", Version: "1.0.0", Root: "CapDeliverable",
		Origin:      "docsapi.CapDeliverable (internal/extensions/docs/api/deliverable.go)",
		Description: "A document written for someone: what wrote it, for whom, in which\nlanguage and format, and where it stands.\n",
		AST:         func() (*schemaapi.Type, error) { return ktwreflect.TypeAST[docsapi.CapDeliverable]() },
	},
}

// TestDocsDataTypesAreCurrent refuses a stale pack data-type.
func TestDocsDataTypesAreCurrent(t *testing.T) {
	dir := filepath.Join("..", "..", "..", "..", "app", "ktw-docs", "embed", "data-types")
	for _, src := range docsDataTypes {
		want, err := datatypegen.RenderAs(src, datatypegen.Header{
			Tool: "TestDocsDataTypesAreCurrent", Regenerate: regen, URI: "ktw-docs:/data-type/" + src.Name,
		})
		if err != nil {
			t.Fatalf("%s: %v", src.Name, err)
		}
		path := filepath.Join(dir, src.Name+".dsl.yaml")
		if os.Getenv("KTW_REGEN_DATATYPES") == "1" {
			if err := os.WriteFile(path, want, 0o644); err != nil {
				t.Fatal(err)
			}
			continue
		}
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("%v (%s)", err, regen)
		}
		if strings.ReplaceAll(string(got), "\r\n", "\n") != string(want) {
			t.Errorf("%s is stale — the Go type is the source; %s", path, regen)
		}
	}
}
