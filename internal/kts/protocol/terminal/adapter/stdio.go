// SPDX-License-Identifier: Apache-2.0

package adapter

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	genstdio "github.com/katyoti/katyoti-fwk/internal/kts/transport/stdio"
)

// Stdio implements terminal.TerminalTransport over line-delimited JSON
// (NDJSON) on stdin/stdout. Used for external TUI clients piped to the terminal
// server, or for testing with io.Pipe().
//
// Backed by stdio.Stream — the read goroutine is internal; just Send/Receive/Close.
type Stdio struct {
	stream *genstdio.Stream
	out    io.WriteCloser // kept to allow Close() to unblock the scanner
}

// NewStdio wraps a reader/writer pair as a terminal transport.
// out must be an io.WriteCloser; Close() closes it to unblock any pending read.
func NewStdio(in io.Reader, out io.WriteCloser) *Stdio {
	return &Stdio{
		stream: genstdio.New(in, out),
		out:    out,
	}
}

// Send marshals event as newline-delimited JSON and writes it to the writer.
// Thread-safe — concurrent sends are serialized by the underlying stream.
func (a *Stdio) Send(ctx context.Context, event any) error {
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("terminal/stdio: marshal event: %w", err)
	}
	return a.stream.Send(ctx, data)
}

// Receive returns a channel of raw JSON messages from stdin.
// The channel closes when the adapter is closed or the context is cancelled.
func (a *Stdio) Receive(ctx context.Context) (<-chan json.RawMessage, error) {
	return a.stream.Receive(ctx)
}

// Close shuts down the adapter. Closes the underlying writer to unblock the
// scanner goroutine if it is blocked on a read.
func (a *Stdio) Close() error {
	a.out.Close()
	return a.stream.Close()
}
