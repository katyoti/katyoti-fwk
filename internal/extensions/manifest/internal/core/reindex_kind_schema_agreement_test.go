// SPDX-License-Identifier: Apache-2.0

package core

import (
	"strings"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
)

// A pack that declares its kind is carried by several binaries: each declares
// the kind under its own provider and ships the schema it names. The
// declarations agreeing makes one kind; the catalog, which sees the
// data-types, reports schemas whose documents differ (binaries carrying
// different versions of the pack), naming both.
func TestReindex_OneKindFromTwoBinariesNeedsTheSameSchema(t *testing.T) {
	member := func(provider, dsl string) []contribapi.Contribution {
		dt := contribapi.Contribution{Provider: provider, Type: contribapi.ResourceTypeDataType, ID: "recipe"}
		dt.SetSource([]byte(dsl), "application/yaml")
		return []contribapi.Contribution{memberKindDecl(provider, "recipe", provider+":/data-type/recipe", "error"), dt}
	}
	same := twoMemberCatalog(t, contribapi.ContributionsConfig{}, "bin-a", "bin-b",
		member("bin-a", "name: recipe\n"), member("bin-b", "name: recipe\n"))
	same.mu.Lock()
	kinds := same.kindsLocked()
	got := same.schemaDisagreementsLocked(kinds)
	same.mu.Unlock()
	for _, f := range kinds.Check() {
		if f.Rule == contribapi.KindRuleDuplicate {
			t.Errorf("same pack in two binaries: %v, want one kind", f)
		}
	}
	if len(got) != 0 {
		t.Fatalf("same pack in two binaries: schema findings %v, want none", got)
	}

	differ := twoMemberCatalog(t, contribapi.ContributionsConfig{}, "bin-a", "bin-b",
		member("bin-a", "name: recipe\n"), member("bin-b", "name: recipe\nversion: 2.0.0\n"))
	differ.mu.Lock()
	got = differ.schemaDisagreementsLocked(differ.kindsLocked())
	differ.mu.Unlock()
	if len(got) != 1 || !strings.Contains(got[0], "bin-a:/data-type/recipe") || !strings.Contains(got[0], "bin-b:/data-type/recipe") {
		t.Errorf("differing schemas: findings %v, want one naming both data-types", got)
	}
}
