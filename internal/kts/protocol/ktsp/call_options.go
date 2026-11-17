// SPDX-License-Identifier: Apache-2.0

package ktsp

import (
	"encoding/json"

	ktsapi "github.com/katyoti/katyoti-fwk/internal/kts/api"
)

// The call-option contract lives in kts/api — the consumer half of KTSP, which
// a caller can depend on without pulling in the container, engine, transport and
// stubs that implement the protocol. These are genuine re-exports, pointing
// DOWNWARD at the lighter package: `ktsp.Sender` stays the public face for code
// that already has the protocol in scope, and no call site changes.
type (
	CallOptions = ktsapi.CallOptions
	CallOption  = ktsapi.CallOption
)

var (
	DefaultCallOptions = ktsapi.DefaultCallOptions
	ApplyCallOptions   = ktsapi.ApplyCallOptions
	WithTimeout        = ktsapi.WithTimeout
	WithRetries        = ktsapi.WithRetries
	WithBackoff        = ktsapi.WithBackoff
	WithIdempotent     = ktsapi.WithIdempotent
)

// Stream is a first-class streaming call result. A CallStream kicks off the
// work and surfaces the correlated event notifications as an ordered stream.
type Stream interface {
	// Recv returns the next event; it blocks until one arrives, the stream
	// completes (io.EOF), or ctx is done. Not safe for concurrent use.
	Recv() (StreamEvent, error)
	// Close stops the stream and releases its subscription. Idempotent.
	Close() error
}

// StreamEvent is one event on a Stream — an event type + its JSON payload
// (mirrors the Observer/notification event shape the transport already carries).
type StreamEvent struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data,omitempty"`
}
