// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"

	"github.com/katyoti/katyoti-fwk/internal/core/meta"
)

// ResourceService is the canonical per-sidecar slot-type for
// readable-resource discovery + retrieval.
//
// Every sidecar publishes ONE instance (named "default") at slot subject:
//
//	ktw.<tenant>.universe.<u>.slot.<provider>.resources.default.<method>
//
// (slot type "resources" = ResourceServiceDef.Name; methods on the wire are
// the kebab of the Go names: get-manifest, list, get, read, open-layer,
// types, get-data-type.)
//
// The hub's catalog refresh path consumes this slot to populate the
// universe-wide CatalogService. Polyglot sidecars implement this
// 7-method contract natively in their language. OpenLayer is the content
// lane; Read is the compatibility raw-source path (see doc.go).
//
// The model it serves, and the lanes it is one reach of, are
// modules/contributions/ (concepts.md, tools.md, contract-matrix.md).
type ResourceService interface {
	// GetManifest returns the full Manifest (Contributions + SlotTypes +
	// Slots + Hash) for THIS binary. Used by ktw-hub's catalog refresh to
	// cache a complete content-addressed manifest in one round-trip.
	GetManifest(ctx context.Context, in ContributionGetManifestInput) (ContributionGetManifestOutput, error)

	// List returns Contributions matching the filter. Defaults to
	// metadata-only (Body=nil per entry); set ContributionListInput.WithBody=true
	// to include bodies. One method serves both views — see
	// the command-surface decision.
	List(ctx context.Context, in ContributionListInput) (ContributionListOutput, error)

	// Get fetches one contribapi.Contribution by URI (Body included).
	// This is the METADATA/manifest view: for file-backed kinds whose
	// enumerate body is a pointer (references), the body stays the
	// pointer — fetch the raw content with Read.
	Get(ctx context.Context, in ContributionGetInput) (ContributionGetOutput, error)

	// Read fetches one resource's RAW content by URI — the bytes of the
	// backing file (markdown, PDF, image, …) plus its media type. The
	// content-lane complement to Get's metadata lane: skills name
	// references by URI; consumers load them through Read.
	Read(ctx context.Context, in ContributionReadInput) (ContributionReadOutput, error)

	// OpenLayer serves ONE named layer of one contribution this binary
	// publishes, band-aware: inline content for anything under the bulk band,
	// a refusal naming the address above it. The bytes come from the
	// producer's own enumeration, so a layer the index does not carry (not
	// resident) is still reachable here — this is the producer-side half of
	// the content lane, the one Read could not give because Read has no
	// layer to name. `Layer == ""` is the kind's default.
	OpenLayer(ctx context.Context, in OpenLayerInput) (OpenLayerOutput, error)

	// Types returns the distinct Type discriminators this binary
	// contributes. Cheap — used by `llm-list-resource-types`.
	Types(ctx context.Context) ([]string, error)

	// GetDataType returns the canonical DSL Schema of a data-type the
	// sidecar OWNS by name (e.g., "notation"). Returns nil + nil if
	// the sidecar doesn't own that data-type — consumers fall back to
	// ktw-hub's universe-wide SchemaService for cross-sidecar lookups.
	GetDataType(ctx context.Context, in ContributionGetDataTypeInput) (ContributionGetDataTypeOutput, error)
}

// ResourceServiceDef is the published-service definition. Hub-side
// callers (ktw-hub catalog refresh, MCP bridge, ...) talk to a remote
// sidecar's ResourceService over KTSP via slot "resources".
//
// The CLI surface is suppressed via WithKTSPSlotTags at publication
// time because users go through `<binary> contributions` directly
// (hand-written Cobra group).
var ResourceServiceDef = meta.ServiceDef{
	Name:    "resources",
	Short:   "Sidecar contribution discovery + retrieval",
	Long:    "Query and fetch the readable-resource Contributions this binary publishes (skills, tasks, references, data-types, configs, future kinds). Used by ktw-hub's catalog refresh to populate the universe-wide CatalogService without spawning the sidecar's runtime services.",
	Version: "1.0.0",
	Methods: map[string]meta.MethodDef{
		"GetManifest": meta.MethodLong(
			"Fetch this binary's full Manifest",
			"Returns the content-addressed Manifest (Contributions + SlotTypes + Slots + Hash) for this binary. Used by ktw-hub's catalog refresh to cache the manifest in one round-trip.",
		),
		// A headers-only twin of List was declared here until 2026-06-05;
		// its removal is recorded in doc.go's compatibility section.
		"List": meta.MethodLong(
			"List Contributions (set input.WithBody=true to include each entry's full body)",
			"Returns every contribapi.Contribution matching the filter. Default returns metadata-only headers (URI + Metadata + Schema + Edges) — set `WithBody: true` to include the full body content (used by agent prompt builders reading skill instructions; ktw-hub catalog refresh caching bodies).",
		),
		"Get": meta.MethodLong(
			"Fetch one contribapi.Contribution by URI (metadata/manifest view)",
			"Returns the full contribapi.Contribution at the given URI. For file-backed kinds (references) the body is the manifest pointer — fetch raw content with Read. Returns ErrContributionNotFound if no provider serves the URI.",
		),
		"Read": meta.MethodLong(
			"Fetch one resource's RAW content by URI",
			"Returns the raw bytes of the resource's backing file (markdown, PDF, image, …) plus its media type. The content lane: skills name references by URI; consumers load them through Read. Returns ErrContributionNotFound if no provider serves the URI.",
		),
		"OpenLayer": meta.MethodLong(
			"Serve one named layer of one contribution, band-aware",
			"Returns the bytes of the named layer (the kind's default when empty) from the producer's own enumeration — including layers the index does not carry — or a refusal naming the content address for a bulk-band layer. The producer-side half of the content lane; ktw-hub forwards here for a layer it cannot serve from the aggregate.",
		),
		"Types": meta.Method(
			"Distinct Type discriminators this binary contributes.",
		),
		"GetDataType": meta.MethodLong(
			"Fetch a data-type this sidecar owns by name",
			"Returns the canonical DSL Schema body of a data-type the sidecar owns (e.g., \"notation\"). Returns empty + nil if the sidecar doesn't own that data-type — consumers fall back to ktw-hub's universe-wide SchemaService.",
		),
	},
}

// === GetManifest ===

// ContributionGetManifestInput scopes the manifest to this binary.
//
// Both toggles are pointers defaulting to TRUE (nil = unset = include):
// the manifest is the binary's COMPLETE descriptor, so by default
// nothing is stripped. This intentionally differs from
// ContributionListInput.IncludeLocal (plain bool, default FALSE) — List
// is the bus-safe query surface; manifest is the bulk full-descriptor
// path whose consumers opt out explicitly.
type ContributionGetManifestInput struct {
	// IncludeSystem toggles inclusion of the embedded `system` meta-model
	// (contribution-type + data-type schemas) in the manifest.
	//
	// The hub's catalog ingest sets FALSE for every sidecar except ONE
	// canonical carrier: every binary embeds the identical system set, so
	// aggregating all manifests un-filtered duplicates the 28 system
	// entries once per sidecar in the universe catalog.
	IncludeSystem *bool `json:"includeSystem,omitempty" cli:"include-system" desc:"Include Provider:\"system\" meta-model entries (default true)"`

	// IncludeLocal toggles inclusion of `Provider == "local"`
	// binary-private entries. Same pointer convention as IncludeSystem.
	IncludeLocal *bool `json:"includeLocal,omitempty" cli:"include-local" desc:"Include Provider:\"local\" binary-private entries (default true)"`
}

// ContributionGetManifestOutput wraps the full Manifest pointer.
type ContributionGetManifestOutput struct {
	Manifest *Manifest `json:"manifest"`
}

// === Filter type ===

// ContributionListFilter narrows List results server-side.
// All fields are AND'd; empty fields mean "no constraint".
type ContributionListFilter struct {
	// Types restricts to the listed kind discriminators (e.g. ["skill", "task"]).
	Types []string `json:"types,omitempty"`

	// IDGlob is a glob pattern matched against contribapi.Contribution.ID (e.g. "doc-*").
	IDGlob string `json:"idGlob,omitempty"`

	// URIs is an explicit URI allowlist — bypasses Types/IDGlob when set.
	URIs []string `json:"uris,omitempty"`
}

// === List ===

// ContributionListInput parameterizes the contribution list query.
// Defaults to metadata-only response; opt in to body inclusion via
// WithBody. One method serves both views, per the command-surface decision
type ContributionListInput struct {
	Filter   ContributionListFilter `json:"filter,omitempty"`
	WithBody bool                   `json:"withBody,omitempty" cli:"with-body" desc:"Include contribapi.Contribution bodies (default false — metadata only)"`

	// IncludeLocal toggles inclusion of `Provider == "local"` entries
	// in the result set. Defaults to FALSE so bus consumers (hub
	// catalog refresh, remote sidecars, LLM agents calling
	// `<provider>:resources/list`) see only the bus-visible
	// subset. The local CLI command sets this to TRUE so operators
	// running `<binary> sc resources list` see everything the
	// binary publishes — including binary-private artifacts.
	//
	// Per contribution-meta-model.md §4.3 (Phase E): local-scope
	// contributions never leave the binary; the IncludeLocal opt-in
	// is the wire-boundary discriminator.
	IncludeLocal bool `json:"includeLocal,omitempty" cli:"include-local" desc:"Include Provider:\"local\" entries (default false — bus-safe)"`

	// IncludeSystem toggles inclusion of `Provider == "system"` entries —
	// the embedded bootstrap meta-model (`contribution-type/*` kind
	// declarations + `data-type/*` body schemas every binary carries).
	// Defaults to TRUE: system contributions are part of every binary's
	// published surface, so omitting the field keeps them visible. Set
	// FALSE to hide them — e.g. a GUI/agent that only wants the binary's
	// own concrete resources, not the meta-schemas.
	//
	// Pointer so the zero value (nil) means "unset → default true";
	// a plain bool can't express a true default. Callers that want them
	// hidden set `IncludeSystem: ptr(false)`.
	IncludeSystem *bool `json:"includeSystem,omitempty" cli:"include-system" desc:"Include Provider:\"system\" meta-model entries (default true)"`
}

// ContributionListOutput returns Contributions matching the filter.
// Each entry's Body field is populated only when the input had
// WithBody=true; otherwise Body is nil. All other fields (Metadata,
// Edges, Schema) are always populated.
type ContributionListOutput struct {
	Contributions []contribapi.Contribution `json:"resources" desc:"resource entries — bodies stripped unless WithBody=true" ui-cli:"mode=table,columns=type;id;provider,primary=id,sort=type;id"`
}

// === Get ===

// ContributionGetInput targets one contribapi.Contribution by URI.
type ContributionGetInput struct {
	URI string `json:"uri" cli:"arg" desc:"Canonical Contribution URI to fetch" required:"true"`
}

// ContributionGetOutput returns the fetched Contribution.
type ContributionGetOutput struct {
	Contribution contribapi.Contribution `json:"resource"`
}

// === Read ===

// ContributionReadInput targets one resource's raw content by URI.
type ContributionReadInput struct {
	URI string `json:"uri" cli:"arg" desc:"Canonical resource URI whose raw content to read" required:"true"`
}

// ContributionReadOutput returns the raw content of the resource's
// backing file. Content is base64 on the JSON wire ([]byte marshaling);
// CLI surfaces decode and print/write it raw.
type ContributionReadOutput struct {
	URI       string `json:"uri"`
	MediaType string `json:"mediaType,omitempty" desc:"MIME-ish type of the content (text/markdown, application/pdf, …)"`
	Content   []byte `json:"content" desc:"Raw file bytes (base64 over JSON)"`
}

// === GetDataType ===

// ContributionGetDataTypeInput targets one data-type by NAME (not URI).
// The sidecar maps "notation" → its `<provider>:/data-type/notation`
// contribapi.Contribution internally.
type ContributionGetDataTypeInput struct {
	Name string `json:"name" cli:"arg" desc:"Data-type name (without provider prefix), e.g. \"notation\"" required:"true"`
}

// ContributionGetDataTypeOutput returns the canonical DSL Schema body.
// Empty Body + nil error means "this sidecar doesn't own that data-type".
type ContributionGetDataTypeOutput struct {
	Body []byte `json:"body,omitempty" desc:"DSL Schema body (YAML or canonical JSON). Empty if absent."`
}

// ErrContributionNotFound is returned by ResourceService.Get when
// the URI doesn't match any contribapi.Contribution this sidecar serves.
// The same type the provider primitive (ContributionProvider.Open) returns:
// one not-found value from the provider to the wire.
type ErrContributionNotFound = contribapi.ErrContributionNotFound
