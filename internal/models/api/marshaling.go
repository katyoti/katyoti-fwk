// SPDX-License-Identifier: Apache-2.0

package api

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
)

func (mc MessageContent) MarshalJSON() ([]byte, error) {
	hasSingleTextPart := false
	if len(mc.Parts) == 1 {
		_, hasSingleTextPart = mc.Parts[0].(TextContent)
	}
	if hasSingleTextPart {
		tp, _ := mc.Parts[0].(TextContent)
		return json.Marshal(struct {
			Role ChatMessageType `json:"role"`
			Text string          `json:"text"`
		}{Role: mc.Role, Text: tp.Text})
	}

	return json.Marshal(struct {
		Role  ChatMessageType `json:"role"`
		Parts []ContentPart   `json:"parts"`
	}{
		Role:  mc.Role,
		Parts: mc.Parts,
	})
}

func (mc *MessageContent) UnmarshalJSON(data []byte) error {
	var m struct {
		Role  ChatMessageType `json:"role"`
		Text  string          `json:"text"`
		Parts []struct {
			Type     string `json:"type"`
			Text     string `json:"text,omitempty"`
			ImageURL struct {
				URL    string `json:"url"`
				Detail string `json:"detail,omitempty"`
			} `json:"image_url,omitempty"`
			Binary struct {
				Data     string `json:"data"`
				MIMEType string `json:"mime_type"`
			} `json:"binary,omitempty"`
			ID       string `json:"id"`
			ToolCall struct {
				ID           string        `json:"id"`
				Type         string        `json:"type"`
				FunctionCall *FunctionCall `json:"function"`
			} `json:"tool_call"`
			ToolResponse struct {
				ToolCallID string `json:"tool_call_id"`
				Name       string `json:"name"`
				Content    string `json:"content"`
			} `json:"tool_response"`
			CacheControl *CacheControl `json:"cache_control,omitempty"`
		} `json:"parts"`
	}
	if err := json.Unmarshal(data, &m); err != nil {
		return err
	}
	mc.Role = m.Role

	for _, part := range m.Parts {
		switch part.Type {
		case "text", "":
			mc.Parts = append(mc.Parts, TextContent{Text: part.Text})
		case "image_url":
			mc.Parts = append(mc.Parts, ImageURLContent{
				URL:    part.ImageURL.URL,
				Detail: part.ImageURL.Detail,
			})
		case "binary":
			decoded, err := base64.StdEncoding.DecodeString(part.Binary.Data)
			if err != nil {
				return fmt.Errorf("failed to decode binary data: %w", err)
			}
			mc.Parts = append(mc.Parts, BinaryContent{MIMEType: part.Binary.MIMEType, Data: decoded})
		case "tool_call":
			mc.Parts = append(mc.Parts, ToolCall{
				ID:           part.ToolCall.ID,
				Type:         part.ToolCall.Type,
				FunctionCall: part.ToolCall.FunctionCall,
			})
		case "tool_response":
			mc.Parts = append(mc.Parts, ToolCallResponse{
				ToolCallID: part.ToolResponse.ToolCallID,
				Name:       part.ToolResponse.Name,
				Content:    part.ToolResponse.Content,
			})
		default:
			return fmt.Errorf("unknown content type: '%s'", part.Type)
		}
		// A part with caching instructions (CachedContent) wraps the part.
		if part.CacheControl != nil {
			last := len(mc.Parts) - 1
			mc.Parts[last] = WithCacheControl(mc.Parts[last], part.CacheControl)
		}
	}
	// Special case: handle single text part directly:
	if len(mc.Parts) == 0 && m.Text != "" {
		mc.Parts = []ContentPart{TextContent{Text: m.Text}}
	}
	return nil
}

// MarshalJSON writes a part with caching instructions as the part it wraps
// (with its "type") plus its "cache_control" — read back by
// MessageContent.UnmarshalJSON. CacheControl.Duration is not written (json:"-").
func (cc CachedContent) MarshalJSON() ([]byte, error) {
	inner, err := json.Marshal(cc.ContentPart)
	if err != nil {
		return nil, err
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(inner, &m); err != nil {
		return nil, fmt.Errorf("cached content: the wrapped part is not a JSON object: %w", err)
	}
	if cc.CacheControl != nil {
		ctl, err := json.Marshal(cc.CacheControl)
		if err != nil {
			return nil, err
		}
		m["cache_control"] = ctl
	}
	return json.Marshal(m)
}

func (tc TextContent) MarshalJSON() ([]byte, error) {
	m := map[string]string{
		"type": "text",
		"text": tc.Text,
	}
	return json.Marshal(m)
}

func (tc *TextContent) UnmarshalJSON(data []byte) error {
	var m map[string]string
	if err := json.Unmarshal(data, &m); err != nil {
		return err
	}
	if m["type"] != "text" {
		return fmt.Errorf("invalid type for TextContent: %v", m["type"])
	}
	tc.Text = m["text"]
	return nil
}

func (iuc ImageURLContent) MarshalJSON() ([]byte, error) {
	r := struct {
		Type     string            `json:"type"`
		ImageURL map[string]string `json:"image_url"`
	}{
		Type: "image_url",
		ImageURL: map[string]string{
			"url": iuc.URL,
		},
	}
	if iuc.Detail != "" {
		r.ImageURL["detail"] = iuc.Detail
	}
	return json.Marshal(r)
}

func (iuc *ImageURLContent) UnmarshalJSON(data []byte) error {
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return err
	}
	_, ok := m["type"].(string)
	if !ok {
		return fmt.Errorf(`missing "type" field in ImageURLContent`)
	}
	imageURL, ok := m["image_url"].(map[string]any)
	if !ok {
		return fmt.Errorf("invalid image_url field in ImageURLContent")
	}
	url, ok := imageURL["url"].(string)
	if !ok {
		return fmt.Errorf("invalid url field in ImageURLContent")
	}
	detail, ok := imageURL["detail"].(string)
	if ok {
		iuc.Detail = detail
	}
	iuc.URL = url
	return nil
}

func (bc BinaryContent) MarshalJSON() ([]byte, error) {
	m := struct {
		Type   string            `json:"type"`
		Binary map[string]string `json:"binary"`
	}{
		Type: "binary",
		Binary: map[string]string{
			"mime_type": bc.MIMEType,
			"data":      base64.StdEncoding.EncodeToString(bc.Data),
		},
	}
	return json.Marshal(m)
}

func (bc *BinaryContent) UnmarshalJSON(data []byte) error {
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		return err
	}
	if m["type"] != "binary" {
		return fmt.Errorf("invalid type for BinaryContent: %v", m["type"])
	}
	binary, ok := m["binary"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid binary field in BinaryContent")
	}
	mimeType, ok := binary["mime_type"].(string)
	if !ok {
		return fmt.Errorf("invalid mime_type field in BinaryContent")
	}
	encodedData, ok := binary["data"].(string)
	if !ok {
		return fmt.Errorf("invalid data field in BinaryContent")
	}
	enc, err := base64.StdEncoding.DecodeString(encodedData)
	if err != nil {
		return fmt.Errorf("error decoding base64 data: %w", err)
	}
	bc.MIMEType = mimeType
	bc.Data = enc
	return nil
}

func (tc ToolCall) MarshalJSON() ([]byte, error) {
	fc, err := json.Marshal(tc.FunctionCall)
	if err != nil {
		return nil, err
	}
	m := struct {
		Type     string         `json:"type"`
		ToolCall map[string]any `json:"tool_call"`
	}{
		Type: "tool_call",
		ToolCall: map[string]any{
			"id":       tc.ID,
			"type":     tc.Type,
			"function": json.RawMessage(fc),
		},
	}
	return json.Marshal(m)
}

// UnmarshalJSON reads back what MarshalJSON writes. The function call is
// decoded from its raw JSON: read through a map[string]any it arrived as a
// map, never as json.RawMessage, and its name and arguments were lost — a
// saved conversation came back with nameless tool calls.
func (tc *ToolCall) UnmarshalJSON(data []byte) error {
	var m struct {
		Type     *string `json:"type"`
		ToolCall *struct {
			ID       *string         `json:"id"`
			Type     *string         `json:"type"`
			Function json.RawMessage `json:"function"`
		} `json:"tool_call"`
	}
	if err := json.Unmarshal(data, &m); err != nil {
		return err
	}
	if m.Type == nil {
		return fmt.Errorf(`missing "type" field in ToolCall`)
	}
	if m.ToolCall == nil {
		return fmt.Errorf("invalid tool_call field in ToolCall")
	}
	if m.ToolCall.ID == nil {
		return fmt.Errorf("invalid id field in ToolCall")
	}
	if m.ToolCall.Type == nil {
		return fmt.Errorf("invalid type field in ToolCall")
	}
	var fc FunctionCall
	if len(m.ToolCall.Function) > 0 && string(m.ToolCall.Function) != "null" {
		if err := json.Unmarshal(m.ToolCall.Function, &fc); err != nil {
			return fmt.Errorf("error unmarshalling function call: %w", err)
		}
	}
	tc.ID = *m.ToolCall.ID
	tc.Type = *m.ToolCall.Type
	tc.FunctionCall = &fc
	return nil
}

func (tc ToolCallResponse) MarshalJSON() ([]byte, error) {
	m := struct {
		Type         string            `json:"type"`
		ToolResponse map[string]string `json:"tool_response"`
	}{
		Type: "tool_response",
		ToolResponse: map[string]string{
			"tool_call_id": tc.ToolCallID,
			"name":         tc.Name,
			"content":      tc.Content,
		},
	}
	return json.Marshal(m)
}

func (tc *ToolCallResponse) UnmarshalJSON(data []byte) error {
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return err
	}
	if m["type"] != "tool_response" {
		return fmt.Errorf("invalid type for ToolCallResponse: %v", m["type"])
	}
	tr, ok := m["tool_response"].(map[string]any)
	if !ok {
		return fmt.Errorf("invalid tool_response field in ToolCallResponse")
	}
	toolCallID, ok := tr["tool_call_id"].(string)
	if !ok {
		return fmt.Errorf("invalid tool_call_id field in ToolCallResponse")
	}
	name, ok := tr["name"].(string)
	if !ok {
		return fmt.Errorf("invalid name field in ToolCallResponse")
	}
	content, ok := tr["content"].(string)
	if !ok {
		return fmt.Errorf("invalid content field in ToolCallResponse")
	}
	tc.ToolCallID = toolCallID
	tc.Name = name
	tc.Content = content
	return nil
}
