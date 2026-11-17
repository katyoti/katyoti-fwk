// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"

	"github.com/katyoti/katyoti-fwk/internal/core/meta"
)

// CatalogService is the bus-exposed view over the in-process Catalog.
// It exists separately from `Catalog` (which is the in-process Go
// interface) because KTSP auto-publishing expects each method to take
// one Input struct and return one Output struct + error — whereas the
// in-process Catalog uses bare-string arguments (Get(ctx, name) etc.).
// The implementation wrapping a Catalog is a thin transcoder; this is
// the surface clients (ktw-gui, agent prompt builder, MCP refresh) hit
// when they want the catalog over the wire.
//
// Per the catalog architecture: every method here is a pure read of the
// in-memory snapshot — no sidecar spawn. The cache is populated either
// by the hub's startup hydration or by explicit `catalog refresh` calls.
type CatalogService interface {
	GetManifest(ctx context.Context, in GetManifestInput) (GetManifestOutput, error)
	ListNames(ctx context.Context, in ListNamesInput) (ListNamesOutput, error)

	// ListOverrides is every layer an override tree served instead of a
	// member's embedded content, across the cached manifests
	// (Manifest.Overrides), optionally from one tree only.
	ListOverrides(ctx context.Context, in ListOverridesInput) (ListOverridesOutput, error)

	// ListContributions is the INDEX lane: every contribution matching the
	// filter, across every cached manifest, in index form (a layer past the
	// inline band keeps its descriptor and loses its bytes — content is read
	// through GetBody or OpenLayer). Pages with Limit + Offset / Cursor.
	ListContributions(ctx context.Context, in CatalogListContributionsInput) (CatalogListContributionsOutput, error)

	// ListResources is the compatibility name of ListContributions (wire
	// catalog-service/list-resources, output key "resources", optional
	// IncludeBody). compat: removed after the next tagged release following
	// 2026-09-25 — see doc.go, "Compatibility names". New callers use
	// ListContributions.
	ListResources(ctx context.Context, in CatalogListResourcesInput) (CatalogListResourcesOutput, error)

	// ListURIs is the cheap projection over ListContributions — returns
	// matching URIs only, no bodies. Use for UIs / pickers that show
	// names without needing the full payload.
	ListURIs(ctx context.Context, in CatalogListURIsInput) (CatalogListURIsOutput, error)

	// GetBody is the CONTENT plane: returns the body bytes for ONE
	// contribution addressed by its content hash. Bounded by a single
	// body's size (never the whole manifest) so it stays well under the
	// NATS MaxPayload limit. Consumers hit this only on a local
	// content-store miss.
	GetBody(ctx context.Context, in GetBodyInput) (GetBodyOutput, error)

	// Read is the compatibility RAW-SOURCE path: returns the raw content a
	// consumer would render for a URI (the markdown of a skill, the DSL
	// text of a data-type, the bytes of a reference file) plus its media
	// type, by delegating to the OWNING sidecar's ResourceService.Read over
	// the bus. It is not a second content model: a new caller asks for a
	// named layer through OpenLayer. Its callers and removal policy are
	// listed in this package's documentation (doc.go, "Compatibility
	// names").
	Read(ctx context.Context, in CatalogReadInput) (CatalogReadOutput, error)

	// OpenLayer is the CONTENT lane: ONE named layer of one contribution
	// (empty = the kind's declared default), band-aware — inline and
	// control-band bytes travel in the reply, a bulk-band layer is refused
	// with its address so the caller streams it out of band. A layer the
	// aggregate does not carry is forwarded to the producer's
	// ResourceService.OpenLayer. Consumers reach it through mbapi.Open.
	OpenLayer(ctx context.Context, in OpenLayerInput) (OpenLayerOutput, error)

	// Translations is the TRANSLATION lane: every translated text of the
	// universe's contributions in one language, flat (`<uri>#<path>` → text),
	// from the translations layers the index carries (contribution-types.md,
	// Translations). The GUI loads it as its `contrib` namespace.
	Translations(ctx context.Context, in TranslationsInput) (TranslationsOutput, error)
}

// CatalogServiceDef declares the bus surface for the hub-side Catalog.
//
// BusName auto-derives to "catalog-service" (mechanical kebab of Name)
// so the wire form matches what clients construct from the PascalCase
// type name in their URIs (e.g. `ktw-hub:/CatalogService/ListContributions`).
var CatalogServiceDef = meta.ServiceDef{
	Name:  "CatalogService",
	Short: "Catalog reads over the bus — enumerate every sidecar's manifest without spawning",
	Long: `Bus-exposed read view over ktw-hub's in-memory catalog (hydrated at
hub boot from the tier-1 manifest cache under <CacheDir>/sidecars/, refreshed via ` + "`ktw-hub catalog refresh`" + `).

Every method is a pure in-memory read — no sidecar spawn, no exec, no probe.
This is the primary path for ktw-gui's skill/task lists and the agent prompt
builder's enumeration of available skills across every binary in the universe.

For runtime invocation (run a skill's script, dispatch a task) clients address
the target sidecar's slot directly; ConnMgr.EnsureSpawned handles lazy spawn
on first call.`,
	Version: "1.0.0",
	Methods: map[string]meta.MethodDef{
		"GetManifest": meta.MethodLong(
			"Fetch one provider's cached manifest",
			"Returns the cached Manifest for the named provider, or a not-found RPC error when no manifest is cached for it yet.",
		),
		"ListOverrides": meta.MethodLong(
			"Every layer an override tree served instead of a member's embedded content",
			"Returns the Overrides of every cached manifest (uri, layer, from: project:<token> or universe), filtered by From when given - what the attached project or the universe overrides of its members.",
		),
		"ListNames": meta.MethodLong(
			"Every provider declared in the Univers.yaml sidecars: block",
			"Returns the full universe membership including providers whose manifest hasn't been cached yet.",
		),
		"ListContributions": meta.MethodLong(
			"Aggregate contributions matching a filter across every provider — the index lane",
			"Returns every contribapi.Contribution matching the filter (Providers + Types + IDGlob), across every cached manifest, in index form: inline-band layers keep their bytes, every larger layer keeps its descriptor only. Supports pagination (Limit + Offset / Cursor) — large universes MUST page. Content is read on demand through GetBody (the data layer) or OpenLayer (any named layer). Behind every universe-reach Query (ktw-gui lists, the agent prompt builder).",
		),
		"ListResources": meta.MethodLong(
			"Compatibility name of ListContributions (removed after the next tagged release following 2026-09-25 — see doc.go)",
			"Same filter and paging as ListContributions; the output key is \"resources\" and IncludeBody (default true when unpaginated) keeps bodies in the reply. New callers use ListContributions.",
		),
		"ListURIs": meta.MethodLong(
			"Cheap aggregate of contribution URIs matching a filter",
			"Same filter as ListContributions but returns URIs only — no layer content. Use for UIs / pickers / cheap discovery; consumers that need content call GetBody (the data layer) or OpenLayer (any named layer).",
		),
		"GetBody": meta.MethodLong(
			"Fetch ONE contribution body by content hash",
			"Returns the body bytes for a single contribution addressed by its BodyHash. Bounded by one body's size — the CONTENT plane of the two-plane cache. Consumers hit this only on a local content-store miss; the index plane (ListContributions / GetManifest) ships body-free.",
		),
		"OpenLayer": meta.MethodLong(
			"Open ONE layer of a contribution by URI — the content lane",
			"Returns one named layer (data, source, …; empty = the kind's default) with its descriptor. Inline and control-band content is returned in the reply; a bulk-band layer is REFUSED with its hash and size so the caller streams it out of band. A layer that is resident in the aggregator's manifest is served from there without contacting the producer; the kind's default layer may reach the producer through the same route Read takes.",
		),
		"Translations": meta.MethodLong(
			"The universe's contribution texts in one language — the translation bundle",
			"Returns every translated text of every contribution whose kind declares translatable texts (object types, panels, capabilities, workflows, gates, tasks, guide recipes), in the language asked, keyed <uri>#<path> — read from the translations layers the index carries, never from a producer. A text absent from the bundle shows in the contribution's own language. The GUI loads it as its contrib i18n namespace and refetches it when the catalog changes.",
		),
		"Read": meta.MethodLong(
			"Fetch ONE contribution's RAW readable content by URI",
			"Delegates to the owning sidecar's ResourceService.Read (spawning it lazily via the ConnectionManager) and returns the raw content + media type a consumer would render (skill markdown, data-type DSL text, reference file bytes). Unlike GetBody — which returns the index's descriptor body — Read returns the actual readable content. Used by ktw-gui's Resources view, which can't address sidecars directly.",
		),
	},
}

// OpenLayerInput addresses ONE layer of ONE contribution.
type OpenLayerInput struct {
	// URI is the canonical contribution URI <provider>:/<type>/<id>.
	URI string `json:"uri" desc:"Canonical contribution URI <provider>:/<type>/<id>" jsonschema:"required: true"`
	// Layer names the layer (data, source, …). Empty resolves to the kind's
	// declared default layer.
	Layer string `json:"layer,omitempty" desc:"Layer name (data, source, …); empty = the kind's default layer"`
}

// OpenLayerOutput carries a layer's descriptor and, when it may travel on the
// control plane, its content.
type OpenLayerOutput struct {
	// Ref is the layer's descriptor: hash, size, media type. Never carries
	// inline bytes here — Content does.
	Ref contribapi.LayerRef `json:"ref"`
	// Content is the layer's bytes for the inline and control bands.
	Content []byte `json:"content,omitempty" desc:"Layer bytes (base64 over JSON); absent when refused"`
	// Refused says why no content travelled: the bulk band (stream it out of
	// band by hash), or a layer the aggregator holds no bytes for and cannot
	// fetch. Empty when Content is the answer.
	Refused string `json:"refused,omitempty"`
}

// CatalogReadInput addresses ONE contribution's readable content by URI.
type CatalogReadInput struct {
	// URI is the canonical contribution URI <provider>:/<type>/<id>.
	// The hub parses the provider, spawns it if needed, and forwards
	// the read to its ResourceService.Read.
	URI string `json:"uri" desc:"Canonical contribution URI <provider>:/<type>/<id>" jsonschema:"required: true"`
}

// CatalogReadOutput carries the raw readable content + its media type.
// Content is empty when the contribution has no readable backing (or the
// owning sidecar returned nothing).
type CatalogReadOutput struct {
	Content   []byte `json:"content,omitempty"   desc:"Raw readable content bytes (base64 over JSON)"`
	MediaType string `json:"mediaType,omitempty" desc:"MIME-ish type of the content (text/markdown, application/yaml, application/pdf, …)"`
}

// GetBodyInput addresses ONE contribution body by its content hash.
// The URI is carried alongside so the hub can locate the entry in its
// loaded manifests when the hash index isn't pre-built.
type GetBodyInput struct {
	// Hash is the content address ("sha256:...") of the wanted body —
	// the BodyHash carried on the index entry.
	Hash string `json:"hash" desc:"Content address of the body (\"sha256:...\") — the index entry's BodyHash" jsonschema:"required: true"`

	// URI is the contribution's canonical URI. Used by the hub to locate
	// the entry (and verify the hash) in its full-body manifests.
	URI string `json:"uri,omitempty" desc:"Canonical contribution URI <provider>:/<type>/<id>"`
}

// GetBodyOutput carries the resolved body bytes. Content is empty when
// the contribution exists but has no body.
type GetBodyOutput struct {
	Content []byte `json:"content,omitempty" desc:"The contribution's body bytes (empty when the contribution has no body)"`
}

// GetManifestInput targets a single provider.
type GetManifestInput struct {
	Provider string `json:"provider" desc:"Provider (binary) name — e.g., \"ktw-docs\"" jsonschema:"required: true"`

	// SlotsOnly answers the manifest WITHOUT its contributions: identity,
	// hash, transports, slot types, slots and provider rows only. A consumer
	// that already holds the index (ListContributions, paged) and needs the
	// callable lane must not receive every entry a second time, unpaged —
	// the one reply that grew with the size of the universe.
	SlotsOnly bool `json:"slotsOnly,omitempty" desc:"Omit contributions: identity, hash, transports, slot types, slots and provider rows only"`

	// SlotType narrows the slot types to the one named (its kebab form or as
	// declared): how a caller reads, one slot type per reply, a provider whose
	// methods do not fit one message (GetManifestOutput.SplitSlotTypes).
	SlotType string `json:"slotType,omitempty" desc:"Only this slot type's methods (kebab form or as declared)"`
}

// GetManifestOutput wraps the Manifest pointer so the wire shape is
// stable when the entry is missing (Manifest=nil).
type GetManifestOutput struct {
	Manifest *Manifest `json:"manifest" desc:"Cached manifest, or null when the provider has no cache entry"`

	// SplitSlotTypes says the provider's methods do not fit one reply: the
	// manifest's slot types carry their names and versions with no methods,
	// and the caller asks each one with GetManifestInput.SlotType.
	SplitSlotTypes bool `json:"splitSlotTypes,omitempty" desc:"Slot types answered without their methods: too large for one reply; ask each with slotType"`
}

// ListNamesInput is intentionally empty.
type ListNamesInput struct{}

// ListOverridesInput filters the overrides by the tree they came from.
type ListOverridesInput struct {
	From string `json:"from,omitempty" desc:"Only the overrides from this tree (project:<token> or universe); empty: all"`
}

// ListOverridesOutput is the overridden layers, by entry and layer.
type ListOverridesOutput struct {
	Overrides []contribapi.Override `json:"overrides"`
}

// OverrideReader is a catalog that can list the overrides its members' manifests
// carry (the hub's CatalogService, read over the bus by manifest/remote).
type OverrideReader interface {
	Overrides(ctx context.Context, from string) ([]contribapi.Override, error)
}

// ListNamesOutput is the full membership list, including uncached entries.
type ListNamesOutput struct {
	Names []string `json:"names" desc:"Every provider declared in the Univers.yaml sidecars: block — cached or not"`
	// Hashes is the aggregator's manifest hash per CACHED provider — the
	// cheapest statement of "what the index is right now". A consumer compares
	// its view against it on reconnect: notification is latency, the digest is
	// the truth, and a missed event must never cost more than one check.
	Hashes map[string]string `json:"hashes,omitempty" desc:"Manifest hash per cached provider; absent for a member with no cache entry"`
	// Unavailable names the members whose declared binary is not on disk.
	// Their entries are still served from the record — the index is the
	// aggregator's — but nothing of theirs can be reached for content that
	// the aggregate does not carry (Read, a non-resident OpenLayer). Stamped
	// so a consumer can say so instead of timing out on a call.
	Unavailable []string `json:"unavailable,omitempty" desc:"Members whose declared binary is absent: entries served from the record, producer unreachable"`
	// Refused names the members the catalog does not add, and why: the
	// attached project's contributions under a token that is reserved or a
	// member's name (the finding names both).
	Refused map[string]string `json:"refused,omitempty" desc:"Members not added, by name, with why: a project's contributions token that is reserved or another member's name"`
}

// CatalogListContributionsInput is the index query: the full
// contribapi.ListFilter (Providers + Types + IDGlob) for server-side
// filtering, plus the paging envelope (mirrors `crudapi.ListInput`'s shape so
// callers reuse the same Pagination envelope across substrate services).
//
// Every reply is ONE page: Limit == 0 means the aggregator's default page size
// (500), never "everything" — a reply that grew with the universe was the one
// unbounded path left on the wire. A caller reads Pagination.HasMore and
// follows NextCursor (manifestremote.Catalog.Index does, once, for everyone).
// The reply is always in index form: there is no body switch, content is read
// through GetBody or OpenLayer.
//
// The cursor is opaque and STAMPED with the index the page was cut from: when
// the catalog reloads between two pages, the next page restarts at offset 0
// (Pagination.Offset == 0 on a follow-up call says so) instead of skipping or
// repeating entries across the old and the new index.
type CatalogListContributionsInput struct {
	// Filter — full criteria (Providers, Types, IDGlob).
	Filter contribapi.ListFilter `json:"filter,omitempty"`

	// Limit caps page size. 0 = the aggregator's default page size (500).
	Limit int `json:"limit,omitempty" desc:"Max records per page (0 = the aggregator's default page size, 500)"`

	// Offset skips the first N matching entries before applying
	// Limit. Set Offset or Cursor, not both.
	Offset int `json:"offset,omitempty" desc:"Skip first N records"`

	// Cursor is the opaque cursor returned by a prior page's
	// Pagination.NextCursor. When set, takes precedence over Offset. It
	// carries the digest of the index it was cut from: a page asked after a
	// reload restarts at offset 0.
	Cursor string `json:"cursor,omitempty" desc:"Opaque cursor returned by a prior page's Pagination.NextCursor; restarts at 0 when the index changed"`
}

// CatalogListContributionsOutput returns one page of the index plus its
// Pagination envelope. The envelope's shape mirrors `crudapi.Pagination` so
// GUI / SDK callers share one paging type across substrate services.
type CatalogListContributionsOutput struct {
	Contributions []contribapi.Contribution `json:"contributions" desc:"One page of matching contributions across every cached manifest, in index form"`
	Pagination    Pagination                `json:"pagination"    desc:"Page envelope: HasMore and NextCursor say whether to ask again"`
}

// CatalogListResourcesInput is the input of the compatibility method
// ListResources: CatalogListContributionsInput plus IncludeBody.
// compat: removed with ListResources after the next tagged release following
// 2026-09-25 (doc.go, "Compatibility names").
//
// Empty / zero paging fields preserve the pre-pagination behaviour: a single
// response with every matching entry, bodies inline.
//
// (Named `CatalogListResourcesInput` rather than `ListResourcesInput`
// to avoid colliding with the existing `ListInput`/`ListOutput` pair
// used by the per-sidecar ResourceService.)
type CatalogListResourcesInput struct {
	// Filter — full criteria (Providers, Types, IDGlob).
	Filter contribapi.ListFilter `json:"filter,omitempty"`

	// Limit caps page size. 0 = unpaginated.
	Limit int `json:"limit,omitempty" desc:"Max records per page (0 = unpaginated; callers that may match thousands MUST set > 0)"`

	// Offset skips the first N matching entries before applying Limit.
	Offset int `json:"offset,omitempty" desc:"Skip first N records"`

	// Cursor is the opaque cursor returned by a prior page's
	// Pagination.NextCursor. When set, takes precedence over Offset.
	Cursor string `json:"cursor,omitempty" desc:"Opaque cursor returned by a prior page's Pagination.NextCursor"`

	// IncludeBody controls whether each returned entry keeps its layer
	// bytes. False (the default when paginated) returns the index form;
	// unpaginated calls keep bodies for backward compat — see hub impl.
	IncludeBody bool `json:"includeBody,omitempty" desc:"Carry the inline data layer in the response (default false when paginated; true when fully unpaginated for backward-compat)"`
}

// CatalogListResourcesOutput is the output of the compatibility method
// ListResources — CatalogListContributionsOutput under the wire key
// "resources". Removed with ListResources.
type CatalogListResourcesOutput struct {
	Contributions []contribapi.Contribution `json:"resources" desc:"Every matching resource across every cached manifest"`
	Pagination    Pagination                `json:"pagination"    desc:"Page envelope (zero-valued when input.Limit == 0)"`
}

// Pagination is the page-envelope returned by paginated catalog
// methods. Shape mirrors `crudapi.Pagination` — kept local (not
// imported) to avoid coupling between two peer extensions; the two
// types MUST be kept in sync.
type Pagination struct {
	Total      int    `json:"total,omitempty"      desc:"Post-filter record count (always populated for in-memory catalog)"`
	PageSize   int    `json:"pageSize"             desc:"Records in this page"`
	Offset     int    `json:"offset"               desc:"Offset of the first record in this page"`
	HasMore    bool   `json:"hasMore"              desc:"true = at least one more page is available"`
	NextCursor string `json:"nextCursor,omitempty" desc:"Opaque cursor for the next page — pass back as ListInput.Cursor"`
}

// CatalogListURIsInput targets a cheap URI-only listing across providers.
type CatalogListURIsInput struct {
	Filter contribapi.ListFilter `json:"filter,omitempty"`
}

// CatalogListURIsOutput returns the matching URIs in stable order.
type CatalogListURIsOutput struct {
	URIs []contribapi.ResourceURI `json:"uris" desc:"Every matching contribapi.ResourceURI across every cached manifest"`
}
