// SPDX-License-Identifier: Apache-2.0

package api

import "encoding/json"

// RAISING AN EVENT. Not every event is an object write: a code change, a
// timer, a build that finished. A pack declares the type once (an
// `event-type` contribution with a point) and raises it on the object it
// concerns with EventRaise: CWM stamps it like a write's event (an event id,
// the time, the object's current revision, the caller's cause), publishes it
// (`object.<token>.event`, which the journal records) and fires the point on
// the object — its async subscriptions matched under the cascade guards, its
// plain on_* entries run — exactly as a write fires on_update. The object is
// read, never changed.
//
// Any caller that reaches CWM may raise any declared type: whether another
// module may SUBSCRIBE to it is the type's Published flag, not a raise
// restriction. CWM's own types (object.created …) are raised by its writes
// only.

// RaisedEventAction is a raised event's Action in a match (MatchedEvent): an
// object write's is created / updated / deleted.
const RaisedEventAction = "event"

// EventRaiseInput raises one event of a declared type on one object.
type EventRaiseInput struct {
	Type     string          `json:"type"              jsonschema:"The event type's name (code.changed); declared with a point; required"`
	ObjectID string          `json:"object_id"         jsonschema:"The object the event concerns; required"`
	Payload  json.RawMessage `json:"payload,omitempty" jsonschema:"The event's data (JSON); hooks read it as event.payload"`
}

// EventRaiseOutput is the event as stamped: its id, time, the object's
// revision and the cause.
type EventRaiseOutput struct {
	EventMeta
}
