// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"sync"
)

// The calls the GUI may give up: a section's load made obsolete -
// a newer refresh, the section scrolled away, the page closed - is cancelled
// by its id, and the cancel reaches the method on the server.
type callRegistry struct {
	mu      sync.Mutex
	running map[string]context.CancelFunc
}

var guiCalls = &callRegistry{running: map[string]context.CancelFunc{}}

// begin derives the context a call runs under id; done ends it. An empty id
// makes a call no one can give up.
func (r *callRegistry) begin(ctx context.Context, id string) (context.Context, func()) {
	c, cancel := context.WithCancel(ctx)
	if id == "" {
		return c, cancel
	}
	r.mu.Lock()
	r.running[id] = cancel
	r.mu.Unlock()
	return c, func() {
		r.mu.Lock()
		delete(r.running, id)
		r.mu.Unlock()
		cancel()
	}
}

// cancel gives up the call running under id; an unknown id is ignored.
func (r *callRegistry) cancel(id string) {
	r.mu.Lock()
	f := r.running[id]
	delete(r.running, id)
	r.mu.Unlock()
	if f != nil {
		f()
	}
}
