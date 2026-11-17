// SPDX-License-Identifier: Apache-2.0

package contribapi

import (
	"context"
	"io"
	"testing"
)

// The enumeration digest names the content: order does not move it, a changed
// layer byte or a renamed entry does.
func TestDigestOf_NamesTheContent(t *testing.T) {
	a := Contribution{Provider: "p", Type: "skill", ID: "a"}
	a.SetData([]byte(`{"name":"a"}`))
	b := Contribution{Provider: "p", Type: "skill", ID: "b"}
	b.SetSource([]byte("# b"), "text/markdown")

	d := DigestOf([]Contribution{a, b})
	if d == "" || d != DigestOf([]Contribution{b, a}) {
		t.Fatalf("digest depends on enumeration order: %s vs %s", d, DigestOf([]Contribution{b, a}))
	}
	changed := b
	changed.SetSource([]byte("# b2"), "text/markdown")
	if DigestOf([]Contribution{a, changed}) == d {
		t.Fatal("a changed layer did not change the digest")
	}
	renamed := b
	renamed.ID = "c"
	if DigestOf([]Contribution{a, renamed}) == d {
		t.Fatal("a renamed entry did not change the digest")
	}
}

type epochProvider struct {
	items []Contribution
	calls int
	epoch ImmutableEpoch
}

func (p *epochProvider) Type() string       { return "skill" }
func (p *epochProvider) Provider() string   { return "p" }
func (p *epochProvider) Identity() Identity { return p.epoch.Identity(p) }
func (p *epochProvider) Enumerate(context.Context) ([]Contribution, error) {
	p.calls++
	return p.items, nil
}
func (p *epochProvider) Open(ctx context.Context, id, layer string) (io.ReadCloser, LayerRef, error) {
	return OpenEnumerated(ctx, p, id, layer)
}

// An immutable provider's digest is computed once, is the enumeration's, and
// keeps the identity rule.
func TestImmutableEpoch_ComputedOnce(t *testing.T) {
	c := Contribution{Provider: "p", Type: "skill", ID: "a"}
	c.SetData([]byte(`{}`))
	p := &epochProvider{items: []Contribution{c}}
	id := p.Identity()
	if IdentityRule(id) != "" || id.Digest != DigestOf(p.items) {
		t.Fatalf("identity %+v, want the enumeration digest %s", id, DigestOf(p.items))
	}
	_ = p.Identity()
	if p.calls != 1 {
		t.Fatalf("digest computed %d times, want once", p.calls)
	}
	// A recording provider is stamped by the enumeration it just produced.
	var e ImmutableEpoch
	e.Record(p.items)
	if got := e.Identity(p); got.Digest != id.Digest || p.calls != 1 {
		t.Fatalf("recorded identity %+v (calls %d), want %s without enumerating", got, p.calls, id.Digest)
	}
}
