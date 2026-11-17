// SPDX-License-Identifier: Apache-2.0

package endpoint_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp/endpoint"
)

// captureTransport records requests sent through it and returns a configurable response.
type captureTransport struct {
	requests      []*ktsp.Request
	notifications []*ktsp.Notification
	response      *ktsp.Response
	err           error
}

func (t *captureTransport) Send(ctx context.Context, req *ktsp.Request) (*ktsp.Response, error) {
	t.requests = append(t.requests, req)
	if t.err != nil {
		return nil, t.err
	}
	if t.response != nil {
		return t.response, nil
	}
	// Default: return empty success
	return &ktsp.Response{ID: req.ID, Result: json.RawMessage(`{"ok":true}`)}, nil
}

func (t *captureTransport) Notify(ctx context.Context, notif *ktsp.Notification) error {
	t.notifications = append(t.notifications, notif)
	return t.err
}

func (t *captureTransport) Close() error {
	return nil
}

func TestRouting_Send_WrapsAsRouterSend(t *testing.T) {
	ct := &captureTransport{}
	rt := endpoint.NewRouting(ct, "sess-1")

	req, _ := ktsp.NewRequest(int64(42), "ui/ask_user", map[string]string{"question": "hello?"})
	resp, err := rt.Send(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have sent exactly one request to the SM
	if len(ct.requests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(ct.requests))
	}

	wrapped := ct.requests[0]
	if wrapped.Slot != "router" {
		t.Errorf("expected slot 'router', got %q", wrapped.Slot)
	}
	if wrapped.Method != "send" {
		t.Errorf("expected method 'send', got %q", wrapped.Method)
	}

	// Parse the wrapped params
	var p struct {
		SessionID string          `json:"sessionId"`
		Path      string          `json:"path"`
		Params    json.RawMessage `json:"params"`
	}
	if err := json.Unmarshal(wrapped.Params, &p); err != nil {
		t.Fatalf("failed to parse wrapped params: %v", err)
	}

	if p.SessionID != "sess-1" {
		t.Errorf("expected sessionId 'sess-1', got %q", p.SessionID)
	}
	if p.Path != "ui/ask_user" {
		t.Errorf("expected path 'ui/ask_user', got %q", p.Path)
	}

	// Verify the original params are preserved
	var innerParams map[string]string
	if err := json.Unmarshal(p.Params, &innerParams); err != nil {
		t.Fatalf("failed to parse inner params: %v", err)
	}
	if innerParams["question"] != "hello?" {
		t.Errorf("expected question 'hello?', got %q", innerParams["question"])
	}

	// Verify the response is returned
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
}

func TestRouting_Send_PreservesID(t *testing.T) {
	ct := &captureTransport{}
	rt := endpoint.NewRouting(ct, "sess-2")

	req, _ := ktsp.NewRequest(int64(99), "tool/call", nil)
	_, err := rt.Send(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wrapped := ct.requests[0]
	// The wrapped request should preserve the original ID
	if wrapped.ID != int64(99) {
		t.Errorf("expected ID 99, got %v", wrapped.ID)
	}
}

func TestRouting_Send_PropagatesError(t *testing.T) {
	ct := &captureTransport{err: ktsp.ErrInternalError("SM down")}
	rt := endpoint.NewRouting(ct, "sess-3")

	req, _ := ktsp.NewRequest(int64(1), "ui/ask_user", nil)
	_, err := rt.Send(context.Background(), req)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRouting_Send_PropagatesRPCError(t *testing.T) {
	ct := &captureTransport{
		response: &ktsp.Response{
			ID:    int64(1),
			Error: ktsp.ErrSlotNotFound("ui"),
		},
	}
	rt := endpoint.NewRouting(ct, "sess-4")

	req, _ := ktsp.NewRequest(int64(1), "ui/ask_user", nil)
	resp, err := rt.Send(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected transport error: %v", err)
	}
	if resp.Error == nil {
		t.Fatal("expected RPC error in response")
	}
}

func TestRouting_Notify_WrapsAsRouterSend(t *testing.T) {
	ct := &captureTransport{}
	rt := endpoint.NewRouting(ct, "sess-5")

	notif, _ := ktsp.NewNotification("journal/emit", map[string]string{"type": "text_delta"})
	err := rt.Notify(context.Background(), notif)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Notify is converted to a Send on the SM
	if len(ct.requests) != 1 {
		t.Fatalf("expected 1 request (from notify), got %d", len(ct.requests))
	}

	wrapped := ct.requests[0]
	if wrapped.Slot != "router" || wrapped.Method != "send" {
		t.Errorf("expected router/send, got %s/%s", wrapped.Slot, wrapped.Method)
	}

	var p struct {
		SessionID string `json:"sessionId"`
		Path      string `json:"path"`
	}
	if err := json.Unmarshal(wrapped.Params, &p); err != nil {
		t.Fatalf("failed to parse: %v", err)
	}
	if p.Path != "journal/emit" {
		t.Errorf("expected path journal/emit, got %s", p.Path)
	}
}

func TestRouting_Notify_PropagatesError(t *testing.T) {
	ct := &captureTransport{err: ktsp.ErrInternalError("fail")}
	rt := endpoint.NewRouting(ct, "sess-6")

	notif, _ := ktsp.NewNotification("journal/emit", nil)
	err := rt.Notify(context.Background(), notif)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRouting_Close(t *testing.T) {
	ct := &captureTransport{}
	rt := endpoint.NewRouting(ct, "sess-7")

	err := rt.Close()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRouting_NilParams(t *testing.T) {
	ct := &captureTransport{}
	rt := endpoint.NewRouting(ct, "sess-8")

	req, _ := ktsp.NewRequest(int64(1), "agent/status", nil)
	_, err := rt.Send(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var p struct {
		Params json.RawMessage `json:"params"`
	}
	json.Unmarshal(ct.requests[0].Params, &p)
	// nil params should result in null or empty in the wrapped params
	if p.Params != nil && string(p.Params) != "null" {
		t.Logf("inner params: %s (ok if null or empty)", string(p.Params))
	}
}
