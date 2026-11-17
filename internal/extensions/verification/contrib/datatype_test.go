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
	"github.com/katyoti/katyoti-fwk/internal/extensions/verification/api"
)

// typeVerification is the `verification` capability's type_schema, rendered from
// api.VerificationSettings — the one source of the contract.
var typeVerification = datatypegen.Source{
	Name:    "type-verification",
	Version: "1.0.0",
	Root:    "TypeVerification",
	Origin:  "api.VerificationSettings (internal/extensions/verification/api/section.go)",
	Description: "The settings a type writes in its `verification:` section: the lowest\n" +
		"lint severity that blocks. Read by the verification service.\n",
	AST: func() (*schemaapi.Type, error) { return ktwreflect.TypeAST[api.VerificationSettings]() },
}

// TestTypeVerificationIsCurrent refuses a data-type that no longer says what
// VerificationSettings says. KTW_DATATYPE_UPDATE=1 rewrites it.
func TestTypeVerificationIsCurrent(t *testing.T) {
	file := filepath.Join("embed", "cwm", "data-types", "type-verification.dsl.yaml")
	want, err := datatypegen.RenderAs(typeVerification, datatypegen.Header{
		Tool:       "the verification contrib test",
		Regenerate: "`KTW_DATATYPE_UPDATE=1 go test ./internal/extensions/verification/contrib/`",
		URI:        "ktw-agents:/data-type/type-verification",
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
		t.Errorf("%s is stale: %s changed — run with KTW_DATATYPE_UPDATE=1", file, typeVerification.Origin)
	}
}
