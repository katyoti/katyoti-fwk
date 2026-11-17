// SPDX-License-Identifier: Apache-2.0

package anthropic

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	anthropicsdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/anthropics/anthropic-sdk-go/packages/ssestream"

	"github.com/katyoti/katyoti-fwk/internal/models/api"
	"github.com/katyoti/katyoti-fwk/internal/models/internal/httputil"
)

var (
	ErrEmptyResponse          = errors.New("no response")
	ErrMissingToken           = errors.New("missing the Anthropic API key, set it in the ANTHROPIC_API_KEY environment variable")
	ErrInvalidContentType     = errors.New("invalid content type")
	ErrUnsupportedMessageType = errors.New("unsupported message type")
	ErrUnsupportedContentType = errors.New("unsupported content type")
)

// LLM implements the Anthropic provider using the official anthropic-sdk-go.
type LLM struct {
	client    *anthropicsdk.Client
	model     string
	reasoning bool // from config, not hardcoded model detection
}

// New creates a new Anthropic LLM client using the official SDK.
func New(apiKey, model string, reasoning bool, opts ...option.RequestOption) (*LLM, error) {
	if apiKey == "" {
		return nil, ErrMissingToken
	}

	allOpts := []option.RequestOption{option.WithAPIKey(apiKey)}
	allOpts = append(allOpts, opts...)

	// KTW_LLM_HTTP_DEBUG=1 dumps every Anthropic request + non-2xx
	// response to stderr (auth headers scrubbed). For diagnosing
	// provider-compat issues (e.g. Anthropic-compatible gateways
	// rejecting a request the upstream API accepts).
	if os.Getenv("KTW_LLM_HTTP_DEBUG") != "" {
		allOpts = append(allOpts, option.WithMiddleware(newDumpMiddleware(os.Stderr, false)))
	}

	client := anthropicsdk.NewClient(allOpts...)

	return &LLM{
		client:    &client,
		model:     model,
		reasoning: reasoning,
	}, nil
}

// newDumpMiddleware builds an SDK middleware that writes HTTP-level
// dumps of model calls to w (auth scrubbed; SSE teed when fullBody).
// The dump logic is shared across provider SDKs in httputil.
func newDumpMiddleware(w io.Writer, fullBody bool) func(*http.Request, option.MiddlewareNext) (*http.Response, error) {
	return func(req *http.Request, next option.MiddlewareNext) (*http.Response, error) {
		return httputil.DumpRoundTrip(w, fullBody, req, func(r *http.Request) (*http.Response, error) {
			return next(r)
		})
	}
}

// GenerateContent implements the Model interface (non-streaming).
func (o *LLM) GenerateContent(ctx context.Context, messages []api.MessageContent, options ...api.CallOption) (*api.ContentResponse, error) {
	opts := parseCallOptions(options)
	params, err := o.buildParams(messages, opts)
	if err != nil {
		return nil, err
	}
	msg, err := o.client.Messages.New(ctx, params, perCallRequestOptions(opts)...)
	if err != nil {
		return nil, MapError(err)
	}
	return convertResponse(msg)
}

// GenerateContentStream implements the StreamingModel interface.
func (o *LLM) GenerateContentStream(ctx context.Context, messages []api.MessageContent, options ...api.CallOption) (api.Stream, error) {
	opts := parseCallOptions(options)
	params, err := o.buildParams(messages, opts)
	if err != nil {
		return nil, err
	}

	sdkStream := o.client.Messages.NewStreaming(ctx, params, perCallRequestOptions(opts)...)
	sender, stream := api.NewChannelStream(64)

	go o.consumeStream(sdkStream, sender)

	return stream, nil
}

func parseCallOptions(options []api.CallOption) *api.CallOptions {
	opts := &api.CallOptions{}
	for _, opt := range options {
		opt(opts)
	}
	return opts
}

// perCallRequestOptions translates per-call observers into SDK request
// options. Currently: the structured call observer (per-session
// llm-model-calls.jsonl).
func perCallRequestOptions(opts *api.CallOptions) []option.RequestOption {
	if opts.CallObserver == nil {
		return nil
	}
	obs := opts.CallObserver
	return []option.RequestOption{option.WithMiddleware(func(req *http.Request, next option.MiddlewareNext) (*http.Response, error) {
		return httputil.ObserveRoundTrip(obs, "anthropic", req, func(r *http.Request) (*http.Response, error) {
			return next(r)
		})
	})}
}

// buildParams converts messages + parsed options into SDK MessageNewParams.
func (o *LLM) buildParams(messages []api.MessageContent, opts *api.CallOptions) (anthropicsdk.MessageNewParams, error) {
	sdkMessages, systemBlocks, err := convertMessages(messages)
	if err != nil {
		return anthropicsdk.MessageNewParams{}, fmt.Errorf("anthropic: %w", err)
	}

	model := opts.Model
	if model == "" {
		model = o.model
	}
	maxTokens := opts.MaxTokens
	if maxTokens == 0 {
		maxTokens = 4096
	}

	params := anthropicsdk.MessageNewParams{
		Model:     anthropicsdk.Model(model),
		Messages:  sdkMessages,
		MaxTokens: int64(maxTokens),
	}

	if len(systemBlocks) > 0 {
		params.System = systemBlocks
	}
	if opts.Temperature > 0 {
		params.Temperature = anthropicsdk.Float(opts.Temperature)
	}
	if opts.TopP > 0 {
		params.TopP = anthropicsdk.Float(opts.TopP)
	}
	if len(opts.StopWords) > 0 {
		params.StopSequences = opts.StopWords
	}
	if len(opts.Tools) > 0 {
		params.Tools = convertTools(opts.Tools)
	}
	if thinking, effort := buildThinkingConfig(o, opts); thinking != nil {
		params.Thinking = *thinking
		if effort != "" {
			params.OutputConfig.Effort = effort
		}
	}

	// Prompt caching: when enabled (auto_cache → WithPromptCaching sets the
	// beta-header marker), place ephemeral cache_control breakpoints on the
	// stable prefix so an agentic loop re-reads it at 0.1× instead of
	// re-sending the whole growing prompt uncached.
	if cachingEnabled(opts) {
		applyPromptCaching(&params)
	}

	return params, nil
}

// cachingEnabled reports whether prompt caching was requested for this call
// (WithPromptCaching records the prompt-caching beta marker in Metadata).
func cachingEnabled(opts *api.CallOptions) bool {
	hdrs, _ := opts.Metadata["anthropic:beta_headers"].([]string)
	for _, h := range hdrs {
		if strings.HasPrefix(h, "prompt-caching") {
			return true
		}
	}
	return false
}

// applyPromptCaching places 5-minute ephemeral cache_control breakpoints on the
// reusable prefix. Anthropic processes breakpoints tools → system → messages
// and caches the longest matching prefix; a breakpoint on the LAST message
// therefore caches the entire conversation-so-far — the dominant cost in a
// multi-turn run. The 5m TTL is refreshed on every cache read, so back-to-back
// turns keep the prefix hot. Three stable+rolling breakpoints (well under the
// 4-breakpoint limit): tools (stable), system (stable), last message (rolling).
func applyPromptCaching(params *anthropicsdk.MessageNewParams) {
	cc := anthropicsdk.NewCacheControlEphemeralParam() // ephemeral, default TTL = 5m

	if n := len(params.Tools); n > 0 {
		if t := params.Tools[n-1].OfTool; t != nil {
			t.CacheControl = cc
		}
	}
	if n := len(params.System); n > 0 {
		params.System[n-1].CacheControl = cc
	}
	if n := len(params.Messages); n > 0 {
		blocks := params.Messages[n-1].Content
		if m := len(blocks); m > 0 {
			setBlockCacheControl(&blocks[m-1], cc)
		}
	}
}

// setBlockCacheControl sets cache_control on whichever variant a content-block
// union carries (the common message block kinds in an agent loop).
func setBlockCacheControl(b *anthropicsdk.ContentBlockParamUnion, cc anthropicsdk.CacheControlEphemeralParam) {
	switch {
	case b.OfText != nil:
		b.OfText.CacheControl = cc
	case b.OfToolResult != nil:
		b.OfToolResult.CacheControl = cc
	case b.OfToolUse != nil:
		b.OfToolUse.CacheControl = cc
	case b.OfImage != nil:
		b.OfImage.CacheControl = cc
	case b.OfDocument != nil:
		b.OfDocument.CacheControl = cc
	}
}

// consumeStream reads the SDK stream in a goroutine and pushes StreamEvents
// to the sender. Assembles the final ContentResponse when complete.
func (o *LLM) consumeStream(sdkStream *ssestream.Stream[anthropicsdk.MessageStreamEventUnion], sender *api.ChannelStreamSender) {
	defer sender.Close()
	defer sdkStream.Close()

	var fullText strings.Builder
	var thinkingContent strings.Builder
	var toolCalls []api.ToolCall
	var stopReason string
	var inputTokens, outputTokens, cacheCreateTokens, cacheReadTokens int64

	type toolAccum struct {
		id   string
		name string
		json strings.Builder
	}
	toolAccums := make(map[int64]*toolAccum)

	for sdkStream.Next() {
		event := sdkStream.Current()

		switch event.Type {
		case "message_start":
			inputTokens = event.Message.Usage.InputTokens
			cacheCreateTokens = event.Message.Usage.CacheCreationInputTokens
			cacheReadTokens = event.Message.Usage.CacheReadInputTokens

		case "content_block_start":
			block := event.ContentBlock
			if block.Type == "tool_use" {
				toolAccums[event.Index] = &toolAccum{id: block.ID, name: block.Name}
				sender.Send(api.StreamEvent{
					Type:     api.StreamEventToolUseStart,
					ToolID:   block.ID,
					ToolName: block.Name,
				})
			}

		case "content_block_delta":
			delta := event.Delta
			switch delta.Type {
			case "text_delta":
				fullText.WriteString(delta.Text)
				sender.Send(api.StreamEvent{Type: api.StreamEventTextDelta, Delta: delta.Text})
			case "thinking_delta":
				thinkingContent.WriteString(delta.Thinking)
				sender.Send(api.StreamEvent{Type: api.StreamEventThinkingDelta, Delta: delta.Thinking})
			case "input_json_delta":
				if ta, ok := toolAccums[event.Index]; ok {
					ta.json.WriteString(delta.PartialJSON)
					sender.Send(api.StreamEvent{
						Type:      api.StreamEventToolUseDelta,
						ToolID:    ta.id,
						ToolName:  ta.name,
						ToolInput: delta.PartialJSON,
					})
				}
			}

		case "content_block_stop":
			if ta, ok := toolAccums[event.Index]; ok {
				tc := api.ToolCall{
					ID: ta.id,
					FunctionCall: &api.FunctionCall{
						Name:      ta.name,
						Arguments: ta.json.String(),
					},
				}
				toolCalls = append(toolCalls, tc)
				sender.Send(api.StreamEvent{
					Type:      api.StreamEventToolUseDone,
					ToolID:    ta.id,
					ToolName:  ta.name,
					ToolInput: ta.json.String(),
				})
				delete(toolAccums, event.Index)
			}

		case "message_delta":
			stopReason = string(event.Delta.StopReason)
			outputTokens = event.Usage.OutputTokens
		}
	}

	if err := sdkStream.Err(); err != nil {
		sender.Send(api.StreamEvent{Type: api.StreamEventError, Error: MapError(err)})
		return
	}

	// Emit usage event
	sender.Send(api.StreamEvent{
		Type:                     api.StreamEventUsage,
		InputTokens:              int(inputTokens),
		OutputTokens:             int(outputTokens),
		CacheCreationInputTokens: int(cacheCreateTokens),
		CacheReadInputTokens:     int(cacheReadTokens),
	})

	// Emit done
	sender.Send(api.StreamEvent{Type: api.StreamEventDone, StopReason: stopReason})

	// Assemble final ContentResponse
	genInfo := map[string]any{
		"InputTokens":              inputTokens,
		"OutputTokens":             outputTokens,
		"CacheCreationInputTokens": cacheCreateTokens,
		"CacheReadInputTokens":     cacheReadTokens,
	}
	if thinkingContent.Len() > 0 {
		genInfo["ThinkingContent"] = thinkingContent.String()
	}

	choices := make([]*api.ContentChoice, 0, 2)
	if fullText.Len() > 0 {
		choices = append(choices, &api.ContentChoice{
			Content:        fullText.String(),
			StopReason:     stopReason,
			GenerationInfo: genInfo,
		})
	}
	for _, tc := range toolCalls {
		choices = append(choices, &api.ContentChoice{
			ToolCalls:      []api.ToolCall{tc},
			StopReason:     stopReason,
			GenerationInfo: genInfo,
		})
	}
	if len(choices) > 0 {
		sender.SetResponse(&api.ContentResponse{Choices: choices})
	}
}

// --- Message conversion ---

// convertMessages converts our api.MessageContent slice into SDK MessageParam slice
// and extracts system prompt blocks separately (Anthropic API requires them at top level).
func convertMessages(messages []api.MessageContent) ([]anthropicsdk.MessageParam, []anthropicsdk.TextBlockParam, error) {
	sdkMessages := make([]anthropicsdk.MessageParam, 0, len(messages))
	var systemBlocks []anthropicsdk.TextBlockParam

	for _, msg := range messages {
		switch msg.Role {
		case api.ChatMessageTypeSystem:
			text, err := extractSystemText(msg)
			if err != nil {
				return nil, nil, err
			}
			systemBlocks = append(systemBlocks, anthropicsdk.TextBlockParam{Text: text})

		case api.ChatMessageTypeHuman:
			blocks, err := convertHumanParts(msg.Parts)
			if err != nil {
				return nil, nil, err
			}
			sdkMessages = append(sdkMessages, anthropicsdk.NewUserMessage(blocks...))

		case api.ChatMessageTypeAI:
			blocks, err := convertAssistantParts(msg.Parts)
			if err != nil {
				return nil, nil, err
			}
			sdkMessages = append(sdkMessages, anthropicsdk.NewAssistantMessage(blocks...))

		case api.ChatMessageTypeTool:
			block, err := convertToolResult(msg.Parts)
			if err != nil {
				return nil, nil, err
			}
			sdkMessages = append(sdkMessages, anthropicsdk.NewUserMessage(block))

		default:
			return nil, nil, fmt.Errorf("%w: %v", ErrUnsupportedMessageType, msg.Role)
		}
	}
	return sdkMessages, systemBlocks, nil
}

func extractSystemText(msg api.MessageContent) (string, error) {
	if len(msg.Parts) == 0 {
		return "", fmt.Errorf("%w for system message: no parts", ErrInvalidContentType)
	}
	part := msg.Parts[0]
	// Unwrap cached content
	if cached, ok := part.(api.CachedContent); ok {
		part = cached.ContentPart
	}
	if tc, ok := part.(api.TextContent); ok {
		return tc.Text, nil
	}
	return "", fmt.Errorf("%w for system message", ErrInvalidContentType)
}

func convertHumanParts(parts []api.ContentPart) ([]anthropicsdk.ContentBlockParamUnion, error) {
	blocks := make([]anthropicsdk.ContentBlockParamUnion, 0, len(parts))
	for _, part := range parts {
		switch p := part.(type) {
		case api.TextContent:
			blocks = append(blocks, anthropicsdk.NewTextBlock(p.Text))
		case api.BinaryContent:
			blocks = append(blocks, anthropicsdk.ContentBlockParamUnion{
				OfImage: &anthropicsdk.ImageBlockParam{
					Source: anthropicsdk.ImageBlockParamSourceUnion{
						OfBase64: &anthropicsdk.Base64ImageSourceParam{
							Data:      base64.StdEncoding.EncodeToString(p.Data),
							MediaType: anthropicsdk.Base64ImageSourceMediaType(p.MIMEType),
						},
					},
				},
			})
		case api.ImageURLContent:
			blocks = append(blocks, anthropicsdk.ContentBlockParamUnion{
				OfImage: &anthropicsdk.ImageBlockParam{
					Source: anthropicsdk.ImageBlockParamSourceUnion{
						OfURL: &anthropicsdk.URLImageSourceParam{
							URL: p.URL,
						},
					},
				},
			})
		case api.CachedContent:
			// Unwrap and recurse
			inner, err := convertHumanParts([]api.ContentPart{p.ContentPart})
			if err != nil {
				return nil, err
			}
			blocks = append(blocks, inner...)
		default:
			return nil, fmt.Errorf("unsupported human message part type: %T", part)
		}
	}
	return blocks, nil
}

func convertAssistantParts(parts []api.ContentPart) ([]anthropicsdk.ContentBlockParamUnion, error) {
	blocks := make([]anthropicsdk.ContentBlockParamUnion, 0, len(parts))
	for _, part := range parts {
		switch p := part.(type) {
		case api.TextContent:
			if p.Text != "" {
				blocks = append(blocks, anthropicsdk.NewTextBlock(p.Text))
			}
		case api.ToolCall:
			// A no-argument tool call arrives with Arguments == "" (or
			// whitespace). `json.RawMessage("")` is INVALID JSON — the
			// Anthropic SDK then fails to marshal the ToolUseBlockParam
			// ("unexpected end of JSON input"), crashing the whole next
			// request. Normalize empty args to an empty object so any
			// no-arg tool (scripts-list, check-messages, …) round-trips.
			args := strings.TrimSpace(p.FunctionCall.Arguments)
			if args == "" {
				args = "{}"
			}
			blocks = append(blocks, anthropicsdk.ContentBlockParamUnion{
				OfToolUse: &anthropicsdk.ToolUseBlockParam{
					ID:    p.ID,
					Name:  p.FunctionCall.Name,
					Input: json.RawMessage(args),
				},
			})
		default:
			// Skip unknown part types
		}
	}
	if len(blocks) == 0 {
		return nil, fmt.Errorf("%w for AI message (no text or tool_use parts)", ErrInvalidContentType)
	}
	return blocks, nil
}

func convertToolResult(parts []api.ContentPart) (anthropicsdk.ContentBlockParamUnion, error) {
	if len(parts) == 0 {
		return anthropicsdk.ContentBlockParamUnion{}, fmt.Errorf("%w for tool message: no parts", ErrInvalidContentType)
	}
	tcr, ok := parts[0].(api.ToolCallResponse)
	if !ok {
		return anthropicsdk.ContentBlockParamUnion{}, fmt.Errorf("%w for tool message: expected ToolCallResponse, got %T", ErrInvalidContentType, parts[0])
	}
	return anthropicsdk.ContentBlockParamUnion{
		OfToolResult: &anthropicsdk.ToolResultBlockParam{
			ToolUseID: tcr.ToolCallID,
			Content: []anthropicsdk.ToolResultBlockParamContentUnion{
				{OfText: &anthropicsdk.TextBlockParam{Text: tcr.Content}},
			},
		},
	}, nil
}

// --- Tool conversion ---

func convertTools(tools []api.Tool) []anthropicsdk.ToolUnionParam {
	result := make([]anthropicsdk.ToolUnionParam, len(tools))
	for i, tool := range tools {
		result[i] = anthropicsdk.ToolUnionParam{
			OfTool: &anthropicsdk.ToolParam{
				Name:        tool.Function.Name,
				Description: anthropicsdk.String(tool.Function.Description),
				InputSchema: convertToolSchema(tool.Function.Parameters),
			},
		}
	}
	return result
}

func convertToolSchema(m map[string]any) anthropicsdk.ToolInputSchemaParam {
	schema := anthropicsdk.ToolInputSchemaParam{}
	if props, ok := m["properties"]; ok {
		schema.Properties = props
	}
	if req, ok := m["required"].([]any); ok {
		for _, r := range req {
			if s, ok := r.(string); ok {
				schema.Required = append(schema.Required, s)
			}
		}
	}
	// Copy additional schema fields (e.g., "additionalProperties")
	for k, v := range m {
		if k == "type" || k == "properties" || k == "required" {
			continue
		}
		if schema.ExtraFields == nil {
			schema.ExtraFields = make(map[string]any)
		}
		schema.ExtraFields[k] = v
	}
	// The Anthropic spec REQUIRES input_schema on every tool; the SDK
	// elides the zero struct entirely, which strict gateways (Azure AI
	// Foundry) reject. Guarantee at least the empty object schema.
	if schema.Properties == nil {
		schema.Properties = map[string]any{}
	}
	return schema
}

// --- Response conversion ---

func convertResponse(msg *anthropicsdk.Message) (*api.ContentResponse, error) {
	if msg == nil || len(msg.Content) == 0 {
		return nil, ErrEmptyResponse
	}

	genInfo := map[string]any{
		"InputTokens":              msg.Usage.InputTokens,
		"OutputTokens":             msg.Usage.OutputTokens,
		"CacheCreationInputTokens": msg.Usage.CacheCreationInputTokens,
		"CacheReadInputTokens":     msg.Usage.CacheReadInputTokens,
	}

	choices := make([]*api.ContentChoice, 0, len(msg.Content))
	for _, block := range msg.Content {
		switch block.Type {
		case "text":
			choices = append(choices, &api.ContentChoice{
				Content:        block.Text,
				StopReason:     string(msg.StopReason),
				GenerationInfo: genInfo,
			})
		case "tool_use":
			argumentsJSON := string(block.Input)
			choices = append(choices, &api.ContentChoice{
				ToolCalls: []api.ToolCall{
					{
						ID: block.ID,
						FunctionCall: &api.FunctionCall{
							Name:      block.Name,
							Arguments: argumentsJSON,
						},
					},
				},
				StopReason:     string(msg.StopReason),
				GenerationInfo: genInfo,
			})
		case "thinking":
			genInfoCopy := make(map[string]any, len(genInfo)+2)
			for k, v := range genInfo {
				genInfoCopy[k] = v
			}
			genInfoCopy["ThinkingContent"] = block.Thinking
			genInfoCopy["ThinkingSignature"] = block.Signature
			choices = append(choices, &api.ContentChoice{
				Content:          "",
				ReasoningContent: block.Thinking,
				StopReason:       string(msg.StopReason),
				GenerationInfo:   genInfoCopy,
			})
		default:
			// Skip unknown content types
		}
	}

	if len(choices) == 0 {
		return nil, ErrEmptyResponse
	}
	return &api.ContentResponse{Choices: choices}, nil
}

// --- Thinking config ---

// buildThinkingConfig resolves the thinking parameter for a request, returning
// the thinking config plus an optional output-config effort level.
//
// Modern Claude models (Opus 4.6+, Sonnet 4.6, Fable 5, Mythos 5) use ADAPTIVE
// thinking — the model decides depth itself — and reject the legacy
// `thinking.type=enabled` + budget_tokens shape with a 400. Depth on those
// models is controlled by output_config.effort. Older models keep the enabled
// + budget_tokens form (effort is unsupported there and would 400).
func buildThinkingConfig(o *LLM, opts *api.CallOptions) (*anthropicsdk.ThinkingConfigParamUnion, anthropicsdk.OutputConfigEffort) {
	if opts.Metadata == nil {
		return nil, ""
	}
	config, ok := opts.Metadata["thinking_config"].(*api.ThinkingConfig)
	if !ok {
		return nil, ""
	}

	if !o.reasoning {
		return nil, ""
	}

	model := opts.Model
	if model == "" {
		model = o.model
	}

	if modelUsesAdaptiveThinking(model) {
		return &anthropicsdk.ThinkingConfigParamUnion{
			OfAdaptive: &anthropicsdk.ThinkingConfigAdaptiveParam{},
		}, effortForThinking(config)
	}

	budgetTokens := config.BudgetTokens
	if budgetTokens == 0 && config.Mode != api.ThinkingModeNone {
		budgetTokens = api.CalculateThinkingBudget(config.Mode, opts.MaxTokens)
	}
	if budgetTokens > 0 {
		if budgetTokens < 1024 {
			budgetTokens = 1024
		} else if budgetTokens > 128000 {
			budgetTokens = 128000
		}
		return &anthropicsdk.ThinkingConfigParamUnion{
			OfEnabled: &anthropicsdk.ThinkingConfigEnabledParam{
				BudgetTokens: int64(budgetTokens),
			},
		}, ""
	}
	return nil, ""
}

// modelUsesAdaptiveThinking reports whether a model requires the adaptive
// thinking + effort shape instead of the legacy enabled + budget_tokens shape.
// Opus 4.7/4.8, Fable 5, and Mythos 5 reject enabled thinking outright; Opus 4.6
// and Sonnet 4.6 accept adaptive (the recommended path). Older models stay on
// budget_tokens, which is still valid there.
func modelUsesAdaptiveThinking(model string) bool {
	m := strings.ToLower(model)
	for _, family := range []string{
		"claude-opus-4-6",
		"claude-opus-4-7",
		"claude-opus-4-8",
		"claude-sonnet-4-6",
		"claude-fable-5",
		"claude-mythos-5",
		"claude-mythos-preview",
	} {
		if strings.Contains(m, family) {
			return true
		}
	}
	return false
}

// effortForThinking maps a thinking mode onto an output-config effort level.
// Modes carry no token budget on adaptive models, so they translate directly;
// an unset mode defaults to the API's own default of "high".
func effortForThinking(config *api.ThinkingConfig) anthropicsdk.OutputConfigEffort {
	switch config.Mode {
	case api.ThinkingModeLow:
		return anthropicsdk.OutputConfigEffortLow
	case api.ThinkingModeMedium:
		return anthropicsdk.OutputConfigEffortMedium
	case api.ThinkingModeHigh:
		return anthropicsdk.OutputConfigEffortHigh
	default:
		return anthropicsdk.OutputConfigEffortHigh
	}
}
