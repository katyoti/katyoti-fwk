// SPDX-License-Identifier: Apache-2.0

package contribapi

import (
	"strings"
	"testing"
)

func derivedKind(t *testing.T) Kinds {
	t.Helper()
	d := complete("skill")
	d.Meta = &MetaDecl{Budget: 2048, From: "data", Fields: []string{"name", "description", "icon", "hashtags", "category", "advertised"}}
	return KindsFrom([]Contribution{kindEntry(t, d)})
}

func frontmatterEntry(t *testing.T) Contribution {
	t.Helper()
	c := Contribution{Provider: "ktw-test", Type: "skill", ID: "company-research"}
	c.SetData([]byte(`{"name":"company-research","description":"Research a company","iconName":"building-2","icon":"🏢","hashtags":["research","company"],"category":"research","advertised":true,"requires":["skill:web"]}`))
	return c
}

// The producer writes the picker fields once, in data; the builder fills
// meta from them. Fields the kind does not derive stay the producer's.
func TestApplyDerivedMeta_FillsExactlyTheDeclaredFields(t *testing.T) {
	kinds := derivedKind(t)
	c := frontmatterEntry(t)
	c.Metadata = &ContributionMeta{MediaType: "text/markdown", Annotations: map[string]string{"k": "v"}}

	if err := ApplyDerivedMeta(&c, kinds); err != nil {
		t.Fatalf("ApplyDerivedMeta: %v", err)
	}
	m := c.Metadata
	if m.Name != "company-research" || m.Description != "Research a company" || m.Category != "research" {
		t.Errorf("scalar fields not derived: %+v", *m)
	}
	if len(m.Hashtags) != 2 || m.Hashtags[1] != "company" {
		t.Errorf("hashtags = %v", m.Hashtags)
	}
	if m.Icon == nil || m.Icon.Tabler != "building-2" || m.Icon.Emoji != "🏢" {
		t.Errorf("icon = %+v, want tabler+emoji from iconName+icon", m.Icon)
	}
	if m.Advertised == nil || !*m.Advertised {
		t.Errorf("advertised = %v, want true", m.Advertised)
	}
	if m.MediaType != "text/markdown" || m.Annotations["k"] != "v" {
		t.Errorf("fields the kind does not derive were touched: %+v", *m)
	}
	if got := MetaDisagreements(c, kinds); len(got) != 0 {
		t.Errorf("a freshly derived meta disagrees with its data: %v", got)
	}

	// A kind that does not derive is untouched, meta nil stays nil.
	plain := Contribution{Provider: "ktw-test", Type: "plain", ID: "p"}
	plain.SetData([]byte(`{"name":"x"}`))
	if err := ApplyDerivedMeta(&plain, kinds); err != nil || plain.Metadata != nil {
		t.Errorf("plain kind: err=%v meta=%+v, want untouched", err, plain.Metadata)
	}
}

// A second writer shows as a disagreement, field by field — the census
// refuses it. A data layer with no bytes cannot derive and is reported too.
func TestMetaDisagreements_NamesEverySecondWriter(t *testing.T) {
	kinds := derivedKind(t)
	c := frontmatterEntry(t)
	c.Metadata = &ContributionMeta{Name: "company-reserch", Description: "Research a company", Category: "research",
		Hashtags: []string{"research", "company"}, Icon: NewIcon("building-2", "🏢")}
	got := MetaDisagreements(c, kinds)
	joined := strings.Join(got, "\n")
	if len(got) != 2 || !strings.Contains(joined, "meta.name is \"company-reserch\"") || !strings.Contains(joined, "meta.advertised is null, data derives true") {
		t.Fatalf("disagreements = %v, want exactly name and advertised", got)
	}

	c.Layers["data"] = LayerRef{Hash: "sha256:gone", Size: 9}
	if got := MetaDisagreements(c, kinds); len(got) != 1 || !strings.Contains(got[0], "carries no bytes") {
		t.Fatalf("a data layer without bytes must be reported, got %v", got)
	}
}
