// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"encoding/json"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
)

// overridingProvider is a provider whose enumeration took a layer from an
// override tree.
type overridingProvider struct {
	staticProvider
	overrides []contribapi.Override
}

func (p overridingProvider) Overrides() []contribapi.Override { return p.overrides }

// The manifest names each overridden layer and its source, as the providers
// said it, sorted by entry and layer; a provider overriding nothing adds none.
// Where a layer came from is not part of the manifest's hash.
func TestTheManifestNamesEachOverriddenLayerAndItsSource(t *testing.T) {
	entry := contribapi.Contribution{Provider: "ktw-x", Type: "skill", ID: "greet", Layers: contribapi.InlineData(json.RawMessage(`{"name":"greet"}`))}
	plain := staticProvider{provider: "ktw-x", typ: "skill", entries: []contribapi.Contribution{entry}}
	over := overridingProvider{staticProvider: plain, overrides: []contribapi.Override{
		{URI: "ktw-x:/skill/greet", Layer: "translations", From: "universe"},
		{URI: "ktw-x:/skill/greet", Layer: "source", From: "project:acme"},
	}}
	m, err := NewBuilder("ktw-x", "dev").BuildFromProviders(context.Background(), []contribapi.ContributionProvider{over})
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Overrides) != 2 || m.Overrides[0].Layer != "source" || m.Overrides[0].From != "project:acme" || m.Overrides[1].From != "universe" {
		t.Fatalf("overrides = %+v; want greet's source from project:acme, then its translations from universe", m.Overrides)
	}
	bare, err := NewBuilder("ktw-x", "dev").BuildFromProviders(context.Background(), []contribapi.ContributionProvider{plain})
	if err != nil {
		t.Fatal(err)
	}
	if len(bare.Overrides) != 0 {
		t.Fatalf("a provider overriding nothing: overrides %+v", bare.Overrides)
	}
	if bare.Hash != m.Hash {
		t.Errorf("the hash moved with where a layer came from: %s vs %s", bare.Hash, m.Hash)
	}
}
