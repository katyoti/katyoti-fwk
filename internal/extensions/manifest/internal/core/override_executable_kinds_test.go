// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/config"
	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	"github.com/katyoti/katyoti-fwk/internal/core/marshal"
)

// executableKindFiles are the declarations, as shipped, of the kinds whose
// content runs, names something to run or widens rights (decision D9 of the
// contribution study, widened by)
// relative to the repository root.
var executableKindFiles = map[string]string{
	"hook":               "internal/core/contribution/embed/contribution-type/hook.yaml",
	"workflow":           "internal/core/contribution/embed/contribution-type/workflow.yaml",
	"policy":             "internal/core/contribution/embed/contribution-type/policy.yaml",
	"gate-requirement":   "internal/core/contribution/embed/contribution-type/gate-requirement.yaml",
	"capability":         "internal/core/contribution/embed/contribution-type/capability.yaml",
	"permission-profile": "internal/extensions/agent/contrib/embed/contribution-types/permission-profile.yaml",
	"skill":              "internal/extensions/skills/contrib/skillkind/skill.yaml",
	"object-type":        "internal/core/contribution/embed/contribution-type/object-type.yaml",
	"display-engine":     "internal/core/contribution/embed/contribution-type/display-engine.yaml",
	"config-program":     "internal/core/contribution/embed/contribution-type/config-program.yaml",
}

// shippedKind reads a kind's declaration file into a contribution-type entry.
func shippedKind(t *testing.T, rel string) contribapi.Contribution {
	t.Helper()
	_, here, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(here), "..", "..", "..", "..", "..")
	raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
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
	name, _ := body["name"].(string)
	c := contribapi.Contribution{Provider: "system", Type: contribapi.ResourceTypeContributionType, ID: name}
	c.SetData(data)
	return c
}

// An untrusted project's override of an entry of each executable kind is
// refused with a finding, read from the kind's declaration as shipped; a
// trusted project's override of the same entry applies.
func TestAnOverrideOfAnExecutableKind(t *testing.T) {
	for kind, file := range executableKindFiles {
		decl := shippedKind(t, file)
		if decl.ID != kind {
			t.Fatalf("%s declares kind %q, want %q", file, decl.ID, kind)
		}
		base := contribapi.Contribution{Provider: "ktw-x", Type: kind, ID: "one"}
		base.SetData(json.RawMessage(`{"name":"one","description":"One."}`))
		base.SetLayer("source", []byte("as shipped"), "text/markdown")
		uri := "ktw-x:/" + kind + "/one"
		for _, trusted := range []bool{false, true} {
			project := overrideEntryProvider{
				staticProvider: staticProvider{provider: "acme", typ: kind},
				ovs:            []contribapi.OverrideEntry{overrideOf(uri, kind, "one", trusted, map[string]string{"source": "overridden"})},
			}
			m, err := NewBuilder("ktw-x", "dev").BuildFromProviders(context.Background(), []contribapi.ContributionProvider{
				staticProvider{provider: "system", typ: contribapi.ResourceTypeContributionType, entries: []contribapi.Contribution{decl}},
				staticProvider{provider: "ktw-x", typ: kind, entries: []contribapi.Contribution{base}},
				project,
			})
			if err != nil {
				t.Fatal(err)
			}
			var hash string
			for _, c := range m.Contributions {
				if c.ResolvedURI() == uri {
					hash = c.Layers["source"].Hash
				}
			}
			refused := false
			for _, d := range m.Drops {
				if d.Path == "acme:/"+kind+"/one" && strings.Contains(d.Reason, "does not trust") {
					refused = true
				}
			}
			want := contribapi.HashContent([]byte("as shipped"))
			if trusted {
				want = contribapi.HashContent([]byte("overridden"))
			}
			if hash != want || refused == trusted {
				t.Errorf("%s, trusted=%v: source applied=%v, refused with a finding=%v; drops %+v", kind, trusted, hash != contribapi.HashContent([]byte("as shipped")), refused, m.Drops)
			}
		}
	}
}
