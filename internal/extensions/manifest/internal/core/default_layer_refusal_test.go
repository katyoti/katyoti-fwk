// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
)

// undeclaredKindProvider publishes one entry of a kind whose declaration this
// producer does not carry (the pack that declares it lives elsewhere, as the
// polyglot producer's `recipe` does).
type undeclaredKindProvider struct{ entry contribapi.Contribution }

func (p undeclaredKindProvider) Type() string     { return p.entry.Type }
func (p undeclaredKindProvider) Provider() string { return p.entry.Provider }
func (p undeclaredKindProvider) Identity() contribapi.Identity {
	return contribapi.Identity{Mutable: true}
}
func (p undeclaredKindProvider) Enumerate(context.Context) ([]contribapi.Contribution, error) {
	return []contribapi.Contribution{p.entry}, nil
}
func (p undeclaredKindProvider) Open(ctx context.Context, id, layer string) (io.ReadCloser, contribapi.LayerRef, error) {
	return contribapi.OpenEnumerated(ctx, p, id, layer)
}

// TestOpenLayer_DefaultOfAnUndeclaredKindIsRefused: a producer that carries
// no declaration of a kind has no default layer for it. It used to guess
// `source`; it refuses with ErrLayerNotDeclared naming the gap, and answers
// the layer when it is named (re-audit N13, 2026-09-26).
func TestOpenLayer_DefaultOfAnUndeclaredKindIsRefused(t *testing.T) {
	var e contribapi.Contribution
	e.Provider, e.Type, e.ID = "poly", "widget", "w1"
	e.SetData(json.RawMessage(`{"name":"w1"}`))
	p := undeclaredKindProvider{entry: e}
	svc := NewResourceService("poly", "1")
	svc.providers = func(_ context.Context, provider, typ string) []contribapi.ContributionProvider {
		if provider == "poly" && typ == "widget" {
			return []contribapi.ContributionProvider{p}
		}
		return nil
	}
	uri := "poly:/widget/w1"
	_, err := svc.OpenLayer(context.Background(), mbapi.OpenLayerInput{URI: uri})
	if err == nil {
		t.Fatalf("Open(default) of a kind this producer never declared succeeded: it guessed a layer")
	}
	var nd contribapi.ErrLayerNotDeclared
	if !errors.As(err, &nd) || nd.URI != uri {
		t.Fatalf("want ErrLayerNotDeclared for %s, got %v", uri, err)
	}
	out, err := svc.OpenLayer(context.Background(), mbapi.OpenLayerInput{URI: uri, Layer: contribapi.LayerData})
	if err != nil || out.Refused != "" || len(out.Content) == 0 {
		t.Fatalf("naming the layer must still answer: err=%v refused=%q content=%d", err, out.Refused, len(out.Content))
	}
}
