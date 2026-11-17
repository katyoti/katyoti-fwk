// SPDX-License-Identifier: Apache-2.0

package api

import "context"

// ModelInfo identifies a model instance and its capabilities.
type ModelInfo struct {
	Provider  string `json:"provider"`
	ModelID   string `json:"modelId"`
	Name      string `json:"name"`
	Reasoning bool   `json:"reasoning,omitempty"`
	// CostInputPerMTok / CostOutputPerMTok are the configured $/MTok
	// rates from LLMProviders.yaml. Surfaced here (informational) so
	// consumers — e.g. the GUI status bar — compute live spend from
	// the authoritative config instead of a hardcoded table. Zero when
	// the model config doesn't declare a cost.
	CostInputPerMTok  float64 `json:"costInputPerMTok,omitempty"`
	CostOutputPerMTok float64 `json:"costOutputPerMTok,omitempty"`
	// CostCacheReadPerMTok / CostCacheWritePerMTok are the cache pricing
	// components ($/MTok). Zero → ComputeCost derives Anthropic-convention
	// defaults from the input rate (read 0.10×, write 1.25×).
	CostCacheReadPerMTok  float64 `json:"costCacheReadPerMTok,omitempty"`
	CostCacheWritePerMTok float64 `json:"costCacheWritePerMTok,omitempty"`
	// ContextWindow is the model's total context size (tokens), from
	// config. Informational — lets the GUI show context fill.
	ContextWindow int `json:"contextWindow,omitempty"`
}

// Cost is the per-call cost breakdown in USD, computed at the source (the
// model knows its own pricing) so consumers never replicate provider-specific
// formulas. Cache reads/writes are separate line items because providers
// price them on different multipliers (and some — OpenAI — don't bill writes).
type Cost struct {
	InputUSD      float64 `json:"inputUsd,omitempty"`
	OutputUSD     float64 `json:"outputUsd,omitempty"`
	CacheReadUSD  float64 `json:"cacheReadUsd,omitempty"`
	CacheWriteUSD float64 `json:"cacheWriteUsd,omitempty"`
	TotalUSD      float64 `json:"totalUsd,omitempty"`
}

// ComputeCost prices a single call's token usage using this model's configured
// rates. Anthropic-style accounting: input_tokens does NOT include cache
// tokens, so each component is billed separately. Zero cache rates fall back
// to the Anthropic convention (read 0.10× input, write 1.25× input) so models
// configured before cache pricing existed still produce a sensible figure.
func (mi ModelInfo) ComputeCost(inputTok, outputTok, cacheReadTok, cacheWriteTok int) Cost {
	readRate := mi.CostCacheReadPerMTok
	if readRate == 0 {
		readRate = 0.10 * mi.CostInputPerMTok
	}
	writeRate := mi.CostCacheWritePerMTok
	if writeRate == 0 {
		writeRate = 1.25 * mi.CostInputPerMTok
	}
	const perMTok = 1.0 / 1_000_000.0
	c := Cost{
		InputUSD:      float64(inputTok) * mi.CostInputPerMTok * perMTok,
		OutputUSD:     float64(outputTok) * mi.CostOutputPerMTok * perMTok,
		CacheReadUSD:  float64(cacheReadTok) * readRate * perMTok,
		CacheWriteUSD: float64(cacheWriteTok) * writeRate * perMTok,
	}
	c.TotalUSD = c.InputUSD + c.OutputUSD + c.CacheReadUSD + c.CacheWriteUSD
	return c
}

// Model is the LLM interface. All consumers use this.
type Model interface {

	// Info returns provider and model identification.
	Info() ModelInfo

	// GenerateContent generates content from a sequence of messages.
	GenerateContent(ctx context.Context, messages []MessageContent, options ...CallOption) (*ContentResponse, error)
}

// StreamingModel extends Model with streaming support.
type StreamingModel interface {
	Model

	// GenerateContentStream starts a streaming response.
	GenerateContentStream(ctx context.Context, messages []MessageContent, options ...CallOption) (Stream, error)
}

// ModelType is a pre-configured model factory (Level 2).
type ModelType interface {
	Name() string
	ProviderName() string
	CreateModel(overrides map[string]any) (Model, error)
}

// LLMProviders resolves model type names to configured LLM model types.
// Service name matches config file LLMProviders.yaml.
type LLMProviders interface {
	GetModelType(name string) (ModelType, error)
	CreateModel(name string, overrides map[string]any) (Model, error)
	DefaultModelName() string
	ListModelTypes() []string
}
