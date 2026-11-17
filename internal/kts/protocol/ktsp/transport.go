// SPDX-License-Identifier: Apache-2.0

package ktsp

import (
	"context"
	"encoding/json"
	"errors"
)

// ErrTransportClosed is returned when sending on a closed transport.
var ErrTransportClosed = errors.New("ktsp: transport closed")

// Transport is the abstraction for sending KTSP messages between containers.
// Implementations handle the physical medium (Go channels, stdio, HTTP, WebSocket).
// The transport knows nothing about routing — it just delivers bytes.
type Transport interface {
	// Send sends a request and waits for a response.
	Send(ctx context.Context, req *Request) (*Response, error)

	// Notify sends a notification (fire-and-forget, no response expected).
	Notify(ctx context.Context, notif *Notification) error

	// Close shuts down the transport.
	Close() error
}

// NotificationHandler handles incoming KTSP notifications pushed by the server.
type NotificationHandler func(params json.RawMessage)

// RequestHandler handles incoming KTSP requests from the server.
type RequestHandler func(ctx context.Context, slot, method string, params json.RawMessage) (json.RawMessage, error)
