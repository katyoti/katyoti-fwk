// SPDX-License-Identifier: Apache-2.0

package openai

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strings"

	openaisdk "github.com/openai/openai-go/v2"
	"github.com/openai/openai-go/v2/option"
	"github.com/openai/openai-go/v2/packages/ssestream"
	"github.com/openai/openai-go/v2/shared"

	"github.com/katyoti/katyoti-fwk/internal/models/api"
	"github.com/katyoti/katyoti-fwk/internal/models/internal/httputil"
)

var (
	ErrEmptyResponse            = errors.New("no response")
	ErrMissingToken             = errors.New("missing the OpenAI API key, set it in the OPENAI_API_KEY environment variable")
	ErrUnexpectedResponseLength = errors.New("unexpected length of response")
)

// LLM implements the OpenAI provider using the official openai-go SDK.
type LLM struct {
	client *openaisdk.Client
	model  string
}

// New creates a new OpenAI LLM client using the official SDK.
func New(apiKey, model string, opts ...option.RequestOption) (*LLM, error) {
	if apiKey == "" {
		return nil, ErrMissingToken
	}

	allOpts := []option.RequestOption{option.WithAPIKey(apiKey)}
	allOpts = append(allOpts, opts...)

	client := openaisdk.NewClient(allOpts...)

	return &LLM{
		client: &client,
		model:  model,
	}, nil
}

// GenerateContent implements the Model interface (non-streaming).
func (o *LLM) GenerateContent(ctx context.Context, messages []api.MessageContent, options ...api.CallOption) (*api.ContentResponse, error) {
	params, err := o.buildParams(messages, options)
	if err != nil {
		return nil, err
	}
	result, err := o.client.Chat.Completions.New(ctx, params, perCallRequestOptions(options)...)
	if err != nil {
		return nil, MapError(err)
	}
	if len(result.Choices) == 0 {
		return nil, ErrEmptyResponse
	}
	return convertResponse(result), nil
}

// GenerateContentStream implements the StreamingModel interface.
func (o *LLM) GenerateContentStream(ctx context.Context, messages []api.MessageContent, options ...api.CallOption) (api.Stream, error) {
	params, err := o.buildParams(messages, options)
	if err != nil {
		return nil, err
	}

	// Enable usage reporting in stream
	params.StreamOptions = openaisdk.ChatCompletionStreamOptionsParam{
		IncludeUsage: openaisdk.Bool(true),
	}

	sdkStream := o.client.Chat.Completions.NewStreaming(ctx, params, perCallRequestOptions(options)...)
	sender, stream := api.NewChannelStream(64)

	go o.consumeStream(sdkStream, sender)

	return stream, nil
}

// perCallRequestOptions translates per-call observers into SDK request
// options. Currently: the structured call observer (per-session
// llm-model-calls.jsonl).
func perCallRequestOptions(options []api.CallOption) []option.RequestOption {
	opts := &api.CallOptions{}
	for _, opt := range options {
		opt(opts)
	}
	if opts.CallObserver == nil {
		return nil
	}
	obs := opts.CallObserver
	return []option.RequestOption{option.WithMiddleware(func(req *http.Request, next option.MiddlewareNext) (*http.Response, error) {
		return httputil.ObserveRoundTrip(obs, "openai", req, func(r *http.Request) (*http.Response, error) {
			return next(r)
		})
	})}
}

// buildParams converts messages + options into SDK ChatCompletionNewParams.
func (o *LLM) buildParams(messages []api.MessageContent, options []api.CallOption) (openaisdk.ChatCompletionNewParams, error) {
	opts := api.CallOptions{}
	for _, opt := range options {
		opt(&opts)
	}

	effectiveModel := opts.Model
	if effectiveModel == "" {
		effectiveModel = o.model
	}

	sdkMessages, err := convertMessages(messages, effectiveModel)
	if err != nil {
		return openaisdk.ChatCompletionNewParams{}, fmt.Errorf("openai: %w", err)
	}

	params := openaisdk.ChatCompletionNewParams{
		Model:    shared.ChatModel(effectiveModel),
		Messages: sdkMessages,
	}

	// Token limits
	useLegacy := false
	if opts.Metadata != nil {
		if v, ok := opts.Metadata["openai:use_legacy_max_tokens"].(bool); ok {
			useLegacy = v
		}
	}
	if opts.MaxTokens > 0 {
		if useLegacy {
			params.MaxTokens = openaisdk.Int(int64(opts.MaxTokens))
		} else {
			params.MaxCompletionTokens = openaisdk.Int(int64(opts.MaxTokens))
		}
	}

	if opts.Temperature > 0 {
		params.Temperature = openaisdk.Float(opts.Temperature)
	}
	if opts.TopP > 0 {
		params.TopP = openaisdk.Float(opts.TopP)
	}
	if opts.FrequencyPenalty != 0 {
		params.FrequencyPenalty = openaisdk.Float(opts.FrequencyPenalty)
	}
	if opts.PresencePenalty != 0 {
		params.PresencePenalty = openaisdk.Float(opts.PresencePenalty)
	}
	if opts.Seed > 0 {
		params.Seed = openaisdk.Int(int64(opts.Seed))
	}
	if opts.N > 0 {
		params.N = openaisdk.Int(int64(opts.N))
	}
	if len(opts.StopWords) > 0 {
		params.Stop = openaisdk.ChatCompletionNewParamsStopUnion{
			OfStringArray: opts.StopWords,
		}
	}
	if opts.JSONMode {
		params.ResponseFormat = openaisdk.ChatCompletionNewParamsResponseFormatUnion{
			OfJSONObject: &openaisdk.ResponseFormatJSONObjectParam{},
		}
	}
	if len(opts.Tools) > 0 {
		params.Tools = convertTools(opts.Tools)
	}
	for _, fn := range opts.Functions {
		params.Tools = append(params.Tools, openaisdk.ChatCompletionToolUnionParam{
			OfFunction: &openaisdk.ChatCompletionFunctionToolParam{
				Function: shared.FunctionDefinitionParam{
					Name:        fn.Name,
					Description: openaisdk.String(fn.Description),
					Parameters:  convertFuncParams(fn.Parameters),
				},
			},
		})
	}
	if opts.ToolChoice != nil {
		params.ToolChoice = convertToolChoice(opts.ToolChoice)
	}

	return params, nil
}

// consumeStream reads the SDK stream in a goroutine and pushes StreamEvents.
func (o *LLM) consumeStream(sdkStream *ssestream.Stream[openaisdk.ChatCompletionChunk], sender *api.ChannelStreamSender) {
	defer sender.Close()
	defer sdkStream.Close()

	var fullText strings.Builder
	var finishReason string

	type toolAccum struct {
		id   string
		typ  string
		name string
		args strings.Builder
	}
	toolAccums := make(map[int64]*toolAccum)

	var promptTokens, completionTokens, reasoningTokens, cachedTokens int64

	for sdkStream.Next() {
		chunk := sdkStream.Current()

		// Capture usage from the final chunk
		if chunk.Usage.TotalTokens > 0 {
			promptTokens = chunk.Usage.PromptTokens
			completionTokens = chunk.Usage.CompletionTokens
			reasoningTokens = chunk.Usage.CompletionTokensDetails.ReasoningTokens
			cachedTokens = chunk.Usage.PromptTokensDetails.CachedTokens
		}

		for _, choice := range chunk.Choices {
			delta := choice.Delta

			if delta.Content != "" {
				fullText.WriteString(delta.Content)
				sender.Send(api.StreamEvent{Type: api.StreamEventTextDelta, Delta: delta.Content})
			}

			for _, tc := range delta.ToolCalls {
				idx := tc.Index
				if _, ok := toolAccums[idx]; !ok {
					toolAccums[idx] = &toolAccum{id: tc.ID, typ: string(tc.Type)}
					sender.Send(api.StreamEvent{
						Type:     api.StreamEventToolUseStart,
						ToolID:   tc.ID,
						ToolName: tc.Function.Name,
					})
				}
				ta := toolAccums[idx]
				if tc.ID != "" {
					ta.id = tc.ID
				}
				if tc.Function.Name != "" {
					ta.name = tc.Function.Name
				}
				if tc.Function.Arguments != "" {
					ta.args.WriteString(tc.Function.Arguments)
					sender.Send(api.StreamEvent{
						Type:      api.StreamEventToolUseDelta,
						ToolID:    ta.id,
						ToolName:  ta.name,
						ToolInput: tc.Function.Arguments,
					})
				}
			}

			if choice.FinishReason != "" {
				finishReason = string(choice.FinishReason)
			}
		}
	}

	if err := sdkStream.Err(); err != nil {
		sender.Send(api.StreamEvent{Type: api.StreamEventError, Error: MapError(err)})
		return
	}

	// Emit tool_use_done for each assembled tool call
	var toolCalls []api.ToolCall
	for _, ta := range toolAccums {
		tc := api.ToolCall{
			ID:   ta.id,
			Type: ta.typ,
			FunctionCall: &api.FunctionCall{
				Name:      ta.name,
				Arguments: ta.args.String(),
			},
		}
		toolCalls = append(toolCalls, tc)
		sender.Send(api.StreamEvent{
			Type:      api.StreamEventToolUseDone,
			ToolID:    ta.id,
			ToolName:  ta.name,
			ToolInput: ta.args.String(),
		})
	}

	// Emit usage
	sender.Send(api.StreamEvent{
		Type:         api.StreamEventUsage,
		InputTokens:  int(promptTokens),
		OutputTokens: int(completionTokens),
	})

	// Emit done
	sender.Send(api.StreamEvent{Type: api.StreamEventDone, StopReason: finishReason})

	// Assemble final ContentResponse
	genInfo := map[string]any{
		"CompletionTokens":   completionTokens,
		"PromptTokens":       promptTokens,
		"TotalTokens":        promptTokens + completionTokens,
		"ReasoningTokens":    reasoningTokens,
		"PromptCachedTokens": cachedTokens,
	}

	choice := &api.ContentChoice{
		Content:        fullText.String(),
		StopReason:     finishReason,
		GenerationInfo: genInfo,
		ToolCalls:      toolCalls,
	}
	if len(toolCalls) > 0 {
		choice.FuncCall = toolCalls[0].FunctionCall
	}

	sender.SetResponse(&api.ContentResponse{Choices: []*api.ContentChoice{choice}})
}

// --- Message conversion ---

func convertMessages(messages []api.MessageContent, model string) ([]openaisdk.ChatCompletionMessageParamUnion, error) {
	supportsSystem := modelSupportsSystem(model)

	var systemContent string
	if !supportsSystem {
		for _, mc := range messages {
			if mc.Role == api.ChatMessageTypeSystem {
				for _, part := range mc.Parts {
					if tp, ok := part.(api.TextContent); ok {
						if systemContent != "" {
							systemContent += "\n\n"
						}
						systemContent += tp.Text
					}
				}
			}
		}
	}

	result := make([]openaisdk.ChatCompletionMessageParamUnion, 0, len(messages))
	firstUser := true

	for _, mc := range messages {
		if mc.Role == api.ChatMessageTypeSystem && !supportsSystem {
			continue
		}

		switch mc.Role {
		case api.ChatMessageTypeSystem:
			text := extractText(mc.Parts)
			result = append(result, openaisdk.ChatCompletionMessageParamUnion{
				OfSystem: &openaisdk.ChatCompletionSystemMessageParam{
					Content: openaisdk.ChatCompletionSystemMessageParamContentUnion{
						OfString: openaisdk.String(text),
					},
				},
			})

		case api.ChatMessageTypeHuman:
			// Check if message has multimodal content (images, binary)
			hasMultimodal := false
			for _, part := range mc.Parts {
				switch part.(type) {
				case api.ImageURLContent, api.BinaryContent:
					hasMultimodal = true
				}
			}

			if hasMultimodal {
				// Use content parts array for multimodal messages
				contentParts := convertUserContentParts(mc.Parts)
				if firstUser && systemContent != "" && !supportsSystem {
					// Prepend system content as text part
					contentParts = append(
						[]openaisdk.ChatCompletionContentPartUnionParam{openaisdk.TextContentPart(systemContent)},
						contentParts...,
					)
					systemContent = ""
				}
				firstUser = false
				result = append(result, openaisdk.ChatCompletionMessageParamUnion{
					OfUser: &openaisdk.ChatCompletionUserMessageParam{
						Content: openaisdk.ChatCompletionUserMessageParamContentUnion{
							OfArrayOfContentParts: contentParts,
						},
					},
				})
			} else {
				// Text-only: use simple string content
				text := extractText(mc.Parts)
				if firstUser && systemContent != "" && !supportsSystem {
					text = systemContent + "\n\n" + text
					systemContent = ""
				}
				firstUser = false
				result = append(result, openaisdk.ChatCompletionMessageParamUnion{
					OfUser: &openaisdk.ChatCompletionUserMessageParam{
						Content: openaisdk.ChatCompletionUserMessageParamContentUnion{
							OfString: openaisdk.String(text),
						},
					},
				})
			}

		case api.ChatMessageTypeAI:
			msg := &openaisdk.ChatCompletionAssistantMessageParam{}
			var textParts []string
			for _, part := range mc.Parts {
				switch p := part.(type) {
				case api.TextContent:
					if p.Text != "" {
						textParts = append(textParts, p.Text)
					}
				case api.ToolCall:
					msg.ToolCalls = append(msg.ToolCalls, openaisdk.ChatCompletionMessageToolCallUnionParam{
						OfFunction: &openaisdk.ChatCompletionMessageFunctionToolCallParam{
							ID: p.ID,
							Function: openaisdk.ChatCompletionMessageFunctionToolCallFunctionParam{
								Name:      p.FunctionCall.Name,
								Arguments: p.FunctionCall.Arguments,
							},
						},
					})
				}
			}
			if len(textParts) > 0 {
				msg.Content = openaisdk.ChatCompletionAssistantMessageParamContentUnion{
					OfString: openaisdk.String(strings.Join(textParts, "\n")),
				}
			}
			result = append(result, openaisdk.ChatCompletionMessageParamUnion{
				OfAssistant: msg,
			})

		case api.ChatMessageTypeTool:
			if len(mc.Parts) != 1 {
				return nil, fmt.Errorf("expected exactly one part for role %v, got %v", mc.Role, len(mc.Parts))
			}
			tcr, ok := mc.Parts[0].(api.ToolCallResponse)
			if !ok {
				return nil, fmt.Errorf("expected ToolCallResponse for role %v, got %T", mc.Role, mc.Parts[0])
			}
			result = append(result, openaisdk.ChatCompletionMessageParamUnion{
				OfTool: &openaisdk.ChatCompletionToolMessageParam{
					ToolCallID: tcr.ToolCallID,
					Content: openaisdk.ChatCompletionToolMessageParamContentUnion{
						OfString: openaisdk.String(tcr.Content),
					},
				},
			})

		default:
			return nil, fmt.Errorf("role %v not supported", mc.Role)
		}
	}
	return result, nil
}

func extractText(parts []api.ContentPart) string {
	var texts []string
	for _, part := range parts {
		if tp, ok := part.(api.TextContent); ok {
			texts = append(texts, tp.Text)
		}
	}
	return strings.Join(texts, "\n")
}

// convertUserContentParts converts multimodal message parts to OpenAI SDK content parts.
func convertUserContentParts(parts []api.ContentPart) []openaisdk.ChatCompletionContentPartUnionParam {
	result := make([]openaisdk.ChatCompletionContentPartUnionParam, 0, len(parts))
	for _, part := range parts {
		switch p := part.(type) {
		case api.TextContent:
			result = append(result, openaisdk.TextContentPart(p.Text))
		case api.ImageURLContent:
			imgParam := openaisdk.ChatCompletionContentPartImageImageURLParam{
				URL: p.URL,
			}
			if p.Detail != "" {
				imgParam.Detail = p.Detail
			}
			result = append(result, openaisdk.ImageContentPart(imgParam))
		case api.BinaryContent:
			// OpenAI accepts base64 images as data URIs in the image_url field
			dataURI := "data:" + p.MIMEType + ";base64," + base64.StdEncoding.EncodeToString(p.Data)
			result = append(result, openaisdk.ImageContentPart(
				openaisdk.ChatCompletionContentPartImageImageURLParam{
					URL: dataURI,
				},
			))
		}
	}
	return result
}

// --- Tool conversion ---

func convertTools(tools []api.Tool) []openaisdk.ChatCompletionToolUnionParam {
	result := make([]openaisdk.ChatCompletionToolUnionParam, len(tools))
	for i, tool := range tools {
		result[i] = openaisdk.ChatCompletionToolUnionParam{
			OfFunction: &openaisdk.ChatCompletionFunctionToolParam{
				Function: shared.FunctionDefinitionParam{
					Name:        tool.Function.Name,
					Description: openaisdk.String(tool.Function.Description),
					Parameters:  convertFuncParams(tool.Function.Parameters),
				},
			},
		}
	}
	return result
}

func convertFuncParams(params map[string]any) shared.FunctionParameters {
	if params == nil {
		return nil
	}
	return shared.FunctionParameters(params)
}

func convertToolChoice(choice any) openaisdk.ChatCompletionToolChoiceOptionUnionParam {
	switch v := choice.(type) {
	case string:
		return openaisdk.ChatCompletionToolChoiceOptionUnionParam{
			OfAuto: openaisdk.String(v),
		}
	case api.ToolChoice:
		if v.Function != nil {
			return openaisdk.ChatCompletionToolChoiceOptionUnionParam{
				OfFunctionToolChoice: &openaisdk.ChatCompletionNamedToolChoiceParam{
					Function: openaisdk.ChatCompletionNamedToolChoiceFunctionParam{
						Name: v.Function.Name,
					},
				},
			}
		}
		return openaisdk.ChatCompletionToolChoiceOptionUnionParam{
			OfAuto: openaisdk.String(v.Type),
		}
	default:
		return openaisdk.ChatCompletionToolChoiceOptionUnionParam{}
	}
}

// --- Response conversion ---

func convertResponse(result *openaisdk.ChatCompletion) *api.ContentResponse {
	genInfo := map[string]any{
		"CompletionTokens":                   result.Usage.CompletionTokens,
		"PromptTokens":                       result.Usage.PromptTokens,
		"TotalTokens":                        result.Usage.TotalTokens,
		"ReasoningTokens":                    result.Usage.CompletionTokensDetails.ReasoningTokens,
		"PromptCachedTokens":                 result.Usage.PromptTokensDetails.CachedTokens,
		"CompletionAudioTokens":              result.Usage.CompletionTokensDetails.AudioTokens,
		"CompletionAcceptedPredictionTokens": result.Usage.CompletionTokensDetails.AcceptedPredictionTokens,
		"CompletionRejectedPredictionTokens": result.Usage.CompletionTokensDetails.RejectedPredictionTokens,
		"ThinkingTokens":                     result.Usage.CompletionTokensDetails.ReasoningTokens,
	}

	choices := make([]*api.ContentChoice, len(result.Choices))
	for i, c := range result.Choices {
		choice := &api.ContentChoice{
			Content:        c.Message.Content,
			StopReason:     string(c.FinishReason),
			GenerationInfo: genInfo,
		}

		for _, tc := range c.Message.ToolCalls {
			fn := tc.AsFunction()
			choice.ToolCalls = append(choice.ToolCalls, api.ToolCall{
				ID:   fn.ID,
				Type: string(fn.Type),
				FunctionCall: &api.FunctionCall{
					Name:      fn.Function.Name,
					Arguments: fn.Function.Arguments,
				},
			})
		}

		if len(choice.ToolCalls) > 0 {
			choice.FuncCall = choice.ToolCalls[0].FunctionCall
		}

		choices[i] = choice
	}

	return &api.ContentResponse{Choices: choices}
}

// --- Model capability detection ---

func modelSupportsSystem(model string) bool {
	m := strings.ToLower(model)
	if strings.HasPrefix(m, "o1-") || strings.HasPrefix(m, "o3-") || strings.HasPrefix(m, "o4-") {
		return false
	}
	return true
}
