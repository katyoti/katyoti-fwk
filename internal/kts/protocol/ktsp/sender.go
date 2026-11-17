// SPDX-License-Identifier: Apache-2.0

package ktsp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"sync/atomic"
	"time"

	ktsapi "github.com/katyoti/katyoti-fwk/internal/kts/api"
)

// Sender is re-exported from kts/api, the consumer half of KTSP: the contract a
// caller needs in order to send a request, and nothing else.
//
// The alias points DOWNWARD at the lighter package. Consumer-side code should
// depend on kts/api directly; code that already has the protocol in scope keeps
// using ktsp.Sender unchanged.
type Sender = ktsapi.Sender

// RetryPolicy configures retry behavior for a RetrySender.
type RetryPolicy struct {
	// MaxRetries is the maximum number of retry attempts (0 = no retries).
	MaxRetries int

	// BaseDelay is the initial delay before the first retry.
	BaseDelay time.Duration

	// MaxDelay caps the exponential backoff.
	MaxDelay time.Duration

	// JitterFactor controls the random jitter range [0, 1].
	// 0 = no jitter, 1 = up to 100% of computed delay added as jitter.
	JitterFactor float64
}

// DefaultRetryPolicy returns a sensible default retry policy.
func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{
		MaxRetries:   3,
		BaseDelay:    100 * time.Millisecond,
		MaxDelay:     5 * time.Second,
		JitterFactor: 0.5,
	}
}

// RetrySender implements Sender with automatic retry on transient errors.
// It wraps a Transport and adds request ID generation, error classification,
// and exponential backoff with random jitter.
type RetrySender struct {
	transport Transport
	policy    RetryPolicy
	nextID    atomic.Int64
}

// NewRetrySender creates a Sender that wraps the given transport with retry logic.
func NewRetrySender(transport Transport, policy RetryPolicy) *RetrySender {
	return &RetrySender{
		transport: transport,
		policy:    policy,
	}
}

// NewSender creates a Sender with the default retry policy.
func NewSender(transport Transport) *RetrySender {
	return NewRetrySender(transport, DefaultRetryPolicy())
}

// Call sends a KTSP request and returns the result (the request/response RPC).
// Retries on transient errors with exponential backoff and jitter. Permanent
// errors (KTSP protocol errors) are returned immediately. opts override the
// per-call timeout / retries / backoff.
func (s *RetrySender) Call(ctx context.Context, path string, params any, opts ...CallOption) (json.RawMessage, error) {
	o := ApplyCallOptions(opts...)
	maxRetries, baseDelay := s.effective(o)
	if o.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, o.Timeout)
		defer cancel()
	}
	var lastErr error
	// One id for the call and all its retries: a retry is the same request
	// sent again, which the serving side may recognise.
	id := s.nextID.Add(1)

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			delay := s.computeDelay(attempt, baseDelay)
			select {
			case <-ctx.Done():
				return nil, fmt.Errorf("call %s cancelled after %d attempts: %w", path, attempt, ctx.Err())
			case <-time.After(delay):
			}
		}

		req, err := NewRequest(id, path, params)
		if err != nil {
			return nil, fmt.Errorf("build request %s: %w", path, err)
		}

		resp, err := s.transport.Send(ctx, req)
		if err != nil {
			if !isRetriable(err) {
				return nil, err
			}
			lastErr = err
			continue
		}

		if resp.Error != nil {
			if !isRetriableRPC(resp.Error) {
				return nil, resp.Error
			}
			lastErr = resp.Error
			continue
		}

		return resp.Result, nil
	}

	return nil, fmt.Errorf("call %s failed after %d attempts: %w", path, maxRetries+1, lastErr)
}

// Notify sends a KTSP notification with retry on transient errors. opts override
// the per-call timeout / retries / backoff.
func (s *RetrySender) Notify(ctx context.Context, path string, params any, opts ...CallOption) error {
	o := ApplyCallOptions(opts...)
	maxRetries, baseDelay := s.effective(o)
	if o.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, o.Timeout)
		defer cancel()
	}
	var lastErr error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			delay := s.computeDelay(attempt, baseDelay)
			select {
			case <-ctx.Done():
				return fmt.Errorf("notify %s cancelled after %d attempts: %w", path, attempt, ctx.Err())
			case <-time.After(delay):
			}
		}

		notif, err := NewNotification(path, params)
		if err != nil {
			return fmt.Errorf("build notification %s: %w", path, err)
		}

		err = s.transport.Notify(ctx, notif)
		if err != nil {
			if !isRetriable(err) {
				return err
			}
			lastErr = err
			continue
		}

		return nil
	}

	return fmt.Errorf("notify %s failed after %d attempts: %w", path, maxRetries+1, lastErr)
}

// effective resolves the per-call retry budget + backoff base from CallOptions,
// falling back to the sender's constructed policy when a field is unset.
func (s *RetrySender) effective(o CallOptions) (maxRetries int, baseDelay time.Duration) {
	maxRetries = s.policy.MaxRetries
	if o.Retries >= 0 {
		maxRetries = o.Retries
	}
	baseDelay = s.policy.BaseDelay
	if o.BaseDelay > 0 {
		baseDelay = o.BaseDelay
	}
	return maxRetries, baseDelay
}

func (s *RetrySender) computeDelay(attempt int, baseDelay time.Duration) time.Duration {
	backoff := float64(baseDelay) * math.Pow(2, float64(attempt-1))
	if backoff > float64(s.policy.MaxDelay) {
		backoff = float64(s.policy.MaxDelay)
	}
	if s.policy.JitterFactor > 0 {
		jitter := backoff * s.policy.JitterFactor * rand.Float64()
		backoff += jitter
	}
	return time.Duration(backoff)
}

func isRetriable(err error) bool {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	return true
}

func isRetriableRPC(err *RPCError) bool {
	switch err.Code {
	case CodeInternalError, CodeContainerBusy:
		return true
	default:
		return false
	}
}
