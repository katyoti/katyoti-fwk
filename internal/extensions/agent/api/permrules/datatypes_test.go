// SPDX-License-Identifier: Apache-2.0

package permrules_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/contribution/datatypegen"
	ktwreflect "github.com/katyoti/katyoti-fwk/internal/core/reflect"
	schemaapi "github.com/katyoti/katyoti-fwk/internal/core/schema/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/agent/api/permrules"
)

const regen = "run `KTW_REGEN_DATATYPES=1 go test ./internal/extensions/agent/api/permrules/ -run TestPermissionProfileDataTypeIsCurrent`"

// TestPermissionProfileDataTypeIsCurrent refuses a stale permission-profile
// data-type: the Go type permrules.ProfileSpec is its one source.
func TestPermissionProfileDataTypeIsCurrent(t *testing.T) {
	src := datatypegen.Source{
		Name: "permission-profile", Version: "1.0.0", Root: "ProfileSpec",
		Origin:      "permrules.ProfileSpec (internal/extensions/agent/api/permrules/profile_yaml.go)",
		Description: "A permission profile: the rules that pre-approve or refuse an agent's\ntool calls before they would ask a person.\n",
		AST:         func() (*schemaapi.Type, error) { return ktwreflect.TypeAST[permrules.ProfileSpec]() },
	}
	want, err := datatypegen.RenderAs(src, datatypegen.Header{
		Tool: "TestPermissionProfileDataTypeIsCurrent", Regenerate: regen, URI: "ktw-agents:/data-type/" + src.Name,
	})
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join("..", "..", "contrib", "embed", "data-types", src.Name+".dsl.yaml")
	if os.Getenv("KTW_REGEN_DATATYPES") == "1" {
		if err := os.WriteFile(p, want, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	got, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("%v (%s)", err, regen)
	}
	if strings.ReplaceAll(string(got), "\r\n", "\n") != string(want) {
		t.Errorf("%s is stale - the Go type is the source; %s", p, regen)
	}
}
