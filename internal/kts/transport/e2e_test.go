// SPDX-License-Identifier: Apache-2.0

// Package transport_test provides end-to-end tests across the full transport stack:
//
//	pipe.Stream (L0) → jsonrpc.Session (L1) → ktsp.Adapter (L2) → typed KTSP messages
//
// These tests verify that all layers integrate correctly after the transport refactoring:
//   - Transport carries json.RawMessage (non-generic)
//   - Session uses standard JSON-RPC 2.0 wire format ("method", structural classification)
//   - KTSP messages use structured fields (ContainerID, Slot, Method) — no Path string
//   - No JSONRPC version string leaks into L2
//   - Notifications, requests, bidirectional communication all work
package transport_test

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp"
	ktspjsonrpc "github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp/jsonrpc"
	"github.com/katyoti/katyoti-fwk/internal/kts/transport/pipe"
)

// sessionPair creates a connected KTSP session pair over an in-process pipe.
// client auto-starts its read loop; server must be Run() by the caller.
func sessionPair(t *testing.T) (client, server *ktspjsonrpc.Session) {
	t.Helper()
	a, b := pipe.New[json.RawMessage](64)

	client = ktspjsonrpc.NewSession(a, "client")
	go client.Run()

	server = ktspjsonrpc.NewSession(b, "server")

	t.Cleanup(func() {
		client.Close()
		server.Close()
	})
	return client, server
}

// TestE2E_RequestResponse verifies the full round-trip:
// client sends a KTSP request → pipe → jsonrpc.Session → server handler → response back.
func TestE2E_RequestResponse(t *testing.T) {
	client, server := sessionPair(t)

	server.OnRequest(func(ctx context.Context, slot, method string, params json.RawMessage) (json.RawMessage, error) {
		// Verify the adapter correctly split the path into slot+method.
		return json.Marshal(map[string]string{
			"slot":   slot,
			"method": method,
		})
	})
	go server.Run()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	req, _ := ktsp.NewRequest(nil, "workspace/read", map[string]string{"path": "test.go"})

	// Verify structured fields on the request.
	if req.ContainerID != "" {
		t.Errorf("ContainerID = %q, want empty (local path)", req.ContainerID)
	}
	if req.Slot != "workspace" {
		t.Errorf("Slot = %q, want %q", req.Slot, "workspace")
	}
	if req.Method != "read" {
		t.Errorf("Method = %q, want %q", req.Method, "read")
	}

	resp, err := client.Send(ctx, req)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if resp.Error != nil {
		t.Fatalf("response error: %v", resp.Error)
	}

	var result map[string]string
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if result["slot"] != "workspace" || result["method"] != "read" {
		t.Errorf("server received slot=%q method=%q, want workspace/read", result["slot"], result["method"])
	}
}

// TestE2E_ThreeLevelPath verifies container/slot/method addressing.
func TestE2E_ThreeLevelPath(t *testing.T) {
	client, server := sessionPair(t)

	server.OnRequest(func(ctx context.Context, slot, method string, params json.RawMessage) (json.RawMessage, error) {
		return json.Marshal(map[string]string{"slot": slot, "method": method})
	})
	go server.Run()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	req, _ := ktsp.NewRequest(nil, "sm/session/create", nil)

	// Verify structured fields.
	if req.ContainerID != "sm" {
		t.Errorf("ContainerID = %q, want %q", req.ContainerID, "sm")
	}
	if req.Path() != "sm/session/create" {
		t.Errorf("Path() = %q, want %q", req.Path(), "sm/session/create")
	}

	resp, err := client.Send(ctx, req)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	// The adapter's OnRequest handler receives slot+method (not containerID).
	var result map[string]string
	_ = json.Unmarshal(resp.Result, &result)
	if result["slot"] != "session" || result["method"] != "create" {
		t.Errorf("server received slot=%q method=%q, want session/create", result["slot"], result["method"])
	}
}

// TestE2E_Notification verifies fire-and-forget notifications.
func TestE2E_Notification(t *testing.T) {
	client, server := sessionPair(t)

	received := make(chan string, 1)
	server.OnNotify("events", "ping", func(params json.RawMessage) {
		received <- "got-ping"
	})
	go server.Run()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	notif, _ := ktsp.NewNotification("events/ping", map[string]string{"data": "hello"})

	// Verify structured fields.
	if notif.Slot != "events" || notif.Method != "ping" {
		t.Errorf("Notification fields: slot=%q method=%q, want events/ping", notif.Slot, notif.Method)
	}

	if err := client.Notify(ctx, notif); err != nil {
		t.Fatalf("Notify: %v", err)
	}

	select {
	case got := <-received:
		if got != "got-ping" {
			t.Errorf("received %q, want %q", got, "got-ping")
		}
	case <-ctx.Done():
		t.Fatal("timeout waiting for notification")
	}
}

// TestE2E_RPCError verifies that server-side errors propagate correctly.
func TestE2E_RPCError(t *testing.T) {
	client, server := sessionPair(t)

	server.OnRequest(func(ctx context.Context, slot, method string, params json.RawMessage) (json.RawMessage, error) {
		return nil, ktsp.ErrMethodNotFound(method)
	})
	go server.Run()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	req, _ := ktsp.NewRequest(nil, "slot/nonexistent", nil)
	resp, err := client.Send(ctx, req)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if resp.Error == nil {
		t.Fatal("expected RPC error, got nil")
	}
	if resp.Error.Code != ktsp.CodeMethodNotFound {
		t.Errorf("error code = %d, want %d", resp.Error.Code, ktsp.CodeMethodNotFound)
	}
}

// TestE2E_BidirectionalRequests verifies that both sides can send requests.
func TestE2E_BidirectionalRequests(t *testing.T) {
	client, server := sessionPair(t)

	// Client handles reverse requests from the server.
	client.OnRequest(func(ctx context.Context, slot, method string, params json.RawMessage) (json.RawMessage, error) {
		return json.Marshal(map[string]string{"from": "client", "slot": slot})
	})

	// Server triggers a reverse call when it receives a request.
	server.OnRequest(func(ctx context.Context, slot, method string, params json.RawMessage) (json.RawMessage, error) {
		if method == "trigger_reverse" {
			reverseReq, _ := ktsp.NewRequest(nil, "reverse/ping", nil)
			reverseResp, err := server.Send(ctx, reverseReq)
			if err != nil {
				return nil, err
			}
			return reverseResp.Result, nil
		}
		return json.Marshal("default")
	})
	go server.Run()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	req, _ := ktsp.NewRequest(nil, "test/trigger_reverse", nil)
	resp, err := client.Send(ctx, req)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	var result map[string]string
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if result["from"] != "client" || result["slot"] != "reverse" {
		t.Errorf("got %v, want from=client slot=reverse", result)
	}
}

// TestE2E_ConcurrentRequests verifies that multiple concurrent requests
// are correctly correlated with their responses.
func TestE2E_ConcurrentRequests(t *testing.T) {
	client, server := sessionPair(t)

	server.OnRequest(func(ctx context.Context, slot, method string, params json.RawMessage) (json.RawMessage, error) {
		// Echo the method name back — each concurrent goroutine sends a unique method.
		return json.Marshal(method)
	})
	go server.Run()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	const n = 20
	var wg sync.WaitGroup
	errs := make(chan string, n)

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			method := fmt.Sprintf("method_%d", i)
			req, _ := ktsp.NewRequest(nil, "slot/"+method, nil)
			resp, err := client.Send(ctx, req)
			if err != nil {
				errs <- fmt.Sprintf("send %d: %v", i, err)
				return
			}
			var got string
			if err := json.Unmarshal(resp.Result, &got); err != nil {
				errs <- fmt.Sprintf("unmarshal %d: %v", i, err)
				return
			}
			if got != method {
				errs <- fmt.Sprintf("send %d: got %q, want %q", i, got, method)
			}
		}(i)
	}

	wg.Wait()
	close(errs)
	for msg := range errs {
		t.Error(msg)
	}
}

// TestE2E_WireFormat intercepts raw JSON on the pipe to verify
// standard JSON-RPC 2.0 wire format (no "path", no "type" fields).
func TestE2E_WireFormat(t *testing.T) {
	a, b := pipe.New[json.RawMessage](64)

	// Intercept what the client sends on the wire.
	wireCapture := make(chan json.RawMessage, 1)

	clientSession := ktspjsonrpc.NewSession(a, "test")
	go clientSession.Run()

	// Read one message from the server side before starting the server session.
	go func() {
		ch, _ := b.Receive(context.Background())
		raw := <-ch
		wireCapture <- raw
		b.Close()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Send a KTSP request — we don't care about the response, just the wire format.
	go func() {
		req, _ := ktsp.NewRequest(nil, "workspace/read", json.RawMessage(`{"path":"test.go"}`))
		_, _ = clientSession.Send(ctx, req)
	}()

	select {
	case raw := <-wireCapture:
		var wire map[string]json.RawMessage
		if err := json.Unmarshal(raw, &wire); err != nil {
			t.Fatalf("unmarshal wire: %v", err)
		}

		// Must have standard JSON-RPC 2.0 fields.
		if _, ok := wire["jsonrpc"]; !ok {
			t.Error("wire missing 'jsonrpc' field")
		}
		if _, ok := wire["id"]; !ok {
			t.Error("wire missing 'id' field")
		}
		if _, ok := wire["method"]; !ok {
			t.Error("wire missing 'method' field")
		}
		if _, ok := wire["params"]; !ok {
			t.Error("wire missing 'params' field")
		}

		// Must NOT have non-standard fields.
		if _, ok := wire["path"]; ok {
			t.Error("wire has non-standard 'path' field — should use 'method'")
		}
		if _, ok := wire["type"]; ok {
			t.Error("wire has non-standard 'type' field — JSON-RPC classifies structurally")
		}

		// Verify method value.
		var method string
		json.Unmarshal(wire["method"], &method)
		if method != "workspace/read" {
			t.Errorf("wire method = %q, want %q", method, "workspace/read")
		}

	case <-ctx.Done():
		t.Fatal("timeout waiting for wire capture")
	}

	a.Close()
}
