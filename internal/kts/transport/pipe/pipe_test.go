// SPDX-License-Identifier: Apache-2.0

package pipe

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/kts/transport/api"
)

func recvOne[T any](t *testing.T, ctx context.Context, ch <-chan T) T {
	t.Helper()
	select {
	case msg, ok := <-ch:
		if !ok {
			t.Fatal("channel closed unexpectedly")
		}
		return msg
	case <-ctx.Done():
		t.Fatalf("recvOne: %v", ctx.Err())
		var zero T
		return zero
	}
}

func TestStream_RoundTrip(t *testing.T) {
	a, b := New[json.RawMessage](8)
	defer a.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	msg := json.RawMessage(`{"type":"ping"}`)
	if err := a.Send(ctx, msg); err != nil {
		t.Fatalf("Send: %v", err)
	}

	ch, err := b.Receive(ctx)
	if err != nil {
		t.Fatalf("Receive: %v", err)
	}
	got := recvOne(t, ctx, ch)
	if string(got) != string(msg) {
		t.Errorf("got %s, want %s", got, msg)
	}
}

func TestStream_Bidirectional(t *testing.T) {
	a, b := New[json.RawMessage](8)
	defer a.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_ = a.Send(ctx, json.RawMessage(`{"dir":"a2b"}`))
	chB, _ := b.Receive(ctx)
	msg := recvOne(t, ctx, chB)
	if string(msg) != `{"dir":"a2b"}` {
		t.Errorf("a→b: got %s", msg)
	}

	_ = b.Send(ctx, json.RawMessage(`{"dir":"b2a"}`))
	chA, _ := a.Receive(ctx)
	msg = recvOne(t, ctx, chA)
	if string(msg) != `{"dir":"b2a"}` {
		t.Errorf("b→a: got %s", msg)
	}
}

func TestStream_CloseUnblocks(t *testing.T) {
	a, b := New[json.RawMessage](8)

	ch, err := b.Receive(context.Background())
	if err != nil {
		t.Fatalf("Receive: %v", err)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		for range ch {
		}
	}()

	a.Close()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Error("Receive channel did not close after Close()")
	}
}

func TestStream_SendAfterClose(t *testing.T) {
	a, _ := New[json.RawMessage](8)
	a.Close()

	err := a.Send(context.Background(), json.RawMessage(`{}`))
	if err != api.ErrClosed {
		t.Errorf("err = %v, want ErrClosed", err)
	}
}

func TestStream_ReceiveAfterClose(t *testing.T) {
	a, _ := New[json.RawMessage](8)
	a.Close()

	_, err := a.Receive(context.Background())
	if err != api.ErrClosed {
		t.Errorf("err = %v, want ErrClosed", err)
	}
}
