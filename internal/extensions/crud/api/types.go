// SPDX-License-Identifier: Apache-2.0

package api

import (
	"encoding/json"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/render"
)

// Record is the canonical record envelope flowing through every CRUD
// method. Body is whatever the connection's schema returns, encoded as
// raw JSON so the framework stays type-agnostic — concrete schemas
// live in the schema extension and are consulted by the connection
// implementations, not by the framework.
type Record struct {
	Type      string          `json:"type"      desc:"Record type name"`
	ID        string          `json:"id"        desc:"Record identifier within (type, connection)"`
	Body      json.RawMessage `json:"body"      desc:"Record body (JSON object matching the type's schema)"`
	CreatedAt time.Time       `json:"createdAt,omitempty" desc:"Server-assigned creation timestamp"`
	UpdatedAt time.Time       `json:"updatedAt,omitempty" desc:"Server-assigned last-mutation timestamp"`
	Version   int64           `json:"version,omitempty"   desc:"Optimistic-concurrency version (0 = not supported)"`
}

// FilterValue mirrors the AbstractRepository (op, value) filter shape
// the Python reference impl exposes. Multi-valued ops (IN, NIN,
// BETWEEN) pack their values into Value as a JSON array. Single-valued
// ops use a scalar.
type FilterValue struct {
	Op    string          `json:"op"    desc:"eq | neq | gt | gte | lt | lte | in | nin | contains | startswith | endswith | between"`
	Value json.RawMessage `json:"value" desc:"Filter operand (JSON-encoded scalar or array depending on Op)"`
}

// OrderBy is a single sort key. ListInput.OrderBy carries them in
// declaration order so backends can apply lexicographic ordering with
// stable tie-breaks.
type OrderBy struct {
	Field string `json:"field" desc:"Field name"`
	Desc  bool   `json:"desc,omitempty" desc:"true = descending"`
}

// Pagination is the envelope returned alongside a page of records - the one
// page shape display's paged nodes read too, so it lives in
// render, never duplicated here. Capabilities.Pagination.ReportsTotal
// indicates whether Total is the post-filter population (true) or unreliable
// / zero (false). Cursor is opaque to the caller and round-trips through
// ListInput.Cursor for the next page when the backend's mode includes
// "cursor".
type Pagination = render.Pagination

// ----------------------------------------------------------------------
// Method I/O — Load / Save / Delete
// ----------------------------------------------------------------------

// LoadInput addresses a single record by URI. Disambiguation by
// method semantics: Load expects a RECORD URI (per cwm-v3 §2.4).
// For flat-shape backends like the in-memory store: `<type>/<id>`.
// For nested-scope backends: `<parent_type>/<parent_id>/.../<type>/<id>`.
type LoadInput struct {
	URI URI `json:"uri" cli:"uri" desc:"Record URI (relative form, e.g. \"book/123\" or \"node/X/item/Y\")" required:"true"`
}

type LoadOutput struct {
	Record Record `json:"record" desc:"The loaded record"`
}

// SaveInput addresses the target of an upsert. The URI form
// distinguishes create-with-server-id from upsert-by-known-id:
//
//	URI=<type>           → create new record; backend assigns ID
//	URI=<type>/<id>      → upsert at the specific ID
//
// Backends with custom scoping accept any path depth — the trailing
// segment is the record ID (or absent for server-assigned).
type SaveInput struct {
	URI  URI             `json:"uri"  cli:"uri" desc:"Target URI. Trailing ID segment optional; absent = server-assigned (see CapabilitiesOutput.IDAssignment)" required:"true"`
	Body json.RawMessage `json:"body"           desc:"Record body (JSON object matching the type's schema)"`
}

type SaveOutput struct {
	Record Record `json:"record" desc:"The saved record (with server-assigned id / version / timestamps populated)"`
}

// DeleteInput addresses a single record by URI (same shape as
// LoadInput).
type DeleteInput struct {
	URI URI `json:"uri" cli:"uri" desc:"Record URI" required:"true"`
}

type DeleteOutput struct {
	OK bool `json:"ok" desc:"true = record removed"`
}

// ----------------------------------------------------------------------
// Method I/O — List
// ----------------------------------------------------------------------

// ListInput addresses a COLLECTION URI (per cwm-v3 §2.4 method-based
// disambiguation). Examples: `book`, `node/X/item`, `node/X/item?type=todo`.
type ListInput struct {
	URI      URI                    `json:"uri"                cli:"uri"    desc:"Collection URI (relative form)" required:"true"`
	Filters  map[string]FilterValue `json:"filters,omitempty"               desc:"field-name → (op, value); AND-combined (see CapabilitiesOutput.Filters.Combine for OR support)"`
	OrderBy  []OrderBy              `json:"orderBy,omitempty"               desc:"Multi-key sort applied in declaration order"`
	Limit    int                    `json:"limit,omitempty"    cli:"limit"  desc:"Max records per page (0 = backend default)"`
	Offset   int                    `json:"offset,omitempty"   cli:"offset" desc:"Skip first N records (cursor-only backends ignore this)"`
	Cursor   string                 `json:"cursor,omitempty"   cli:"cursor" desc:"Opaque cursor returned by a prior page's Pagination.NextCursor"`
	Distinct bool                   `json:"distinct,omitempty"              desc:"Apply DISTINCT-equivalent dedup"`
	Fields   []string               `json:"fields,omitempty"                desc:"Projection: limit Record.Body to these fields (when supported by the backend)"`
}

type ListOutput struct {
	Records    []Record   `json:"records"    desc:"Page of records"`
	Pagination Pagination `json:"pagination" desc:"Page envelope"`
}

// ----------------------------------------------------------------------
// Capabilities
// ----------------------------------------------------------------------

// CapabilitiesOutput describes what a connection supports. GUIs and
// LLM tools consult it to render appropriate controls and to skip
// operations that would error.
type CapabilitiesOutput struct {
	Types        []string      `json:"types"        desc:"Record types served by this connection (empty = open / introspectable; see backend docs)"`
	URIPatterns  []string      `json:"uriPatterns"  desc:"URI shapes this connection accepts (e.g. \"book/{id}\", \"node/{node_id}/item/{id}\", \"node/{node_id}/item?type={kind}\"). cwm-v3 §6 — makes the connection self-describing without bespoke docs."`
	Filters      FiltersCap    `json:"filters"      desc:"Filter operator + combination support"`
	Pagination   PaginationCap `json:"pagination"   desc:"Pagination model"`
	Watch        WatchCap      `json:"watch"        desc:"Change-notification support"`
	Distinct     bool          `json:"distinct"     desc:"Honors ListInput.Distinct"`
	Projection   bool          `json:"projection"   desc:"Honors ListInput.Fields for selective output"`
	Writable     bool          `json:"writable"     desc:"Supports Save / Delete (false = read-only)"`
	IDAssignment string        `json:"idAssignment" desc:"client | server | hybrid (where IDs originate on Save with empty trailing-ID segment in URI)"`
}

// FiltersCap describes the filter-op vocabulary the backend honors.
// Combine controls whether multiple filter clauses can be OR-joined;
// the framework default is "and".
type FiltersCap struct {
	Ops      []string `json:"ops"                desc:"Supported subset of: eq, neq, gt, gte, lt, lte, in, nin, contains, startswith, endswith, between"`
	Combine  string   `json:"combine"            desc:"and (AND-only; framework default) | or | both"`
	MaxCount int      `json:"maxCount,omitempty" desc:"Max simultaneous filters per call (0 = unlimited)"`
}

// PaginationCap describes the pagination model. Modes can be any
// combination of "offset", "cursor", or "hybrid" (offset on the first
// page, cursor thereafter).
type PaginationCap struct {
	Modes        []string `json:"modes"                  desc:"Subset of: offset, cursor, hybrid"`
	MaxPageSize  int      `json:"maxPageSize,omitempty"  desc:"Upper bound on ListInput.Limit (0 = unlimited)"`
	DefaultLimit int      `json:"defaultLimit,omitempty" desc:"Page size when ListInput.Limit is 0"`
	ReportsTotal bool     `json:"reportsTotal"           desc:"true = Pagination.Total is the post-filter record count"`
}

// WatchCap describes change-notification support. Granularities lists
// the watch shapes the backend supports — "type" = subscribe to a
// type, "id" = subscribe to a single record, "filter" = subscribe
// with a server-side prefilter.
type WatchCap struct {
	Supported         bool     `json:"supported"                  desc:"false = Watch always returns ErrUnsupported"`
	Granularities     []string `json:"granularities,omitempty"    desc:"Subset of: type, id, filter"`
	ServerSideFilters bool     `json:"serverSideFilters"          desc:"true = WatchInput.Filters honored server-side; false = subscriber must filter locally"`
}
