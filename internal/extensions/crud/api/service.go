// SPDX-License-Identifier: Apache-2.0

// Package api defines the CRUDService contract — a unified, schema-driven
// read/write surface implementable by many backends (in-memory per-universe
// or per-session, CWM catalog/manifests, SQL via sqlx, REST adapters,
// federated overlays, …). Every "connection" is a named instance of a
// CRUD strategy.
//
// Contextual scope. user_id / tenant_id / universe_id / session_id are
// NEVER method parameters. Implementations read them from ctx through the
// standard scope helpers. This keeps the contract uniform across backends:
// only `type`, `id`, `body`, and query params vary between methods.
//
// Wire addressing. The slot wire-name is "crud-service" (kebab). Each
// configured connection becomes a named instance: callers reach
// "books-pg" via `<provider>:crud-service(books-pg)/load`. The
// substrate that makes this work is the (slot, instance) dispatch in
// `internal/kts/protocol/ktsp` and the `WithKTSPNamedInstances` publish
// option in `internal/core/publish`.
//
// Explicitly NOT on this interface:
//
//   - Tree / linked-entity navigation. CWM-specific; exposed there.
//   - Cross-model joins or link traversal. Out of scope for the framework;
//     backends that need richer surfaces publish a separate service.
//   - Generic call_function escape hatch. A connection that needs extra
//     surface area publishes a separate KTSP service alongside CRUDService.
package api

import (
	"context"

	"github.com/katyoti/katyoti-fwk/internal/core/meta"
)

// CRUDService is the unified CRUD contract. Every backend strategy
// implements it; ConnectionService publishes one named instance per
// configured connection through `WithKTSPNamedInstances`.
type CRUDService interface {
	// Load fetches a single record by (type, id). Wraps ErrNotFound
	// when no matching record exists.
	Load(ctx context.Context, in LoadInput) (LoadOutput, error)

	// Save inserts or upserts a record. Behaviour around ID assignment
	// and conflict handling is connection-dependent — see
	// CapabilitiesOutput.IDAssignment and ErrConflict. The returned
	// Record carries server-assigned fields (id, createdAt, version).
	Save(ctx context.Context, in SaveInput) (SaveOutput, error)

	// Delete removes a record by (type, id). Wraps ErrNotFound when no
	// matching record exists, ErrReadOnly when the connection rejects
	// writes.
	Delete(ctx context.Context, in DeleteInput) (DeleteOutput, error)

	// List returns a page of records of a type, with optional filters,
	// sort, projection, and pagination (offset- or cursor-based per
	// CapabilitiesOutput.Pagination). The Pagination envelope carries
	// total count when the backend reports it.
	List(ctx context.Context, in ListInput) (ListOutput, error)

	// Watch declares interest in change events for a (type, optional
	// filter or id). The returned WatchOutput describes the
	// MessageBus subject events flow on; callers attach a bus
	// subscription to consume them. Connections that don't support
	// change notifications wrap ErrUnsupported.
	Watch(ctx context.Context, in WatchInput) (WatchOutput, error)

	// Capabilities reports what this connection supports — filter ops,
	// pagination modes, watch semantics, writability, etc. Callers
	// (GUI, LLM agent, federation layers) use this to render
	// appropriate controls and skip operations that wouldn't work.
	Capabilities(ctx context.Context) (CapabilitiesOutput, error)
}

// CRUDServiceDef drives AutoKTSPContainer's slot publication + CLI
// generation. The kebab name `crud-service` becomes the slottype
// token in NATS subjects (`ktw.<t>.universe.<u>.slot.<p>.crud-service.<instance>.<method>`)
// and in URIs (`<provider>:crud-service(<instance>)/<method>`).
var CRUDServiceDef = meta.ServiceDef{
	Name:  "crud-service",
	Short: "Generic CRUD over named connections (memory / session / CWM / SQL / REST)",
	Long: "Single contract for record load / save / delete / list / watch across heterogeneous backends. " +
		"Each configured connection is a named instance addressable as " +
		"`<provider>:crud-service(<connection>)/<method>`. Scope (user / tenant / universe / session) " +
		"is read from ctx; only type / id / body / query params travel in method I/O.",
	Version: "0.1.0",
	Methods: map[string]meta.MethodDef{
		"Load":         {Short: "Load a single record by (type, id)"},
		"Save":         {Short: "Create or upsert a record"},
		"Delete":       {Short: "Delete a record by (type, id)"},
		"List":         {Short: "Page records of a type with filters / order"},
		"Watch":        {Short: "Subscribe to change events on a (type, optional filter or id)"},
		"Capabilities": {Short: "Describe what this connection supports"},
	},
}
