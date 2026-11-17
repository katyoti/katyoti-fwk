// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"unicode/utf8"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	"github.com/katyoti/katyoti-fwk/internal/core/render"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	manifestremote "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/remote"
)

// ContributionSummary is the GUI-facing record for one Contribution
// — any kind: skill, task, data-type, service, sidecar-info,
// config-file, env, contribution-type, …
//
// Stays slim on purpose: the debug Contributions tab lists hundreds
// of rows across a typical universe (one connector sidecar alone can publish 73
// + 5 system = 78). Body is omitted; consumers fetch it on click via
// a separate round-trip if they need it. Schema + Metadata kept
// because they're the only fields that give a row independent context
// without opening the detail pane.
type ContributionSummary struct {
	Provider string `json:"provider"`
	Type     string `json:"type"`
	ID       string `json:"id"`
	URI      string `json:"uri"`
	Schema   string `json:"schema,omitempty"`
	// Name + Description come from the Contribution's ContributionMeta
	// (the picker-level descriptor). Flattened here so the GUI can
	// render rows without unpacking a nested object — the Contribution
	// browser doesn't need the full Icon/Hashtags/Category set in the
	// list view (the detail pane can fetch them on click).
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
}

// ListContributionsResult is the Wails return for the Contributions
// tab's initial fetch. Error is inline so the UI can show a banner
// instead of throwing.
type ListContributionsResult struct {
	Contributions []ContributionSummary `json:"contributions"`
	Error         string                `json:"error,omitempty"`
}

// ListContributions returns every Contribution registered in the
// universe's catalog across every sidecar — all types, all scopes
// EXCEPT `Provider == "local"` (binary-private entries never leave
// their owning binary; the catalog wouldn't have them anyway).
//
// Drains the universe's index through the query surface with
// an empty filter (every type, every provider), passing
// nothing else (the listing is always the index form), so each entry's payload stays on the
// hub — the list view only renders metadata (Name / Description /
// URI / Schema). When a row is opened, the GUI re-fetches that one
// Contribution with the body inline.
//
// This replaces the previous hardcoded `knownTypes` enumeration which
// failed open whenever a sidecar registered a new type: discovery is
// now substrate-driven, not GUI-curated.
//
// Wails-bound: window.go.main.App.ListContributions(universeName).
const contribListPageSize = 500

func (a *App) ListContributions(universeName string) ListContributionsResult {
	cat, err := a.universeCatalog(universeName)
	if err != nil {
		return ListContributionsResult{Error: fmt.Sprintf("Cannot reach universe %q: %v", universeName, err)}
	}

	// One query at universe reach. The paging, the decode and the body-free
	// index are the surface's business, not this browser's — it used to run
	// its own cursor loop, and a paging bug is invisible until a universe
	// crosses a page boundary.
	entries, err := mbapi.Query(a.ctx, mbapi.Q{Reach: mbapi.ReachUniverse, Catalog: cat})
	if err != nil {
		return ListContributionsResult{Error: fmt.Sprintf("list contributions: %v", err)}
	}

	// Every kind, once. A schema is the kind's statement, so the column comes
	// from a lookup per KIND rather than a field repeated on every entry —
	// kinds are few and entries are many.
	kinds, err := mbapi.KindSchemas(a.ctx, mbapi.Q{Reach: mbapi.ReachUniverse, Catalog: cat})
	if err != nil {
		kinds = nil // the schema column is worth less than the listing
	}

	out := make([]ContributionSummary, 0, len(entries))
	for _, e := range entries {
		meta := e.ResolvedMetadata()
		out = append(out, ContributionSummary{
			Provider:    e.Provider,
			Type:        e.Type,
			ID:          e.ID,
			URI:         contribResolvedURI(e),
			Schema:      mbapi.SchemaFor(kinds, e.Type),
			Name:        meta.Name,
			Description: meta.Description,
		})
	}
	return ListContributionsResult{Contributions: sortContributions(out)}
}

// sortContributions returns the slice with a stable (provider, type,
// id) ordering. Catalog enumeration order is map-based and
// non-deterministic; sorting in one place keeps the GUI from
// shuffling between fetches and from differing across the
// success / partial-error code paths.
func sortContributions(in []ContributionSummary) []ContributionSummary {
	sort.Slice(in, func(i, j int) bool {
		if in[i].Provider != in[j].Provider {
			return in[i].Provider < in[j].Provider
		}
		if in[i].Type != in[j].Type {
			return in[i].Type < in[j].Type
		}
		return in[i].ID < in[j].ID
	})
	return in
}

// contribResolvedURI returns the Contribution's URI, falling back to
// the conventional shape when the catalog response carried an empty
// URI field.
func contribResolvedURI(c contribapi.Contribution) string {
	if c.URI != "" {
		return c.URI
	}
	return c.ResolvedURI()
}

// KindView is what the kind DECLARATION says about an entry's kind — the
// contract the entry is held to, shown beside the entry so "why does this row
// look like this?" has an answer one click away.
type KindView struct {
	URI          string   `json:"uri,omitempty"`
	Name         string   `json:"name"`
	Declared     bool     `json:"declared"`
	Schema       string   `json:"schema,omitempty"`
	Validate     string   `json:"validate,omitempty"`
	Execution    string   `json:"execution,omitempty"`
	DefaultLayer string   `json:"defaultLayer,omitempty"`
	Budget       int      `json:"budget,omitempty"`
	Findings     []string `json:"findings,omitempty"`
}

// LayerView is one row of the Layers table: what the kind declares for the
// layer and what the entry actually carries, side by side. A declared layer
// the entry does not carry, or a carried layer the kind never declared, is
// visible as such — that is the whole point of showing both.
type LayerView struct {
	Name      string `json:"name"`
	Declared  bool   `json:"declared"`
	Carried   bool   `json:"carried"`
	Required  bool   `json:"required,omitempty"`
	Default   bool   `json:"default,omitempty"`
	Hash      string `json:"hash,omitempty"`
	Size      int64  `json:"size,omitempty"`
	Band      string `json:"band,omitempty"`
	MediaType string `json:"mediaType,omitempty"`
	Resident  bool   `json:"resident,omitempty"`
}

// ProviderView is the producer's identity as the manifest recorded it —
// whether its source can change, and whether it said so at all.
type ProviderView struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Known    bool   `json:"known"`
	Declared bool   `json:"declared"`
	Mutable  bool   `json:"mutable"`
	Digest   string `json:"digest,omitempty"`
	Drops    int    `json:"drops"`
}

// LayerContent is one layer's content, opened through the CONTENT lane
// (CatalogService/OpenLayer). A refused layer (bulk band, or one the
// aggregator neither holds nor may fetch) carries its descriptor and the
// reason, never a substitute body.
type LayerContent struct {
	Name      string `json:"name"`
	Hash      string `json:"hash,omitempty"`
	Size      int64  `json:"size"`
	Band      string `json:"band,omitempty"`
	MediaType string `json:"mediaType,omitempty"`
	Refused   string `json:"refused,omitempty"`
	IsBinary  bool   `json:"isBinary"`
	Content   string `json:"content,omitempty"`
	Error     string `json:"error,omitempty"`
}

// ContributionDetail is the GUI-facing FULL record for ONE Contribution,
// fetched on row-click in the debug Resources view: the complete envelope
// (identity, metadata, verdict), the KIND that governs it, every LAYER it
// declares or carries, the PROVIDER that produced it, and the default layer's
// content already opened — so the pane renders the whole model without a
// second round-trip, and opens any other layer on demand.
type ContributionDetail struct {
	// Identity.
	Provider string `json:"provider"`
	Type     string `json:"type"`
	ID       string `json:"id"`
	URI      string `json:"uri"`
	Schema   string `json:"schema,omitempty"`

	// Full metadata — the complete ContributionMeta.
	Name        string            `json:"name,omitempty"`
	Description string            `json:"description,omitempty"`
	Category    string            `json:"category,omitempty"`
	IconTabler  string            `json:"iconTabler,omitempty"`
	IconEmoji   string            `json:"iconEmoji,omitempty"`
	Hashtags    []string          `json:"hashtags,omitempty"`
	Advertised  bool              `json:"advertised"`
	Annotations map[string]string `json:"annotations,omitempty"`

	// Validation verdict as recorded at ingest: "valid" / "invalid" / "unchecked".
	Validation       string   `json:"validation"`
	ValidationErrors []string `json:"validationErrors,omitempty"`

	// The model around the entry.
	Kind         KindView     `json:"kind"`
	Layers       []LayerView  `json:"layers"`
	ProviderInfo ProviderView `json:"providerInfo"`

	// The DEFAULT layer's content, opened through the content lane so the pane
	// shows something at once. BodySource names the layer ("layer:source").
	HasBody    bool   `json:"hasBody"`
	IsBinary   bool   `json:"isBinary"`
	BodySize   int    `json:"bodySize"`
	MediaType  string `json:"mediaType,omitempty"`
	BodySource string `json:"bodySource,omitempty"`
	Body       string `json:"body,omitempty"`
	Refused    string `json:"refused,omitempty"`

	Error string `json:"error,omitempty"`
}

// ReadContribution fetches the FULL detail for one Contribution by URI: the
// envelope (index lane, `mbapi.Get`), its kind's declaration (`KindsAt`), its
// layers (declared ∪ carried), its provider's identity (the manifest), and the
// default layer's content (content lane, `OpenLayer`). Nothing is decoded and
// no lane answers another lane's question.
//
// Wails-bound: window.go.main.App.ReadContribution(universeName, uri).
func (a *App) ReadContribution(universeName, uri string) ContributionDetail {
	d := ContributionDetail{URI: uri, Validation: "unchecked", Layers: []LayerView{}}
	cat, err := a.universeCatalog(universeName)
	if err != nil {
		d.Error = fmt.Sprintf("Cannot reach universe %q: %v", universeName, err)
		return d
	}
	parsed, perr := contribapi.ParseResourceURI(uri)
	if perr != nil {
		d.Error = fmt.Sprintf("Bad URI %q: %v", uri, perr)
		return d
	}
	d.Provider, d.Type, d.ID = parsed.Provider, parsed.Type, parsed.ID

	// 1 — the envelope, addressed. Nothing decoded.
	q := mbapi.Q{Reach: mbapi.ReachUniverse, Catalog: cat}
	c, gerr := mbapi.Get(a.ctx, q, uri)
	if gerr != nil {
		d.Error = fmt.Sprintf("Contribution not found in catalog (try `ktw-hub catalog refresh`): %v", gerr)
		return d
	}
	meta := c.ResolvedMetadata()
	d.Name, d.Description, d.Category = meta.Name, meta.Description, meta.Category
	if meta.Icon != nil {
		d.IconTabler, d.IconEmoji = meta.Icon.Tabler, meta.Icon.Emoji
	}
	d.Hashtags = meta.Hashtags
	d.Advertised = meta.IsAdvertised()
	d.Annotations = meta.Annotations
	// The verdict is READ, never re-derived.
	switch {
	case c.Valid == nil:
		d.Validation = "unchecked"
	case *c.Valid:
		d.Validation = "valid"
	default:
		d.Validation = "invalid"
	}
	d.ValidationErrors = c.ValidationErrors

	// 2 — the kind that governs it, from the registry of this view.
	kinds, kerr := mbapi.KindsAt(a.ctx, q)
	if kerr != nil {
		d.Error = fmt.Sprintf("kind registry: %v", kerr)
	}
	d.Kind = kindViewFor(kinds, parsed.Type)
	d.Schema = d.Kind.Schema

	// 3 — the layers: declared by the kind ∪ carried by the entry.
	decl, _ := kinds.Get(parsed.Type)
	d.Layers = layerViews(c, decl)

	// 4 — the provider's identity, from its manifest.
	d.ProviderInfo = providerViewFor(a.ctx, cat, parsed.Provider, parsed.Type)

	// 5 — the default layer, through the content lane.
	def := kinds.DefaultLayer(parsed.Type)
	if _, carried := c.Layers[def]; carried {
		lc := openLayerContent(a.ctx, cat, uri, def)
		d.BodySource = "layer:" + def
		d.MediaType = lc.MediaType
		d.Refused = lc.Refused
		if lc.Error != "" && d.Error == "" {
			d.Error = lc.Error
		}
		d.HasBody = lc.Content != "" || lc.IsBinary
		d.IsBinary = lc.IsBinary
		d.BodySize = int(lc.Size)
		d.Body = lc.Content
	} else if meta.MediaType != "" {
		d.MediaType = meta.MediaType
	}
	return d
}

// OpenContributionLayer opens ONE named layer of ONE contribution through the
// content lane — the pane's per-tab fetch. `layer` empty = the kind's default.
//
// Wails-bound: window.go.main.App.OpenContributionLayer(universeName, uri, layer).
func (a *App) OpenContributionLayer(universeName, uri, layer string) LayerContent {
	cat, err := a.universeCatalog(universeName)
	if err != nil {
		return LayerContent{Name: layer, Error: fmt.Sprintf("Cannot reach universe %q: %v", universeName, err)}
	}
	return openLayerContent(a.ctx, cat, uri, layer)
}

// openLayerContent is the one place the pane turns a layer into something it
// can show: bytes for the inline and control bands (text or "binary, N bytes"),
// the refusal and descriptor for the bulk band.
func openLayerContent(ctx context.Context, cat *manifestremote.Catalog, uri, layer string) LayerContent {
	rc, ref, err := cat.OpenLayer(ctx, uri, layer)
	out := LayerContent{Name: layer, Hash: ref.Hash, Size: ref.Size, MediaType: ref.MediaType}
	if ref.Size > 0 || ref.Hash != "" {
		out.Band = string(ref.Band())
	}
	if err != nil {
		if strings.Contains(err.Error(), "refused") {
			out.Refused = err.Error()
		} else {
			out.Error = err.Error()
		}
		return out
	}
	defer func() { _ = rc.Close() }()
	b, rerr := io.ReadAll(rc)
	if rerr != nil {
		out.Error = fmt.Sprintf("read layer %q: %v", layer, rerr)
		return out
	}
	if out.Size == 0 {
		out.Size = int64(len(b))
	}
	if isProbablyBinary(b) {
		out.IsBinary = true
		return out
	}
	out.Content = prettyIfJSON(b)
	if out.MediaType == "" {
		out.MediaType = render.Sniff(b)
	}
	return out
}

// kindViewFor renders the registry's answer about one kind.
func kindViewFor(kinds contribapi.Kinds, kind string) KindView {
	v := KindView{Name: kind, Schema: kinds.SchemaFor(kind), DefaultLayer: kinds.DefaultLayer(kind)}
	decl, ok := kinds.Get(kind)
	if !ok {
		v.Execution = string(kinds.Execution(kind))
		if !contribapi.IsMetaKind(kind) {
			v.Findings = []string{"no declaration in this universe"}
		}
		return v
	}
	v.Declared = true
	v.URI = kinds.DeclaredBy(kind)
	v.Validate = decl.Severity()
	v.Execution = string(decl.ExecutionClass())
	v.Budget = decl.Budget()
	for _, f := range kinds.Check() {
		if f.Kind == kind {
			v.Findings = append(v.Findings, f.Message)
		}
	}
	return v
}

// layerViews joins what the kind declares with what the entry carries.
func layerViews(c contribapi.Contribution, decl contribapi.KindDeclaration) []LayerView {
	names := map[string]bool{}
	for n := range decl.Layers {
		names[n] = true
	}
	for n := range c.Layers {
		names[n] = true
	}
	out := make([]LayerView, 0, len(names))
	for n := range names {
		v := LayerView{Name: n}
		if ld, ok := decl.Layers[n]; ok {
			v.Declared, v.Required, v.Default, v.Resident = true, ld.Required, ld.Default, ld.Resident
			v.MediaType = ld.MediaType
		}
		if ref, ok := c.Layers[n]; ok {
			v.Carried = true
			v.Hash, v.Size, v.Band = ref.Hash, ref.Size, string(ref.Band())
			if ref.MediaType != "" {
				v.MediaType = ref.MediaType
			}
		}
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool {
		// data first, then source, then the rest by name — the order a reader expects.
		rank := func(n string) int {
			switch n {
			case contribapi.LayerData:
				return 0
			case contribapi.LayerSource:
				return 1
			}
			return 2
		}
		if rank(out[i].Name) != rank(out[j].Name) {
			return rank(out[i].Name) < rank(out[j].Name)
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// providerViewFor reads the producer's recorded identity from its manifest.
func providerViewFor(ctx context.Context, cat *manifestremote.Catalog, provider, typ string) ProviderView {
	v := ProviderView{Name: provider, Type: typ}
	if provider == contribapi.SystemProvider {
		// The meta-model every binary embeds: no manifest of its own, and
		// immutable by construction — changing it means rebuilding every binary.
		v.Known, v.Declared, v.Mutable = true, true, false
		return v
	}
	m, err := cat.Manifest(ctx, provider)
	if err != nil || m == nil {
		return v
	}
	v.Known = true
	v.Drops = len(m.Drops)
	for _, p := range m.Providers {
		if p.Name == provider && p.Type == typ {
			// A recorded row IS the declaration: Identity is on the provider
			// contract, so every provider the builder walked has one.
			v.Declared, v.Mutable, v.Digest = true, p.Mutable, p.Digest
			return v
		}
	}
	return v
}

// isProbablyBinary reports whether content should be treated as binary
// (not safely renderable as text): invalid UTF-8, or contains a NUL byte.
func isProbablyBinary(b []byte) bool {
	if !utf8.Valid(b) {
		return true
	}
	for _, c := range b {
		if c == 0 {
			return true
		}
	}
	return false
}

// prettyIfJSON pretty-prints content when it parses as JSON; otherwise
// returns it verbatim as a string.
func prettyIfJSON(b []byte) string {
	trimmed := strings.TrimSpace(string(b))
	if len(trimmed) > 0 && (trimmed[0] == '{' || trimmed[0] == '[') {
		var v any
		if json.Unmarshal(b, &v) == nil {
			if pretty, mErr := json.MarshalIndent(v, "", "  "); mErr == nil {
				return string(pretty)
			}
		}
	}
	return string(b)
}
