// SPDX-License-Identifier: Apache-2.0

package anthropic

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

// anthropicErrorMappings defines the error mappings for Anthropic.
var anthropicErrorMappings = []errorMapping{
	{
		patterns: []string{"invalid api key", "authentication failed", "401"},
		code:     api.ErrCodeAuthentication,
		message:  "Invalid or missing API key",
	},
	{
		patterns: []string{"rate limit", "too many requests", "429"},
		code:     api.ErrCodeRateLimit,
		message:  "Rate limit exceeded",
	},
	{
		patterns: []string{"model not found", "invalid model"},
		code:     api.ErrCodeResourceNotFound,
		message:  "Model not found",
	},
	{
		patterns: []string{"maximum tokens", "context window"},
		code:     api.ErrCodeTokenLimit,
		message:  "Token limit exceeded",
	},
	{
		patterns: []string{"content blocked", "safety violation"},
		code:     api.ErrCodeContentFilter,
		message:  "Content blocked by safety filter",
	},
	{
		patterns: []string{"credit limit", "quota exceeded"},
		code:     api.ErrCodeQuotaExceeded,
		message:  "API quota exceeded",
	},
	{
		patterns: []string{"invalid request", "400"},
		code:     api.ErrCodeInvalidRequest,
		message:  "Invalid request",
	},
	{
		patterns: []string{"overloaded", "503", "service unavailable"},
		code:     api.ErrCodeProviderUnavailable,
		message:  "Anthropic service temporarily unavailable",
	},
}

// MapError maps Anthropic-specific errors to standardized error codes.
func MapError(err error) error {
	if err == nil {
		return nil
	}

	errStr := strings.ToLower(err.Error())

	// Check each error mapping
	for _, mapping := range anthropicErrorMappings {
		for _, pattern := range mapping.patterns {
			if strings.Contains(errStr, pattern) {
				return api.NewError(mapping.code, "anthropic", mapping.message).WithCause(err)
			}
		}
	}

	// Use the generic error mapper for unrecognized errors
	mapper := api.NewErrorMapper("anthropic")
	return mapper.Map(err)
}
