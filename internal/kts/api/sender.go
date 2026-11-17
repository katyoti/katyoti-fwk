// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"
	"encoding/json"
	"time"
)

// This file holds the CONSUMER half of KTSP: the contract a caller needs to send
// a request, and nothing else.
//
// It lives here rather than in the protocol package so that code which only
// calls services depends on a two-method interface instead of the container,
// endpoint, engine, slot factory, transport and stubs that implement the
// protocol. The motivation is direction, not bytes — the protocol package is
// small, but nothing stops a client reaching into machinery it has no business
// touching once it imports it.
//
// The protocol package re-exports these names, so every existing caller compiles
// unchanged and `ktsp.Sender` remains the public face for code that already has
// the protocol in scope.

// Sender sends requests to a KTSP endpoint.
//
// Path is "container_id/slot/method", or "slot/method" when the sender is
// already bound to one container.
type Sender interface {
	// Call sends a request and returns the result (request/response RPC).
	// Returns an error if the request fails or the response carries an RPC
	// error. opts override the per-call defaults.
	Call(ctx context.Context, path string, params any, opts ...CallOption) (json.RawMessage, error)

	// Notify sends a notification — fire and forget, no response expected.
	Notify(ctx context.Context, path string, params any, opts ...CallOption) error
}

// CallOptions overrides the per-call defaults for a Call or Notify. Zero-value
// fields mean "no override": the sentinels below preserve behaviour when a
// caller passes no options.
//
// Timeout resolution: an explicit WithTimeout > the method's advertised timeout
// > the transport default. Retry resolution: an explicit WithRetries > the
// sender's configured policy.
type CallOptions struct {
	// Timeout caps this one call. 0 = no override (use the advertised
	// per-method timeout, else the transport default).
	Timeout time.Duration
	// Retries is the max retry attempts for this call. -1 = no override (use
	// the sender's policy); 0 = no retries; N > 0 = N attempts.
	Retries int
	// BaseDelay is the initial backoff before the first retry. 0 = no override.
	BaseDelay time.Duration
	// Idempotent marks the call safe to retry after an ambiguous failure (a
	// timeout, a lost response). Non-idempotent calls suppress retries on
	// ambiguous failures, to avoid double execution.
	Idempotent bool
}

// CallOption mutates CallOptions (the functional-options pattern).
type CallOption func(*CallOptions)

// DefaultCallOptions returns the "no override" baseline — every field a
// sentinel that preserves default behaviour.
func DefaultCallOptions() CallOptions {
	return CallOptions{Timeout: 0, Retries: -1, BaseDelay: 0, Idempotent: false}
}

// ApplyCallOptions folds opts onto the no-override baseline.
func ApplyCallOptions(opts ...CallOption) CallOptions {
	o := DefaultCallOptions()
	for _, opt := range opts {
		if opt != nil {
			opt(&o)
		}
	}
	return o
}

// WithTimeout caps this one call at d (overriding the advertised timeout).
func WithTimeout(d time.Duration) CallOption { return func(o *CallOptions) { o.Timeout = d } }

// WithRetries sets the max retry attempts for this one call (0 = none).
func WithRetries(n int) CallOption { return func(o *CallOptions) { o.Retries = n } }

// WithBackoff sets the initial retry backoff for this one call.
func WithBackoff(base time.Duration) CallOption { return func(o *CallOptions) { o.BaseDelay = base } }

// WithIdempotent marks this call safe to retry after an ambiguous failure.
func WithIdempotent() CallOption { return func(o *CallOptions) { o.Idempotent = true } }
