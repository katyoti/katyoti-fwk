// SPDX-License-Identifier: Apache-2.0

package sm

import (
	"context"

	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp"
)

// Router handles cross-container request dispatch within a session.
// All requests MUST use three-level paths (container_id/slot/method).
// Two-level paths are rejected — container_id is always required.
type Router struct {
	session *Session
	cm      *ContainerManager
}

// NewRouter creates a router for the given session and container manager.
func NewRouter(session *Session, cm *ContainerManager) *Router {
	return &Router{session: session, cm: cm}
}

// Route dispatches a KTSP request to the container identified by the path's
// container_id. The container_id may be a UUID or a well-known name (e.g. "sm").
// Paths without a container_id return an error — backward-compat 2-level paths
// are not supported.
func (r *Router) Route(ctx context.Context, req *ktsp.Request) (*ktsp.Response, error) {
	cid := req.ContainerID
	if cid == "" {
		return nil, ktsp.ErrInvalidRequest("path must include container_id: use \"container/slot/method\"")
	}

	// Resolve by UUID first; fall back to name lookup.
	transport, err := r.session.ResolveByID(cid)
	if err != nil {
		transport, err = r.session.ResolveByName(cid)
	}
	if err != nil {
		// On-demand start via ContainerManager (by name) if available.
		if r.cm != nil {
			if startErr := r.startAndRegister(cid); startErr == nil {
				transport, err = r.session.ResolveByName(cid)
			}
		}
	}
	if err != nil {
		return nil, ktsp.ErrSlotNotFound(cid)
	}

	return transport.Send(ctx, req)
}

// RouteNotification sends a fire-and-forget notification to the container
// identified by the notification path's container_id.
func (r *Router) RouteNotification(ctx context.Context, notif *ktsp.Notification) error {
	cid := notif.ContainerID
	if cid == "" {
		return ktsp.ErrInvalidRequest("notification path must include container_id")
	}

	transport, err := r.session.ResolveByID(cid)
	if err != nil {
		transport, err = r.session.ResolveByName(cid)
	}
	if err != nil {
		return err
	}
	return transport.Notify(ctx, notif)
}

// startAndRegister starts a container via CM and registers it in the session.
func (r *Router) startAndRegister(name string) error {
	container, transport, err := r.cm.Start(name)
	if err != nil {
		return err
	}
	_, err = r.session.Register("", name, container, transport)
	return err
}
