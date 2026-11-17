// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"encoding/json"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
)

// A kind declared by neither this binary nor `system` is FOREIGN, not
// undeclared: another member of the universe may declare it, and only the
// aggregator — which holds every member's declarations — can tell. The
// builder keeps the entries and records the kind on the manifest; refusing
// here made the rule depend on the producer's language (blind audit H3: the
// polyglot producer, which has no Go builder, could publish `recipe` against
// another pack's declaration; a Go provider could not).
func TestBuilder_KeepsEntriesOfAForeignKindAndRecordsIt(t *testing.T) {
	entry := contribapi.Contribution{Provider: "go-producer", Type: "recipe", ID: "crepes",
		Layers: contribapi.InlineData(json.RawMessage(`{"name":"Crepes","serves":2}`))}
	m, err := NewBuilder("go-producer", "1.0.0").BuildFromProviders(context.Background(), []contribapi.ContributionProvider{
		&fakeProvider{provider: "go-producer", resType: "recipe", entries: []contribapi.Contribution{entry}},
	})
	if err != nil {
		t.Fatalf("manifest=%v err=%v — a foreign kind must not fail the build", m != nil, err)
	}
	var kept bool
	for _, c := range m.Contributions {
		if c.ResolvedURI() == "go-producer:/recipe/crepes" {
			kept = true
		}
	}
	if !kept {
		t.Errorf("the foreign entry was dropped: %v", m.Contributions)
	}
	if len(m.ForeignKinds) != 1 || m.ForeignKinds[0] != "recipe" {
		t.Errorf("ForeignKinds = %v, want [recipe]: the manifest must say which kinds it declares nowhere", m.ForeignKinds)
	}
}

// A kind the binary declares itself is not foreign, and the meta-model never is.
func TestBuilder_RecordsNoForeignKindForDeclaredOrMetaKinds(t *testing.T) {
	entry := contribapi.Contribution{Provider: "ktw-test", Type: "skill", ID: "intro",
		Layers: contribapi.InlineData(json.RawMessage(`{"name":"intro"}`))}
	m, err := NewBuilder("ktw-test", "1.0.0").BuildFromProviders(context.Background(), []contribapi.ContributionProvider{
		declaring("ktw-test", "skill"),
		&fakeProvider{provider: "ktw-test", resType: "skill", entries: []contribapi.Contribution{entry}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(m.ForeignKinds) != 0 {
		t.Errorf("ForeignKinds = %v, want none", m.ForeignKinds)
	}
}
