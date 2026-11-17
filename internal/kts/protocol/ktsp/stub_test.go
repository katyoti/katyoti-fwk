// SPDX-License-Identifier: Apache-2.0

package ktsp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
)

// mockSender implements Sender for testing GenericStub.
type mockSender struct {
	sendFn   func(ctx context.Context, path string, params any) (json.RawMessage, error)
	notifyFn func(ctx context.Context, path string, params any) error
}

func (m *mockSender) Call(ctx context.Context, path string, params any, opts ...CallOption) (json.RawMessage, error) {
	if m.sendFn != nil {
		return m.sendFn(ctx, path, params)
	}
	return json.RawMessage(`{}`), nil
}

func (m *mockSender) Notify(ctx context.Context, path string, params any, opts ...CallOption) error {
	if m.notifyFn != nil {
		return m.notifyFn(ctx, path, params)
	}
	return nil
}

func TestGenericStub_Call_ComposesPath(t *testing.T) {
	var gotPath string
	sender := &mockSender{
		sendFn: func(_ context.Context, path string, _ any) (json.RawMessage, error) {
			gotPath = path
			return json.RawMessage(`{"ok":true}`), nil
		},
	}

	stub := NewGenericStub(sender, "mycontainer/myslot", SlotInfo{})
	_, err := stub.Call(context.Background(), "convert", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotPath != "mycontainer/myslot/convert" {
		t.Errorf("got path %q, want %q", gotPath, "mycontainer/myslot/convert")
	}
}

func TestGenericStub_Notify_ComposesPath(t *testing.T) {
	var gotPath string
	sender := &mockSender{
		notifyFn: func(_ context.Context, path string, _ any) error {
			gotPath = path
			return nil
		},
	}

	stub := NewGenericStub(sender, "c/s", SlotInfo{})
	err := stub.Notify(context.Background(), "ping", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotPath != "c/s/ping" {
		t.Errorf("got path %q, want %q", gotPath, "c/s/ping")
	}
}

func TestGenericStub_Call_WrapsTransportError(t *testing.T) {
	sender := &mockSender{
		sendFn: func(_ context.Context, _ string, _ any) (json.RawMessage, error) {
			return nil, fmt.Errorf("connection refused")
		},
	}

	stub := NewGenericStub(sender, "c/s", SlotInfo{})
	_, err := stub.Call(context.Background(), "method", nil)
	if err == nil {
		t.Fatal("expected error")
	}

	var ktspErr *KTSPError
	if !errors.As(err, &ktspErr) {
		t.Fatalf("expected KTSPError, got %T: %v", err, err)
	}
	if ktspErr.Kind != ErrKindTransport {
		t.Errorf("got kind %q, want %q", ktspErr.Kind, ErrKindTransport)
	}
	if ktspErr.Op != "call" {
		t.Errorf("got op %q, want %q", ktspErr.Op, "call")
	}
}

func TestGenericStub_Call_PassthroughRPCError(t *testing.T) {
	rpcErr := &RPCError{Code: CodeMethodNotFound, Message: "not found"}
	sender := &mockSender{
		sendFn: func(_ context.Context, _ string, _ any) (json.RawMessage, error) {
			return nil, rpcErr
		},
	}

	stub := NewGenericStub(sender, "c/s", SlotInfo{})
	_, err := stub.Call(context.Background(), "method", nil)
	if err == nil {
		t.Fatal("expected error")
	}

	// Should NOT be wrapped in KTSPError
	var ktspErr *KTSPError
	if errors.As(err, &ktspErr) {
		t.Error("RPCError should not be wrapped in KTSPError")
	}

	// Should be the original RPCError
	var gotRPC *RPCError
	if !errors.As(err, &gotRPC) {
		t.Fatalf("expected RPCError, got %T", err)
	}
	if gotRPC.Code != CodeMethodNotFound {
		t.Errorf("got code %d, want %d", gotRPC.Code, CodeMethodNotFound)
	}
}

func TestGenericStub_Call_WrapsTimeoutError(t *testing.T) {
	sender := &mockSender{
		sendFn: func(_ context.Context, _ string, _ any) (json.RawMessage, error) {
			return nil, context.DeadlineExceeded
		},
	}

	stub := NewGenericStub(sender, "c/s", SlotInfo{})
	_, err := stub.Call(context.Background(), "method", nil)

	var ktspErr *KTSPError
	if !errors.As(err, &ktspErr) {
		t.Fatalf("expected KTSPError, got %T: %v", err, err)
	}
	if ktspErr.Kind != ErrKindTimeout {
		t.Errorf("got kind %q, want %q", ktspErr.Kind, ErrKindTimeout)
	}
}

func TestGenericStub_Notify_WrapsTransportError(t *testing.T) {
	sender := &mockSender{
		notifyFn: func(_ context.Context, _ string, _ any) error {
			return fmt.Errorf("broken pipe")
		},
	}

	stub := NewGenericStub(sender, "c/s", SlotInfo{})
	err := stub.Notify(context.Background(), "ping", nil)

	var ktspErr *KTSPError
	if !errors.As(err, &ktspErr) {
		t.Fatalf("expected KTSPError, got %T: %v", err, err)
	}
	if ktspErr.Kind != ErrKindTransport {
		t.Errorf("got kind %q, want %q", ktspErr.Kind, ErrKindTransport)
	}
}

func TestGenericStub_Info(t *testing.T) {
	info := SlotInfo{Name: "test", SlotType: SlotTypeSkill}
	stub := NewGenericStub(&mockSender{}, "c/s", info)
	got := stub.Info()
	if got.Name != "test" || got.SlotType != SlotTypeSkill {
		t.Errorf("Info() mismatch: got %+v", got)
	}
}

func TestGenericStub_BasePath(t *testing.T) {
	stub := NewGenericStub(&mockSender{}, "container/slot", SlotInfo{})
	if stub.BasePath() != "container/slot" {
		t.Errorf("got %q, want %q", stub.BasePath(), "container/slot")
	}
}

// --- CallTyped tests ---

type echoOutput struct {
	Message string `json:"message"`
	Count   int    `json:"count"`
}

func TestCallTyped_Success(t *testing.T) {
	sender := &mockSender{
		sendFn: func(_ context.Context, _ string, _ any) (json.RawMessage, error) {
			return json.RawMessage(`{"message":"hello","count":42}`), nil
		},
	}

	stub := NewGenericStub(sender, "c/s", SlotInfo{})
	out, err := CallTyped[echoOutput](stub, context.Background(), "echo", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Message != "hello" || out.Count != 42 {
		t.Errorf("got %+v, want {hello 42}", out)
	}
}

func TestCallTyped_DeserializeError(t *testing.T) {
	sender := &mockSender{
		sendFn: func(_ context.Context, _ string, _ any) (json.RawMessage, error) {
			return json.RawMessage(`not valid json`), nil
		},
	}

	stub := NewGenericStub(sender, "c/s", SlotInfo{})
	_, err := CallTyped[echoOutput](stub, context.Background(), "echo", nil)
	if err == nil {
		t.Fatal("expected error")
	}

	var ktspErr *KTSPError
	if !errors.As(err, &ktspErr) {
		t.Fatalf("expected KTSPError, got %T: %v", err, err)
	}
	if ktspErr.Kind != ErrKindDeserialize {
		t.Errorf("got kind %q, want %q", ktspErr.Kind, ErrKindDeserialize)
	}
}

func TestCallTyped_PropagatesSenderError(t *testing.T) {
	sender := &mockSender{
		sendFn: func(_ context.Context, _ string, _ any) (json.RawMessage, error) {
			return nil, fmt.Errorf("network down")
		},
	}

	stub := NewGenericStub(sender, "c/s", SlotInfo{})
	_, err := CallTyped[echoOutput](stub, context.Background(), "echo", nil)

	var ktspErr *KTSPError
	if !errors.As(err, &ktspErr) {
		t.Fatalf("expected KTSPError, got %T: %v", err, err)
	}
	// Should be transport (from GenericStub.Call), not deserialize
	if ktspErr.Kind != ErrKindTransport {
		t.Errorf("got kind %q, want %q", ktspErr.Kind, ErrKindTransport)
	}
}
