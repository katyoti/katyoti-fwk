// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"
	"encoding/json"
	"errors"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
)

// Catalog is the in-host aggregator over every sidecar's cached Manifest.
// Per ADR (modules/bus/decisions/OD-002-how-are-the-bus-s-subjects-streams-and-buckets.cwm.md) and §3.6 of the analysis, catalog reads
// happen at universe activation — without spawning any sidecar. The Manifest
// for each binary is cached on disk in the tier-1 manifest cache under
// `<CacheDir>/sidecars/<name>/`, keyed by the binary's stat.
//
// Catalog is read-only at the API surface for resource queries. Refresh
// paths (the exec probe `<binary> sc resources manifest`,
// appcore.RunResourcesList, writing into that cache) are owned by ktw-hub
// and exposed as separate CLI / KTSP operations.
//
// The two-phase bootstrap protocol (`modules/contributions/decisions/OD-006-in-what-order-does-the-hub-build-its-catalog-and.cwm.md`)
// adds SetSchema / Reindex / MarkReady / Ready methods. SchemaService is
// expected to be attached AFTER Phase 1's raw load and BEFORE Phase 2's
// reindex. Until MarkReady is called, the catalog's edge-extraction and
// schema-validation are skipped.
// CatalogReader is everything READING a universe needs, and nothing else.
//
// The full Catalog below also carries the aggregator's two-phase bootstrap —
// SetSchema, Reindex, MarkReady, Ready. That is the writer's protocol, and a
// consumer that only wants to read had to implement it anyway to be read from,
// which is a contract asking for what it does not need.
//
// Splitting it is what lets a LIGHT consumer participate. The query surface
// takes this, so anything that can answer three questions can be queried —
// including a remote reader that holds a Sender and eleven packages, rather
// than the fifty-five an in-process catalog costs. Every existing Catalog
// satisfies it without changing a line.
type CatalogReader interface {
	// List returns every Manifest the reader holds, in stable order.
	List(ctx context.Context) ([]*Manifest, error)

	// GetContributionsByType returns every contribution of one kind, across
	// every manifest. Entries may be body-free; GetBody materialises one.
	GetContributionsByType(ctx context.Context, t string) ([]contribapi.Contribution, error)

	// GetBody resolves a contribution's content by URI — the content plane.
	GetBody(ctx context.Context, uri string) ([]byte, error)
}

type Catalog interface {
	// Get returns the cached Manifest for a sidecar by binary name (e.g.
	// "ktw-docs"). Returns ErrSidecarNotInCatalog if the name is unknown
	// to this catalog (not listed in the Univers.yaml sidecars: block or no cached manifest
	// available).
	Get(ctx context.Context, name string) (*Manifest, error)

	// List returns every Manifest currently held by the catalog, in stable
	// order (by binary name). Sidecars listed in Univers.yaml but
	// without a cached manifest are omitted — use Names() for the full
	// membership list including uncached entries.
	List(ctx context.Context) ([]*Manifest, error)

	// Names returns the full membership of the universe per the
	// Univers.yaml sidecars: block — including sidecars whose manifest hasn't been
	// cached yet. Useful for "show me everything we know about, even if
	// we haven't probed it" UIs.
	Names(ctx context.Context) ([]string, error)

	// GetContributionsByType returns every contribapi.Contribution across every
	// cached manifest whose Type matches t. Use with DecodeContributions[T]
	// to get typed bodies cross-binary.
	//
	// On the two-plane index, the returned entries are body-free
	// (Body=nil, BodyHash set). Use GetBody to materialise a body lazily.
	GetContributionsByType(ctx context.Context, t string) ([]contribapi.Contribution, error)

	// GetBody resolves the body bytes for the contribution at the given
	// URI — the CONTENT plane. Resolution order:
	//
	//   1. The index entry's BodyHash → content-store hit (zero bus).
	//   2. On a content-store miss, ONE bounded per-hash fetch over the
	//      bus (CatalogService.GetBody), then stored locally.
	//
	// For an in-binary catalog (the hub's own CatalogImpl) GetBody reads
	// the body straight from its loaded manifest (it already holds full
	// bodies). Returns an error when the URI is unknown or the body
	// cannot be resolved. An empty-body contribution returns (nil, nil).
	GetBody(ctx context.Context, uri string) ([]byte, error)

	// === Two-phase bootstrap protocol (hub-boot-order ADR) ===

	// SetSchema attaches a SchemaService to the catalog. Called by the
	// hub's bootstrap factory between Phase 1 (raw load) and Phase 2
	// (reindex). Re-attaching after MarkReady is allowed (e.g., refresh
	// path); the catalog re-binds and the next Reindex picks up the
	// updated schema state.
	SetSchema(schema CatalogSchemaService) error

	// Reindex re-runs schema-aware processing (validation + edge
	// extraction + linting) over the affected Contributions. If
	// changedURIs is nil or empty, reindexes every contribapi.Contribution.
	// Otherwise, reindexes only the listed URIs.
	//
	// Reindex is a no-op until SetSchema has been called — Phase 1 mode.
	Reindex(ctx context.Context, changedURIs []string) error

	// MarkReady flips the catalog's ready flag and emits the initial
	// `contributions-changed` event (via the EventBus when attached).
	// Called at end of Phase 2.
	MarkReady(ctx context.Context) error

	// Ready reports whether MarkReady has been called.
	Ready() bool
}

// CatalogSchemaService is the subset of schema.SchemaService the catalog
// needs for Reindex (validation + edge extraction). Defined locally
// (rather than importing schema.SchemaService verbatim) to keep
// manifest/api free of cross-extension imports and to constrain the
// surface area the catalog couples against.
//
// Concrete schema.SchemaService satisfies this interface via method
// signatures: both `GetDataType` and `ValidateAgainst` have identical
// shapes there. The wiring layer (ktw-hub bootstrap) verifies the
// satisfaction at compile time.
type CatalogSchemaService interface {
	// GetDataTypeByURI fetches a registered data-type by URI. Returns
	// the raw AST (as json.RawMessage to keep this package decoupled
	// from schema/api's Schema type). Catalog impls type-assert at
	// use sites where needed.
	GetDataTypeByURI(ctx context.Context, uri string) (json.RawMessage, error)

	// ValidateBody validates a JSON body against a data-type URI.
	// Returns nil on success; error with the catalog's lint-friendly
	// message on failure. Catalog uses this at Phase 2 ingest +
	// per-contribapi.Contribution writes.
	ValidateBody(ctx context.Context, dataTypeURI string, body json.RawMessage) error
}

// ErrSidecarNotInCatalog is returned by Catalog.Get when the binary name
// is unknown to the catalog (either absent from the Univers.yaml sidecars: block or with
// no cached manifest available).
var ErrSidecarNotInCatalog = errors.New("manifest: sidecar not in catalog")

// RegistrationOutcome is what registering a data-type's DSL produced: it
// parsed, or it did not, and why. A data-type's structured content is its
// schema text, so this IS its verdict (concepts §11: "a schema's check is that
// it parses") — the one validation result the body check cannot produce,
// because there is no body beside the DSL.
type RegistrationOutcome struct {
	Registered bool
	Reason     string
}

// RegistrationOutcomeSetter is implemented by an aggregator that records
// data-type registration outcomes so Reindex can stamp them on the entries as
// verdicts. Optional: an aggregator without it leaves data-types unchecked,
// which is what every consumer saw until this existed.
type RegistrationOutcomeSetter interface {
	SetRegistrationOutcomes(map[string]RegistrationOutcome)
}
