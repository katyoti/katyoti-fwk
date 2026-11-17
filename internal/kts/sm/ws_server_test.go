// SPDX-License-Identifier: Apache-2.0

package sm_test

import (
	"context"
	"encoding/json"
	"net"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp"
	ktransport "github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp/jsonrpc"
	ktsm "github.com/katyoti/katyoti-fwk/internal/kts/sm"
)

// startTestWsServer starts a WsServer on an OS-assigned port and returns
// the SM and ws:// URL. The server runs until ctx is cancelled.
func startTestWsServer(t *testing.T, ctx context.Context) (*ktsm.Manager, string) {
	t.Helper()
	cm := ktsm.NewContainerManager()
	sm := ktsm.NewManager(cm)
	srv := ktsm.NewWsServer(sm, 0) // port arg unused — we pass our own listener

	ln, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	wsURL := "ws://" + ln.Addr().String() + "/ktsp"
	go func() { _ = srv.ServeOn(ctx, ln) }()

	// Brief pause for the goroutine to enter Serve().
	time.Sleep(20 * time.Millisecond)
	return sm, wsURL
}

// TestWsServer_RequestResponse verifies basic KTSP request/response over WebSocket.
// Calls session/list (no sessions → empty array) and checks the response.
func TestWsServer_RequestResponse(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, wsURL := startTestWsServer(t, ctx)

	client, err := ktransport.Dial(ctx, wsURL)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer client.Close()

	req, err := ktsp.NewRequest("id-1", "session/list", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Send(ctx, req)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if resp.Error != nil {
		t.Fatalf("unexpected RPC error: %v", resp.Error)
	}

	var sessions []any
	if err := json.Unmarshal(resp.Result, &sessions); err != nil {
		t.Fatalf("parse result: %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("expected 0 sessions, got %d", len(sessions))
	}
}

// TestWsServer_JournalSubscribe verifies that journal events are pushed as
// KTSP notifications (journal/event) to a subscribed WebSocket client.
func TestWsServer_JournalSubscribe(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	sm, wsURL := startTestWsServer(t, ctx)

	// Create a session directly on the SM (not via WS — just for setup).
	sessResult, err := sm.Handle(context.Background(), "session", ktsp.DefaultInstance, "create", nil)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	var info struct {
		SessionID string `json:"sessionId"`
	}
	if err := json.Unmarshal(sessResult, &info); err != nil {
		t.Fatalf("parse session info: %v", err)
	}

	// Connect a WsTransport client.
	client, err := ktransport.Dial(ctx, wsURL)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer client.Close()

	// Register notification handler for journal/event.
	received := make(chan json.RawMessage, 1)
	client.OnNotify("journal", "event", func(params json.RawMessage) {
		received <- params
	})

	// Subscribe via KTSP over WS.
	subReq, err := ktsp.NewRequest("sub-1", "journal/subscribe", map[string]string{
		"sessionId": info.SessionID,
	})
	if err != nil {
		t.Fatal(err)
	}
	subResp, err := client.Send(ctx, subReq)
	if err != nil {
		t.Fatalf("subscribe Send: %v", err)
	}
	if subResp.Error != nil {
		t.Fatalf("subscribe RPC error: %v", subResp.Error)
	}

	// Emit a journal event directly on the SM.
	emitParams, _ := json.Marshal(map[string]any{
		"sessionId":     info.SessionID,
		"containerName": "test",
		"type":          "test_event",
		"data":          json.RawMessage(`{"msg":"hello"}`),
	})
	if _, err := sm.Handle(context.Background(), "journal", ktsp.DefaultInstance, "emit", emitParams); err != nil {
		t.Fatalf("emit: %v", err)
	}

	// Expect the notification to arrive at the client.
	select {
	case params := <-received:
		if len(params) == 0 {
			t.Error("received empty notification params")
		}
		t.Logf("journal/event received: %s", params)
	case <-ctx.Done():
		t.Fatal("timeout: journal/event notification not received within 5s")
	}
}

// TestWsServer_RemoteRegistration verifies Path B: a WS client self-registers
// as a container, the SM queries its slots via ktsp/slots, and then routes
// requests to the remote container via the WS connection.
func TestWsServer_RemoteRegistration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, wsURL := startTestWsServer(t, ctx)

	// Connect a client that acts as a remote container.
	client, err := ktransport.Dial(ctx, wsURL)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer client.Close()

	// Set up a local container with a "greet" slot.
	localContainer := ktsp.NewEndpoint("greeter", "A test container", "1.0.0")
	greetSlot := ktsp.NewSlot("greet", ktsp.DefaultInstance, "Greeting slot", nil)
	greetSlot.Add(ktsp.RawMethod("hello", ktsp.MethodOpts{}, func(_ context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p struct {
			Name string `json:"name"`
		}
		if params != nil {
			json.Unmarshal(params, &p)
		}
		return json.Marshal(map[string]string{"message": "Hello, " + p.Name + "!"})
	}))
	localContainer.AddSlot(greetSlot)

	// Register OnRequest handler: forward to local container's Handle.
	client.OnRequest(func(ctx context.Context, slot, method string, params json.RawMessage) (json.RawMessage, error) {
		return localContainer.Handle(ctx, slot, ktsp.DefaultInstance, method, params)
	})

	// Step 1: Create a session.
	createReq, _ := ktsp.NewRequest("c-1", "session/create", nil)
	createResp, err := client.Send(ctx, createReq)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if createResp.Error != nil {
		t.Fatalf("create session RPC error: %v", createResp.Error)
	}
	var sessInfo struct {
		SessionID string `json:"sessionId"`
	}
	json.Unmarshal(createResp.Result, &sessInfo)

	// Step 2: Self-register as "greeter" container.
	regReq, _ := ktsp.NewRequest("r-1", "registry/register", map[string]string{
		"sessionId":     sessInfo.SessionID,
		"containerName": "greeter",
	})
	regResp, err := client.Send(ctx, regReq)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if regResp.Error != nil {
		t.Fatalf("register RPC error: %v", regResp.Error)
	}
	var regResult struct {
		Status string `json:"status"`
		Path   string `json:"path"`
	}
	json.Unmarshal(regResp.Result, &regResult)
	if regResult.Path != "remote" {
		t.Errorf("path = %q, want %q", regResult.Path, "remote")
	}

	// Step 3: Route a request to the remote container via router/send.
	routeReq, _ := ktsp.NewRequest("route-1", "router/send", map[string]any{
		"sessionId": sessInfo.SessionID,
		"path":      "greeter/greet/hello",
		"params":    json.RawMessage(`{"name":"World"}`),
	})
	routeResp, err := client.Send(ctx, routeReq)
	if err != nil {
		t.Fatalf("router/send: %v", err)
	}
	if routeResp.Error != nil {
		t.Fatalf("router/send RPC error: %v", routeResp.Error)
	}

	var greeting struct {
		Message string `json:"message"`
	}
	json.Unmarshal(routeResp.Result, &greeting)
	if greeting.Message != "Hello, World!" {
		t.Errorf("message = %q, want %q", greeting.Message, "Hello, World!")
	}
}
