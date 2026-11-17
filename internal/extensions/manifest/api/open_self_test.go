// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
)

// stubResourceService is a self-reach index of one binary: an index (with
// resident bytes, as a binary's own index has) and a Read that serves the
// provider's raw source bytes.
type stubResourceService struct {
	index  []contribapi.Contribution
	read   map[string][]byte // the source layer, as Read would answer it
	layers map[string][]byte // any named layer, keyed uri + "#" + layer
	reads  int
}

func (s *stubResourceService) GetManifest(context.Context, ContributionGetManifestInput) (ContributionGetManifestOutput, error) {
	return ContributionGetManifestOutput{}, nil
}
func (s *stubResourceService) List(_ context.Context, in ContributionListInput) (ContributionListOutput, error) {
	var out []contribapi.Contribution
	for _, c := range s.index {
		for _, u := range in.Filter.URIs {
			if c.ResolvedURI() == u {
				out = append(out, c)
			}
		}
	}
	return ContributionListOutput{Contributions: out}, nil
}
func (s *stubResourceService) Get(context.Context, ContributionGetInput) (ContributionGetOutput, error) {
	return ContributionGetOutput{}, nil
}
func (s *stubResourceService) Read(_ context.Context, in ContributionReadInput) (ContributionReadOutput, error) {
	s.reads++
	b, ok := s.read[in.URI]
	if !ok {
		return ContributionReadOutput{}, ErrContributionNotFound{URI: in.URI}
	}
	return ContributionReadOutput{URI: in.URI, Content: b, MediaType: "text/markdown"}, nil
}
func (s *stubResourceService) OpenLayer(_ context.Context, in OpenLayerInput) (OpenLayerOutput, error) {
	s.reads++
	if b, ok := s.layers[in.URI+"#"+in.Layer]; ok {
		return OpenLayerOutput{Ref: contribapi.LayerRef{Hash: contribapi.HashContent(b), Size: int64(len(b)), MediaType: "text/markdown"}, Content: b}, nil
	}
	if in.Layer == contribapi.LayerSource {
		if b, ok := s.read[in.URI]; ok {
			return OpenLayerOutput{Ref: contribapi.LayerRef{Hash: contribapi.HashContent(b), Size: int64(len(b)), MediaType: "text/markdown"}, Content: b}, nil
		}
	}
	return OpenLayerOutput{}, ErrContributionNotFound{URI: in.URI}
}
func (s *stubResourceService) Types(context.Context) ([]string, error) { return nil, nil }
func (s *stubResourceService) GetDataType(context.Context, ContributionGetDataTypeInput) (ContributionGetDataTypeOutput, error) {
	return ContributionGetDataTypeOutput{}, nil
}

func readAll(t *testing.T, l *Layer) string {
	t.Helper()
	b, err := io.ReadAll(l)
	if err != nil {
		t.Fatalf("read layer: %v", err)
	}
	return string(b)
}

// Open(uri, "data") and Open(uri, "source") return DIFFERENT bytes for a skill:
// the structured body and the authored markdown. Before this, self reach called
// Read whatever layer was asked and universe reach fell back to GetBody — two
// paths, each ignoring the argument in its own way.
func TestOpenSelf_HonoursTheLayerArgument(t *testing.T) {
	skill := contribapi.Contribution{Provider: "p", Type: "skill", ID: "doc-create"}
	skill.SetData([]byte(`{"name":"doc-create"}`))
	md := []byte("# doc-create\n\nthe instructions\n")
	skill.SetSource(md, "text/markdown")
	// The index does not carry the skill's markdown (not resident) — only its
	// descriptor. A real binary's index looks exactly like this after the
	// builder applies the kind's residency.
	skill.Layers[contribapi.LayerSource] = contribapi.LayerRef{
		Hash: contribapi.HashContent(md), Size: int64(len(md)), MediaType: "text/markdown",
	}
	svc := &stubResourceService{index: []contribapi.Contribution{skill}, read: map[string][]byte{"p:/skill/doc-create": md}}

	data, err := OpenVia(context.Background(), svc, "p:/skill/doc-create", contribapi.LayerData)
	if err != nil {
		t.Fatalf("open data: %v", err)
	}
	if got := readAll(t, data); got != `{"name":"doc-create"}` {
		t.Errorf("data layer = %q", got)
	}
	if svc.reads != 0 {
		t.Error("the data layer is resident in the index; opening it must not call Read")
	}

	src, err := OpenVia(context.Background(), svc, "p:/skill/doc-create", contribapi.LayerSource)
	if err != nil {
		t.Fatalf("open source: %v", err)
	}
	if got := readAll(t, src); got != string(md) {
		t.Errorf("source layer = %q", got)
	}
	if src.Ref.MediaType != "text/markdown" || src.Ref.Hash != contribapi.HashContent(md) {
		t.Errorf("source descriptor = %+v", src.Ref)
	}
	if svc.reads != 1 {
		t.Errorf("a non-resident source is read from the provider exactly once, got %d reads", svc.reads)
	}
}

// A provider whose file changed since the index was built is caught: the
// bytes do not match the layer's address, and Open says so rather than
// serving content the index does not describe.
func TestOpenSelf_RefusesContentThatDoesNotMatchItsAddress(t *testing.T) {
	skill := contribapi.Contribution{Provider: "p", Type: "skill", ID: "s"}
	skill.SetData([]byte(`{}`))
	skill.Layers[contribapi.LayerSource] = contribapi.LayerRef{Hash: contribapi.HashContent([]byte("old")), Size: 3}
	svc := &stubResourceService{index: []contribapi.Contribution{skill}, read: map[string][]byte{"p:/skill/s": []byte("new")}}

	_, err := OpenVia(context.Background(), svc, "p:/skill/s", contribapi.LayerSource)
	if err == nil || !strings.Contains(err.Error(), "does not match its address") {
		t.Fatalf("want an address-mismatch error, got %v", err)
	}
}

// An undeclared layer is refused by name; a bulk-band layer is refused rather
// than materialised; an unknown URI is not found.
func TestOpenSelf_Refusals(t *testing.T) {
	big := contribapi.Contribution{Provider: "p", Type: "reference", ID: "dataset"}
	big.SetData([]byte(`{}`))
	big.Layers[contribapi.LayerSource] = contribapi.LayerRef{Hash: "sha256:x", Size: contribapi.BandControl + 1}
	svc := &stubResourceService{index: []contribapi.Contribution{big}}

	if _, err := OpenVia(context.Background(), svc, "p:/reference/dataset", "thumbnail"); err == nil || !strings.Contains(err.Error(), `declares no layer "thumbnail"`) {
		t.Errorf("undeclared layer: %v", err)
	}
	if _, err := OpenVia(context.Background(), svc, "p:/reference/dataset", contribapi.LayerSource); !errors.Is(err, ErrLayerTooLargeForMemory) {
		t.Errorf("bulk layer at self reach must be refused, got %v", err)
	}
	var nf ErrContributionNotFound
	if _, err := OpenVia(context.Background(), svc, "p:/reference/nope", contribapi.LayerData); !errors.As(err, &nf) {
		t.Errorf("unknown URI: %v", err)
	}
}

// Self reach opens ANY declared layer the index does not carry through the
// producer's OpenLayer — not only `source` through Read. A skill's
// `instructions` layer (the markdown body, not the file) is the case that
// made this necessary.
func TestOpenSelf_OpensANonSourceLayerThroughTheProducer(t *testing.T) {
	body := []byte("the instructions\n")
	skill := contribapi.Contribution{Provider: "p", Type: "skill", ID: "doc-create"}
	skill.SetData([]byte(`{"name":"doc-create"}`))
	skill.Layers["instructions"] = contribapi.LayerRef{Hash: contribapi.HashContent(body), Size: int64(len(body)), MediaType: "text/markdown"}
	svc := &stubResourceService{index: []contribapi.Contribution{skill}, layers: map[string][]byte{"p:/skill/doc-create#instructions": body}}

	l, err := OpenVia(context.Background(), svc, "p:/skill/doc-create", "instructions")
	if err != nil {
		t.Fatalf("open instructions: %v", err)
	}
	if got := readAll(t, l); got != string(body) {
		t.Fatalf("instructions = %q, want the producer's bytes", got)
	}
	if svc.reads != 1 {
		t.Errorf("producer contacted %d times, want once", svc.reads)
	}

	// The producer's answer must match the index's address.
	svc.layers["p:/skill/doc-create#instructions"] = []byte("edited since\n")
	if _, err := OpenVia(context.Background(), svc, "p:/skill/doc-create", "instructions"); err == nil || !strings.Contains(err.Error(), "does not match its address") {
		t.Errorf("a producer answer with another hash must be refused, got %v", err)
	}
}
