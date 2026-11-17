// SPDX-License-Identifier: Apache-2.0

package ktsp

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"testing"
	"time"
)

// mockTransport is a test transport that records calls and returns configurable responses.
type mockTransport struct {
	sendCalls   atomic.Int32
	notifyCalls atomic.Int32
	sendFn      func(ctx context.Context, req *Request) (*Response, error)
	notifyFn    func(ctx context.Context, notif *Notification) error
}

func (t *mockTransport) Send(ctx context.Context, req *Request) (*Response, error) {
	t.sendCalls.Add(1)
	if t.sendFn != nil {
		return t.sendFn(ctx, req)
	}
	return &Response{ID: req.ID, Result: json.RawMessage(`{"ok":true}`)}, nil
}

func (t *mockTransport) Notify(ctx context.Context, notif *Notification) error {
	t.notifyCalls.Add(1)
	if t.notifyFn != nil {
		return t.notifyFn(ctx, notif)
	}
	return nil
}

func (t *mockTransport) Close() error { return nil }

func TestRetrySender_SendSuccess(t *testing.T) {
	mt := &mockTransport{}
	sender := NewRetrySender(mt, DefaultRetryPolicy())

	result, err := sender.Call(context.Background(), "test/echo", map[string]string{"msg": "hello"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed map[string]bool
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}
	if !parsed["ok"] {
		t.Errorf("expected ok=true, got %v", parsed)
	}
	if mt.sendCalls.Load() != 1 {
		t.Errorf("expected 1 send call, got %d", mt.sendCalls.Load())
	}
}

func TestRetrySender_SendRetryOnTransientError(t *testing.T) {
	mt := &mockTransport{
		sendFn: func(ctx context.Context, req *Request) (*Response, error) {
			return nil, fmt.Errorf("connection refused")
		},
	}

	policy := RetryPolicy{
		MaxRetries:   2,
		BaseDelay:    1 * time.Millisecond,
		MaxDelay:     10 * time.Millisecond,
		JitterFactor: 0,
	}
	sender := NewRetrySender(mt, policy)

	_, err := sender.Call(context.Background(), "test/echo", nil)
	if err == nil {
		t.Fatal("expected error after retries exhausted")
	}

	// Should have tried 1 initial + 2 retries = 3 total
	if mt.sendCalls.Load() != 3 {
		t.Errorf("expected 3 send calls, got %d", mt.sendCalls.Load())
	}
}

func TestRetrySender_SendNoRetryOnPermanentRPCError(t *testing.T) {
	mt := &mockTransport{
		sendFn: func(ctx context.Context, req *Request) (*Response, error) {
			return NewErrorResponse(req.ID, ErrSlotNotFound("missing")), nil
		},
	}

	policy := RetryPolicy{
		MaxRetries:   3,
		BaseDelay:    1 * time.Millisecond,
		MaxDelay:     10 * time.Millisecond,
		JitterFactor: 0,
	}
	sender := NewRetrySender(mt, policy)

	_, err := sender.Call(context.Background(), "test/echo", nil)
	if err == nil {
		t.Fatal("expected error for slot not found")
	}

	rpcErr, ok := err.(*RPCError)
	if !ok {
		t.Fatalf("expected RPCError, got %T: %v", err, err)
	}
	if rpcErr.Code != CodeSlotNotFound {
		t.Errorf("expected code %d, got %d", CodeSlotNotFound, rpcErr.Code)
	}

	// Should have tried exactly once — no retries for permanent errors
	if mt.sendCalls.Load() != 1 {
		t.Errorf("expected 1 send call, got %d", mt.sendCalls.Load())
	}
}

func TestRetrySender_SendRetryOnContainerBusy(t *testing.T) {
	var calls atomic.Int32
	mt := &mockTransport{
		sendFn: func(ctx context.Context, req *Request) (*Response, error) {
			n := calls.Add(1)
			if n < 3 {
				return NewErrorResponse(req.ID, ErrContainerBusy()), nil
			}
			return &Response{ID: req.ID, Result: json.RawMessage(`"done"`)}, nil
		},
	}

	policy := RetryPolicy{
		MaxRetries:   3,
		BaseDelay:    1 * time.Millisecond,
		MaxDelay:     10 * time.Millisecond,
		JitterFactor: 0,
	}
	sender := NewRetrySender(mt, policy)

	result, err := sender.Call(context.Background(), "test/echo", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if string(result) != `"done"` {
		t.Errorf("expected \"done\", got %s", result)
	}

	// 2 busy + 1 success = 3
	if mt.sendCalls.Load() != 3 {
		t.Errorf("expected 3 send calls, got %d", mt.sendCalls.Load())
	}
}

func TestRetrySender_SendContextCancellation(t *testing.T) {
	mt := &mockTransport{
		sendFn: func(ctx context.Context, req *Request) (*Response, error) {
			return nil, fmt.Errorf("connection refused")
		},
	}

	policy := RetryPolicy{
		MaxRetries:   10,
		BaseDelay:    100 * time.Millisecond,
		MaxDelay:     1 * time.Second,
		JitterFactor: 0,
	}
	sender := NewRetrySender(mt, policy)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := sender.Call(ctx, "test/echo", nil)
	if err == nil {
		t.Fatal("expected error from context cancellation")
	}

	// Should have tried at most 2 times (initial + maybe 1 retry before timeout)
	if mt.sendCalls.Load() > 2 {
		t.Errorf("expected at most 2 send calls with short timeout, got %d", mt.sendCalls.Load())
	}
}

func TestRetrySender_SendNoRetryOnContextCancelError(t *testing.T) {
	mt := &mockTransport{
		sendFn: func(ctx context.Context, req *Request) (*Response, error) {
			return nil, context.Canceled
		},
	}

	policy := RetryPolicy{
		MaxRetries:   3,
		BaseDelay:    1 * time.Millisecond,
		MaxDelay:     10 * time.Millisecond,
		JitterFactor: 0,
	}
	sender := NewRetrySender(mt, policy)

	_, err := sender.Call(context.Background(), "test/echo", nil)
	if err == nil {
		t.Fatal("expected error from context.Canceled")
	}

	// context.Canceled is not retriable — should try exactly once
	if mt.sendCalls.Load() != 1 {
		t.Errorf("expected 1 send call, got %d", mt.sendCalls.Load())
	}
}

func TestRetrySender_NotifySuccess(t *testing.T) {
	mt := &mockTransport{}
	sender := NewSender(mt)

	err := sender.Notify(context.Background(), "journal/emit", map[string]string{"type": "test"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mt.notifyCalls.Load() != 1 {
		t.Errorf("expected 1 notify call, got %d", mt.notifyCalls.Load())
	}
}

func TestRetrySender_NotifyRetryOnTransientError(t *testing.T) {
	var calls atomic.Int32
	mt := &mockTransport{
		notifyFn: func(ctx context.Context, notif *Notification) error {
			n := calls.Add(1)
			if n < 3 {
				return fmt.Errorf("transport error")
			}
			return nil
		},
	}

	policy := RetryPolicy{
		MaxRetries:   3,
		BaseDelay:    1 * time.Millisecond,
		MaxDelay:     10 * time.Millisecond,
		JitterFactor: 0,
	}
	sender := NewRetrySender(mt, policy)

	err := sender.Notify(context.Background(), "journal/emit", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mt.notifyCalls.Load() != 3 {
		t.Errorf("expected 3 notify calls, got %d", mt.notifyCalls.Load())
	}
}

func TestRetrySender_ExponentialBackoff(t *testing.T) {
	mt := &mockTransport{
		sendFn: func(ctx context.Context, req *Request) (*Response, error) {
			return nil, fmt.Errorf("unavailable")
		},
	}

	policy := RetryPolicy{
		MaxRetries:   3,
		BaseDelay:    10 * time.Millisecond,
		MaxDelay:     1 * time.Second,
		JitterFactor: 0, // no jitter for predictable timing
	}
	sender := NewRetrySender(mt, policy)

	start := time.Now()
	_, _ = sender.Call(context.Background(), "test/echo", nil)
	elapsed := time.Since(start)

	// Expected delays: 10ms + 20ms + 40ms = 70ms minimum
	if elapsed < 60*time.Millisecond {
		t.Errorf("expected at least 60ms total delay, got %v", elapsed)
	}
}

func TestRetrySender_MaxDelayCap(t *testing.T) {
	sender := &RetrySender{
		policy: RetryPolicy{
			BaseDelay:    100 * time.Millisecond,
			MaxDelay:     200 * time.Millisecond,
			JitterFactor: 0,
		},
	}

	// Attempt 10: 100ms * 2^9 = 51200ms, should be capped at 200ms
	delay := sender.computeDelay(10, sender.policy.BaseDelay)
	if delay > 200*time.Millisecond {
		t.Errorf("expected delay capped at 200ms, got %v", delay)
	}
}

func TestRetrySender_JitterAddsRandomness(t *testing.T) {
	sender := &RetrySender{
		policy: RetryPolicy{
			BaseDelay:    100 * time.Millisecond,
			MaxDelay:     10 * time.Second,
			JitterFactor: 1.0,
		},
	}

	// Generate multiple delays and check they vary
	delays := make(map[time.Duration]bool)
	for i := 0; i < 20; i++ {
		d := sender.computeDelay(1, sender.policy.BaseDelay)
		delays[d] = true
		// With jitterFactor=1.0: delay should be in [100ms, 200ms]
		if d < 100*time.Millisecond || d > 200*time.Millisecond {
			t.Errorf("delay %v outside expected range [100ms, 200ms]", d)
		}
	}

	// With randomness, we should get more than 1 unique value
	if len(delays) < 2 {
		t.Errorf("expected jitter to produce varied delays, got %d unique values", len(delays))
	}
}

func TestRetrySender_IDAutoIncrement(t *testing.T) {
	var ids []any
	mt := &mockTransport{
		sendFn: func(ctx context.Context, req *Request) (*Response, error) {
			ids = append(ids, req.ID)
			return &Response{ID: req.ID, Result: json.RawMessage(`null`)}, nil
		},
	}

	sender := NewSender(mt)

	_, _ = sender.Call(context.Background(), "a/m1", nil)
	_, _ = sender.Call(context.Background(), "b/m2", nil)
	_, _ = sender.Call(context.Background(), "c/m3", nil)

	if len(ids) != 3 {
		t.Fatalf("expected 3 calls, got %d", len(ids))
	}

	// IDs should be monotonically increasing
	for i := 1; i < len(ids); i++ {
		prev := ids[i-1].(int64)
		curr := ids[i].(int64)
		if curr <= prev {
			t.Errorf("IDs not monotonically increasing: %d, %d", prev, curr)
		}
	}
}

func TestRetrySender_SendRetryOnInternalError(t *testing.T) {
	var calls atomic.Int32
	mt := &mockTransport{
		sendFn: func(ctx context.Context, req *Request) (*Response, error) {
			n := calls.Add(1)
			if n < 2 {
				return NewErrorResponse(req.ID, ErrInternalError("temporary")), nil
			}
			return &Response{ID: req.ID, Result: json.RawMessage(`"ok"`)}, nil
		},
	}

	policy := RetryPolicy{
		MaxRetries:   3,
		BaseDelay:    1 * time.Millisecond,
		MaxDelay:     10 * time.Millisecond,
		JitterFactor: 0,
	}
	sender := NewRetrySender(mt, policy)

	result, err := sender.Call(context.Background(), "test/method", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(result) != `"ok"` {
		t.Errorf("expected \"ok\", got %s", result)
	}
	if mt.sendCalls.Load() != 2 {
		t.Errorf("expected 2 send calls, got %d", mt.sendCalls.Load())
	}
}

func TestNewSender_DefaultPolicy(t *testing.T) {
	mt := &mockTransport{}
	sender := NewSender(mt)

	if sender.policy.MaxRetries != 3 {
		t.Errorf("expected MaxRetries=3, got %d", sender.policy.MaxRetries)
	}
	if sender.policy.BaseDelay != 100*time.Millisecond {
		t.Errorf("expected BaseDelay=100ms, got %v", sender.policy.BaseDelay)
	}
	if sender.policy.MaxDelay != 5*time.Second {
		t.Errorf("expected MaxDelay=5s, got %v", sender.policy.MaxDelay)
	}
	if sender.policy.JitterFactor != 0.5 {
		t.Errorf("expected JitterFactor=0.5, got %v", sender.policy.JitterFactor)
	}
}

// TestRetrySender_CallOptions verifies per-call overrides: WithRetries caps the
// retry loop for that one call, and WithTimeout bounds it via a ctx deadline.
func TestRetrySender_CallOptions(t *testing.T) {
	// Retries override — a transport that always fails with a retriable error.
	failing := &mockTransport{sendFn: func(context.Context, *Request) (*Response, error) {
		return nil, fmt.Errorf("transient")
	}}
	sender := NewRetrySender(failing, RetryPolicy{MaxRetries: 3, BaseDelay: time.Millisecond, MaxDelay: time.Millisecond})

	failing.sendCalls.Store(0)
	_, _ = sender.Call(context.Background(), "s/m", nil) // default policy → 4 attempts
	if n := failing.sendCalls.Load(); n != 4 {
		t.Fatalf("default policy: want 4 attempts, got %d", n)
	}
	failing.sendCalls.Store(0)
	_, _ = sender.Call(context.Background(), "s/m", nil, WithRetries(0)) // per-call → 1 attempt
	if n := failing.sendCalls.Load(); n != 1 {
		t.Fatalf("WithRetries(0): want 1 attempt, got %d", n)
	}

	// Timeout override — a transport that blocks until ctx is done.
	blocking := &mockTransport{sendFn: func(ctx context.Context, _ *Request) (*Response, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}}
	bs := NewRetrySender(blocking, RetryPolicy{MaxRetries: 0})
	start := time.Now()
	if _, err := bs.Call(context.Background(), "s/m", nil, WithTimeout(30*time.Millisecond)); err == nil {
		t.Fatal("WithTimeout: expected a timeout error")
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("WithTimeout should cap the call quickly; took %v", elapsed)
	}
}
