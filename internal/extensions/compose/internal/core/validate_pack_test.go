// SPDX-License-Identifier: Apache-2.0

package core

import (
	"encoding/json"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/authoring"
)

// A kind's severity comes from the kind's own declaration, wherever it lives:
// the pack carries its own kinds, and the binary carries the kinds the pack
// uses without declaring — skill, declared by the binary's skills pack under
// the binary's own provider. Both arrive as ordinary contributions, so one
// registry reads both; the same skill declaration carried by the pack too is
// one kind, the binary's, and no finding.
func TestKindRegistryReadsKindsFromEitherSide(t *testing.T) {
	kind := func(provider, id, body string) contribapi.Contribution {
		return contribapi.Contribution{
			Provider: provider, Type: contribapi.ResourceTypeContributionType,
			ID: id, Layers: contribapi.InlineData(json.RawMessage(body)),
		}
	}

	kinds := contribapi.KindsFrom([]contribapi.Contribution{
		kind("ktw-compose", "skill", `{"name":"skill","schema":"ktw-compose:/data-type/skill","validate":"warn"}`),
		kind("pack", "skill", `{"name":"skill","schema":"pack:/data-type/skill","validate":"warn"}`),
		kind("pack", "recipe", `{"name":"recipe","validate":"off"}`),
		// A kind whose body names nothing falls back to the URI's id — the
		// name is what every other contribution's Type refers to.
		kind("pack", "note", `{"description":"no name field"}`),
		// Not a kind declaration; must not appear.
		{Provider: "pack", Type: "skill", ID: "a", Layers: contribapi.InlineData(json.RawMessage(`{"name":"a"}`))},
	})

	want := map[string]string{"skill": "warn", "recipe": "off", "note": ""}
	if kinds.Len() != len(want) {
		t.Fatalf("got %v, want %v", kinds.Names(), want)
	}
	for k, v := range want {
		d, ok := kinds.Get(k)
		if !ok || d.Validate != v {
			t.Errorf("kind %q: got severity %q (declared=%v), want %q", k, d.Validate, ok, v)
		}
	}
	if got := kinds.DeclaredBy("skill"); got != "ktw-compose:/contribution-type/skill" {
		t.Errorf("skill resolves to the binary's skills pack declaration, got %q", got)
	}
	for _, f := range kinds.Check() {
		if f.Kind == "skill" && (f.Rule == contribapi.KindRuleDuplicate || f.Rule == contribapi.KindRuleRedeclaresSystem) {
			t.Errorf("one skill declaration carried by the binary and the pack is one kind, got %v", f)
		}
	}
	// Packaging escalates warn to error and honours off — from the same registry.
	cfg := authoring.PackagingConfig(kinds)
	if got := kinds.SeverityFor("skill", cfg); got != contribapi.ValidateOn {
		t.Errorf("packaging: skill (warn) should escalate to on, got %q", got)
	}
	if got := kinds.SeverityFor("recipe", cfg); got != contribapi.ValidateOff {
		t.Errorf("packaging: recipe (off) must stay off, got %q", got)
	}
}
