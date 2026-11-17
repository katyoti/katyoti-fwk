// SPDX-License-Identifier: Apache-2.0

// Package providertest is the contract test every ContributionProvider
// family runs: one definition of what "a provider" promises, so a new
// implementation is checked against the contract rather than against the
// habits of the one it was copied from.
package providertest

import (
	"context"
	"errors"
	"io"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
)

// undeclaredLayer is a layer name no kind declares.
const undeclaredLayer = "providertest-undeclared-layer"

// unknownID is an id no provider enumerates.
const unknownID = "providertest-unknown-id/does-not-exist"

// Run checks p against the ContributionProvider contract:
//
//   - Identity keeps the identity rule: an immutable provider carries a
//     digest, and it is the digest of what Enumerate served
//     (contribapi.DigestOf); a mutable one carries none;
//   - Enumerate succeeds;
//   - every declared, non-bulk layer that carries inline bytes opens through
//     Open, and the bytes hash to the layer's address (LayerRef.Hash), with
//     the returned ref describing the same layer and carrying no inline bytes;
//   - a bulk layer is refused with ErrBulkLayer;
//   - a layer the entry does not declare is ErrLayerNotDeclared;
//   - an id the provider does not enumerate is ErrContributionNotFound.
//
// Run enumerates with context.Background(); a provider that reads the
// application's extension points runs through RunContext.
func Run(t *testing.T, p contribapi.ContributionProvider) {
	t.Helper()
	RunContext(t, context.Background(), p)
}

// RunContext is Run under the context the provider enumerates with (an
// application context, for a provider that reads extension points).
func RunContext(t *testing.T, ctx context.Context, p contribapi.ContributionProvider) {
	t.Helper()
	entries, err := p.Enumerate(ctx)
	if err != nil {
		t.Fatalf("%s/%s: Enumerate: %v", p.Provider(), p.Type(), err)
	}
	// Identity is read after Enumerate, as the builder reads it: it names
	// the version just enumerated.
	id := p.Identity()
	if why := contribapi.IdentityRule(id); why != "" {
		t.Errorf("%s/%s: %s (identity %+v)", p.Provider(), p.Type(), why, id)
	}
	if !id.Mutable && id.Digest != "" {
		if want := contribapi.DigestOf(entries); id.Digest != want {
			t.Errorf("%s/%s: digest %s is not the enumeration's epoch %s", p.Provider(), p.Type(), id.Digest, want)
		}
	}
	opened := 0
	for _, c := range entries {
		for name, decl := range c.Layers {
			switch {
			case decl.IsBulk():
				_, _, oerr := p.Open(ctx, c.ID, name)
				var bulk contribapi.ErrBulkLayer
				if !errors.As(oerr, &bulk) {
					t.Errorf("%s layer %q: bulk band must be ErrBulkLayer, got %v", c.ResolvedURI(), name, oerr)
				}
			case len(decl.Inline) > 0:
				rc, ref, oerr := p.Open(ctx, c.ID, name)
				if oerr != nil {
					t.Errorf("%s layer %q: Open: %v", c.ResolvedURI(), name, oerr)
					continue
				}
				got, rerr := io.ReadAll(rc)
				_ = rc.Close()
				if rerr != nil {
					t.Errorf("%s layer %q: read: %v", c.ResolvedURI(), name, rerr)
					continue
				}
				if h := contribapi.HashContent(got); h != decl.Hash {
					t.Errorf("%s layer %q: opened bytes hash to %s, the layer's address is %s", c.ResolvedURI(), name, h, decl.Hash)
				}
				if ref.Hash != decl.Hash || ref.MediaType != decl.MediaType || ref.Size != decl.Size {
					t.Errorf("%s layer %q: Open's ref %+v does not describe the enumerated layer %+v", c.ResolvedURI(), name, withoutInline(ref), withoutInline(decl))
				}
				if ref.Inline != nil {
					t.Errorf("%s layer %q: Open's ref carries inline bytes; they travel through the reader", c.ResolvedURI(), name)
				}
				opened++
			}
		}
		if _, _, oerr := p.Open(ctx, c.ID, undeclaredLayer); !errors.As(oerr, new(contribapi.ErrLayerNotDeclared)) {
			t.Errorf("%s: an undeclared layer must be ErrLayerNotDeclared, got %v", c.ResolvedURI(), oerr)
		}
	}
	if _, _, oerr := p.Open(ctx, unknownID, contribapi.LayerData); !errors.As(oerr, new(contribapi.ErrContributionNotFound)) {
		t.Errorf("%s/%s: an unknown id must be ErrContributionNotFound, got %v", p.Provider(), p.Type(), oerr)
	}
	t.Logf("%s/%s: %d entries, %d layers opened", p.Provider(), p.Type(), len(entries), opened)
}

func withoutInline(r contribapi.LayerRef) contribapi.LayerRef {
	r.Inline = nil
	return r
}
