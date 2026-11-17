// SPDX-License-Identifier: Apache-2.0

// Package pipe provides an in-process Transport[T] pair over Go channels.
// Use New to create a connected pair — messages sent by one side are received
// by the other, and vice versa. The zero allocation cost makes it ideal for
// embedded mode and tests.
package pipe

import (
	"context"
	"sync"

	"github.com/katyoti/katyoti-fwk/internal/kts/transport/api"
)

// Stream implements api.Transport[T] over Go channels.
// Both sides share a single close signal — closing either end unblocks Receive
// on both sides.
type Stream[T any] struct {
	send chan T
	recv chan T
	done chan struct{}
	once *sync.Once // shared between both endpoints
}

// New creates a connected pair of Stream endpoints.
// Messages sent by a are received by b, and vice versa.
// bufSize controls the channel buffer depth (0 = unbuffered).
func New[T any](bufSize int) (*Stream[T], *Stream[T]) {
	aTob := make(chan T, bufSize)
	bToa := make(chan T, bufSize)
	done := make(chan struct{})
	once := &sync.Once{}
	a := &Stream[T]{send: aTob, recv: bToa, done: done, once: once}
	b := &Stream[T]{send: bToa, recv: aTob, done: done, once: once}
	return a, b
}

// Send transmits obj to the peer. Blocks if the buffer is full.
// Returns ErrClosed if the transport is closed; the closed state takes priority
// over a ready send channel to prevent the non-deterministic select race.
func (s *Stream[T]) Send(ctx context.Context, obj T) error {
	select {
	case <-s.done:
		return api.ErrClosed
	default:
	}
	select {
	case s.send <- obj:
		return nil
	case <-s.done:
		return api.ErrClosed
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Receive returns a channel that delivers incoming messages.
// The channel is closed when the transport closes or the context is cancelled.
func (s *Stream[T]) Receive(ctx context.Context) (<-chan T, error) {
	select {
	case <-s.done:
		return nil, api.ErrClosed
	default:
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	out := make(chan T)
	go func() {
		defer close(out)
		for {
			select {
			case msg, ok := <-s.recv:
				if !ok {
					return
				}
				select {
				case out <- msg:
				case <-ctx.Done():
					return
				case <-s.done:
					return
				}
			case <-ctx.Done():
				return
			case <-s.done:
				return
			}
		}
	}()
	return out, nil
}

// Close shuts down both sides of the pair. Safe to call multiple times.
func (s *Stream[T]) Close() error {
	s.once.Do(func() { close(s.done) })
	return nil
}
