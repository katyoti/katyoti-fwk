// SPDX-License-Identifier: Apache-2.0

package core

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
)

// residencyProvider is a fake producer for one kind: every entry carries its
// bytes (up to the bulk band), exactly as a real provider does since d3 — the
// provider never decides residency.
type residencyProvider struct {
	typ     string
	entries []contribapi.Contribution
}

func (p *residencyProvider) Type() string     { return p.typ }
func (p *residencyProvider) Provider() string { return "ktw-test" }
func (p *residencyProvider) Enumerate(context.Context) ([]contribapi.Contribution, error) {
	return p.entries, nil
}
func (p *residencyProvider) Identity() contribapi.Identity { return contribapi.Identity{} }
func (p *residencyProvider) Open(ctx context.Context, id, layer string) (io.ReadCloser, contribapi.LayerRef, error) {
	return contribapi.OpenEnumerated(ctx, p, id, layer)
}

// kindProvider declares kinds, the way the system provider does: one
// contribution-type entry per kind, its declaration in the data layer.
func kindProvider(decls ...contribapi.KindDeclaration) *residencyProvider {
	p := &residencyProvider{typ: contribapi.ResourceTypeContributionType}
	for _, d := range decls {
		body, _ := json.Marshal(d)
		c := contribapi.Contribution{Provider: "ktw-test", Type: contribapi.ResourceTypeContributionType, ID: d.Name}
		c.SetData(body)
		p.entries = append(p.entries, c)
	}
	return p
}

func sourceEntry(typ, id string, src []byte) contribapi.Contribution {
	c := contribapi.Contribution{Provider: "ktw-test", Type: typ, ID: id}
	c.SetData([]byte(`{}`))
	c.SetSource(src, "text/plain")
	return c
}

// The kind decides what the index carries; the builder applies it. This is
// the rule the provider field `ResidentSource` used to hold at one call site,
// and the case that broke under it: 18 KiB of DSL, well inside the control
// band, shipped as a hash with no bytes because residency was gated on the
// inline band. Now: a resident kind keeps its source up to the bulk band, a
// non-resident kind keeps only the descriptor.
func TestBuilder_ResidencyComesFromTheKind(t *testing.T) {
	large := append([]byte("name: large\n"), bytes.Repeat([]byte("# pad\n"), 4000)...)
	small := []byte("name: small\n")

	kinds := kindProvider(
		contribapi.KindDeclaration{Name: "resident-kind", Schema: "ktw-test:/data-type/x", Validate: "warn",
			Layers: map[string]contribapi.LayerDecl{"data": {}, "source": {Default: true, Resident: true}},
			Meta:   &contribapi.MetaDecl{Budget: 1024}},
		contribapi.KindDeclaration{Name: "lean-kind", Schema: "ktw-test:/data-type/x", Validate: "warn",
			Layers: map[string]contribapi.LayerDecl{"data": {}, "source": {Default: true}},
			Meta:   &contribapi.MetaDecl{Budget: 1024}},
	)
	resident := &residencyProvider{typ: "resident-kind", entries: []contribapi.Contribution{
		sourceEntry("resident-kind", "large", large),
		sourceEntry("resident-kind", "small", small),
	}}
	lean := &residencyProvider{typ: "lean-kind", entries: []contribapi.Contribution{
		sourceEntry("lean-kind", "large", large),
		sourceEntry("lean-kind", "small", small),
	}}

	m, err := NewBuilder("ktw-test", "0").BuildFromProviders(context.Background(),
		[]contribapi.ContributionProvider{kinds, resident, lean})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	byURI := map[string]contribapi.Contribution{}
	for _, c := range m.Contributions {
		byURI[c.ResolvedURI()] = c
	}

	// Resident: bytes kept above the inline band (control band) and below it.
	rl := byURI["ktw-test:/resident-kind/large"]
	if ref, ok := rl.SourceRef(); !ok || contribapi.BandOf(ref.Size) != contribapi.BandNameControl {
		t.Fatalf("setup: want a control-band source on the large entry")
	}
	if len(rl.Source()) == 0 {
		t.Error("resident kind: control-band source must be carried — the consumer that needs it cannot reach it otherwise")
	}
	if len(byURI["ktw-test:/resident-kind/small"].Source()) == 0 {
		t.Error("resident kind: inline-band source must be carried")
	}

	// Lean: the descriptor stays, the bytes go — at every band.
	for _, id := range []string{"large", "small"} {
		e := byURI["ktw-test:/lean-kind/"+id]
		if _, declared := e.SourceRef(); !declared {
			t.Errorf("lean kind %s: must still DECLARE its source layer — a consumer has to know it exists", id)
		}
		if len(e.Source()) != 0 {
			t.Errorf("lean kind %s: carries source bytes the kind did not declare resident", id)
		}
		if len(e.Data()) == 0 {
			t.Errorf("lean kind %s: the data layer is exempt from residency and must stay", id)
		}
	}
}

// A provider's own cached slice must not be written through: the builder
// drops bytes on a copy, exactly as DropInline does.
func TestBuilder_ResidencyIsCopyOnWrite(t *testing.T) {
	kinds := kindProvider(contribapi.KindDeclaration{Name: "lean-kind", Schema: "ktw-test:/data-type/x", Validate: "warn",
		Layers: map[string]contribapi.LayerDecl{"data": {}, "source": {Default: true}}, Meta: &contribapi.MetaDecl{Budget: 1024}})
	lean := &residencyProvider{typ: "lean-kind", entries: []contribapi.Contribution{sourceEntry("lean-kind", "a", []byte("hello"))}}
	if _, err := NewBuilder("ktw-test", "0").BuildFromProviders(context.Background(),
		[]contribapi.ContributionProvider{kinds, lean}); err != nil {
		t.Fatalf("build: %v", err)
	}
	if len(lean.entries[0].Source()) == 0 {
		t.Fatal("the provider's own entry lost its bytes: residency wrote through a shared layer map")
	}
}
