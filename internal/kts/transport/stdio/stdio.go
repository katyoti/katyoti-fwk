// SPDX-License-Identifier: Apache-2.0

// Package stdio provides a Transport over line-delimited JSON (NDJSON) on an
// io.Reader/io.Writer pair (typically stdin/stdout pipes).
// The read loop is internal — no Run() call required; just Send/Receive/Close.
package stdio

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"sync"

	"github.com/katyoti/katyoti-fwk/internal/kts/transport/api"
)

// Stream implements api.Transport over line-delimited JSON.
// An internal goroutine scans the reader line-by-line and feeds a buffered channel.
// Receive() returns a proxy channel scoped to the caller's context.
//
// The read goroutine starts immediately in New; call Close() when done.
type Stream struct {
	out     io.Writer
	recv    chan json.RawMessage
	writeMu sync.Mutex
	done    chan struct{}
	once    sync.Once
}

// New creates a stdio transport from a reader/writer pair.
// The read goroutine starts immediately.
func New(in io.Reader, out io.Writer) *Stream {
	s := &Stream{
		out:  out,
		recv: make(chan json.RawMessage, 64),
		done: make(chan struct{}),
	}
	go s.readLoop(in)
	return s
}

// Send writes msg as a newline-terminated JSON line.
// Thread-safe — concurrent sends are serialized.
func (s *Stream) Send(_ context.Context, msg json.RawMessage) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	data := append(msg, '\n')
	_, err := s.out.Write(data)
	return err
}

// Receive returns a channel that delivers incoming messages.
// The channel is closed when the transport closes or the context is cancelled.
func (s *Stream) Receive(ctx context.Context) (<-chan json.RawMessage, error) {
	select {
	case <-s.done:
		return nil, api.ErrClosed
	default:
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	out := make(chan json.RawMessage)
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

// Close shuts down the stream.
func (s *Stream) Close() error {
	s.once.Do(func() { close(s.done) })
	return nil
}

func (s *Stream) readLoop(in io.Reader) {
	defer close(s.recv) // signal Receive that no more messages will arrive
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := make([]byte, len(scanner.Bytes()))
		copy(line, scanner.Bytes())
		select {
		case s.recv <- json.RawMessage(line):
		case <-s.done:
			return
		}
	}
}
