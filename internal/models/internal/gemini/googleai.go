// SPDX-License-Identifier: Apache-2.0

//nolint:all
package gemini

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/google/generative-ai-go/genai"
	"github.com/katyoti/katyoti-fwk/internal/models/api"
	"github.com/katyoti/katyoti-fwk/internal/models/internal/httputil"
	"google.golang.org/api/iterator"
)

var (
	ErrNoContentInResponse   = errors.New("no content in generation response")
	ErrUnknownPartInResponse = errors.New("unknown part type in generation response")
	ErrInvalidMimeType       = errors.New("invalid mime type on content")
)

const (
	CITATIONS            = "citations"
	SAFETY               = "safety"
	RoleSystem           = "system"
	RoleModel            = "model"
	RoleUser             = "user"
	RoleTool             = "tool"
	ResponseMIMETypeJson = "application/json"
)

// GenerateContent implements the [api.Model] interface (non-streaming).
func (g *GoogleAI) GenerateContent(
	ctx context.Context,
	messages []api.MessageContent,
	options ...api.CallOption,
) (*api.ContentResponse, error) {
	model, opts, err := g.setupModel(options)
	if err != nil {
		return nil, err
	}

	if len(messages) == 1 {
		theMessage := messages[0]
		if theMessage.Role != api.ChatMessageTypeHuman {
			return nil, fmt.Errorf("got %v message role, want human", theMessage.Role)
		}
		return generateFromSingleMessage(ctx, model, theMessage.Parts, opts)
	}
	return generateFromMessages(ctx, model, messages, opts)
}

// GenerateContentStream implements the [api.StreamingModel] interface.
func (g *GoogleAI) GenerateContentStream(
	ctx context.Context,
	messages []api.MessageContent,
	options ...api.CallOption,
) (api.Stream, error) {
	model, opts, err := g.setupModel(options)
	if err != nil {
		return nil, err
	}

	sender, stream := api.NewChannelStream(64)

	if len(messages) == 1 {
		theMessage := messages[0]
		if theMessage.Role != api.ChatMessageTypeHuman {
			return nil, fmt.Errorf("got %v message role, want human", theMessage.Role)
		}
		convertedParts, err := convertParts(theMessage.Parts)
		if err != nil {
			return nil, err
		}
		iter := model.GenerateContentStream(ctx, convertedParts...)
		go consumeGenaiIterator(iter, sender)
	} else {
		history, reqContent, err := prepareChat(model, messages)
		if err != nil {
			return nil, err
		}
		session := model.StartChat()
		session.History = history
		iter := session.SendMessageStream(ctx, reqContent.Parts...)
		go consumeGenaiIterator(iter, sender)
	}

	_ = opts // already applied to model
	return stream, nil
}

// setupModel creates and configures the genai model from call options.
func (g *GoogleAI) setupModel(options []api.CallOption) (*genai.GenerativeModel, *api.CallOptions, error) {
	opts := &api.CallOptions{
		Model:          g.opts.DefaultModel,
		CandidateCount: g.opts.DefaultCandidateCount,
		MaxTokens:      g.opts.DefaultMaxTokens,
		Temperature:    g.opts.DefaultTemperature,
		TopP:           g.opts.DefaultTopP,
		TopK:           g.opts.DefaultTopK,
	}
	for _, opt := range options {
		opt(opts)
	}

	model := g.client.GenerativeModel(opts.Model)
	model.SetCandidateCount(int32(opts.CandidateCount))
	model.SetMaxOutputTokens(int32(opts.MaxTokens))
	model.SetTemperature(float32(opts.Temperature))
	model.SetTopP(float32(opts.TopP))
	model.SetTopK(int32(opts.TopK))
	model.StopSequences = opts.StopWords

	if cachedContentName, ok := opts.Metadata["CachedContentName"].(string); ok && cachedContentName != "" {
		model.CachedContentName = cachedContentName
	}

	model.SafetySettings = []*genai.SafetySetting{
		{Category: genai.HarmCategoryDangerousContent, Threshold: genai.HarmBlockThreshold(g.opts.HarmThreshold)},
		{Category: genai.HarmCategoryHarassment, Threshold: genai.HarmBlockThreshold(g.opts.HarmThreshold)},
		{Category: genai.HarmCategoryHateSpeech, Threshold: genai.HarmBlockThreshold(g.opts.HarmThreshold)},
		{Category: genai.HarmCategorySexuallyExplicit, Threshold: genai.HarmBlockThreshold(g.opts.HarmThreshold)},
	}

	var err error
	if model.Tools, err = convertTools(opts.Tools); err != nil {
		return nil, nil, err
	}

	switch {
	case opts.ResponseMIMEType != "" && opts.JSONMode:
		return nil, nil, fmt.Errorf("conflicting options, can't use JSONMode and ResponseMIMEType together")
	case opts.ResponseMIMEType != "":
		model.ResponseMIMEType = opts.ResponseMIMEType
	case opts.JSONMode:
		model.ResponseMIMEType = ResponseMIMETypeJson
	}

	return model, opts, nil
}

// prepareChat builds history and request content from messages.
func prepareChat(model *genai.GenerativeModel, messages []api.MessageContent) ([]*genai.Content, *genai.Content, error) {
	history := make([]*genai.Content, 0, len(messages))
	for _, mc := range messages {
		content, err := convertContent(mc)
		if err != nil {
			return nil, nil, err
		}
		if mc.Role == RoleSystem {
			model.SystemInstruction = content
			continue
		}
		history = append(history, content)
	}
	n := len(history)
	if n == 0 {
		return nil, nil, fmt.Errorf("no messages after system extraction")
	}
	reqContent := history[n-1]
	history = history[:n-1]
	return history, reqContent, nil
}

// consumeGenaiIterator reads a Gemini streaming iterator and pushes StreamEvents.
func consumeGenaiIterator(iter *genai.GenerateContentResponseIterator, sender *api.ChannelStreamSender) {
	defer sender.Close()

	var fullText strings.Builder
	var toolCalls []api.ToolCall
	var stopReason string
	var inputTokens, outputTokens, cachedTokens int32

	for {
		resp, err := iter.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			sender.Send(api.StreamEvent{Type: api.StreamEventError, Error: fmt.Errorf("gemini stream: %w", err)})
			return
		}

		// Capture usage
		if resp.UsageMetadata != nil {
			inputTokens = resp.UsageMetadata.PromptTokenCount
			outputTokens = resp.UsageMetadata.CandidatesTokenCount
			cachedTokens = resp.UsageMetadata.CachedContentTokenCount
		}

		if len(resp.Candidates) == 0 {
			continue
		}

		candidate := resp.Candidates[0]
		if candidate.Content == nil {
			continue
		}

		if candidate.FinishReason > 0 {
			stopReason = candidate.FinishReason.String()
		}

		for _, part := range candidate.Content.Parts {
			switch v := part.(type) {
			case genai.Text:
				text := string(v)
				fullText.WriteString(text)
				sender.Send(api.StreamEvent{Type: api.StreamEventTextDelta, Delta: text})
			case genai.FunctionCall:
				b, _ := json.Marshal(v.Args)
				tc := api.ToolCall{
					FunctionCall: &api.FunctionCall{
						Name:      v.Name,
						Arguments: string(b),
					},
				}
				toolCalls = append(toolCalls, tc)
				sender.Send(api.StreamEvent{
					Type:      api.StreamEventToolUseDone,
					ToolName:  v.Name,
					ToolInput: string(b),
				})
			}
		}
	}

	// Emit usage
	sender.Send(api.StreamEvent{
		Type:                 api.StreamEventUsage,
		InputTokens:          int(inputTokens),
		OutputTokens:         int(outputTokens),
		CacheReadInputTokens: int(cachedTokens),
	})

	// Emit done
	sender.Send(api.StreamEvent{Type: api.StreamEventDone, StopReason: stopReason})

	// Assemble final response
	genInfo := map[string]any{
		"PromptTokens":     inputTokens,
		"CompletionTokens": outputTokens,
		"TotalTokens":      inputTokens + outputTokens,
		"CachedTokens":     cachedTokens,
	}

	choices := []*api.ContentChoice{{
		Content:        fullText.String(),
		StopReason:     stopReason,
		GenerationInfo: genInfo,
		ToolCalls:      toolCalls,
	}}
	if len(toolCalls) > 0 {
		choices[0].FuncCall = toolCalls[0].FunctionCall
	}

	sender.SetResponse(&api.ContentResponse{Choices: choices})
}

// --- Non-streaming helpers ---

func generateFromSingleMessage(
	ctx context.Context,
	model *genai.GenerativeModel,
	parts []api.ContentPart,
	opts *api.CallOptions,
) (*api.ContentResponse, error) {
	convertedParts, err := convertParts(parts)
	if err != nil {
		return nil, err
	}
	resp, err := model.GenerateContent(ctx, convertedParts...)
	if err != nil {
		return nil, err
	}
	if len(resp.Candidates) == 0 {
		return nil, ErrNoContentInResponse
	}
	return convertCandidates(resp.Candidates, resp.UsageMetadata)
}

func generateFromMessages(
	ctx context.Context,
	model *genai.GenerativeModel,
	messages []api.MessageContent,
	opts *api.CallOptions,
) (*api.ContentResponse, error) {
	history, reqContent, err := prepareChat(model, messages)
	if err != nil {
		return nil, err
	}
	session := model.StartChat()
	session.History = history
	resp, err := session.SendMessage(ctx, reqContent.Parts...)
	if err != nil {
		return nil, err
	}
	if len(resp.Candidates) == 0 {
		return nil, ErrNoContentInResponse
	}
	return convertCandidates(resp.Candidates, resp.UsageMetadata)
}

// --- Response conversion ---

func convertCandidates(candidates []*genai.Candidate, usage *genai.UsageMetadata) (*api.ContentResponse, error) {
	var contentResponse api.ContentResponse
	var toolCalls []api.ToolCall

	for _, candidate := range candidates {
		buf := strings.Builder{}

		if candidate.Content != nil {
			for _, part := range candidate.Content.Parts {
				switch v := part.(type) {
				case genai.Text:
					buf.WriteString(string(v))
				case genai.FunctionCall:
					b, err := json.Marshal(v.Args)
					if err != nil {
						return nil, err
					}
					toolCalls = append(toolCalls, api.ToolCall{
						FunctionCall: &api.FunctionCall{
							Name:      v.Name,
							Arguments: string(b),
						},
					})
				default:
					return nil, ErrUnknownPartInResponse
				}
			}
		}

		metadata := make(map[string]any)
		metadata[CITATIONS] = candidate.CitationMetadata
		metadata[SAFETY] = candidate.SafetyRatings

		if usage != nil {
			metadata["PromptTokens"] = usage.PromptTokenCount
			metadata["CompletionTokens"] = usage.CandidatesTokenCount
			metadata["TotalTokens"] = usage.TotalTokenCount
			if usage.CachedContentTokenCount > 0 {
				metadata["CachedTokens"] = usage.CachedContentTokenCount
			}
		}

		contentResponse.Choices = append(contentResponse.Choices,
			&api.ContentChoice{
				Content:        buf.String(),
				StopReason:     candidate.FinishReason.String(),
				GenerationInfo: metadata,
				ToolCalls:      toolCalls,
			})
	}
	return &contentResponse, nil
}

// --- Part conversion ---

func convertParts(parts []api.ContentPart) ([]genai.Part, error) {
	convertedParts := make([]genai.Part, 0, len(parts))
	for _, part := range parts {
		var out genai.Part
		switch p := part.(type) {
		case api.TextContent:
			out = genai.Text(p.Text)
		case api.BinaryContent:
			out = genai.Blob{MIMEType: p.MIMEType, Data: p.Data}
		case api.ImageURLContent:
			typ, data, err := httputil.DownloadImageData(p.URL)
			if err != nil {
				return nil, err
			}
			out = genai.ImageData(typ, data)
		case api.ToolCall:
			fc := p.FunctionCall
			var argsMap map[string]any
			if err := json.Unmarshal([]byte(fc.Arguments), &argsMap); err != nil {
				return convertedParts, err
			}
			out = genai.FunctionCall{Name: fc.Name, Args: argsMap}
		case api.ToolCallResponse:
			out = genai.FunctionResponse{
				Name:     p.Name,
				Response: map[string]any{"response": p.Content},
			}
		}
		convertedParts = append(convertedParts, out)
	}
	return convertedParts, nil
}

func convertContent(content api.MessageContent) (*genai.Content, error) {
	parts, err := convertParts(content.Parts)
	if err != nil {
		return nil, err
	}
	c := &genai.Content{Parts: parts}
	switch content.Role {
	case api.ChatMessageTypeSystem:
		c.Role = RoleSystem
	case api.ChatMessageTypeAI:
		c.Role = RoleModel
	case api.ChatMessageTypeHuman, api.ChatMessageTypeGeneric, api.ChatMessageTypeTool:
		c.Role = RoleUser
	default:
		return nil, fmt.Errorf("role %v not supported", content.Role)
	}
	return c, nil
}

// --- Tool schema conversion ---

func convertSchemaRecursive(schemaMap map[string]any, toolIndex int, propertyPath string) (*genai.Schema, error) {
	schema := &genai.Schema{}

	if ty, ok := schemaMap["type"]; ok {
		tyString, ok := ty.(string)
		if !ok {
			return nil, fmt.Errorf("tool [%d], property [%s]: expected string for type", toolIndex, propertyPath)
		}
		schema.Type = convertToolSchemaType(tyString)
	}

	if desc, ok := schemaMap["description"]; ok {
		descString, ok := desc.(string)
		if !ok {
			return nil, fmt.Errorf("tool [%d], property [%s]: expected string for description", toolIndex, propertyPath)
		}
		schema.Description = descString
	}

	if properties, ok := schemaMap["properties"]; ok {
		propMap, ok := properties.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("tool [%d], property [%s]: expected map for properties", toolIndex, propertyPath)
		}
		schema.Properties = make(map[string]*genai.Schema)
		for propName, propValue := range propMap {
			valueMap, ok := propValue.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("tool [%d], property [%s.%s]: expect to find a value map", toolIndex, propertyPath, propName)
			}
			nestedPath := propName
			if propertyPath != "" {
				nestedPath = propertyPath + "." + propName
			}
			nestedSchema, err := convertSchemaRecursive(valueMap, toolIndex, nestedPath)
			if err != nil {
				return nil, err
			}
			schema.Properties[propName] = nestedSchema
		}
	} else if schema.Type == genai.TypeObject && propertyPath == "" {
		return nil, fmt.Errorf("tool [%d]: expected to find a map of properties", toolIndex)
	}

	if items, ok := schemaMap["items"]; ok && schema.Type == genai.TypeArray {
		itemMap, ok := items.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("tool [%d], property [%s]: expect to find a map for array items", toolIndex, propertyPath)
		}
		itemsSchema, err := convertSchemaRecursive(itemMap, toolIndex, propertyPath+"[]")
		if err != nil {
			return nil, err
		}
		schema.Items = itemsSchema
	}

	if required, ok := schemaMap["required"]; ok {
		if rs, ok := required.([]string); ok {
			schema.Required = rs
		} else if ri, ok := required.([]interface{}); ok {
			rs := make([]string, 0, len(ri))
			for _, r := range ri {
				rString, ok := r.(string)
				if !ok {
					return nil, fmt.Errorf("tool [%d], property [%s]: expected string for required", toolIndex, propertyPath)
				}
				rs = append(rs, rString)
			}
			schema.Required = rs
		}
	}

	return schema, nil
}

func convertTools(tools []api.Tool) ([]*genai.Tool, error) {
	genaiFuncDecls := make([]*genai.FunctionDeclaration, 0, len(tools))
	for i, tool := range tools {
		if tool.Type != "function" {
			return nil, fmt.Errorf("tool [%d]: unsupported type %q, want 'function'", i, tool.Type)
		}
		genaiFuncDecl := &genai.FunctionDeclaration{
			Name:        tool.Function.Name,
			Description: tool.Function.Description,
		}
		schema, err := convertSchemaRecursive(tool.Function.Parameters, i, "")
		if err != nil {
			return nil, err
		}
		genaiFuncDecl.Parameters = schema
		genaiFuncDecls = append(genaiFuncDecls, genaiFuncDecl)
	}
	if len(genaiFuncDecls) == 0 {
		return nil, nil
	}
	return []*genai.Tool{{FunctionDeclarations: genaiFuncDecls}}, nil
}

func convertToolSchemaType(ty string) genai.Type {
	switch ty {
	case "object":
		return genai.TypeObject
	case "string":
		return genai.TypeString
	case "number":
		return genai.TypeNumber
	case "integer":
		return genai.TypeInteger
	case "boolean":
		return genai.TypeBoolean
	case "array":
		return genai.TypeArray
	default:
		return genai.TypeUnspecified
	}
}

func showContent(w io.Writer, cs []*genai.Content) {
	fmt.Fprintf(w, "Content (len=%v)\n", len(cs))
	for i, c := range cs {
		fmt.Fprintf(w, "[%d]: Role=%s\n", i, c.Role)
		for j, p := range c.Parts {
			fmt.Fprintf(w, "  Parts[%v]: ", j)
			switch pp := p.(type) {
			case genai.Text:
				fmt.Fprintf(w, "Text %q\n", pp)
			case genai.Blob:
				fmt.Fprintf(w, "Blob MIME=%q, size=%d\n", pp.MIMEType, len(pp.Data))
			case genai.FunctionCall:
				fmt.Fprintf(w, "FunctionCall Name=%v, Args=%v\n", pp.Name, pp.Args)
			case genai.FunctionResponse:
				fmt.Fprintf(w, "FunctionResponse Name=%v Response=%v\n", pp.Name, pp.Response)
			default:
				fmt.Fprintf(w, "unknown type %T\n", pp)
			}
		}
	}
}
