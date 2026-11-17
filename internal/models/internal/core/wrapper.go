// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"

	"github.com/katyoti/katyoti-fwk/internal/models/api"
)

// contentGenerator is the minimal interface provider LLMs implement.
// The wrapper adds Info() on top to satisfy the full api.Model.
type contentGenerator interface {
	GenerateContent(ctx context.Context, messages []api.MessageContent, opts ...api.CallOption) (*api.ContentResponse, error)
}

// streamingContentGenerator is implemented by providers that support streaming.
type streamingContentGenerator interface {
	GenerateContentStream(ctx context.Context, messages []api.MessageContent, opts ...api.CallOption) (api.Stream, error)
}

// ModelWrapper wraps a provider LLM with ModelInfo and default options.
// This is the single model implementation for all providers.
type ModelWrapper struct {
	LLM      contentGenerator
	Info_    ModelInfo
	Defaults []api.CallOption
}

func (m *ModelWrapper) Info() ModelInfo { return m.Info_ }

func (m *ModelWrapper) GenerateContent(ctx context.Context, messages []api.MessageContent, opts ...api.CallOption) (*api.ContentResponse, error) {
	allOpts := append(m.Defaults, opts...)
	return m.LLM.GenerateContent(ctx, messages, allOpts...)
}

// GenerateContentStream starts a streaming response if the underlying LLM supports it.
// Falls back to a single-event stream wrapping GenerateContent if not.
func (m *ModelWrapper) GenerateContentStream(ctx context.Context, messages []api.MessageContent, opts ...api.CallOption) (api.Stream, error) {
	allOpts := append(m.Defaults, opts...)

	if sm, ok := m.LLM.(streamingContentGenerator); ok {
		return sm.GenerateContentStream(ctx, messages, allOpts...)
	}

	// Fallback: non-streaming → single-event stream
	resp, err := m.LLM.GenerateContent(ctx, messages, allOpts...)
	if err != nil {
		return nil, err
	}
	return newSingleResponseStream(resp), nil
}

func newSingleResponseStream(resp *api.ContentResponse) api.Stream {
	sender, stream := api.NewChannelStream(16)
	go func() {
		defer sender.Close()
		for _, choice := range resp.Choices {
			if choice.Content != "" {
				sender.Send(api.StreamEvent{Type: api.StreamEventTextDelta, Delta: choice.Content})
			}
			for _, tc := range choice.ToolCalls {
				sender.Send(api.StreamEvent{
					Type:      api.StreamEventToolUseDone,
					ToolID:    tc.ID,
					ToolName:  tc.FunctionCall.Name,
					ToolInput: tc.FunctionCall.Arguments,
				})
			}
		}
		sender.Send(api.StreamEvent{Type: api.StreamEventDone})
		sender.SetResponse(resp)
	}()
	return stream
}

var (
	_ api.Model          = (*ModelWrapper)(nil)
	_ api.StreamingModel = (*ModelWrapper)(nil)
)
