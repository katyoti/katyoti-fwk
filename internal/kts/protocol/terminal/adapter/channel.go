// SPDX-License-Identifier: Apache-2.0

package adapter

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/katyoti/katyoti-fwk/internal/kts/transport/pipe"
)

// Channel implements terminal.TerminalTransport over Go channels (in-process).
// Used for embedded TUI mode where producer and consumer run in the same binary.
type Channel struct {
	stream *pipe.Stream[json.RawMessage]
}

// NewChannelPair creates a bidirectional in-process terminal adapter pair.
// bufSize controls the channel buffer depth.
func NewChannelPair(bufSize int) (*Channel, *Channel) {
	a, b := pipe.New[json.RawMessage](bufSize)
	return &Channel{stream: a}, &Channel{stream: b}
}

func (a *Channel) Send(_ context.Context, event any) error {
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("terminal/channel: marshal event: %w", err)
	}
	return a.stream.Send(context.Background(), data)
}

func (a *Channel) Receive(ctx context.Context) (<-chan json.RawMessage, error) {
	return a.stream.Receive(ctx)
}

func (a *Channel) Close() error {
	return a.stream.Close()
}
