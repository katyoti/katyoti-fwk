// SPDX-License-Identifier: Apache-2.0

package core

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
)

// The producer serves any declared layer from its own enumeration — a
// provider carries every layer's bytes up to the bulk band, and residency
// is applied afterwards by the builder, so what the index dropped is still
// here. Until e6 (2026-09-22) only Read existed on this side, and Read has
// no layer to name: every non-resident, non-Read layer was refused at the
// hub with "producer-side layer access is not available".
func TestResourceService_OpenLayer_ServesAnyDeclaredLayerFromTheProducer(t *testing.T) {
	instructions := []byte("# doc-create\n\nthe instructions\n")
	file := append([]byte("---\nname: doc-create\n---\n"), instructions...)
	bulk := bytes.Repeat([]byte("x"), int(contribapi.BandControl)+1)

	skill := contribapi.Contribution{Provider: "ktw-test", Type: "skill", ID: "doc-create"}
	skill.SetData([]byte(`{"name":"doc-create"}`))
	skill.SetLayer("instructions", instructions, "text/markdown")
	skill.SetSource(file, "text/markdown")
	big := contribapi.Contribution{Provider: "ktw-test", Type: "skill", ID: "big"}
	big.SetData([]byte(`{}`))
	big.SetSource(bulk, "text/markdown")

	kindDecl, _ := json.Marshal(contribapi.KindDeclaration{Name: "skill", Schema: "ktw-test:/data-type/skill", Validate: "warn",
		Layers: map[string]contribapi.LayerDecl{"data": {}, "instructions": {Default: true}, "source": {}},
		Meta:   &contribapi.MetaDecl{Budget: 1}})
	kind := contribapi.Contribution{Provider: "ktw-test", Type: contribapi.ResourceTypeContributionType, ID: "skill"}
	kind.SetData(kindDecl)

	s := NewResourceService("ktw-test", "dev")
	s.providers = func(_ context.Context, provider, typ string) []contribapi.ContributionProvider {
		switch typ {
		case "skill":
			return []contribapi.ContributionProvider{&residencyProvider{typ: "skill", entries: []contribapi.Contribution{skill, big}}}
		case contribapi.ResourceTypeContributionType:
			if provider == "ktw-test" {
				return []contribapi.ContributionProvider{&residencyProvider{typ: typ, entries: []contribapi.Contribution{kind}}}
			}
		}
		return nil
	}
	ctx := context.Background()

	out, err := s.OpenLayer(ctx, mbapi.OpenLayerInput{URI: "ktw-test:/skill/doc-create", Layer: "instructions"})
	if err != nil || out.Refused != "" || string(out.Content) != string(instructions) {
		t.Fatalf("instructions: err=%v refused=%q content=%q", err, out.Refused, out.Content)
	}
	if out.Ref.Hash != contribapi.HashContent(instructions) || out.Ref.MediaType != "text/markdown" || out.Ref.Inline != nil {
		t.Errorf("ref = %+v, want the layer's address without inline bytes", out.Ref)
	}

	// "" resolves to the kind's default, read from THIS binary's declarations.
	out, err = s.OpenLayer(ctx, mbapi.OpenLayerInput{URI: "ktw-test:/skill/doc-create"})
	if err != nil || string(out.Content) != string(instructions) {
		t.Fatalf("default layer: err=%v content=%q, want the declared default (instructions)", err, out.Content)
	}

	out, err = s.OpenLayer(ctx, mbapi.OpenLayerInput{URI: "ktw-test:/skill/doc-create", Layer: "source"})
	if err != nil || string(out.Content) != string(file) {
		t.Fatalf("source: err=%v content=%q", err, out.Content)
	}

	if _, err := s.OpenLayer(ctx, mbapi.OpenLayerInput{URI: "ktw-test:/skill/doc-create", Layer: "references"}); err == nil || !strings.Contains(err.Error(), `declares no layer "references"`) {
		t.Errorf("an undeclared layer must be an error, got %v", err)
	}

	out, err = s.OpenLayer(ctx, mbapi.OpenLayerInput{URI: "ktw-test:/skill/big", Layer: "source"})
	if err != nil || !strings.Contains(out.Refused, "bulk band") || len(out.Content) != 0 {
		t.Errorf("a bulk-band layer is refused with its address, got err=%v refused=%q content=%d bytes", err, out.Refused, len(out.Content))
	}

	if _, err := s.OpenLayer(ctx, mbapi.OpenLayerInput{URI: "ktw-test:/skill/nobody", Layer: "source"}); err == nil {
		t.Error("an entry no provider enumerates must be an error")
	}
}

// mbapi.ReadLayer and ResourceService.Read are one fact stated twice — which
// layer the producer's Read answers with — and this holds them together: for
// any entry, the bytes Read returns hash to the address of the layer
// ReadLayer names, with that layer's media type. If Read ever answered from
// another layer, the hub routing on ReadLayer would fetch bytes that fail
// their own hash check.
func TestResourceService_Read_AnswersWithTheLayerReadLayerNames(t *testing.T) {
	both := contribapi.Contribution{Provider: "ktw-test", Type: "skill", ID: "a"}
	both.SetData([]byte(`{}`))
	both.SetSource([]byte("# a\n"), "text/markdown")
	dataOnly := contribapi.Contribution{Provider: "ktw-test", Type: "capability", ID: "b"}
	dataOnly.SetData([]byte(`{"id":"b"}`))
	sourceOnly := contribapi.Contribution{Provider: "ktw-test", Type: "data-type", ID: "c"}
	sourceOnly.SetSource([]byte("name: c\n"), "application/yaml")

	byType := map[string]contribapi.Contribution{"skill": both, "capability": dataOnly, "data-type": sourceOnly}
	s := NewResourceService("ktw-test", "dev")
	s.providers = func(_ context.Context, _, typ string) []contribapi.ContributionProvider {
		if c, ok := byType[typ]; ok {
			return []contribapi.ContributionProvider{&residencyProvider{typ: typ, entries: []contribapi.Contribution{c}}}
		}
		return nil
	}
	for _, c := range byType {
		layer := mbapi.ReadLayer(c)
		want := c.Layers[layer]
		out, err := s.Read(context.Background(), mbapi.ContributionReadInput{URI: c.ResolvedURI()})
		if err != nil {
			t.Fatalf("%s: Read: %v", c.ResolvedURI(), err)
		}
		if got := contribapi.HashContent(out.Content); got != want.Hash || out.MediaType != want.MediaType {
			t.Errorf("%s: ReadLayer = %q (%s, %s) but Read answers %s, %s", c.ResolvedURI(), layer, want.Hash, want.MediaType, got, out.MediaType)
		}
	}
	var notFound mbapi.ErrContributionNotFound
	if _, err := s.Read(context.Background(), mbapi.ContributionReadInput{URI: "ktw-test:/skill/nobody"}); !errors.As(err, &notFound) {
		t.Errorf("Read of an unknown entry: want ErrContributionNotFound, got %v", err)
	}
}
