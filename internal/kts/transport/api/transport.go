// SPDX-License-Identifier: Apache-2.0

// Package api defines the core transport interface for KTS.
//
// Transport is a bidirectional raw message stream carrying json.RawMessage.
// Each transport implementation (ws, stdio, pipe) moves raw JSON bytes;
// protocol-level typing (JSON-RPC 2.0, terminal protocol) happens in
// higher layers (jsonrpc.Session, terminal.Room).
//
// Implementations live in sibling packages:
//
//	ws/    — WebSocket
//	stdio/ — stdin/stdout NDJSON
//	pipe/  — in-process channel pair
package api

import (
	"context"
	"encoding/json"
	"errors"
)

// ErrClosed is returned by Send or Receive when the transport has been closed.
var ErrClosed = errors.New("transport: closed")

// Transport is a bidirectional raw JSON message stream.
//
// Implementations must be safe for concurrent use.
// Close must unblock any active Receive channels.
type Transport interface {
	// Send transmits a raw JSON message to the peer.
	// Returns ErrClosed if the transport has been closed, ctx.Err() on cancellation.
	Send(ctx context.Context, msg json.RawMessage) error

	// Receive returns a channel that delivers incoming raw JSON messages.
	// The channel is closed when the transport closes or the context is cancelled.
	// Returns ErrClosed if the transport is already closed before Receive is called.
	Receive(ctx context.Context) (<-chan json.RawMessage, error)

	// Close shuts down the transport. Safe to call multiple times.
	Close() error
}
