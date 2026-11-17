// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"errors"
	"strings"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
)

// openOnlyResources is a ResourceService that answers Get (the index entry)
// and OpenLayer only: a call to any other method (Read above all) panics on
// the nil embedded interface, which is the proof ReadResource no longer
// reaches the compatibility Read.
type openOnlyResources struct {
	mbapi.ResourceService
	layers map[string]mbapi.OpenLayerOutput // layer -> answer; absent = not declared
	opened []string
}

func (r *openOnlyResources) Get(_ context.Context, in mbapi.ContributionGetInput) (mbapi.ContributionGetOutput, error) {
	c := contribapi.Contribution{URI: in.URI, Layers: map[string]contribapi.LayerRef{}}
	for name, out := range r.layers {
		c.Layers[name] = out.Ref
	}
	return mbapi.ContributionGetOutput{Contribution: c}, nil
}

func (r *openOnlyResources) OpenLayer(_ context.Context, in mbapi.OpenLayerInput) (mbapi.OpenLayerOutput, error) {
	r.opened = append(r.opened, in.Layer)
	out, ok := r.layers[in.Layer]
	if !ok {
		return mbapi.OpenLayerOutput{}, contribapi.ErrLayerNotDeclared{URI: in.URI, Layer: in.Layer}
	}
	return out, nil
}

func TestReadResourceLayer_SourceWhenDeclared(t *testing.T) {
	rs := &openOnlyResources{layers: map[string]mbapi.OpenLayerOutput{
		contribapi.LayerSource: {Ref: contribapi.LayerRef{MediaType: "text/markdown"}, Content: []byte("# raw")},
		contribapi.LayerData:   {Content: []byte(`{"name":"x"}`)},
	}}
	out, err := readResourceLayer(context.Background(), rs, "ktw-x:/skill/x")
	if err != nil {
		t.Fatal(err)
	}
	if string(out.Content) != "# raw" || out.MediaType != "text/markdown" || out.URI != "ktw-x:/skill/x" {
		t.Fatalf("got %+v, want the source layer", out)
	}
	if strings.Join(rs.opened, ",") != contribapi.LayerSource {
		t.Fatalf("opened %v, want only the source layer", rs.opened)
	}
}

func TestReadResourceLayer_DataWhenSourceNotDeclared(t *testing.T) {
	rs := &openOnlyResources{layers: map[string]mbapi.OpenLayerOutput{
		contribapi.LayerData: {Ref: contribapi.LayerRef{MediaType: "application/json"}, Content: []byte(`{"name":"x"}`)},
	}}
	out, err := readResourceLayer(context.Background(), rs, "ktw-x:/task/x")
	if err != nil {
		t.Fatal(err)
	}
	if string(out.Content) != `{"name":"x"}` || out.MediaType != "application/json" {
		t.Fatalf("got %+v, want the data layer", out)
	}
}

func TestReadResourceLayer_RefusalAndOtherErrorsAreTheAnswer(t *testing.T) {
	refused := &openOnlyResources{layers: map[string]mbapi.OpenLayerOutput{
		contribapi.LayerSource: {Ref: contribapi.LayerRef{Hash: "sha256:big", Size: contribapi.BandControl + 1}, Refused: "layer \"source\" is in the bulk band"},
		contribapi.LayerData:   {Content: []byte("must not be read")},
	}}
	if _, err := readResourceLayer(context.Background(), refused, "ktw-x:/reference/big"); !errors.As(err, new(contribapi.ErrBulkLayer)) {
		t.Fatalf("refused source: err=%v, want the refusal", err)
	}
	if len(refused.opened) != 1 {
		t.Fatalf("a refused source layer must not fall to data; opened %v", refused.opened)
	}

	notFound := mbapi.ErrContributionNotFound{URI: "ktw-x:/skill/none"}
	failing := &failingResources{err: notFound}
	_, err := readResourceLayer(context.Background(), failing, "ktw-x:/skill/none")
	if !errors.As(err, &mbapi.ErrContributionNotFound{}) || failing.calls != 1 {
		t.Fatalf("not-found: err=%v calls=%d, want the error after one lookup", err, failing.calls)
	}
}

type failingResources struct {
	mbapi.ResourceService
	err   error
	calls int
}

func (r *failingResources) Get(context.Context, mbapi.ContributionGetInput) (mbapi.ContributionGetOutput, error) {
	r.calls++
	return mbapi.ContributionGetOutput{}, r.err
}
