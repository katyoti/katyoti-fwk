// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
)

// A kind with `meta.from: data` gets its picker fields from the builder, not
// from the producer: the producer writes the frontmatter once, in the data
// layer, and the manifest's meta is derived from it. A kind without it keeps
// whatever meta the producer wrote.
func TestBuilder_DerivesMetaFromTheDataLayer(t *testing.T) {
	kinds := kindProvider(
		contribapi.KindDeclaration{Name: "fm-kind", Schema: "ktw-test:/data-type/x", Validate: "warn",
			Layers: map[string]contribapi.LayerDecl{"data": {Default: true, Required: true}},
			Meta:   &contribapi.MetaDecl{Budget: 1024, From: "data", Fields: []string{"name", "description", "icon", "hashtags", "category", "advertised"}}},
		contribapi.KindDeclaration{Name: "plain-kind", Schema: "ktw-test:/data-type/x", Validate: "warn",
			Layers: map[string]contribapi.LayerDecl{"data": {Default: true}},
			Meta:   &contribapi.MetaDecl{Budget: 1024}},
	)
	fm := contribapi.Contribution{Provider: "ktw-test", Type: "fm-kind", ID: "research"}
	fm.SetData([]byte(`{"name":"research","description":"Look things up","iconName":"search","icon":"🔎","hashtags":["web"],"category":"tools","advertised":false}`))
	fm.Metadata = &contribapi.ContributionMeta{MediaType: "text/markdown"} // the producer says only what it owns
	plain := contribapi.Contribution{Provider: "ktw-test", Type: "plain-kind", ID: "p"}
	plain.SetData([]byte(`{"name":"not-this"}`))
	plain.Metadata = &contribapi.ContributionMeta{Name: "p", Description: "the producer's own"}

	m, err := NewBuilder("ktw-test", "0").BuildFromProviders(context.Background(),
		[]contribapi.ContributionProvider{kinds,
			&residencyProvider{typ: "fm-kind", entries: []contribapi.Contribution{fm}},
			&residencyProvider{typ: "plain-kind", entries: []contribapi.Contribution{plain}}})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	byID := map[string]contribapi.Contribution{}
	for _, c := range m.Contributions {
		byID[c.ID] = c
	}
	got := byID["research"].Metadata
	if got == nil || got.Name != "research" || got.Description != "Look things up" || got.Category != "tools" ||
		len(got.Hashtags) != 1 || got.Icon == nil || got.Icon.Tabler != "search" || got.Advertised == nil || *got.Advertised {
		t.Fatalf("fm-kind meta not derived from data: %+v", got)
	}
	if got.MediaType != "text/markdown" {
		t.Errorf("the producer's own field was overwritten: %+v", got)
	}
	if p := byID["p"].Metadata; p == nil || p.Name != "p" || p.Description != "the producer's own" {
		t.Errorf("plain-kind meta must stay the producer's: %+v", p)
	}
}
