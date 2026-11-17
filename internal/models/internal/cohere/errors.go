// SPDX-License-Identifier: Apache-2.0

package cohere

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

// cohereErrorMappings defines the error mappings for Cohere.
var cohereErrorMappings = []errorMapping{
	{
		patterns: []string{"invalid api key", "unauthorized", "401"},
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
		patterns: []string{"context length", "too many tokens"},
		code:     api.ErrCodeTokenLimit,
		message:  "Token limit exceeded",
	},
	{
		patterns: []string{"invalid request", "400"},
		code:     api.ErrCodeInvalidRequest,
		message:  "Invalid request",
	},
	{
		patterns: []string{"quota exceeded", "billing"},
		code:     api.ErrCodeQuotaExceeded,
		message:  "API quota exceeded",
	},
	{
		patterns: []string{"service unavailable", "503"},
		code:     api.ErrCodeProviderUnavailable,
		message:  "Cohere service temporarily unavailable",
	},
	{
		patterns: []string{"internal error", "500"},
		code:     api.ErrCodeProviderUnavailable,
		message:  "Cohere service error",
	},
}

// MapError maps Cohere-specific errors to standardized error codes.
func MapError(err error) error {
	if err == nil {
		return nil
	}

	errStr := strings.ToLower(err.Error())

	// Check each error mapping
	for _, mapping := range cohereErrorMappings {
		for _, pattern := range mapping.patterns {
			if strings.Contains(errStr, pattern) {
				return api.NewError(mapping.code, "cohere", mapping.message).WithCause(err)
			}
		}
	}

	// Use the generic error mapper for unrecognized errors
	mapper := api.NewErrorMapper("cohere")
	return mapper.Map(err)
}
