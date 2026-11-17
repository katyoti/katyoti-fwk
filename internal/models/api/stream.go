// SPDX-License-Identifier: Apache-2.0

package api

import (
	"sync"
)

// StreamEventType identifies the type of streaming event.
type StreamEventType string

const (
	// StreamEventTextDelta is a chunk of generated text.
	StreamEventTextDelta StreamEventType = "text_delta"

	// StreamEventThinkingDelta is a chunk of thinking/reasoning content.
	StreamEventThinkingDelta StreamEventType = "thinking_delta"

	// StreamEventToolUseStart signals the beginning of a tool call.
	StreamEventToolUseStart StreamEventType = "tool_use_start"

	// StreamEventToolUseDelta is a partial JSON input chunk for a tool call.
	StreamEventToolUseDelta StreamEventType = "tool_use_delta"

	// StreamEventToolUseDone signals a tool call is fully received.
	StreamEventToolUseDone StreamEventType = "tool_use_done"

	// StreamEventUsage reports token usage (may arrive mid-stream or at end).
	StreamEventUsage StreamEventType = "usage"

	// StreamEventError signals a streaming error.
	StreamEventError StreamEventType = "error"

	// StreamEventDone signals the stream is complete.
	StreamEventDone StreamEventType = "done"
)

// StreamEvent is a single event from a streaming LLM response.
type StreamEvent struct {
	// Type identifies what this event represents.
	Type StreamEventType `json:"type"`

	// Delta is the text chunk (for text_delta, thinking_delta).
	Delta string `json:"delta,omitempty"`

	// ToolCall fields (for tool_use_start, tool_use_delta, tool_use_done).
	ToolName  string `json:"tool_name,omitempty"`
	ToolID    string `json:"tool_id,omitempty"`
	ToolInput string `json:"tool_input,omitempty"` // partial JSON for delta, full JSON for done

	// StopReason is set on the done event.
	StopReason string `json:"stop_reason,omitempty"`

	// Usage is set on usage events.
	InputTokens              int `json:"input_tokens,omitempty"`
	OutputTokens             int `json:"output_tokens,omitempty"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens,omitempty"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens,omitempty"`

	// Error is set on error events.
	Error error `json:"-"`
}

// Stream is an iterator over streaming events from an LLM response.
// Usage:
//
//	stream, err := model.GenerateContentStream(ctx, msgs, opts...)
//	if err != nil { ... }
//	defer stream.Close()
//	for stream.Next() {
//	    ev := stream.Event()
//	    // handle ev
//	}
//	if err := stream.Err(); err != nil { ... }
//	resp := stream.Response() // final assembled ContentResponse
type Stream interface {
	// Next advances to the next event. Returns false when the stream is
	// exhausted or an error occurred. Must be called before Event().
	Next() bool

	// Event returns the current event. Only valid after Next() returns true.
	Event() StreamEvent

	// Err returns the first error encountered during streaming, or nil.
	Err() error

	// Close cancels the stream and releases resources. Safe to call multiple times.
	Close()

	// Response returns the final assembled ContentResponse.
	// Blocks until the stream is complete. Returns nil if the stream errored.
	Response() *ContentResponse
}

// --- ChannelStream: adapter from chan StreamEvent → Stream interface ---

// ChannelStreamSender is the provider-side handle for pushing events.
// Call Send() for each event, then call Close() when done.
type ChannelStreamSender struct {
	ch       chan<- StreamEvent
	response *ContentResponse
	mu       sync.Mutex
	closed   bool
}

// Send pushes an event to the stream. Safe to call from any goroutine.
// Returns false if the stream was closed (consumer called Close).
func (s *ChannelStreamSender) Send(ev StreamEvent) bool {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return false
	}
	s.mu.Unlock()

	// Non-blocking check for closed channel
	select {
	case s.ch <- ev:
		return true
	default:
		// Channel full or closed — try blocking send
		s.ch <- ev
		return true
	}
}

// SetResponse sets the final assembled response. Call before Close().
func (s *ChannelStreamSender) SetResponse(resp *ContentResponse) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.response = resp
}

// Close closes the channel. Must be called exactly once by the provider.
func (s *ChannelStreamSender) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.closed {
		s.closed = true
		close(s.ch)
	}
}

// channelStream is the consumer-side Stream implementation.
type channelStream struct {
	ch      <-chan StreamEvent
	sender  *ChannelStreamSender
	current StreamEvent
	err     error
	done    bool
}

func (s *channelStream) Next() bool {
	if s.done {
		return false
	}
	ev, ok := <-s.ch
	if !ok {
		s.done = true
		return false
	}
	if ev.Type == StreamEventError && ev.Error != nil {
		s.err = ev.Error
		s.done = true
		return false
	}
	s.current = ev
	return true
}

func (s *channelStream) Event() StreamEvent { return s.current }
func (s *channelStream) Err() error         { return s.err }

func (s *channelStream) Close() {
	s.done = true
	s.sender.Close()
	// Drain remaining events to unblock sender goroutine
	for range s.ch {
	}
}

func (s *channelStream) Response() *ContentResponse {
	// Drain stream if not already done
	for s.Next() {
	}
	s.sender.mu.Lock()
	defer s.sender.mu.Unlock()
	return s.sender.response
}

// NewChannelStream creates a paired sender/stream. The provider pushes events
// via the sender; the consumer reads via the Stream interface.
// Buffer size controls how many events can be buffered before the sender blocks.
func NewChannelStream(bufferSize int) (*ChannelStreamSender, Stream) {
	if bufferSize < 1 {
		bufferSize = 32
	}
	ch := make(chan StreamEvent, bufferSize)
	sender := &ChannelStreamSender{ch: ch}
	stream := &channelStream{ch: ch, sender: sender}
	return sender, stream
}
