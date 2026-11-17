// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"
	"errors"
	"fmt"
	"io"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
)

// layer.go — reading a layer's content, at any size.
//
// One operation covers every band, which is the point: a caller asks for a
// layer and reads it, and nothing in that code says how big the content is or
// where it came from. Reading a 2 KB skill body and reading a 40 GB dataset are
// the same three lines.
//
// What differs is what the caller may then DO with the handle. Bytes are
// available for content small enough to hold, and refused for content that is
// not — refused explicitly, at the point of asking, rather than by exhausting
// memory somewhere later.

// LayerOpener is implemented by a catalog that can stream a layer's content.
//
// Optional: a catalog that does not implement it still serves bodies through
// the ordinary read path. Transport belongs with the catalog, so the decision
// between "inline", "one bounded fetch" and "object store" is made once, where
// the information to make it lives.
type LayerOpener interface {
	OpenLayer(ctx context.Context, uri, layer string) (io.ReadCloser, contribapi.LayerRef, error)
}

// Layer is an open handle on one layer's content.
//
// It is a reader, not bytes. Nothing has been read when Open returns, so a
// caller that wants a prefix reads a prefix, one that wants a file streams to
// a file, and one that changes its mind closes the handle having moved nothing.
//
// The caller closes it.
type Layer struct {
	// URI and Name identify what was opened.
	URI  string
	Name string

	// Ref is the layer's descriptor: hash, size and media type. Size is
	// known BEFORE any content moves, so a caller can decide what to do
	// with a layer without fetching it first.
	Ref contribapi.LayerRef

	rc io.ReadCloser
}

// Read implements io.Reader.
func (l *Layer) Read(p []byte) (int, error) {
	if l.rc == nil {
		return 0, io.EOF
	}
	return l.rc.Read(p)
}

// Close releases the handle. Safe on a Layer that was never read.
func (l *Layer) Close() error {
	if l.rc == nil {
		return nil
	}
	return l.rc.Close()
}

// Band reports how this layer travels.
func (l *Layer) Band() contribapi.Band { return l.Ref.Band() }

// ErrLayerTooLargeForMemory is returned by Bytes for bulk-band content.
var ErrLayerTooLargeForMemory = fmt.Errorf("layer is in the bulk band: stream it with WriteFile or Read, do not materialise it")

// Bytes reads the whole layer into memory, and REFUSES to do so for the bulk
// band.
//
// The refusal is the feature. Content in that band has no upper bound — a
// dataset, an archive, a video — so a function that quietly returned its bytes
// would be a function that works in every test and exhausts memory in
// production. A caller that genuinely wants bulk content in memory reads it
// through Read and takes responsibility explicitly.
//
// The size is checked before reading, so nothing is transferred to discover
// that it was too big.
//
// Bytes is TERMINAL: it closes the handle either way. On the refusal path too —
// a caller that gets an error is not going to call Close, and an open handle on
// an object store is a held resource, so refusing without releasing it would
// leak exactly where content is largest. Check Band or Ref.IsBulk first if you
// want to choose before committing.
func (l *Layer) Bytes() ([]byte, error) {
	defer func() { _ = l.Close() }()
	if l.Ref.IsBulk() {
		return nil, fmt.Errorf("%s layer %q (%d bytes): %w", l.URI, l.Name, l.Ref.Size, ErrLayerTooLargeForMemory)
	}
	return io.ReadAll(l.rc)
}

// WriteFile streams the layer to a local file and returns the bytes written.
//
// This is how content of any size is consumed: it goes from the object store to
// disk through a buffer, so peak memory is the buffer whether the layer is a
// kilobyte or a hundred gigabytes.
//
// The content is verified against its hash as it streams, and a mismatch leaves
// no file behind. The destination path never names a partial file — the write
// lands beside it and is renamed into place.
func (l *Layer) WriteFile(path string) (int64, error) {
	defer func() { _ = l.Close() }()
	if l.rc == nil {
		return 0, fmt.Errorf("%s layer %q: nothing to read", l.URI, l.Name)
	}
	return contribapi.StreamToFile(path, l.Ref.Hash, l.rc)
}

// Open returns a handle on one layer's content, at either reach.
//
// `layer` names which content: contribapi.LayerData is the structured body that
// validates against the kind's schema, contribapi.LayerSource the raw bytes a
// consumer renders. Empty means the kind's declared default layer.
//
// Nothing is read here. Open resolves where the content is and hands back a
// reader; the caller decides what moves.
func Open(ctx context.Context, q Q, uri string, layer string) (*Layer, error) {
	if layer == "" {
		// The KIND says which layer is the default — not a constant here. A
		// data-type opens its DSL, a skill its markdown, a task its body; the
		// declaration is where that is written, and this is where it is read.
		parsed, err := contribapi.ParseResourceURI(uri)
		if err != nil {
			return nil, fmt.Errorf("contributions: open %q: %w", uri, err)
		}
		kinds, err := KindsAt(ctx, q)
		if err != nil {
			return nil, err
		}
		layer = kinds.DefaultLayer(parsed.Type)
	}
	switch q.Reach {
	case ReachSelf:
		return openLayerSelf(ctx, uri, layer)
	case ReachUniverse:
		return openLayerUniverse(ctx, q, uri, layer)
	default:
		return nil, ErrReachUnset
	}
}

// openLayerUniverse streams through the aggregate's transport. An aggregate
// that cannot open layers is an error, not a downgrade to the body lane: a
// caller asked for a NAMED layer, and handing back a different one is the
// fault this surface exists to remove.
func openLayerUniverse(ctx context.Context, q Q, uri, layer string) (*Layer, error) {
	cat, err := q.aggregate(ctx)
	if err != nil {
		return nil, err
	}
	opener, ok := cat.(LayerOpener)
	if !ok {
		return nil, fmt.Errorf("contributions: open %s layer %q: this aggregate (%T) cannot open layers", uri, layer, cat)
	}
	rc, ref, oerr := opener.OpenLayer(ctx, uri, layer)
	if oerr != nil {
		return nil, fmt.Errorf("contributions: open %s layer %q: %w", uri, layer, oerr)
	}
	return &Layer{URI: uri, Name: layer, Ref: ref, rc: rc}, nil
}

// openLayerSelf opens a layer through the binary's own resource service.
func openLayerSelf(ctx context.Context, uri, layer string) (*Layer, error) {
	svc, err := selfResourceService(ctx)
	if err != nil {
		return nil, err
	}
	return OpenVia(ctx, svc, uri, layer)
}

// OpenVia is the self-reach content lane over any ResourceService: the entry
// from the service's own index (WITH resident bytes), answered by
// contribapi.OpenLayerFrom — the one band-aware open. A layer the index does
// not carry is fetched through the service's OpenLayer (the producer's own
// enumeration) and checked against its address, so a producer whose file
// changed since the index was built is caught rather than served.
//
// Known asymmetry with the universe reach: this path MATERIALISES. The wire
// behind it carries whole layers, so a bulk-band layer is refused
// (contribapi.ErrBulkLayer, which also matches ErrLayerTooLargeForMemory here).
func OpenVia(ctx context.Context, svc ResourceService, uri, layer string) (*Layer, error) {
	yes := true
	out, err := svc.List(ctx, ContributionListInput{
		WithBody: true, IncludeLocal: true, IncludeSystem: &yes,
		Filter: ContributionListFilter{URIs: []string{uri}},
	})
	if err != nil {
		return nil, fmt.Errorf("contributions: open %s: %w", uri, err)
	}
	var entry contribapi.Contribution
	found := false
	for _, c := range out.Contributions {
		if c.ResolvedURI() == uri {
			entry, found = c, true
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("contributions: open %s: %w", uri, ErrContributionNotFound{URI: uri})
	}
	rc, ref, err := contribapi.OpenLayerFrom(entry, layer, contribapi.LayerLanes{
		Fetch: func(contribapi.LayerRef) ([]byte, string, error) {
			opened, err := svc.OpenLayer(ctx, OpenLayerInput{URI: uri, Layer: layer})
			if err != nil {
				return nil, "", err
			}
			if rerr := RefusedErr(uri, layer, opened); rerr != nil {
				return nil, "", rerr
			}
			return opened.Content, opened.Ref.MediaType, nil
		},
	})
	if err != nil {
		var bulk contribapi.ErrBulkLayer
		if errors.As(err, &bulk) {
			return nil, fmt.Errorf("contributions: open %s layer %q: %w: %w", uri, layer, err, ErrLayerTooLargeForMemory)
		}
		return nil, fmt.Errorf("contributions: open %s: %w", uri, err)
	}
	return &Layer{URI: uri, Name: layer, Ref: ref, rc: rc}, nil
}

// Refusal renders a typed refusal — contribapi.ErrBulkLayer or
// contribapi.ErrLayerNotCarried, wherever in err's chain — as the wire's
// OpenLayerOutput.Refused text. ok is false for any other error: that one is
// an error on the wire, not a refusal.
func Refusal(err error) (text string, ok bool) {
	var bulk contribapi.ErrBulkLayer
	if errors.As(err, &bulk) {
		return bulk.Error(), true
	}
	var notCarried contribapi.ErrLayerNotCarried
	if errors.As(err, &notCarried) {
		return notCarried.Error(), true
	}
	return "", false
}

// RefusedErr turns a wire refusal back into its typed error, or nil when out
// carries content: the bulk band when the descriptor says so, else a layer the
// far side knows the address of but carries no bytes for. The two are the
// only refusals Refusal renders, so the round trip keeps the type.
func RefusedErr(uri, layer string, out OpenLayerOutput) error {
	if out.Refused == "" {
		return nil
	}
	if out.Ref.IsBulk() {
		return contribapi.ErrBulkLayer{Ref: out.Ref}
	}
	return contribapi.ErrLayerNotCarried{URI: uri, Layer: layer}
}

// BlobOrigins names where published bulk content can be read from.
//
// The consumer's own store is a cache and starts empty — installing a pack that
// references a 40 GB dataset must cost what installing a 500-byte archive
// costs. So a miss has to fall through to wherever the content was published,
// which for this system is the marketplace it came from.
//
// An interface rather than a parameter because the two sides sit on opposite
// layers: the catalog serves content and must not know what a marketplace is,
// while the marketplace knows exactly where it put things. A binary that has
// marketplaces registers this; one that does not reads only what it holds, and
// says so rather than pretending the content is unreachable for some other
// reason.
type BlobOrigins interface {
	// BlobRoots returns the roots to consult, most-preferred first. A root is
	// a filesystem path today and a base URL when the store is remote —
	// resolved by scheme at fetch time, which is why the same list serves
	// both.
	BlobRoots() []string
}
