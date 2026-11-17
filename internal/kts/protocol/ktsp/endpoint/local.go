// SPDX-License-Identifier: Apache-2.0

// Package endpoint provides in-process KTSP dispatch (no wire transport).
package endpoint

import (
	"context"
	"fmt"
	"sync"

	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp"
)

// Local dispatches KTSP requests directly to a Container via function call.
// No serialization, no wire format. Used when containers run in-process.
type Local struct {
	container ktsp.Container

	mu     sync.Mutex
	closed bool
}

// NewLocal creates an in-process endpoint for the given container.
func NewLocal(container ktsp.Container) *Local {
	return &Local{container: container}
}

func (e *Local) Send(ctx context.Context, req *ktsp.Request) (*ktsp.Response, error) {
	e.mu.Lock()
	if e.closed {
		e.mu.Unlock()
		return nil, fmt.Errorf("endpoint closed")
	}
	e.mu.Unlock()

	// In-process Request shapes don't yet carry an instance token —
	// every local dispatch addresses the singleton DefaultInstance.
	// When ktsp.Request gains an Instance field (for routing named
	// CRUD connections via in-process composition), thread req.Instance
	// here.
	result, err := e.container.Handle(ctx, req.Slot, ktsp.DefaultInstance, req.Method, req.Params)
	if err != nil {
		return ktsp.NewErrorResponse(req.ID, ktsp.FromError(err)), nil
	}

	return &ktsp.Response{ID: req.ID, Result: result}, nil
}

func (e *Local) Notify(ctx context.Context, notif *ktsp.Notification) error {
	e.mu.Lock()
	if e.closed {
		e.mu.Unlock()
		return fmt.Errorf("endpoint closed")
	}
	e.mu.Unlock()

	_, _ = e.container.Handle(ctx, notif.Slot, ktsp.DefaultInstance, notif.Method, notif.Params)
	return nil
}

func (e *Local) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.closed = true
	return nil
}
