// SPDX-License-Identifier: Apache-2.0

package ktsp

import "context"

// senderCtxKey is the unexported context key under which a server-side
// session-bound Sender is stored when a request is being handled. Allows
// service implementations to send fire-and-forget notifications back to
// the calling client without holding a long-lived reference to the session.
type senderCtxKey struct{}

// WithSender stores the given Sender in ctx. KTSP transport bootstrap
// (e.g., StdioServe) wraps each incoming request's ctx with this so that
// service handlers — and any goroutines they spawn off the request — can
// emit notifications via SenderFromContext.
//
// The stored Sender writes back to the same wire that delivered the
// request. Stash the value during a request handler if the goroutine
// outlives the request (e.g., a long-running task whose events stream
// after StartTask has returned).
func WithSender(ctx context.Context, s Sender) context.Context {
	if s == nil {
		return ctx
	}
	return context.WithValue(ctx, senderCtxKey{}, s)
}

// SenderFromContext returns the session-bound Sender stored by WithSender,
// or nil if none was injected. nil indicates the handler is running
// outside a transport-bound request (e.g., test harness, in-process
// embedded mode) — services should treat this as "no notifications can
// be sent" rather than panicking.
func SenderFromContext(ctx context.Context) Sender {
	if v, ok := ctx.Value(senderCtxKey{}).(Sender); ok {
		return v
	}
	return nil
}
