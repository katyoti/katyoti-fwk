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
	"github.com/katyoti/katyoti-fwk/internal/extensions/demosignoff/api"
)

const regen = "run `KTW_REGEN_DATATYPES=1 go test ./internal/extensions/demosignoff/api/ -run TestSignoffDataTypes`"

// signoffDataTypes are the data-types of ktw-demo's pack the sign-off service
// decodes with a Go type, rendered from that type into
// app/ktw-demo/embed/cwm/data-types/ — so the form, CWM's check and the
// service read one declaration.
var signoffDataTypes = []struct {
	folder string
	src    datatypegen.Source
}{
	{"", datatypegen.Source{
		Name: "cap-signoff", Version: "1.0.0", Root: "CapSignoff",
		Origin:      "api.SignoffGroup (internal/extensions/demosignoff/api/signoff.go)",
		Description: "The sign-off capability's field-group on an object: who reviews it, who\napproved, the verdict and the notes.\n",
		AST:         func() (*schemaapi.Type, error) { return ktwreflect.TypeAST[api.SignoffGroup]() },
	}},
	{"", datatypegen.Source{
		Name: "type-signoff", Version: "1.0.0", Root: "TypeSignoff",
		Origin:      "api.SignoffSettings (internal/extensions/demosignoff/api/signoff.go)",
		Description: "The sign-off capability's section on a type: how many approvals its objects\nneed.\n",
		AST:         func() (*schemaapi.Type, error) { return ktwreflect.TypeAST[api.SignoffSettings]() },
	}},
	{"", datatypegen.Source{
		Name: "signoff-item", Version: "1.0.0", Root: "SignoffItem",
		Origin:      "api.ItemFields (internal/extensions/demosignoff/api/signoff.go)",
		Description: "The signoff-item type's own fields.\n",
		AST:         func() (*schemaapi.Type, error) { return ktwreflect.TypeAST[api.ItemFields]() },
	}},
	{"", datatypegen.Source{
		Name: "signoff-reviewer", Version: "1.0.0", Root: "ReviewerRow",
		Origin:      "api.ReviewerRow (internal/extensions/demosignoff/api/signoff.go)",
		Description: "One row of the Sign-off panel's reviewers table.\n",
		AST:         func() (*schemaapi.Type, error) { return ktwreflect.TypeAST[api.ReviewerRow]() },
	}},
	{"", datatypegen.Source{
		Name: "signoff-verdict", Version: "1.0.0", Root: "VerdictForm",
		Origin:      "api.VerdictForm (internal/extensions/demosignoff/api/signoff.go)",
		Description: "The Sign-off panel's verdict form: the verdict and the notes.\n",
		AST:         func() (*schemaapi.Type, error) { return ktwreflect.TypeAST[api.VerdictForm]() },
	}},
	{"", datatypegen.Source{
		Name: "contest-row", Version: "1.0.0", Root: "ContestRow",
		Origin:      "api.ContestRow (internal/extensions/demosignoff/api/signoff.go)",
		Description: "One row of the Contests panel: an object that contests this one.\n",
		AST:         func() (*schemaapi.Type, error) { return ktwreflect.TypeAST[api.ContestRow]() },
	}},
}

// TestSignoffDataTypesAreCurrent refuses a stale pack data-type.
func TestSignoffDataTypesAreCurrent(t *testing.T) {
	dir := filepath.Join("..", "..", "..", "..", "app", "ktw-demo", "embed", "cwm", "data-types")
	for _, d := range signoffDataTypes {
		want, err := datatypegen.RenderAs(d.src, datatypegen.Header{
			Tool: "TestSignoffDataTypesAreCurrent", Regenerate: regen, URI: "ktw-demo:/data-type/" + d.src.Name,
		})
		if err != nil {
			t.Fatalf("%s: %v", d.src.Name, err)
		}
		path := filepath.Join(dir, d.folder, d.src.Name+".dsl.yaml")
		if os.Getenv("KTW_REGEN_DATATYPES") == "1" {
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
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
