// SPDX-License-Identifier: Apache-2.0

package terminal

import (
	"context"
	"encoding/json"
)

// TerminalTransport is the typed transport interface for terminal protocol communication.
// It carries json.RawMessage on the wire; callers marshal typed events for Send and
// unmarshal the raw bytes received from the channel.
//
// Implementations in the adapter sub-package:
//   - adapter.Channel   — in-process (Go channels via pipe)
//   - adapter.WS        — WebSocket
//   - adapter.Stdio     — NDJSON over stdin/stdout
type TerminalTransport interface {
	// Send marshals event as JSON and transmits it to the peer.
	Send(ctx context.Context, event any) error

	// Receive returns a channel of raw JSON messages from the peer.
	// The channel closes when the transport closes or the context is cancelled.
	Receive(ctx context.Context) (<-chan json.RawMessage, error)

	Close() error
}
