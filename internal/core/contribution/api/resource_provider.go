// SPDX-License-Identifier: Apache-2.0

package contribapi

import (
	"bytes"
	"context"
	"fmt"
	"io"
)

// ContributionProvider produces Contribution entries for the descriptor
// without instantiating the runtime that consumes them.
//
// One generic implementation (`EmbedFSResourceProvider` in the manifest contrib
// package) covers the common case of "a folder of files in an embedded FS."
// Specialized providers exist for static lists, generated capabilities, etc. —
// but the design goal is "few generic providers, parameterized by type."
//
// The contract is five methods and nothing optional: a provider names what it
// publishes (Type, Provider), states what its source is (Identity), lists it
// (Enumerate) and serves one named layer of one entry (Open). There is no
// second read path: the enumeration IS the content, and Open answers from it.
type ContributionProvider interface {
	// Type is the discriminator written into descriptor entries
	// ("skill", "task", "reference", "config-file", "ktw-docs.template", ...).
	// Open set; lowercase + hyphen, ≤63 chars.
	Type() string

	// Provider is the binary that hosts these contributions ("ktw-docs", etc.).
	Provider() string

	// Identity states whether this provider's source can change while the
	// process runs, and which version of it is being served: an immutable
	// provider carries its enumeration digest, a mutable one none (see
	// Identity). Mandatory: every cache downstream keys on it, and a provider
	// that cannot prove its source is fixed must say it is mutable. Read after
	// Enumerate: a provider whose enumeration needs the application context
	// stamps its digest there (ImmutableEpoch.Record).
	Identity() Identity

	// Enumerate returns descriptor contributions — pure data, no side effects.
	// Called at descriptor-build time; MUST NOT spawn anything or touch
	// runtime services. Every layer carries its bytes up to the bulk band;
	// residency is the manifest builder's decision, applied after.
	Enumerate(ctx context.Context) ([]Contribution, error)

	// Open serves ONE named layer of the entry `id` (the part after
	// `<provider>:/<type>/`). The layer name is always explicit here: the
	// kind's default layer is the resource service's business, never the
	// provider's. The returned LayerRef describes the layer with Inline
	// cleared — the bytes travel through the reader.
	//
	// Errors: ErrContributionNotFound (no such id), ErrLayerNotDeclared (the
	// entry declares no such layer), ErrLayerNotCarried (declared and
	// addressed, but no bytes held), ErrBulkLayer (the layer is in the bulk
	// band and is streamed out of band by hash). OpenEnumerated implements all
	// of it from the provider's own enumeration.
	Open(ctx context.Context, id, layer string) (io.ReadCloser, LayerRef, error)
}

// ErrContributionNotFound reports that no provider serves the URI. One error
// for every reach: the producer primitive (Open), the resource service above
// it (mbapi.ErrContributionNotFound is this type) and the universe views
// (which may wrap it in mbapi.ErrSidecarNotInCatalog).
//
// errors.Is(err, ErrContributionNotFound{}) holds for any URI (see Is), so a
// caller asks "not found?" without knowing which URI was asked.
type ErrContributionNotFound struct{ URI string }

func (e ErrContributionNotFound) Error() string { return "contribution not found: " + e.URI }

// Is matches any ErrContributionNotFound, whatever its URI.
func (e ErrContributionNotFound) Is(target error) bool {
	_, ok := target.(ErrContributionNotFound)
	return ok
}

// ErrLayerNotDeclared reports that the entry exists but declares no layer of
// that name.
type ErrLayerNotDeclared struct{ URI, Layer string }

func (e ErrLayerNotDeclared) Error() string {
	return fmt.Sprintf("open %s: declares no layer %q", e.URI, e.Layer)
}

// ErrLayerNotCarried reports a declared layer whose address this reach knows
// but whose bytes it neither holds nor can fetch.
type ErrLayerNotCarried struct{ URI, Layer string }

func (e ErrLayerNotCarried) Error() string {
	return fmt.Sprintf("layer %q of %s: its address is known here but its bytes are not carried", e.Layer, e.URI)
}

// ErrBulkLayer reports a layer in the bulk band: it is never carried in a
// message, and is streamed out of band by its hash. Ref describes it.
type ErrBulkLayer struct{ Ref LayerRef }

func (e ErrBulkLayer) Error() string {
	return fmt.Sprintf("layer is in the bulk band (%d bytes, %s): stream it out of band by hash", e.Ref.Size, e.Ref.Hash)
}

// ErrContentMismatch reports fetched bytes that do not hash to the address the
// index holds for them: the source changed since the index was built, or the
// sender is wrong. Content is addressed, so it is refused rather than served
// or stored.
type ErrContentMismatch struct{ URI, Layer, Want, Got string }

func (e ErrContentMismatch) Error() string {
	return fmt.Sprintf("layer %q of %s: content does not match its address (index %s, fetched %s)", e.Layer, e.URI, e.Want, e.Got)
}

// VerifyContent checks fetched bytes against the address the index holds for
// them. Producers hash the exact bytes they carry (HashContent over what
// SetData / SetLayer received) and every transport moves those bytes verbatim
// (LayerRef.Inline and the wire's Content are []byte, base64 over JSON), so
// the check needs no canonicalisation.
func VerifyContent(uri, layer, want string, b []byte) error {
	if got := HashContent(b); got != want {
		return ErrContentMismatch{URI: uri, Layer: layer, Want: want, Got: got}
	}
	return nil
}

// LayerLanes are the routes a reach has to a layer's bytes beyond the entry it
// holds. Both are optional: a reach without a lane answers with the typed
// refusal instead.
type LayerLanes struct {
	// Fetch reaches the bytes of a declared, non-bulk layer the entry does not
	// carry inline (the control band, or a non-resident layer). It returns the
	// bytes and, when known, their media type; OpenLayerFrom verifies them
	// against the layer's address. Nil: ErrLayerNotCarried.
	Fetch func(ref LayerRef) ([]byte, string, error)
	// Bulk streams a bulk-band layer out of band. Nil: ErrBulkLayer.
	Bulk func(ref LayerRef) (io.ReadCloser, error)
}

// OpenLayerFrom is THE band-aware answer to "open layer L of this entry": the
// one place the decision and its typed errors live. Every reach finds its
// entry its own way (a provider's enumeration, the aggregator's tier 1, a
// consumer view, a producer's index) and then asks this.
//
//   - a layer the entry does not declare: ErrLayerNotDeclared;
//   - a bulk-band layer: the Bulk lane, else ErrBulkLayer;
//   - inline bytes: served, no call;
//   - otherwise the Fetch lane, verified against the address
//     (ErrContentMismatch), else ErrLayerNotCarried.
//
// The returned LayerRef describes the layer with Inline cleared — the bytes
// travel through the reader — and still describes it on ErrBulkLayer and
// ErrLayerNotCarried, so a caller can say what it refused.
func OpenLayerFrom(entry Contribution, layer string, lanes LayerLanes) (io.ReadCloser, LayerRef, error) {
	uri := entry.ResolvedURI()
	ref, declared := entry.Layers[layer]
	if !declared {
		return nil, LayerRef{}, ErrLayerNotDeclared{URI: uri, Layer: layer}
	}
	inline := ref.Inline
	ref.Inline = nil
	if ref.IsBulk() {
		if lanes.Bulk == nil {
			return nil, ref, ErrBulkLayer{Ref: ref}
		}
		rc, err := lanes.Bulk(ref)
		return rc, ref, err
	}
	if len(inline) > 0 {
		return io.NopCloser(bytes.NewReader(inline)), ref, nil
	}
	if lanes.Fetch == nil {
		return nil, ref, ErrLayerNotCarried{URI: uri, Layer: layer}
	}
	b, mediaType, err := lanes.Fetch(ref)
	if err != nil {
		return nil, ref, err
	}
	if err := VerifyContent(uri, layer, ref.Hash, b); err != nil {
		return nil, ref, err
	}
	if ref.MediaType == "" {
		ref.MediaType = mediaType
	}
	return io.NopCloser(bytes.NewReader(b)), ref, nil
}

// OpenEnumerated implements ContributionProvider.Open from p's own
// enumeration: find the entry by id and answer through OpenLayerFrom with no
// lanes. A provider carries every layer's bytes up to the bulk band, so the
// entry it enumerates holds exactly what Open serves — one answer, not a
// second read path that can disagree with the first.
func OpenEnumerated(ctx context.Context, p ContributionProvider, id, layer string) (io.ReadCloser, LayerRef, error) {
	uri := p.Provider() + ":/" + p.Type() + "/" + id
	items, err := p.Enumerate(ctx)
	if err != nil {
		return nil, LayerRef{}, fmt.Errorf("open %s: %w", uri, err)
	}
	for _, c := range items {
		if c.ID != id {
			continue
		}
		if c.Provider == "" || c.Type == "" {
			c.Provider, c.Type = p.Provider(), p.Type()
		}
		return OpenLayerFrom(c, layer, LayerLanes{})
	}
	return nil, LayerRef{}, ErrContributionNotFound{URI: uri}
}

// Identity describes a provider's source: whether it can change, and which
// version of it a consumer is looking at.
//
// Identity is an EPOCH (epoch.go). An immutable source has one version for the
// process lifetime and names it: Digest is the digest of its enumeration
// (DigestOf — sorted URI + every layer hash), computed once. A mutable source
// has no single version, so it carries no digest and says Mutable; its
// consumers re-read it by the stat and content digests the tier-1 cache keeps.
// The census and providertest refuse the two other combinations (IdentityRule).
type Identity struct {
	// Digest names this version of an immutable source: the enumeration
	// digest. Empty for a mutable source, and only for one.
	Digest string

	// Mutable reports whether the source can change while the process runs.
	// DECLARED, never inferred: a provider that cannot prove its source is
	// fixed must say it is mutable.
	Mutable bool
}
