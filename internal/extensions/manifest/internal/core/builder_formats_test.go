// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"strings"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
)

// The builder derives the formats a binary writes — the data-type major.minor
// of every kind its own entries are of — into its sidecar-info.
func TestBuilder_DerivesFormatsIntoSidecarInfo(t *testing.T) {
	kinds := kindProvider(contribapi.KindDeclaration{Name: "fm-kind", Schema: "ktw-test:/data-type/x", Validate: "warn",
		Layers: map[string]contribapi.LayerDecl{"data": {Default: true, Required: true}}, Meta: &contribapi.MetaDecl{Budget: 1024}})
	dt := contribapi.Contribution{Provider: "ktw-test", Type: contribapi.ResourceTypeDataType, ID: "x"}
	dt.SetSource([]byte("name: x\nversion: \"2.3.0\"\ntypes: {}\n"), "application/yaml")
	info := contribapi.Contribution{Provider: "ktw-test", Type: contribapi.SidecarInfoKind, ID: "default"}
	info.SetData([]byte(`{"name":"ktw-test","version":"0","description":"d"}`))
	entry := contribapi.Contribution{Provider: "ktw-test", Type: "fm-kind", ID: "e"}
	entry.SetData([]byte(`{"name":"e"}`))

	m, err := NewBuilder("ktw-test", "0").BuildFromProviders(context.Background(), []contribapi.ContributionProvider{kinds,
		&residencyProvider{typ: contribapi.ResourceTypeDataType, entries: []contribapi.Contribution{dt}},
		&residencyProvider{typ: contribapi.SidecarInfoKind, entries: []contribapi.Contribution{info}},
		&residencyProvider{typ: "fm-kind", entries: []contribapi.Contribution{entry}}})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if got := contribapi.MemberFormats(m.Contributions); got["fm-kind"] != "2.3" {
		t.Fatalf("sidecar-info formats = %v, want fm-kind: 2.3", got)
	}
}

// The hub reads a member's entry by its format: a newer major refuses it
// saying to update the hub; a newer minor keeps it valid, its schema findings
// marked tolerated; the same format keeps the verdict.
func TestCatalog_JudgesAnEntryByItsMembersFormat(t *testing.T) {
	cn := contribapi.Contribution{Provider: "ktw-new", Type: "hook", ID: "h"}
	host := map[string]string{"hook": "1.0"}
	invalid := false
	v := entryVerdict{valid: &invalid, errs: []string{"colour: unknown property"}}

	if got := judgeFormat(v, "ktw-new", cn, host, map[string]string{"hook": "1.0"}); got.valid == nil || *got.valid {
		t.Errorf("same format must stay strict: %+v", got)
	}
	got := judgeFormat(v, "ktw-new", cn, host, map[string]string{"hook": "1.1"})
	if got.valid == nil || !*got.valid || len(got.errs) != 1 || !strings.HasPrefix(got.errs[0], "tolerated") {
		t.Errorf("a newer minor must be valid with the finding marked tolerated: %+v", got)
	}
	ok := true
	got = judgeFormat(entryVerdict{valid: &ok}, "ktw-new", cn, host, map[string]string{"hook": "2.0"})
	if got.valid == nil || *got.valid || len(got.errs) != 1 || !strings.Contains(got.errs[0], "update the hub") {
		t.Errorf("a newer major must be refused saying to update the hub: %+v", got)
	}
}
