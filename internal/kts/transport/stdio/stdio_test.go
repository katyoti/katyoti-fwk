// SPDX-License-Identifier: Apache-2.0

package stdio

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestStream_RoundTrip(t *testing.T) {
	reader := strings.NewReader(`{"type":"hello","text":"world"}` + "\n")
	var buf bytes.Buffer

	s := New(reader, &buf)
	defer s.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	ch, err := s.Receive(ctx)
	if err != nil {
		t.Fatalf("Receive: %v", err)
	}

	var msg json.RawMessage
	select {
	case m, ok := <-ch:
		if !ok {
			t.Fatal("channel closed unexpectedly")
		}
		msg = m
	case <-ctx.Done():
		t.Fatalf("timeout: %v", ctx.Err())
	}

	var parsed map[string]string
	if err := json.Unmarshal(msg, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if parsed["type"] != "hello" {
		t.Errorf("type = %q, want hello", parsed["type"])
	}

	if err := s.Send(ctx, json.RawMessage(`{"type":"reply"}`)); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if !strings.Contains(buf.String(), `{"type":"reply"}`) {
		t.Errorf("output = %q, missing reply", buf.String())
	}
}

func TestStream_ReceiveCancelled(t *testing.T) {
	pr, _ := makeBlockingPipe()
	defer pr.Close()

	var buf bytes.Buffer
	s := New(pr, &buf)
	defer s.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	ch, err := s.Receive(ctx)
	if err != nil {
		t.Fatalf("Receive returned error immediately: %v", err)
	}

	select {
	case _, ok := <-ch:
		if ok {
			t.Error("expected channel to close on cancellation, got message")
		}
		// channel closed — correct
	case <-time.After(500 * time.Millisecond):
		t.Error("channel did not close after context cancellation")
	}
}

// makeBlockingPipe returns a reader that never produces data (write end kept open).
func makeBlockingPipe() (*blockReader, func()) {
	ch := make(chan struct{})
	return &blockReader{done: ch}, func() { close(ch) }
}

type blockReader struct{ done chan struct{} }

func (b *blockReader) Read(p []byte) (int, error) {
	<-b.done
	return 0, nil
}
func (b *blockReader) Close() error { return nil }
