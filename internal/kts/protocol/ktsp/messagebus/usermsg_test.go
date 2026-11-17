// SPDX-License-Identifier: Apache-2.0

package messagebus

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/usermsg"
	teststub "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/teststub"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp"
)

// A handler's person-facing refusal crosses the bus with its message key: the
// caller gets the English text as Message and the key + params in Data.msg,
// which usermsg.Of finds on the RPCError — the GUI then says it in the
// person's language.
func TestSendCarriesTheMessageKey(t *testing.T) {
	bus := teststub.New()
	ep := ktsp.NewEndpoint("test-container", "test", "1.0.0")
	slot := ktsp.NewSlot("hello", ktsp.DefaultInstance, "test slot", nil)
	slot.Add(ktsp.RawMethod("refuse", ktsp.MethodOpts{Desc: "a keyed refusal"},
		func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
			base := fmt.Errorf("%w: object %q", errors.New("cwm: not found"), "m.a")
			return nil, fmt.Errorf("ObjectGet: %w", usermsg.Wrap(base, "cwm.objectNotFound", map[string]any{"id": "m.a"}))
		}))
	ep.AddSlot(slot)
	group, _ := Subscribe(context.Background(), bus, testTenant, testUniverse, testProvider, ep)
	defer group.Unsubscribe()

	_, err := NewSender(bus, testTenant, testUniverse).Call(context.Background(), testProvider+":/hello/refuse", nil)
	var rpc *ktsp.RPCError
	if !errors.As(err, &rpc) {
		t.Fatalf("want an RPCError, got %T: %v", err, err)
	}
	if rpc.Message != `Internal error: ObjectGet: cwm: not found: object "m.a"` {
		t.Errorf("Message = %q: the text changed", rpc.Message)
	}
	m, ok := usermsg.Of(err)
	if !ok || m.Key != "cwm.objectNotFound" || m.Params["id"] != "m.a" {
		t.Fatalf("Of = %+v, %v; Data = %s", m, ok, rpc.Data)
	}
	var data struct {
		Detail string `json:"detail"`
	}
	if json.Unmarshal(rpc.Data, &data) != nil || data.Detail == "" {
		t.Errorf("the detail is lost: Data = %s", rpc.Data)
	}
}

// An RPCError a handler builds itself keeps its code when given a key.
func TestFromError_KeepsTheCode(t *testing.T) {
	e := ktsp.ErrInvalidParams("x is required").WithMsg(usermsg.Msg{Key: "k.required", Params: map[string]any{"field": "x"}})
	got := ktsp.FromError(fmt.Errorf("wrapped: %w", e))
	if got.Code != ktsp.CodeInvalidParams {
		t.Errorf("Code = %d", got.Code)
	}
	if m, ok := got.UserMsg(); !ok || m.Key != "k.required" {
		t.Errorf("UserMsg = %+v %v", m, ok)
	}
	if ktsp.FromError(nil) != nil {
		t.Error("FromError(nil) is not nil")
	}
	if _, ok := ktsp.ErrInternalError("plain").UserMsg(); ok {
		t.Error("a detail string reads as a key")
	}
}

// refusedInput is a pack's input-refusal sentinel, as cwmapi.ErrInvalidInput
// is one: it satisfies ktsp.InputRefusal without importing ktsp.
type refusedInput string

func (e refusedInput) Error() string    { return string(e) }
func (refusedInput) InputRefused() bool { return true }

// A handler's refusal of its caller's input crosses the bus as invalid params,
// never as an internal error: the caller tells it from a fault
// and does not retry it; the text stays whole in Message, for the readers of
// Error() (cwmapi.Restore, the GUI).
func TestAnInputRefusalCrossesTheBusAsInvalidParams(t *testing.T) {
	bus := teststub.New()
	ep := ktsp.NewEndpoint("test-container", "test", "1.0.0")
	slot := ktsp.NewSlot("hello", ktsp.DefaultInstance, "test slot", nil)
	slot.Add(ktsp.RawMethod("ambiguous", ktsp.MethodOpts{Desc: "an input refusal"},
		func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
			return nil, fmt.Errorf("ObjectGet: %w: object %q is in several projects (walk-a, walk-b)", refusedInput("cwm: invalid input"), "sales.N-1")
		}))
	slot.Add(ktsp.RawMethod("fault", ktsp.MethodOpts{Desc: "a fault"},
		func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
			return nil, errors.New("disk on fire")
		}))
	ep.AddSlot(slot)
	group, _ := Subscribe(context.Background(), bus, testTenant, testUniverse, testProvider, ep)
	defer group.Unsubscribe()
	sender := NewSender(bus, testTenant, testUniverse)

	_, err := sender.Call(context.Background(), testProvider+":/hello/ambiguous", nil)
	var rpc *ktsp.RPCError
	if !errors.As(err, &rpc) {
		t.Fatalf("want an RPCError, got %T: %v", err, err)
	}
	if rpc.Code != ktsp.CodeInvalidParams {
		t.Errorf("Code = %d, want invalid params %d: %q", rpc.Code, ktsp.CodeInvalidParams, rpc.Message)
	}
	const want = `Invalid params: ObjectGet: cwm: invalid input: object "sales.N-1" is in several projects (walk-a, walk-b)`
	if rpc.Message != want {
		t.Errorf("Message = %q, want %q", rpc.Message, want)
	}

	_, err = sender.Call(context.Background(), testProvider+":/hello/fault", nil)
	if !errors.As(err, &rpc) || rpc.Code != ktsp.CodeInternalError {
		t.Errorf("a fault answers %v, want an internal error", err)
	}
}
