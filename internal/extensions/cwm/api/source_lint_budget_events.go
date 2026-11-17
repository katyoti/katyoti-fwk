// SPDX-License-Identifier: Apache-2.0

package api

import (
	"encoding/json"
	"time"
)

// ----------------------------------------------------------------------
// events.* — typed envelope + subscribe I/O
// ----------------------------------------------------------------------

// EventEnvelope is the on-the-wire shape for one event.
//
// Topic convention: `cwm.{node|item|collection|file}.<id>.<verb>`
// or `resource.<id>.snapshot` for shared-state. Method packs may
// emit additional topics (e.g. `safe.coverage.<node>.changed`).
type EventEnvelope struct {
	Topic     string          `json:"topic"     jsonschema:"Hierarchical topic name"`
	Timestamp time.Time       `json:"timestamp"`
	Payload   json.RawMessage `json:"payload,omitempty" jsonschema:"Event-kind-specific payload; opaque to CWM"`
}

// EventsSubscribeInput requests subscription to a topic pattern.
//
// Note: this method is the only non-unary in CWMService. EventsSubscribe
// returns (channel, cancel, error). It is hidden from auto-CLI via
// MethodDef.Hidden=true. The KTSP server-stream wire adapter lives
// in the local/ package.
type EventsSubscribeInput struct {
	TopicPattern string `json:"topic_pattern" jsonschema:"Topic pattern (supports glob: cwm.node.*.* etc.); required"`
}
