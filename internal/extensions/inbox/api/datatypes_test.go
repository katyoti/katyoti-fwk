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
	"github.com/katyoti/katyoti-fwk/internal/extensions/inbox/api"
)

const regen = "run `KTW_REGEN_DATATYPES=1 go test ./internal/extensions/inbox/api/ -run TestInboxDataTypes`"

// inboxDataTypes are rendered from the Go types into the inbox pack
// (contrib/embed/cwm/data-types), published by the binary hosting it.
var inboxDataTypes = []datatypegen.Source{
	{
		Name: "ask-kind", Version: "1.0.0", Root: "AskKind",
		Origin:      "api.AskKind (internal/extensions/inbox/api/humantask.go)",
		Description: "A kind of question a person is asked: its short label, the shape of\nits answer, who may answer it and what its answer settles, by default.\n",
		AST:         func() (*schemaapi.Type, error) { return ktwreflect.TypeAST[api.AskKind]() },
	},
	{
		Name: "holding-list", Version: "1.0.0", Root: "HoldingListFields",
		Origin:      "api.HoldingListFields (internal/extensions/inbox/api/humantask.go)",
		Description: "A holding list: who may answer the questions it holds - persons and\nroles and agents.\n",
		AST:         func() (*schemaapi.Type, error) { return ktwreflect.TypeAST[api.HoldingListFields]() },
	},
	{
		Name: "permission-answer", Version: "1.0.0", Root: "PermissionAnswer",
		Origin:      "api.PermissionAnswer (internal/extensions/inbox/api/humantask.go)",
		Description: "The answer of a permission question: allow or deny - once or always -\nand the rule an always keeps.\n",
		AST:         func() (*schemaapi.Type, error) { return ktwreflect.TypeAST[api.PermissionAnswer]() },
	},
	{
		Name: "answer", Version: "1.0.0", Root: "AnswerRecord",
		Origin:      "api.AnswerRecord (internal/extensions/inbox/api/humantask.go)",
		Description: "An answer kept on what it is about: what was asked, what was answered,\nby whom and with what proof.\n",
		AST:         func() (*schemaapi.Type, error) { return ktwreflect.TypeAST[api.AnswerRecord]() },
	},
}

// TestInboxDataTypesAreCurrent refuses a stale pack data-type.
func TestInboxDataTypesAreCurrent(t *testing.T) {
	dir := filepath.Join("..", "contrib", "embed", "cwm", "data-types")
	for _, src := range inboxDataTypes {
		want, err := datatypegen.RenderAs(src, datatypegen.Header{
			Tool: "TestInboxDataTypesAreCurrent", Regenerate: regen, URI: "ktw-agents:/data-type/" + src.Name,
		})
		if err != nil {
			t.Fatalf("%s: %v", src.Name, err)
		}
		p := filepath.Join(dir, src.Name+".dsl.yaml")
		if os.Getenv("KTW_REGEN_DATATYPES") == "1" {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
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
			t.Errorf("%s is stale - the Go type is the source; %s", p, regen)
		}
	}
}
