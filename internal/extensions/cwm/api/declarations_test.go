// SPDX-License-Identifier: Apache-2.0

package api_test

import (
	"context"
	"encoding/json"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	manifestlocal "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/local"
)

// One contribution of every kind lands in the set; a panel's id is its
// CONTRIBUTION's (the file's basename), never the body's — two places for one
// id is how they disagree; the lists come sorted.
func TestEveryKindIsReadOnce(t *testing.T) {
	body := func(v any) json.RawMessage {
		b, _ := json.Marshal(v)
		return b
	}
	c := func(typ, id string, v any) contribapi.Contribution {
		return contribapi.Contribution{Provider: "ktw-fake", Type: typ, ID: id, Layers: contribapi.InlineData(body(v))}
	}
	manifest := &mbapi.Manifest{Provider: "ktw-fake", Contributions: []contribapi.Contribution{
		c("edge-type", "e", cwmapi.EdgeTypeRegistration{Name: "e"}),
		c("object-type", "o", cwmapi.ObjectType{Name: "o"}),
		c("capability", "c", cwmapi.Capability{Name: "c"}),
		c("aspect", "t", cwmapi.Aspect{Name: "t", Capabilities: []string{"c"}}),
		// An extension's own kind: not CWM's to read.
		c("object-panel", "zeta", map[string]any{"name": "Z", "source": "x:y/z"}),
		c("workflow", "w", cwmapi.Workflow{Name: "w", Initial: "a"}),
		c("policy", "p", cwmapi.Policy{Name: "p"}),
		c("hook-type", "review", cwmapi.HookType{Name: "review", Timing: "on-demand", Answer: "findings"}),
		c("hook", "h", cwmapi.Hook{Name: "h", Type: "lint", URI: "x:y/z"}),
	}}
	set := cwmapi.DeclarationsFromCatalog(context.Background(), manifestlocal.NewMemoryCatalog([]*mbapi.Manifest{manifest}, []string{"ktw-fake"}))
	for _, row := range []struct {
		kind      string
		got, want int
	}{
		{"edge-type", len(set.Edges), 1}, {"object-type", len(set.Objects), 1}, {"capability", len(set.Capabilities), 1},
		{"aspect", len(set.Aspects), 1}, {"workflow", len(set.Workflows), 1},
		{"policy", len(set.Policies), 1}, {"hook-type", len(set.HookTypes), 1}, {"hook", len(set.Hooks), 1},
	} {
		if row.got != row.want {
			t.Errorf("%s: read %d, want %d", row.kind, row.got, row.want)
		}
	}
	if len(set.Policies) == 1 && set.Policies[0].Provider != "ktw-fake" {
		t.Errorf("a policy carries its provider: %+v", set.Policies[0])
	}
}

// An edge type whose cardinality is not 1:1 | 1:N | N:1 | M:N is refused by
// the catalog read, like any declaration CWM cannot honour; its valid
// neighbour is still read.
func TestAnEdgeTypeWithAnUnknownCardinalityIsRefused(t *testing.T) {
	body := func(v any) json.RawMessage {
		b, _ := json.Marshal(v)
		return b
	}
	manifest := &mbapi.Manifest{Provider: "ktw-fake", Contributions: []contribapi.Contribution{
		{Provider: "ktw-fake", Type: "edge-type", ID: "bad", Layers: contribapi.InlineData(body(cwmapi.EdgeTypeRegistration{Name: "bad", Cardinality: "2:3"}))},
		{Provider: "ktw-fake", Type: "edge-type", ID: "good", Layers: contribapi.InlineData(body(cwmapi.EdgeTypeRegistration{Name: "good", Cardinality: "N:1"}))},
	}}
	set := cwmapi.DeclarationsFromCatalog(context.Background(), manifestlocal.NewMemoryCatalog([]*mbapi.Manifest{manifest}, []string{"ktw-fake"}))
	if len(set.Edges) != 1 || set.Edges[0].Name != "good" {
		t.Fatalf("edges read = %+v, want only good (2:3 is not a cardinality)", set.Edges)
	}
}
