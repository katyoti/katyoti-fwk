// SPDX-License-Identifier: Apache-2.0

package jsonrpc_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp"
	ktransport "github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp/jsonrpc"

	"github.com/gorilla/websocket"
	genstdio "github.com/katyoti/katyoti-fwk/internal/kts/transport/stdio"
)

// pipeAdapterPair creates a connected KTSP adapter pair using io.Pipe (no subprocess).
// client is a self-starting adapter; server wraps a ServerTransport that must be Run().
func pipeAdapterPair(t *testing.T) (client *ktransport.Session, server *ktransport.Session) {
	t.Helper()
	serverInR, serverInW := io.Pipe()
	clientInR, clientInW := io.Pipe()

	client = ktransport.NewFromPipes(clientInR, serverInW)

	// Server side: raw stdio stream → KTSP session
	stream := genstdio.New(serverInR, clientInW)
	server = ktransport.NewSession(stream, "srvstdio")

	t.Cleanup(func() {
		client.Close()
		server.Close()
		serverInR.Close()
		serverInW.Close()
		clientInR.Close()
		clientInW.Close()
	})
	return client, server
}

func TestAdapter_Stdio_SendReceive(t *testing.T) {
	client, server := pipeAdapterPair(t)

	server.OnRequest(func(ctx context.Context, slot, method string, params json.RawMessage) (json.RawMessage, error) {
		data, _ := json.Marshal(slot + "." + method)
		return data, nil
	})
	go server.Run()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, _ := ktsp.NewRequest(nil, "myslot/mymethod", nil)
	resp, err := client.Send(ctx, req)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if resp.Error != nil {
		t.Fatalf("response error: %v", resp.Error)
	}

	var result string
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if result != "myslot.mymethod" {
		t.Errorf("got %q, want %q", result, "myslot.mymethod")
	}
}

func TestAdapter_Stdio_ConcurrentSends(t *testing.T) {
	client, server := pipeAdapterPair(t)

	server.OnRequest(func(ctx context.Context, slot, method string, params json.RawMessage) (json.RawMessage, error) {
		data, _ := json.Marshal(method)
		return data, nil
	})
	go server.Run()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	const n = 20
	var wg sync.WaitGroup
	errs := make(chan error, n)

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			method := fmt.Sprintf("method_%d", i)
			req, _ := ktsp.NewRequest(nil, "slot/"+method, nil)
			resp, err := client.Send(ctx, req)
			if err != nil {
				errs <- fmt.Errorf("send %d: %w", i, err)
				return
			}
			var got string
			if err := json.Unmarshal(resp.Result, &got); err != nil {
				errs <- fmt.Errorf("unmarshal %d: %w", i, err)
				return
			}
			if got != method {
				errs <- fmt.Errorf("send %d: got %q, want %q", i, got, method)
			}
		}(i)
	}

	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}

func TestAdapter_Stdio_RPCError(t *testing.T) {
	client, server := pipeAdapterPair(t)

	server.OnRequest(func(ctx context.Context, slot, method string, params json.RawMessage) (json.RawMessage, error) {
		return nil, ktsp.ErrMethodNotFound(method)
	})
	go server.Run()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, _ := ktsp.NewRequest(nil, "slot/nonexistent", nil)
	resp, err := client.Send(ctx, req)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if resp.Error == nil {
		t.Fatal("expected RPC error in response")
	}
	if resp.Error.Code != ktsp.CodeMethodNotFound {
		t.Errorf("error code = %d, want %d", resp.Error.Code, ktsp.CodeMethodNotFound)
	}
}

func TestAdapter_Stdio_Notify(t *testing.T) {
	client, server := pipeAdapterPair(t)

	received := make(chan string, 1)
	server.OnNotify("events", "ping", func(params json.RawMessage) {
		received <- "ping"
	})
	go server.Run()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	notif, _ := ktsp.NewNotification("events/ping", nil)
	if err := client.Notify(ctx, notif); err != nil {
		t.Fatalf("Notify: %v", err)
	}

	select {
	case got := <-received:
		if got != "ping" {
			t.Errorf("got %q, want %q", got, "ping")
		}
	case <-ctx.Done():
		t.Fatal("timeout waiting for notification")
	}
}

func TestAdapter_Stdio_BidirectionalSend(t *testing.T) {
	client, server := pipeAdapterPair(t)

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

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
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
		t.Errorf("unexpected result: %v", result)
	}
}

// --- WebSocket tests ---

func TestAdapter_WS_ServerToClientRequest(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	serverReady := make(chan *ktransport.Session, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/test", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		adapter := ktransport.NewWSConn(conn)
		defer adapter.Close()
		serverReady <- adapter
		adapter.Run() // blocks until connection closes
	})

	ln, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: mux}
	go func() { _ = srv.Serve(ln) }()
	defer srv.Shutdown(context.Background())

	wsURL := "ws://" + ln.Addr().String() + "/test"

	// Connect client — handles incoming requests by echoing params.
	client, err := ktransport.Dial(ctx, wsURL)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	client.OnRequest(func(_ context.Context, slot, method string, params json.RawMessage) (json.RawMessage, error) {
		return params, nil
	})

	var serverConn *ktransport.Session
	select {
	case serverConn = <-serverReady:
	case <-ctx.Done():
		t.Fatal("timeout waiting for server connection")
	}

	req, _ := ktsp.NewRequest(nil, "test/echo", json.RawMessage(`{"hello":"world"}`))
	resp, err := serverConn.Send(ctx, req)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if resp.Error != nil {
		t.Fatalf("unexpected RPC error: %v", resp.Error)
	}
	if string(resp.Result) != `{"hello":"world"}` {
		t.Errorf("result = %s, want %s", resp.Result, `{"hello":"world"}`)
	}
}

func TestAdapter_WS_ServerToClientRPCError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	serverReady := make(chan *ktransport.Session, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/test", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		adapter := ktransport.NewWSConn(conn)
		defer adapter.Close()
		serverReady <- adapter
		adapter.Run()
	})

	ln, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: mux}
	go func() { _ = srv.Serve(ln) }()
	defer srv.Shutdown(context.Background())

	client, err := ktransport.Dial(ctx, "ws://"+ln.Addr().String()+"/test")
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	client.OnRequest(func(_ context.Context, slot, method string, params json.RawMessage) (json.RawMessage, error) {
		return nil, ktsp.ErrSlotNotFound("no such slot: " + slot)
	})

	var serverConn *ktransport.Session
	select {
	case serverConn = <-serverReady:
	case <-ctx.Done():
		t.Fatal("timeout")
	}

	req, _ := ktsp.NewRequest(nil, "missing/anything", nil)
	resp, err := serverConn.Send(ctx, req)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if resp.Error == nil {
		t.Fatal("expected RPC error, got nil")
	}
	if resp.Error.Code != -40001 {
		t.Errorf("error code = %d, want -40001 (CodeSlotNotFound)", resp.Error.Code)
	}
}
