// SPDX-License-Identifier: Apache-2.0

package endpoint

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp"
)

// Routing dispatches KTSP requests through the Session Manager's router/send slot.
// It wraps outgoing requests so the SM can resolve the target container by path.
type Routing struct {
	smTransport ktsp.Transport
	sessionID   string
}

// NewRouting creates a routing endpoint that dispatches through the SM.
func NewRouting(smTransport ktsp.Transport, sessionID string) *Routing {
	return &Routing{smTransport: smTransport, sessionID: sessionID}
}

type routerSendParams struct {
	SessionID string          `json:"sessionId"`
	Path      string          `json:"path"`
	Params    json.RawMessage `json:"params,omitempty"`
}

func (e *Routing) Send(ctx context.Context, req *ktsp.Request) (*ktsp.Response, error) {
	wrapped, err := e.wrapRequest(req)
	if err != nil {
		return nil, fmt.Errorf("wrap request for router/send: %w", err)
	}
	return e.smTransport.Send(ctx, wrapped)
}

func (e *Routing) Notify(ctx context.Context, notif *ktsp.Notification) error {
	params, err := json.Marshal(routerSendParams{
		SessionID: e.sessionID,
		Path:      notif.Path(),
		Params:    notif.Params,
	})
	if err != nil {
		return fmt.Errorf("marshal router/send params: %w", err)
	}
	req, err := ktsp.NewRequest(nil, "router/send", json.RawMessage(params))
	if err != nil {
		return fmt.Errorf("build router/send request: %w", err)
	}
	_, sendErr := e.smTransport.Send(ctx, req)
	return sendErr
}

func (e *Routing) Close() error {
	return e.smTransport.Close()
}

func (e *Routing) wrapRequest(req *ktsp.Request) (*ktsp.Request, error) {
	params, err := json.Marshal(routerSendParams{
		SessionID: e.sessionID,
		Path:      req.Path(),
		Params:    req.Params,
	})
	if err != nil {
		return nil, err
	}
	return ktsp.NewRequest(req.ID, "router/send", json.RawMessage(params))
}
