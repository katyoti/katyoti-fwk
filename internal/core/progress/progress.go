// SPDX-License-Identifier: Apache-2.0

// Package progress is the neutral live-progress contract: a long-running
// operation emits data snapshots through a context-carried Emitter, and a
// consumer (e.g. the CLI in rich mode) renders them live. Transport-neutral —
// no import of tui / gui / a specific renderer.
//
// A method opts in by calling progress.Emit(ctx, snapshot) on each update; when
// no emitter is installed (text/json mode, non-TTY, tests) it is a no-op, so
// emitting is always safe.
package progress

import "context"

// Emitter receives live data snapshots from a long-running operation. The
// snapshot is the same data object the method ultimately returns (e.g. a
// progress/status tree) — the renderer displays it per its display schema.
type Emitter interface {
	Emit(snapshot any)
}

type ctxKey struct{}

// WithEmitter installs e on the context so downstream code can Emit to it.
func WithEmitter(ctx context.Context, e Emitter) context.Context {
	if e == nil {
		return ctx
	}
	return context.WithValue(ctx, ctxKey{}, e)
}

// EmitterFrom returns the installed emitter, if any.
func EmitterFrom(ctx context.Context) (Emitter, bool) {
	e, ok := ctx.Value(ctxKey{}).(Emitter)
	return e, ok && e != nil
}

// Emit sends a snapshot to the ctx emitter when one is installed (no-op otherwise).
func Emit(ctx context.Context, snapshot any) {
	if e, ok := EmitterFrom(ctx); ok {
		e.Emit(snapshot)
	}
}
