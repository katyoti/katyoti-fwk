// SPDX-License-Identifier: Apache-2.0

package extraction

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// AnthropicVisionClient calls the Anthropic Messages API with image input.
type AnthropicVisionClient struct {
	APIKey string
	Model  string
}

// NewAnthropicVisionClient creates a client from environment.
func NewAnthropicVisionClient() *AnthropicVisionClient {
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	model := "claude-haiku-4-5-20251001"
	if m := os.Getenv("ANTHROPIC_VISION_MODEL"); m != "" {
		model = m
	}
	return &AnthropicVisionClient{APIKey: apiKey, Model: model}
}

// Available returns true if the API key is set.
func (c *AnthropicVisionClient) Available() bool {
	return c.APIKey != ""
}

// DescribeImage sends an image to Claude Vision and returns the markdown description.
func (c *AnthropicVisionClient) DescribeImage(ctx context.Context, imagePath string, prompt string) (string, error) {
	if c.APIKey == "" {
		return "", fmt.Errorf("ANTHROPIC_API_KEY not set")
	}

	// Read and base64-encode the image
	imgData, err := os.ReadFile(imagePath)
	if err != nil {
		return "", fmt.Errorf("read image: %w", err)
	}
	b64 := base64.StdEncoding.EncodeToString(imgData)

	// Detect media type from extension
	mediaType := imageMediaType(filepath.Ext(imagePath))

	// Build request
	reqBody := anthropicRequest{
		Model:     c.Model,
		MaxTokens: 4096,
		Messages: []anthropicMessage{
			{
				Role: "user",
				Content: []anthropicBlock{
					{
						Type: "image",
						Source: &anthropicImage{
							Type:      "base64",
							MediaType: mediaType,
							Data:      b64,
						},
					},
					{
						Type: "text",
						Text: prompt,
					},
				},
			},
		},
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST",
		"https://api.anthropic.com/v1/messages",
		bytes.NewReader(body),
	)
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("API call: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("anthropic API %d: %s", resp.StatusCode, string(respBody))
	}

	var result anthropicResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}
	if result.Error != nil {
		return "", fmt.Errorf("anthropic API: %s", result.Error.Message)
	}

	for _, block := range result.Content {
		if block.Type == "text" {
			return block.Text, nil
		}
	}
	return "", fmt.Errorf("no text in response")
}

// --- Anthropic API types ---

type anthropicRequest struct {
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	Messages  []anthropicMessage `json:"messages"`
}

type anthropicMessage struct {
	Role    string           `json:"role"`
	Content []anthropicBlock `json:"content"`
}

type anthropicBlock struct {
	Type   string          `json:"type"`
	Text   string          `json:"text,omitempty"`
	Source *anthropicImage `json:"source,omitempty"`
}

type anthropicImage struct {
	Type      string `json:"type"`
	MediaType string `json:"media_type"`
	Data      string `json:"data"`
}

type anthropicResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// --- OpenAI Vision Client ---

// OpenAIVisionClient calls the OpenAI Chat Completions API with image input.
type OpenAIVisionClient struct {
	APIKey string
	Model  string
}

func NewOpenAIVisionClient() *OpenAIVisionClient {
	apiKey := os.Getenv("OPENAI_API_KEY")
	model := "gpt-4o-mini"
	if m := os.Getenv("OPENAI_VISION_MODEL"); m != "" {
		model = m
	}
	return &OpenAIVisionClient{APIKey: apiKey, Model: model}
}

func (c *OpenAIVisionClient) Available() bool {
	return c.APIKey != ""
}

func (c *OpenAIVisionClient) DescribeImage(ctx context.Context, imagePath string, prompt string) (string, error) {
	if c.APIKey == "" {
		return "", fmt.Errorf("OPENAI_API_KEY not set")
	}

	imgData, err := os.ReadFile(imagePath)
	if err != nil {
		return "", fmt.Errorf("read image: %w", err)
	}
	b64 := base64.StdEncoding.EncodeToString(imgData)
	mediaType := imageMediaType(filepath.Ext(imagePath))

	reqBody := map[string]any{
		"model":      c.Model,
		"max_tokens": 4096,
		"messages": []map[string]any{
			{
				"role": "user",
				"content": []map[string]any{
					{
						"type": "image_url",
						"image_url": map[string]string{
							"url": fmt.Sprintf("data:%s;base64,%s", mediaType, b64),
						},
					},
					{
						"type": "text",
						"text": prompt,
					},
				},
			},
		},
	}

	body, _ := json.Marshal(reqBody)
	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.openai.com/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.APIKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("openai API: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("openai API %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}
	if len(result.Choices) == 0 {
		return "", fmt.Errorf("no choices in response")
	}
	return result.Choices[0].Message.Content, nil
}

// --- Multi-provider Vision Client ---

// NewVisionFn creates a vision function that tries Anthropic first, then OpenAI.
func NewVisionFn() func(ctx context.Context, imagePath string, prompt string) (string, error) {
	anthropic := NewAnthropicVisionClient()
	openai := NewOpenAIVisionClient()

	return func(ctx context.Context, imagePath string, prompt string) (string, error) {
		// Try Anthropic first
		if anthropic.Available() {
			result, err := anthropic.DescribeImage(ctx, imagePath, prompt)
			if err == nil {
				return result, nil
			}
			// If Anthropic fails (credits, etc.), fall through to OpenAI
		}
		// Try OpenAI
		if openai.Available() {
			return openai.DescribeImage(ctx, imagePath, prompt)
		}
		return "", fmt.Errorf("no vision API available (set ANTHROPIC_API_KEY or OPENAI_API_KEY)")
	}
}

func imageMediaType(ext string) string {
	switch strings.ToLower(ext) {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".bmp":
		return "image/bmp"
	default:
		return "image/png"
	}
}
