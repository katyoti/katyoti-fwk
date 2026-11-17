// SPDX-License-Identifier: Apache-2.0

package gemini

import (
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/models/api"
)

// errorMapping represents a mapping from error patterns to error codes.
type errorMapping struct {
	patterns []string
	code     api.ErrorCode
	message  string
}

// googleAIErrorMappings defines the error mappings for Google AI.
var googleAIErrorMappings = []errorMapping{
	{
		patterns: []string{"invalid api key", "api key not valid", "401"},
		code:     api.ErrCodeAuthentication,
		message:  "Invalid or missing API key",
	},
	{
		patterns: []string{"quota exceeded", "rate limit", "429"},
		code:     api.ErrCodeRateLimit,
		message:  "Rate limit or quota exceeded",
	},
	{
		patterns: []string{"model not found", "404"},
		code:     api.ErrCodeResourceNotFound,
		message:  "Model not found",
	},
	{
		patterns: []string{"token limit", "context length"},
		code:     api.ErrCodeTokenLimit,
		message:  "Token limit exceeded",
	},
	{
		patterns: []string{"harmful content", "safety"},
		code:     api.ErrCodeContentFilter,
		message:  "Content blocked by safety filter",
	},
	{
		patterns: []string{"billing", "payment required"},
		code:     api.ErrCodeQuotaExceeded,
		message:  "Billing quota exceeded",
	},
	{
		patterns: []string{"invalid request", "400"},
		code:     api.ErrCodeInvalidRequest,
		message:  "Invalid request",
	},
	{
		patterns: []string{"service unavailable", "503"},
		code:     api.ErrCodeProviderUnavailable,
		message:  "Google AI service temporarily unavailable",
	},
}

// MapError maps Google AI-specific errors to standardized error codes.
func MapError(err error) error {
	if err == nil {
		return nil
	}

	errStr := strings.ToLower(err.Error())

	// Check each error mapping
	for _, mapping := range googleAIErrorMappings {
		for _, pattern := range mapping.patterns {
			if strings.Contains(errStr, pattern) {
				return api.NewError(mapping.code, "googleai", mapping.message).WithCause(err)
			}
		}
	}

	// Use the generic error mapper for unrecognized errors
	mapper := api.NewErrorMapper("googleai")
	return mapper.Map(err)
}
