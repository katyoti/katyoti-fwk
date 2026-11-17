// SPDX-License-Identifier: Apache-2.0

package sm

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp"
)

// RemoteContainer is a proxy for a container running in a remote process.
// It implements ktsp.Container by forwarding Handle() calls over a transport
// (typically WsConnTransport). Slot info is cached from the initial ktsp/slots query.
type RemoteContainer struct {
	name      string
	info      ktsp.ContainerInfo
	transport ktsp.Transport
}

// newRemoteContainer creates a RemoteContainer by querying the remote process
// for its descriptor. It prefers `ktsp/describe` (the full ContainerInfo, which
// carries the KTSP protocol version) so the hub can NEGOTIATE compatibility at
// first contact — an out-of-range sidecar is refused HERE with a clear message,
// never as a silent runtime break. It falls back to `ktsp/slots` for a sidecar
// that predates `describe` (treated as the baseline protocol major).
func newRemoteContainer(ctx context.Context, name string, transport ktsp.Transport) (*RemoteContainer, error) {
	// Preferred path: full descriptor + protocol negotiation.
	if req, err := ktsp.NewRequest(nil, "ktsp/describe", nil); err == nil {
		if resp, err := transport.Send(ctx, req); err == nil && resp.Error == nil {
			var info ktsp.ContainerInfo
			if json.Unmarshal(resp.Result, &info) == nil && len(info.Slots) >= 0 {
				if nerr := ktsp.NegotiateProtocol(info.ProtocolVersion); nerr != nil {
					return nil, fmt.Errorf("connect %q: %w", name, nerr)
				}
				info.Name = name // the connection name is authoritative for routing
				return &RemoteContainer{name: name, transport: transport, info: info}, nil
			}
		}
	}

	// Fallback: slots-only (a sidecar without `describe`) → baseline protocol.
	req, _ := ktsp.NewRequest(nil, "ktsp/slots", nil)
	resp, err := transport.Send(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("query slots on %q: %w", name, err)
	}
	if resp.Error != nil {
		return nil, fmt.Errorf("query slots on %q: %s", name, resp.Error.Message)
	}
	var slots []ktsp.SlotInfo
	if err := json.Unmarshal(resp.Result, &slots); err != nil {
		return nil, fmt.Errorf("parse slots for %q: %w", name, err)
	}
	if nerr := ktsp.NegotiateProtocol(0); nerr != nil { // baseline for a describe-less sidecar
		return nil, fmt.Errorf("connect %q: %w", name, nerr)
	}
	return &RemoteContainer{
		name:      name,
		transport: transport,
		info:      ktsp.ContainerInfo{Name: name, Slots: slots},
	}, nil
}

// Info returns the cached container metadata.
func (c *RemoteContainer) Info() ktsp.ContainerInfo {
	return c.info
}

// Handle forwards the request to the remote container via the transport.
// The instance dimension is not yet wired through `ktsp.Request` /
// `ktsp.FormatPath` — local-form paths address the singleton
// DefaultInstance only. CRUD-style named-instance dispatch goes through
// the messagebus surface (which encodes instance on the subject), not
// through this in-process Remote forwarder.
func (c *RemoteContainer) Handle(ctx context.Context, slot, instance, method string, params json.RawMessage) (json.RawMessage, error) {
	if instance != "" && instance != ktsp.DefaultInstance {
		return nil, ktsp.ErrInvalidRequest("sm.RemoteContainer: named-instance dispatch not supported on the in-process Remote forwarder (use the messagebus surface)")
	}
	req, _ := ktsp.NewRequest(nil, ktsp.FormatPath("", slot, method), params)
	resp, err := c.transport.Send(ctx, req)
	if err != nil {
		return nil, err
	}
	if resp.Error != nil {
		return nil, resp.Error
	}
	return resp.Result, nil
}
