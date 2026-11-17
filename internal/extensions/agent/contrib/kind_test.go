// SPDX-License-Identifier: Apache-2.0

package contrib

import (
	"encoding/json"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/config"
	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	"github.com/katyoti/katyoti-fwk/internal/core/marshal"
	"github.com/katyoti/katyoti-fwk/internal/extensions/agent/api/permrules"
)

// The permission-profile kind is executable (decision D9 of the contribution
// study): a profile widens an agent's rights, so the override
// gate - which asks contribapi.Kinds for the kind's execution class - refuses
// an untrusted origin's override of one. The declaration the agent ships,
// read through Kinds, must answer executable.
func TestThePermissionProfileKindIsExecutable(t *testing.T) {
	raw, err := kindFS.ReadFile("embed/contribution-types/permission-profile.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := config.DecodeBytes(raw, marshal.YAML, &body); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	decl := contribapi.Contribution{Provider: "ktw-agents", Type: contribapi.ResourceTypeContributionType, ID: permrules.ContributionTypePermissionProfile}
	decl.SetData(data)
	kinds := contribapi.KindsFrom([]contribapi.Contribution{decl})
	if got := kinds.Execution(permrules.ContributionTypePermissionProfile); got != contribapi.ExecutionExecutable {
		t.Fatalf("permission-profile execution = %q; want %q, so an untrusted project's override of a profile is refused", got, contribapi.ExecutionExecutable)
	}
}
