// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"
	"sync"
)

// ServingHook is work a process does once it SERVES its universe — when it
// runs as a long-lived sidecar answering its slots over the bus (`ktsp nats`),
// not as a one-shot command. Extensions contribute one to start what only the
// serving process may do: resume what a previous run of it left, start a
// background loop.
//
// Start is called once, on its own goroutine, with the serving context: it
// ends when the process stops serving (SIGINT, SIGTERM, a lost serving lock).
// The process then waits a bounded time (10 s) for the hooks to return before
// it exits, so a hook may do its last work at stop.
//
// First hooks start before the others, which start once every First hook is
// ready: it called ServingReady(ctx), or returned (the task restore decides
// the runs a previous process left before the planner looks at them).
type ServingHook struct {
	Name  string
	Start func(ctx context.Context)
	First bool
}

type servingReadyKey struct{}

// WithServingReady gives a First hook's context the signal it calls when the
// others may start.
func WithServingReady(ctx context.Context, ready func()) context.Context {
	var once sync.Once
	return context.WithValue(ctx, servingReadyKey{}, func() { once.Do(ready) })
}

// ServingReady says this First hook has done what the others wait for. A no-op
// outside a First hook.
func ServingReady(ctx context.Context) {
	if ready, ok := ctx.Value(servingReadyKey{}).(func()); ok {
		ready()
	}
}
