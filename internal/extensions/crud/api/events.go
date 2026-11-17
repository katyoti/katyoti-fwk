// SPDX-License-Identifier: Apache-2.0

package api

import (
	"strings"
	"time"
)

// EventSubject builds the MessageBus subject CRUD change events flow on
// for a (tenant, universe, connection, record-type) tuple. Subscribers
// subscribe to it directly (specific recordType) or with a NATS
// wildcard (`*`) to watch every type on a connection.
//
// Pattern:
//
//	ktw.<tenant>.universe.<u>.crud.<connection>.<type>.event
//
// Empty `recordType` is rendered as `*` so type-wide subscriptions
// produce the canonical wildcard form. Empty tenant / universe pass
// through unchanged — backends that haven't been wired into NATS yet
// (e.g. the in-process MemoryStore during early bring-up) consume the
// raw string as an EventBus topic key. Once ConnectionService injects
// scope from ctx at Watch time, the same builder produces wire-valid
// subjects.
//
// Builder lives here (not under `task/api/engine/subjects.go`) because
// CRUD events are a separate namespace from the task subject tree.
// Lint enforcement: no other package should construct CRUD event
// subjects inline.
func EventSubject(tenantID, universeID, connection, recordType string) string {
	if recordType == "" {
		recordType = "*"
	}
	return strings.Join([]string{
		"ktw", tenantID, "universe", universeID, "crud", connection, recordType, "event",
	}, ".")
}

// WatchInput declares interest in change events on a connection. The
// granularity comes from the URI shape:
//
//	URI=<collection>          → type-wide / collection-wide subscription
//	URI=<collection>/<id>     → single-record subscription
//	URI=<collection>?<facets> → faceted collection subscription
//
// Backends honor only the URI shapes listed in
// CapabilitiesOutput.URIPatterns + the corresponding
// WatchCap.Granularities. Filters layer on top for server-side
// prefiltering (best-effort; consult WatchCap.ServerSideFilters).
type WatchInput struct {
	URI     URI                    `json:"uri" cli:"uri" desc:"Collection or record URI (granularity comes from URI shape)"`
	Filters map[string]FilterValue `json:"filters,omitempty"        desc:"Server-side prefilter (only honored when CapabilitiesOutput.Watch.ServerSideFilters is true)"`
}

// WatchOutput points the subscriber at the MessageBus subject events
// flow on. Callers attach a bus subscription to it and decode each
// payload as an Event. The connection owns the subject's lifecycle —
// subscribers are responsible for unsubscribing when interest ends;
// the connection MAY publish a final WatchKindClosed event when it
// tears the subject down (e.g. session destruction).
type WatchOutput struct {
	Subject    string   `json:"subject"              desc:"MessageBus subject to subscribe to for event delivery"`
	EventTypes []string `json:"eventTypes,omitempty" desc:"Hint at the WatchKind values the subscriber should expect"`
}

// WatchKind values used in Event.Kind. Kept as exported constants so
// consumers can switch on them without spelling-mistake risk and the
// schema generator can surface them in type docs.
const (
	WatchKindCreated = "created"
	WatchKindUpdated = "updated"
	WatchKindDeleted = "deleted"
	// WatchKindClosed marks the end of a subscription's life. The
	// connection emits this on teardown (e.g. session destruction)
	// so subscribers can drop local caches and stop renderers.
	WatchKindClosed = "closed"
)

// Event is the wire shape for one change notification. Body carries
// the new full record for create/update, nil for delete/closed. The
// optional OldRecord is populated by backends that retain pre-images
// (memory stores, audit-overlay strategies); SQL backends without an
// audit trigger may leave it unset.
type Event struct {
	Kind      string    `json:"kind"                desc:"created | updated | deleted | closed"`
	Type      string    `json:"type"                desc:"Record type the event applies to"`
	ID        string    `json:"id,omitempty"        desc:"Record ID (omitted for type-wide control events like closed)"`
	Record    *Record   `json:"record,omitempty"    desc:"Post-mutation record body (nil for delete / closed)"`
	OldRecord *Record   `json:"oldRecord,omitempty" desc:"Pre-mutation record (only when the backend retains pre-images)"`
	Stamp     time.Time `json:"stamp"               desc:"Server-side timestamp"`
}
