// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"errors"
	"io"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
)

// reach_errors_test.go — one open, one set of errors (e25, N6).
//
// The band-aware "open layer L" decision lives in contribapi.OpenLayerFrom;
// every reach finds its entry its own way and asks it. So the SAME question
// gets the SAME typed answer at self reach (mbapi.OpenVia over the binary's
// own ResourceService, down to the provider's OpenEnumerated), in the
// aggregator's tier 1 (CatalogImpl) and in a consumer view (MemoryCatalog):
// not found, not declared, not carried, bulk.

// reachFixture is one binary's content: a skill whose source layer is
// addressed but not carried, and a reference whose source is in the bulk band.
func reachFixture() []contribapi.Contribution {
	skill := contribapi.Contribution{Provider: "p", Type: "skill", ID: "s"}
	skill.SetData([]byte(`{"name":"s"}`))
	skill.Layers[contribapi.LayerSource] = contribapi.LayerRef{Hash: "sha256:abc", Size: 7000, MediaType: "text/markdown"}
	big := contribapi.Contribution{Provider: "p", Type: "reference", ID: "dataset"}
	big.SetData([]byte(`{}`))
	big.Layers[contribapi.LayerSource] = contribapi.LayerRef{Hash: "sha256:big", Size: contribapi.BandControl + 1}
	return []contribapi.Contribution{skill, big}
}

// fixtureProvider serves reachFixture's entries of one type.
type fixtureProvider struct{ typ string }

func (p fixtureProvider) Type() string     { return p.typ }
func (p fixtureProvider) Provider() string { return "p" }
func (p fixtureProvider) Identity() contribapi.Identity {
	return contribapi.Identity{Digest: "sha256:fixture"}
}
func (p fixtureProvider) Enumerate(context.Context) ([]contribapi.Contribution, error) {
	var out []contribapi.Contribution
	for _, c := range reachFixture() {
		if c.Type == p.typ {
			out = append(out, c)
		}
	}
	return out, nil
}
func (p fixtureProvider) Open(ctx context.Context, id, layer string) (io.ReadCloser, contribapi.LayerRef, error) {
	return contribapi.OpenEnumerated(ctx, p, id, layer)
}

// selfService is the binary's own ResourceService as OpenVia reaches it: the
// index its List answers (every layer with its resident bytes), and the real
// ResourceServiceImpl.OpenLayer over the real providers for the rest.
type selfService struct {
	mbapi.ResourceService
	real *ResourceServiceImpl
}

func (s selfService) List(context.Context, mbapi.ContributionListInput) (mbapi.ContributionListOutput, error) {
	return mbapi.ContributionListOutput{Contributions: reachFixture()}, nil
}
func (s selfService) OpenLayer(ctx context.Context, in mbapi.OpenLayerInput) (mbapi.OpenLayerOutput, error) {
	return s.real.OpenLayer(ctx, in)
}

func TestOpenErrors_IdenticalAtEveryReach(t *testing.T) {
	ctx := context.Background()
	real := &ResourceServiceImpl{builder: NewBuilder("p", "dev"), providers: func(_ context.Context, provider, typ string) []contribapi.ContributionProvider {
		if provider != "p" {
			return nil
		}
		return []contribapi.ContributionProvider{fixtureProvider{typ: typ}}
	}}
	self := selfService{real: real}
	hub := seedCatalog(t, contribapi.ContributionsConfig{}, &validatorStub{}, reachFixture())
	view := NewMemoryCatalog([]*mbapi.Manifest{{Provider: "p", Contributions: reachFixture()}}, nil)

	reaches := map[string]func(uri, layer string) error{
		"self": func(uri, layer string) error {
			_, err := mbapi.OpenVia(ctx, self, uri, layer)
			return err
		},
		"aggregator (CatalogImpl)": func(uri, layer string) error {
			_, _, err := hub.OpenLayer(ctx, uri, layer)
			return err
		},
		"consumer view (MemoryCatalog)": func(uri, layer string) error {
			_, _, err := view.OpenLayer(ctx, uri, layer)
			return err
		},
	}
	cases := []struct {
		name, uri, layer string
		is               func(error) bool
	}{
		{"not found", "p:/skill/none", contribapi.LayerData, func(err error) bool {
			return errors.Is(err, contribapi.ErrContributionNotFound{})
		}},
		{"not declared", "p:/skill/s", "thumbnail", func(err error) bool {
			var e contribapi.ErrLayerNotDeclared
			return errors.As(err, &e) && e.Layer == "thumbnail"
		}},
		{"not carried", "p:/skill/s", contribapi.LayerSource, func(err error) bool {
			var e contribapi.ErrLayerNotCarried
			return errors.As(err, &e) && e.Layer == contribapi.LayerSource
		}},
		{"bulk", "p:/reference/dataset", contribapi.LayerSource, func(err error) bool {
			var e contribapi.ErrBulkLayer
			return errors.As(err, &e) && e.Ref.Hash == "sha256:big"
		}},
	}
	for reach, open := range reaches {
		for _, c := range cases {
			if err := open(c.uri, c.layer); !c.is(err) {
				t.Errorf("%s, %s (%s layer %q): err = %v (%T)", reach, c.name, c.uri, c.layer, err, err)
			}
		}
	}
	// The universe views keep their own sentinel too, wrapping the one
	// not-found: callers that test for it still match.
	for name, err := range map[string]error{
		"CatalogImpl":   func() error { _, _, err := hub.OpenLayer(ctx, "p:/skill/none", ""); return err }(),
		"MemoryCatalog": func() error { _, _, err := view.OpenLayer(ctx, "p:/skill/none", ""); return err }(),
	} {
		if !errors.Is(err, mbapi.ErrSidecarNotInCatalog) || !errors.Is(err, contribapi.ErrContributionNotFound{}) {
			t.Errorf("%s not-found = %v, want ErrSidecarNotInCatalog wrapping ErrContributionNotFound", name, err)
		}
	}
}
