// SPDX-License-Identifier: Apache-2.0

// Package remote reads a universe's catalog over the bus.
//
// It exists so that a consumer in another process — the GUI, a CLI, anything
// holding a Sender — asks the same question through the same surface as a
// consumer in the aggregator's own process. Before it, each such consumer
// hand-built the aggregator's address, hand-decoded its wire shape, and
// hand-rolled its cursor loop; four browsers in one binary did this four
// different times, and none of them got the view cache, the typed view or the
// recorded verdict that the query surface provides for free.
//
// # Weight is the reason this is its own package
//
// The in-process catalog costs 55 first-party packages. A GUI that imported it
// to read a list would roughly double its own weight, which is the cost this
// milestone spent itself removing. This package holds a Sender and the
// contribution contract, and nothing else.
//
// # One address, in one place
//
// Every `ktw-hub:/CatalogService/*` literal in the tree belongs here. A
// consumer names a UNIVERSE, not a method on a service in a binary — and when
// the aggregator's address changes, it changes once.
package remote

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	ktsapi "github.com/katyoti/katyoti-fwk/internal/kts/api"
)

// aggregator is the catalog service's address. The ONE place it is spelled.
const aggregator = "ktw-hub:/CatalogService/"

// indexPageLimit is how many entries one page of the index carries.
//
// Paging is not an optimisation here — it is what keeps the payload bounded by
// a constant rather than by the size of the universe (I4). A consumer that
// asked for everything at once would work until a universe grew.
const indexPageLimit = 500

// Catalog reads a universe through a Sender.
//
// Satisfies mbapi.CatalogReader, which is the three questions reading asks —
// not the aggregator's write protocol. Holds no cache of its own: what it
// serves is what the aggregator said, and a consumer that wants to hold the
// answer holds it (see Snapshot).
type Catalog struct {
	sender ktsapi.Sender
}

// New returns a reader over `sender`.
func New(sender ktsapi.Sender) *Catalog { return &Catalog{sender: sender} }

// Index returns every contribution the universe publishes, body-free.
//
// The cursor loop lives HERE, once. Every consumer that wrote its own had the
// same three lines to get wrong — and a paging bug is invisible until a
// universe crosses a page boundary, which is the worst moment to find one.
func (c *Catalog) Index(ctx context.Context) ([]contribapi.Contribution, error) {
	return c.index(ctx, contribapi.ListFilter{})
}

// index is the cursor loop over ListContributions with a server-side filter.
//
// The aggregator restarts a cursor cut from an index that reloaded between
// two pages (Pagination.Offset == 0 on a follow-up call): what was read so
// far belongs to the old index and is dropped, so the result is one index,
// never a splice of two.
func (c *Catalog) index(ctx context.Context, filter contribapi.ListFilter) ([]contribapi.Contribution, error) {
	var out []contribapi.Contribution
	cursor := ""
	for {
		raw, err := c.sender.Call(ctx, aggregator+"ListContributions",
			mbapi.CatalogListContributionsInput{Filter: filter, Limit: indexPageLimit, Cursor: cursor})
		if err != nil {
			return nil, fmt.Errorf("catalog index (cursor=%q): %w", cursor, err)
		}
		var resp mbapi.CatalogListContributionsOutput
		if err := json.Unmarshal(raw, &resp); err != nil {
			return nil, fmt.Errorf("decode catalog index: %w", err)
		}
		if cursor != "" && resp.Pagination.Offset == 0 {
			out = out[:0] // the index changed under the cursor: start over
		}
		out = append(out, resp.Contributions...)
		if !resp.Pagination.HasMore || resp.Pagination.NextCursor == "" {
			return out, nil
		}
		cursor = resp.Pagination.NextCursor
	}
}

// GetContributionsByType implements mbapi.CatalogReader — with the type as
// the server-side filter, so a kind's listing moves that kind's entries and
// not the universe's.
func (c *Catalog) GetContributionsByType(ctx context.Context, t string) ([]contribapi.Contribution, error) {
	if t == "" {
		return c.Index(ctx)
	}
	return c.index(ctx, contribapi.ListFilter{Types: []string{t}})
}

// List implements mbapi.CatalogReader, grouping the index by each
// contribution's OWN provider.
//
// By the contribution's provider, never by the manifest that carried it: a
// layer provider's contributions live inside another binary's manifest, and
// attributing them to the carrier gives one logical entry two URIs — which
// breaks dedupe silently, in a way that shows up as a duplicate in a list.
func (c *Catalog) List(ctx context.Context) ([]*mbapi.Manifest, error) {
	index, err := c.Index(ctx)
	if err != nil {
		return nil, err
	}
	byProvider := map[string][]contribapi.Contribution{}
	var order []string
	for _, e := range index {
		if _, seen := byProvider[e.Provider]; !seen {
			order = append(order, e.Provider)
		}
		byProvider[e.Provider] = append(byProvider[e.Provider], e)
	}
	out := make([]*mbapi.Manifest, 0, len(order))
	for _, p := range order {
		out = append(out, &mbapi.Manifest{Provider: p, Contributions: byProvider[p]})
	}
	return out, nil
}

// GetBody implements mbapi.CatalogReader: one bounded fetch, by hash.
//
// By hash rather than by URI because content is addressed by what it IS — the
// aggregator serves the same bytes to every asker, and two contributions
// sharing content share the answer.
func (c *Catalog) GetBody(ctx context.Context, uri string) ([]byte, error) {
	entry, found, err := c.at(ctx, uri)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, fmt.Errorf("%w: %w", mbapi.ErrSidecarNotInCatalog, contribapi.ErrContributionNotFound{URI: uri})
	}
	// Data, and only data: the body lane answers one question. A kind that
	// keeps its content in the source layer — every data-type — has no body,
	// and says so with nil; its content is reached through Open(uri, layer).
	// This used to fall back to the source layer, which is how YAML reached a
	// JSON decoder in the GUI.
	if b := entry.Data(); len(b) > 0 {
		return b, nil
	}
	hash := entry.DataHash()
	if hash == "" {
		return nil, nil // no data layer
	}
	return c.fetchBody(ctx, uri, hash)
}

// fetchBody asks the aggregator for one body by hash — the call itself, with
// no locating step — and checks the bytes against that hash before answering:
// content is addressed, so bytes that do not hash to their address are
// refused (contribapi.ErrContentMismatch), never served.
//
// Split out so a Snapshot, which has ALREADY located the entry, can fetch
// without re-entering GetBody and paying for another index. That re-entry was
// the whole cost this package's doc promised a snapshot would remove.
func (c *Catalog) fetchBody(ctx context.Context, uri, hash string) ([]byte, error) {
	raw, err := c.sender.Call(ctx, aggregator+"GetBody",
		mbapi.GetBodyInput{Hash: hash, URI: uri})
	if err != nil {
		return nil, fmt.Errorf("catalog body %s: %w", uri, err)
	}
	var resp mbapi.GetBodyOutput
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("decode catalog body %s: %w", uri, err)
	}
	if err := contribapi.VerifyContent(uri, contribapi.LayerData, hash, resp.Content); err != nil {
		return nil, fmt.Errorf("catalog body %s: %w", uri, err)
	}
	return resp.Content, nil
}

// Read returns a contribution's RAW readable content — the read lane.
//
// The one lane that may reach the owning binary, and the only one that does.
// Kept distinct from GetBody on purpose: GetBody serves the structured body
// from the aggregator and contacts nobody, while Read asks for the bytes as
// the producer holds them. A consumer choosing between them is choosing
// whether it needs the file or the description of it.
// OpenLayer is the content lane over the wire: one named layer, band-aware.
// A refusal from the aggregator (bulk band, or a layer it neither holds nor
// can fetch) is an error naming the layer, never a silent substitute.
func (c *Catalog) OpenLayer(ctx context.Context, uri, layer string) (io.ReadCloser, contribapi.LayerRef, error) {
	raw, err := c.sender.Call(ctx, aggregator+"OpenLayer", mbapi.OpenLayerInput{URI: uri, Layer: layer})
	if err != nil {
		return nil, contribapi.LayerRef{}, fmt.Errorf("catalog open %s layer %q: %w", uri, layer, err)
	}
	var resp mbapi.OpenLayerOutput
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, contribapi.LayerRef{}, fmt.Errorf("decode catalog open %s: %w", uri, err)
	}
	// A refusal comes back typed (contribapi.ErrBulkLayer or
	// ErrLayerNotCarried), as it left the far side.
	if rerr := mbapi.RefusedErr(uri, layer, resp); rerr != nil {
		return nil, resp.Ref, fmt.Errorf("catalog open %s layer %q refused: %w", uri, layer, rerr)
	}
	// The bytes must be the ones the descriptor addresses: a layer is
	// content-addressed at every reach, and this one used to trust the wire.
	if resp.Ref.Hash != "" {
		if err := contribapi.VerifyContent(uri, layer, resp.Ref.Hash, resp.Content); err != nil {
			return nil, resp.Ref, fmt.Errorf("catalog open %s layer %q: %w", uri, layer, err)
		}
	}
	return io.NopCloser(bytes.NewReader(resp.Content)), resp.Ref, nil
}

var _ mbapi.LayerOpener = (*Catalog)(nil)

func (c *Catalog) Read(ctx context.Context, uri string) (content []byte, mediaType string, err error) {
	raw, err := c.sender.Call(ctx, aggregator+"Read", mbapi.CatalogReadInput{URI: uri})
	if err != nil {
		return nil, "", fmt.Errorf("catalog read %s: %w", uri, err)
	}
	var resp mbapi.CatalogReadOutput
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, "", fmt.Errorf("decode catalog read %s: %w", uri, err)
	}
	return resp.Content, resp.MediaType, nil
}

// at finds one entry by URI through the server-side filter — provider, type
// and id — so locating one entry moves one entry, not the universe (blind
// audit M6: this used to page the whole index per body).
func (c *Catalog) at(ctx context.Context, uri string) (contribapi.Contribution, bool, error) {
	parsed, err := contribapi.ParseResourceURI(uri)
	if err != nil {
		return contribapi.Contribution{}, false, fmt.Errorf("catalog at %q: %w", uri, err)
	}
	matches, err := c.index(ctx, contribapi.ListFilter{
		Providers: []string{parsed.Provider},
		Types:     []string{parsed.Type},
		IDGlob:    parsed.ID,
	})
	if err != nil {
		return contribapi.Contribution{}, false, err
	}
	want := parsed.String()
	for _, e := range matches {
		if e.ResolvedURI() == want {
			return e, true, nil
		}
	}
	return contribapi.Contribution{}, false, nil
}

var _ mbapi.CatalogReader = (*Catalog)(nil)
var _ mbapi.OverrideReader = (*Catalog)(nil)

// Overrides implements mbapi.OverrideReader: the hub's ListOverrides.
func (c *Catalog) Overrides(ctx context.Context, from string) ([]contribapi.Override, error) {
	raw, err := c.sender.Call(ctx, aggregator+"ListOverrides", mbapi.ListOverridesInput{From: from})
	if err != nil {
		return nil, err
	}
	var resp mbapi.ListOverridesOutput
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("decode ListOverrides: %w", err)
	}
	return resp.Overrides, nil
}

// Manifest returns one provider's descriptor.
//
// Targeted rather than filtering List, because a manifest carries a provider's
// slot types and methods — the CALLABLE lane — and a caller asking for one
// provider's methods should not pay for every provider's contributions.
func (c *Catalog) Manifest(ctx context.Context, provider string) (*mbapi.Manifest, error) {
	return c.manifest(ctx, provider, false)
}

// ManifestSlots returns one provider's descriptor WITHOUT its contributions:
// identity, hash, transports, slot types, slots and provider rows.
//
// For the consumer that already holds the paged index and needs the callable
// lane: asking for the full manifest here received every entry a second
// time, unpaged — the one reply that grew with the size of the universe
// (blind audit M5).
func (c *Catalog) ManifestSlots(ctx context.Context, provider string) (*mbapi.Manifest, error) {
	return c.manifest(ctx, provider, true)
}

func (c *Catalog) manifest(ctx context.Context, provider string, slotsOnly bool) (*mbapi.Manifest, error) {
	resp, err := c.getManifest(ctx, mbapi.GetManifestInput{Provider: provider, SlotsOnly: slotsOnly})
	if err != nil || !resp.SplitSlotTypes || resp.Manifest == nil {
		return resp.Manifest, err
	}
	// Methods too large for one reply: each slot type is asked on its own.
	m := resp.Manifest
	for i, st := range m.SlotTypes {
		one, err := c.getManifest(ctx, mbapi.GetManifestInput{Provider: provider, SlotsOnly: true, SlotType: st.Type})
		if err != nil {
			return nil, err
		}
		if one.Manifest == nil || len(one.Manifest.SlotTypes) != 1 {
			return nil, fmt.Errorf("catalog manifest %s: slot type %s not answered", provider, st.Type)
		}
		m.SlotTypes[i] = one.Manifest.SlotTypes[0]
	}
	return m, nil
}

func (c *Catalog) getManifest(ctx context.Context, in mbapi.GetManifestInput) (mbapi.GetManifestOutput, error) {
	raw, err := c.sender.Call(ctx, aggregator+"GetManifest", in)
	if err != nil {
		return mbapi.GetManifestOutput{}, fmt.Errorf("catalog manifest %s: %w", in.Provider, err)
	}
	var resp mbapi.GetManifestOutput
	if err := json.Unmarshal(raw, &resp); err != nil {
		return mbapi.GetManifestOutput{}, fmt.Errorf("decode catalog manifest %s: %w", in.Provider, err)
	}
	return resp, nil
}

// Providers returns the universe's full membership — including members whose
// manifest has not been cached yet.
//
// Distinct from ranging List: List reports what is KNOWN, this reports what is
// MEMBER. A UI that showed only the first would quietly hide a member that had
// not been probed, which looks identical to a member that does not exist.
func (c *Catalog) Providers(ctx context.Context) ([]string, error) {
	raw, err := c.sender.Call(ctx, aggregator+"ListNames", mbapi.ListNamesInput{})
	if err != nil {
		return nil, fmt.Errorf("catalog providers: %w", err)
	}
	var resp mbapi.ListNamesOutput
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("decode catalog providers: %w", err)
	}
	return resp.Names, nil
}

// Digests returns the aggregator's manifest hash per cached provider — one
// call, no entries. The consumer's reconnect check reads this and refetches
// the index only when a hash it built from has moved.
func (c *Catalog) Digests(ctx context.Context) (map[string]string, error) {
	raw, err := c.sender.Call(ctx, aggregator+"ListNames", mbapi.ListNamesInput{})
	if err != nil {
		return nil, fmt.Errorf("catalog digests: %w", err)
	}
	var resp mbapi.ListNamesOutput
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("decode catalog digests: %w", err)
	}
	return resp.Hashes, nil
}

// GetBodyPath is the aggregator's per-hash body address.
//
// Exported for the one caller that must build the request itself: a consumer
// catalog's content lane is a `func(hash, uri)` by contract, because the lane
// is keyed by hash and a reader is keyed by URI. Exporting the path keeps even
// that caller from spelling the address — which is the whole reason this
// package exists.
const GetBodyPath = aggregator + "GetBody"
