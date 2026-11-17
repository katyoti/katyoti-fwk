// SPDX-License-Identifier: Apache-2.0

// Package api defines the EventService contract — a thin facade over
// the NATS-backed `MessageBus` (`internal/extensions/manifest/api`)
// that speaks the new `EventURI` grammar instead of raw subject
// strings.
//
// EventService is the third leg of the URI dispatcher trio alongside
// ActionService (runnables) and the catalog ResourceService (readable
// contributions). It is intentionally minimal in v1 — Subscribe +
// Publish only; Enumerate / List / Replay are v2 follow-ups.
//
// Why a facade over MessageBus instead of consuming MessageBus
// directly: callers think in scoped, semantically-named URIs
// (`cwm:!workspace/object/abc/created`) — NOT in NATS subjects
// (`ktw.<tenant>.universe.<u>.workspace.<ws>.node.abc.created`).
// EventService owns the URI ↔ subject translation so the substrate's
// scope vocabulary stays in one place.
package api

import (
	"context"
	"encoding/json"

	"github.com/katyoti/katyoti-fwk/internal/core/meta"
)

// EventServiceDef describes the EventService for KTSP publishing.
var EventServiceDef = meta.ServiceDef{
	Name:    "event",
	Short:   "Cross-sidecar pub/sub by EventURI (thin facade over NATS MessageBus)",
	Version: "1.0.0",
	Methods: map[string]meta.MethodDef{
		"Publish": meta.Method("Publish an event payload to the NATS subject the EventURI maps to"),
		// Subscribe is intentionally NOT in the KTSP method set —
		// long-lived handler-bound subscriptions don't fit KTSP's
		// request/reply shape. In-process Go callers use the
		// interface method directly; cross-binary subscribers wire
		// their own MessageBus.Subscribe at the bus level.
	},
}

// Subscription is the handle returned by Subscribe. Calling
// Unsubscribe stops handler delivery and frees the NATS subscription.
type Subscription interface {
	Unsubscribe() error
}

// EventService is the in-process facade over the NATS MessageBus.
// Subscribe + Publish take EventURI strings and resolve them to
// concrete NATS subjects through `EventURI.Subject(tenantID,
// universeID, scopeID)` — see the EventURI godoc for the per-scope
// mapping.
type EventService interface {
	// Subscribe registers handler against the subject the EventURI
	// resolves to. The handler is invoked once per matching event
	// with the raw payload bytes. scopeID is the scope-relevant ID
	// (workspaceID / sessionID / roomID per the EventURI's scope);
	// pass empty for tenant- and universe-scoped events.
	//
	// Wildcards (`*` per segment) in the EventURI Path map to NATS
	// wildcards in the subject, so one Subscribe can match many
	// concrete events.
	//
	// Context cancellation unsubscribes the handler automatically.
	Subscribe(ctx context.Context, uri string, scopeID string, handler Handler) (Subscription, error)

	// Publish emits payload on the subject the EventURI resolves to.
	// payload is JSON-encoded internally; pass any value the standard
	// `encoding/json` package can serialize.
	Publish(ctx context.Context, uri string, scopeID string, payload any) error
}

// Handler is the per-event callback. The payload is the raw bytes
// the publisher serialized (typically JSON). Handlers receive the
// invoking context so they can derive their own per-event scope IDs
// via the standard `messagebus.TenantFromContext` /
// `UniverseFromContext` helpers.
type Handler func(ctx context.Context, payload json.RawMessage)
