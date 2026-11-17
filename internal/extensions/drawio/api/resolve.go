// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
)

// ErrNotFound is returned when no contribution matches a requested URI.
var ErrNotFound = errors.New("drawio contribution not found")

// Drawing is everything a viewpoint brings to a render: the viewpoint
// itself, its notation and its template library, resolved from the
// catalog.
type Drawing struct {
	ViewpointURI string
	Viewpoint    *ViewpointBody
	Notation     *NotationBody
	Library      *TemplateLibraryBody
}

// ResolveDrawing resolves a viewpoint URI (provider:/viewpoint/id) and the
// notation and template library it names.
func ResolveDrawing(ctx context.Context, viewpointURI string) (*Drawing, error) {
	vp, err := ResolveViewpoint(ctx, viewpointURI)
	if err != nil {
		return nil, err
	}
	n, err := ResolveNotation(ctx, vp.Notation)
	if err != nil {
		return nil, fmt.Errorf("viewpoint %s: %w", viewpointURI, err)
	}
	lib, err := ResolveTemplateLibrary(ctx, vp.Templates)
	if err != nil {
		return nil, fmt.Errorf("viewpoint %s: %w", viewpointURI, err)
	}
	return &Drawing{ViewpointURI: viewpointURI, Viewpoint: vp, Notation: n, Library: lib}, nil
}

// ResolveViewpoint fetches a viewpoint contribution by URI.
func ResolveViewpoint(ctx context.Context, uri string) (*ViewpointBody, error) {
	var b ViewpointBody
	return &b, resolveBody(ctx, uri, ContributionTypeViewpoint, &b)
}

// ResolveNotation fetches a notation contribution by URI.
func ResolveNotation(ctx context.Context, uri string) (*NotationBody, error) {
	var b NotationBody
	return &b, resolveBody(ctx, uri, ContributionTypeNotation, &b)
}

// ResolveTemplateLibrary fetches a drawio-template contribution by URI.
func ResolveTemplateLibrary(ctx context.Context, uri string) (*TemplateLibraryBody, error) {
	var b TemplateLibraryBody
	return &b, resolveBody(ctx, uri, ContributionTypeDrawioTemplate, &b)
}

// resolveBody fetches the contribution at uri, checks its kind, and
// decodes its data layer into out.
//
// Resolution path: parse the URI, look up the provider's manifest through
// the universe Catalog, find the matching contribution, decode its body
// (fetched lazily through Catalog.GetBody when the index carries none).
// It behaves the same standalone (the sidecar sees its own manifest) and
// under the hub (the catalog aggregates every sidecar), so a sidecar can
// publish its own notations, viewpoints and template libraries.
func resolveBody(ctx context.Context, uri, kind string, out any) error {
	parsed, err := contribapi.ParseResourceURI(uri)
	if err != nil {
		return fmt.Errorf("parse %s URI %q: %w", kind, uri, err)
	}
	if parsed.Type != kind {
		return fmt.Errorf("URI %q is not a %s (type=%q)", uri, kind, parsed.Type)
	}
	cat, err := application.UseService[mbapi.Catalog](ctx)
	if err != nil {
		return fmt.Errorf("resolve catalog: %w", err)
	}
	manifest, err := cat.Get(ctx, parsed.Provider)
	if err != nil {
		return fmt.Errorf("resolve manifest for provider %q: %w", parsed.Provider, err)
	}
	for i := range manifest.Contributions {
		c := &manifest.Contributions[i]
		if c.ResolvedURI() != uri {
			continue
		}
		raw := c.Data()
		if len(raw) == 0 {
			if raw, err = cat.GetBody(ctx, uri); err != nil {
				return fmt.Errorf("resolve %s body %s: %w", kind, uri, err)
			}
		}
		if err := json.Unmarshal(raw, out); err != nil {
			return fmt.Errorf("decode %s body %s: %w", kind, uri, err)
		}
		return nil
	}
	return fmt.Errorf("%w: %s", ErrNotFound, uri)
}
