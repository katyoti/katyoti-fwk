// SPDX-License-Identifier: Apache-2.0

package census

import (
	"strings"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
)

// The system-provider guard accepts exactly one registration per system type
// plus a system kind declaration, and names both failure shapes: a missing
// carrier and a duplicate one. (A binary that loses the carrier entirely is
// ALSO refused earlier, by the builder — "undeclared contribution kind(s)" —
// but a binary publishing only data-types would not be, so the guard does not
// lean on that.)
func TestSystemProviderGuard_OnceEachAndADeclaration(t *testing.T) {
	kindDecl := contribapi.Contribution{Provider: SystemProvider, Type: contribapi.ResourceTypeContributionType, ID: "skill"}
	once := []mbapi.ProviderInfo{
		{Name: SystemProvider, Type: contribapi.ResourceTypeContributionType, Digest: "sha256:x"},
		{Name: SystemProvider, Type: contribapi.ResourceTypeDataType, Digest: "sha256:x"},
		{Name: "ktw-weather", Type: "skill", Digest: "sha256:x"},
	}

	if bad := SystemProviderFindings(&mbapi.Manifest{Providers: once, Contributions: []contribapi.Contribution{kindDecl}}); len(bad) != 0 {
		t.Fatalf("a correct carrier was refused: %v", bad)
	}

	missing := SystemProviderFindings(&mbapi.Manifest{Providers: once[2:]})
	if len(missing) != 3 || !strings.Contains(strings.Join(missing, "\n"), "registered 0 time(s)") ||
		!strings.Contains(strings.Join(missing, "\n"), "no system:/contribution-type/ entry") {
		t.Fatalf("missing carrier: findings = %v, want both types at 0 and no declaration", missing)
	}

	doubled := append(append([]mbapi.ProviderInfo{}, once...), once[0], once[1])
	dup := SystemProviderFindings(&mbapi.Manifest{Providers: doubled, Contributions: []contribapi.Contribution{kindDecl}})
	if len(dup) != 2 || !strings.Contains(strings.Join(dup, "\n"), "registered 2 time(s)") {
		t.Fatalf("duplicate carrier: findings = %v, want both types at 2", dup)
	}
}
