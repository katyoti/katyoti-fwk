// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"errors"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"

	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
)

// TestResourceService_Get tests the public Get path against an in-context
// ContributionProvider. List/Types use the EM-fetch path which would require
// a full Application setup to test — covered by the builder tests via
// BuildFromProviders. Here we verify Get's URI parsing + provider resolution.
//
// Since FindProvider also reads from EM, a true unit test of the Get path
// in core requires either an app-fixture or a mockable EM. For now we
// demonstrate the contract — full integration coverage lands when we have
// the shared app-fixture helper from a later phase.

func TestParseURI_RoundTrip(t *testing.T) {
	// Sanity: any URI built via Contribution.URI() round-trips through ParseURI.
	// Type tokens must be strict kebab; ID is loose (paths + extensions OK).
	cases := []contribapi.Contribution{
		{Provider: "ktw-docs", Type: "skill", ID: "doc-create"},
		{Provider: "ktw-x", Type: "reference", ID: "templates/report.typ"},
		{Provider: "ktw-y", Type: "template", ID: "deep/nested/path/file.tmpl"},
	}
	for _, e := range cases {
		t.Run(e.ResolvedURI(), func(t *testing.T) {
			uri, err := contribapi.ParseResourceURI(e.ResolvedURI())
			if err != nil {
				t.Fatalf("ParseURI: %v", err)
			}
			if uri.Provider != e.Provider || uri.Type != e.Type || uri.ID != e.ID {
				t.Errorf("round-trip failure: got (%q,%q,%q) want (%q,%q,%q)",
					uri.Provider, uri.Type, uri.ID, e.Provider, e.Type, e.ID)
			}
		})
	}
}

func TestResourceService_InterfaceContract(t *testing.T) {
	// Compile-time: the impl satisfies the api interface.
	var _ mbapi.ResourceService = NewResourceService("test", "dev")
}

func TestResourceService_GetURI_BadURI(t *testing.T) {
	s := NewResourceService("ktw-test", "dev")
	_, err := s.Get(context.Background(), mbapi.ContributionGetInput{URI: "no-colon-slash-here"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	// New ParseURI returns errors wrapping sentinel contribapi.ErrInvalidURI.
	// Either via errors.Is or by inspecting the wrapped chain — both work.
	if !errors.Is(err, contribapi.ErrInvalidURI) {
		t.Errorf("expected wrapped contribapi.ErrInvalidURI, got %T: %v", err, err)
	}
}

func TestResourceService_Read_BadURI(t *testing.T) {
	s := NewResourceService("ktw-test", "dev")
	_, err := s.Read(context.Background(), mbapi.ContributionReadInput{URI: "no-colon-slash-here"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, contribapi.ErrInvalidURI) {
		t.Errorf("expected wrapped contribapi.ErrInvalidURI, got %T: %v", err, err)
	}
}

func TestResourceService_Read_NoProvider(t *testing.T) {
	// Bare ctx → no Application → no contributed providers → the Read
	// content lane must answer ErrContributionNotFound, never a panic
	// or a silent empty body (the regression the eval run exposed: a
	// reference URI must either yield content or a loud miss).
	s := NewResourceService("ktw-test", "dev")
	_, err := s.Read(context.Background(), mbapi.ContributionReadInput{URI: "ktw-test:/reference/missing.md"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var notFound mbapi.ErrContributionNotFound
	if !errors.As(err, &notFound) {
		t.Errorf("expected ErrContributionNotFound, got %T: %v", err, err)
	}
}
