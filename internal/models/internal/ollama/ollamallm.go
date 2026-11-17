// SPDX-License-Identifier: Apache-2.0

package ollama

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/models/api"
	"github.com/katyoti/katyoti-fwk/internal/models/internal/ollama/internal/client"
)

var (
	ErrEmptyResponse       = errors.New("no response")
	ErrIncompleteEmbedding = errors.New("not all input got embedded")
	ErrPullError           = errors.New("ollama model pull error")
	ErrPullTimeout         = errors.New("ollama model pull deadline exceeded")
)

// LLM is an Ollama LLM implementation.
type LLM struct {
	client  *client.Client
	options options
}

// New creates a new Ollama LLM implementation.
func New(opts ...Option) (*LLM, error) {
	o := options{}
	for _, opt := range opts {
		opt(&o)
	}

	c, err := client.NewClient(o.ollamaServerURL, o.httpClient)
	if err != nil {
		return nil, err
	}

	return &LLM{client: c, options: o}, nil
}

// GenerateContent implements the Model interface (non-streaming).
func (o *LLM) GenerateContent(ctx context.Context, messages []api.MessageContent, options ...api.CallOption) (*api.ContentResponse, error) {
	req, ollamaOpts, err := o.buildRequest(messages, options)
	if err != nil {
		return nil, err
	}
	req.Stream = false

	var resp client.ChatResponse
	fn := func(response client.ChatResponse) error {
		resp = response
		return nil
	}

	if err := o.client.GenerateChat(ctx, req, fn); err != nil {
		return nil, err
	}

	return o.buildResponse(resp, ollamaOpts), nil
}

// GenerateContentStream implements the StreamingModel interface.
func (o *LLM) GenerateContentStream(ctx context.Context, messages []api.MessageContent, options ...api.CallOption) (api.Stream, error) {
	req, ollamaOpts, err := o.buildRequest(messages, options)
	if err != nil {
		return nil, err
	}
	req.Stream = true

	sender, stream := api.NewChannelStream(64)

	go o.consumeStream(ctx, req, ollamaOpts, sender)

	return stream, nil
}

// buildRequest converts messages + options into an Ollama ChatRequest.
func (o *LLM) buildRequest(messages []api.MessageContent, options []api.CallOption) (*client.ChatRequest, client.Options, error) {
	opts := api.CallOptions{}
	for _, opt := range options {
		opt(&opts)
	}

	model := o.options.model
	if opts.Model != "" {
		model = opts.Model
	}

	if o.options.pullModel {
		// pullModelIfNeeded would go here — kept as no-op for now
	}

	chatMsgs := make([]*client.Message, 0, len(messages))
	for _, mc := range messages {
		msg := &client.Message{Role: typeToRole(mc.Role)}

		var text string
		foundText := false
		var images []client.ImageData

		for _, p := range mc.Parts {
			switch pt := p.(type) {
			case api.TextContent:
				if foundText {
					return nil, client.Options{}, errors.New("expecting a single Text content")
				}
				foundText = true
				text = pt.Text
			case api.BinaryContent:
				images = append(images, client.ImageData(pt.Data))
			case api.ToolCall:
				// Skip tool calls in message conversion
			case api.ToolCallResponse:
				// Skip tool responses in message conversion
			default:
				// Skip unknown parts
			}
		}

		msg.Content = text
		msg.Images = images
		chatMsgs = append(chatMsgs, msg)
	}

	format := o.options.format
	if opts.JSONMode {
		format = "json"
	}

	ollamaOpts := makeOllamaOptionsFromOptions(o.options.ollamaOptions, opts)

	req := &client.ChatRequest{
		Model:    model,
		Format:   format,
		Messages: chatMsgs,
		Options:  ollamaOpts,
	}

	if o.options.keepAlive != "" {
		req.KeepAlive = o.options.keepAlive
	}

	return req, ollamaOpts, nil
}

// consumeStream runs the Ollama streaming call in a goroutine and pushes StreamEvents.
func (o *LLM) consumeStream(ctx context.Context, req *client.ChatRequest, ollamaOpts client.Options, sender *api.ChannelStreamSender) {
	defer sender.Close()

	var fullText strings.Builder
	var thinkingText strings.Builder
	var resp client.ChatResponse
	inThinking := false

	fn := func(response client.ChatResponse) error {
		if response.Message != nil {
			chunk := response.Message.Content

			// For reasoning models with Think enabled, parse <think>...</think> tags
			if ollamaOpts.Think && chunk != "" {
				remaining := chunk
				for len(remaining) > 0 {
					if !inThinking {
						// Look for <think> tag
						idx := strings.Index(remaining, "<think>")
						if idx >= 0 {
							// Text before the tag
							if idx > 0 {
								textBefore := remaining[:idx]
								fullText.WriteString(textBefore)
								sender.Send(api.StreamEvent{Type: api.StreamEventTextDelta, Delta: textBefore})
							}
							inThinking = true
							remaining = remaining[idx+7:] // skip "<think>"
						} else {
							// No tag — all text
							fullText.WriteString(remaining)
							sender.Send(api.StreamEvent{Type: api.StreamEventTextDelta, Delta: remaining})
							remaining = ""
						}
					} else {
						// Inside thinking — look for </think>
						idx := strings.Index(remaining, "</think>")
						if idx >= 0 {
							thinking := remaining[:idx]
							if thinking != "" {
								thinkingText.WriteString(thinking)
								sender.Send(api.StreamEvent{Type: api.StreamEventThinkingDelta, Delta: thinking})
							}
							inThinking = false
							remaining = remaining[idx+8:] // skip "</think>"
						} else {
							// Still thinking
							thinkingText.WriteString(remaining)
							sender.Send(api.StreamEvent{Type: api.StreamEventThinkingDelta, Delta: remaining})
							remaining = ""
						}
					}
				}
			} else if chunk != "" {
				// Non-reasoning model — simple text delta
				fullText.WriteString(chunk)
				sender.Send(api.StreamEvent{Type: api.StreamEventTextDelta, Delta: chunk})
			}
		}

		if response.Done {
			resp = response
			resp.Message = &client.Message{
				Role:    "assistant",
				Content: fullText.String(),
			}
		}
		return nil
	}

	if err := o.client.GenerateChat(ctx, req, fn); err != nil {
		sender.Send(api.StreamEvent{Type: api.StreamEventError, Error: err})
		return
	}

	// Emit usage
	sender.Send(api.StreamEvent{
		Type:         api.StreamEventUsage,
		InputTokens:  resp.PromptEvalCount,
		OutputTokens: resp.EvalCount,
	})

	// Emit done
	sender.Send(api.StreamEvent{Type: api.StreamEventDone})

	// Assemble final response
	genInfo := map[string]any{
		"CompletionTokens": resp.EvalCount,
		"PromptTokens":     resp.PromptEvalCount,
		"TotalTokens":      resp.EvalCount + resp.PromptEvalCount,
	}
	if thinkingText.Len() > 0 {
		genInfo["ThinkingContent"] = thinkingText.String()
	}

	content := fullText.String()
	sender.SetResponse(&api.ContentResponse{
		Choices: []*api.ContentChoice{{
			Content:        content,
			GenerationInfo: genInfo,
		}},
	})
}

// buildResponse creates a ContentResponse from a non-streaming Ollama response.
func (o *LLM) buildResponse(resp client.ChatResponse, ollamaOpts client.Options) *api.ContentResponse {
	content := ""
	if resp.Message != nil {
		content = resp.Message.Content
	}

	genInfo := map[string]any{
		"CompletionTokens": resp.EvalCount,
		"PromptTokens":     resp.PromptEvalCount,
		"TotalTokens":      resp.EvalCount + resp.PromptEvalCount,
	}

	if ollamaOpts.Think {
		genInfo["ThinkingEnabled"] = true
	}

	return &api.ContentResponse{
		Choices: []*api.ContentChoice{{
			Content:        content,
			GenerationInfo: genInfo,
		}},
	}
}

func (o *LLM) CreateEmbedding(ctx context.Context, inputTexts []string) ([][]float32, error) {
	if o.options.pullModel {
		if err := o.pullModelIfNeeded(ctx, o.options.model); err != nil {
			return nil, err
		}
	}

	embeddings := [][]float32{}
	for _, input := range inputTexts {
		req := &client.EmbeddingRequest{
			Input: input,
			Model: o.options.model,
		}
		if o.options.keepAlive != "" {
			req.KeepAlive = o.options.keepAlive
		}

		embedding, err := o.client.CreateEmbedding(ctx, req)
		if err != nil {
			return nil, err
		}
		if len(embedding.Embeddings) == 0 {
			return nil, ErrEmptyResponse
		}
		embeddings = append(embeddings, embedding.Embeddings...)
	}

	if len(inputTexts) != len(embeddings) {
		return embeddings, ErrIncompleteEmbedding
	}
	return embeddings, nil
}

func typeToRole(typ api.ChatMessageType) string {
	switch typ {
	case api.ChatMessageTypeSystem:
		return "system"
	case api.ChatMessageTypeAI:
		return "assistant"
	case api.ChatMessageTypeHuman, api.ChatMessageTypeGeneric:
		return "user"
	case api.ChatMessageTypeFunction:
		return "function"
	case api.ChatMessageTypeTool:
		return "tool"
	}
	return ""
}

func makeOllamaOptionsFromOptions(ollamaOptions client.Options, opts api.CallOptions) client.Options {
	ollamaOptions.NumPredict = opts.MaxTokens
	ollamaOptions.Temperature = float32(opts.Temperature)
	ollamaOptions.Stop = opts.StopWords
	ollamaOptions.TopK = opts.TopK
	ollamaOptions.TopP = float32(opts.TopP)
	ollamaOptions.Seed = opts.Seed
	ollamaOptions.RepeatPenalty = float32(opts.RepetitionPenalty)
	ollamaOptions.FrequencyPenalty = float32(opts.FrequencyPenalty)
	ollamaOptions.PresencePenalty = float32(opts.PresencePenalty)

	if opts.Metadata != nil {
		if config, ok := opts.Metadata["thinking_config"].(*api.ThinkingConfig); ok {
			if config.Mode != api.ThinkingModeNone {
				ollamaOptions.Think = true
			}
		}
	}

	return ollamaOptions
}

func (o *LLM) pullModelIfNeeded(ctx context.Context, model string) error {
	pullCtx := ctx
	if o.options.pullTimeout > 0 {
		var cancel context.CancelFunc
		pullCtx, cancel = context.WithTimeoutCause(ctx, o.options.pullTimeout, ErrPullTimeout)
		defer cancel()
	}

	req := &client.PullRequest{
		Model:  model,
		Stream: false,
	}

	err := o.client.Pull(pullCtx, req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		if cause := context.Cause(pullCtx); cause != nil {
			return fmt.Errorf("%w: %w", cause, err)
		}
	}
	return err
}
