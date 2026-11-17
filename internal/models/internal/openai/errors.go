// SPDX-License-Identifier: Apache-2.0

package openai

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

// openaiErrorMappings defines the error mappings for OpenAI.
var openaiErrorMappings = []errorMapping{
	{
		patterns: []string{"incorrect api key", "invalid api key", "api key not found"},
		code:     api.ErrCodeAuthentication,
		message:  "Invalid or missing API key",
	},
	{
		patterns: []string{"rate limit exceeded", "too many requests", "429"},
		code:     api.ErrCodeRateLimit,
		message:  "Rate limit exceeded",
	},
	{
		patterns: []string{"model not found", "no such model"},
		code:     api.ErrCodeResourceNotFound,
		message:  "Model not found",
	},
	{
		patterns: []string{"context length exceeded", "maximum context length"},
		code:     api.ErrCodeTokenLimit,
		message:  "Context length exceeded",
	},
	{
		patterns: []string{"content filtering", "content policy violation"},
		code:     api.ErrCodeContentFilter,
		message:  "Content filtered due to policy violation",
	},
	{
		patterns: []string{"quota exceeded", "billing hard limit"},
		code:     api.ErrCodeQuotaExceeded,
		message:  "API quota exceeded",
	},
	{
		patterns: []string{"invalid request", "400"},
		code:     api.ErrCodeInvalidRequest,
		message:  "Invalid request",
	},
	{
		patterns: []string{"service unavailable", "503"},
		code:     api.ErrCodeProviderUnavailable,
		message:  "OpenAI service temporarily unavailable",
	},
}

// MapError maps OpenAI-specific errors to standardized error codes.
func MapError(err error) error {
	if err == nil {
		return nil
	}

	errStr := strings.ToLower(err.Error())

	// Check each error mapping
	for _, mapping := range openaiErrorMappings {
		for _, pattern := range mapping.patterns {
			if strings.Contains(errStr, pattern) {
				return api.NewError(mapping.code, "openai", mapping.message).WithCause(err)
			}
		}
	}

	// Use the generic error mapper for unrecognized errors
	mapper := api.NewErrorMapper("openai")
	return mapper.Map(err)
}
