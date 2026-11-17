// SPDX-License-Identifier: Apache-2.0

package messagebus

import "context"

// Scope-context wiring for dispatched handlers.
//
// When a request arrives through `Subscribe`, the dispatch path reads
// the `meta.tenantId` / `meta.universeId` headers (set by every
// `Sender.Send` / `Sender.Notify`) and injects them into the handler's
// `context.Context`. Handlers then read them via `TenantFromContext` /
// `UniverseFromContext` — no need to parse subject strings or walk the
// header map themselves.
//
// Why context (rather than passing scope as a separate parameter):
//   - `ktsp.Endpoint.Handle(ctx, slot, method, params)` is a fixed
//     signature owned by the KTSP package; we can't change it.
//   - The Go convention for "originator metadata that crosses a
//     dispatch boundary" is context-bound. Tracing libraries, auth
//     gates, and audit loggers all expect to find this kind of data
//     on ctx.
//
// Scope keys live in this package because they're tightly coupled to
// the wire-header format (HeaderTenantID / HeaderUniverseID in
// sender.go). Lifting them to `internal/core/application` would
// invert the dependency.

// ctxKey is a private alias to avoid collisions with other packages
// putting strings on ctx.
type ctxKey int

const (
	tenantKey ctxKey = iota
	universeKey
)

// WithTenant returns a context carrying the tenantID. Used by the
// dispatch path; rarely useful directly to application code (handlers
// receive ctx already populated).
func WithTenant(ctx context.Context, tenantID string) context.Context {
	if tenantID == "" {
		return ctx
	}
	return context.WithValue(ctx, tenantKey, tenantID)
}

// WithUniverse returns a context carrying the universeID.
func WithUniverse(ctx context.Context, universeID string) context.Context {
	if universeID == "" {
		return ctx
	}
	return context.WithValue(ctx, universeKey, universeID)
}

// WithScope is a convenience wrapper that applies both tenant + universe
// to a context.
func WithScope(ctx context.Context, tenantID, universeID string) context.Context {
	return WithUniverse(WithTenant(ctx, tenantID), universeID)
}

// TenantFromContext reads the tenant ID set by an upstream Send/Notify.
// Returns an empty string when no scope is bound to ctx — handlers
// invoked locally (not over the bus) see "" and should fall back to
// their own configured tenant.
func TenantFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(tenantKey).(string); ok {
		return v
	}
	return ""
}

// UniverseFromContext reads the universe ID set by an upstream
// Send/Notify. Same fallback semantics as TenantFromContext.
func UniverseFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(universeKey).(string); ok {
		return v
	}
	return ""
}
