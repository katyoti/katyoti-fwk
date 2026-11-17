// SPDX-License-Identifier: Apache-2.0

package file_read

import (
	"context"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
)

// FileReadConfig holds the tunable limits and defaults applied by the
// file_read tool. The struct is registered as an application-scoped service
// (no YAML file needed in MVP — defaults are baked in via DefaultConfig()).
//
// Tests and embedding binaries can override the defaults by registering
// a custom *FileReadConfig instance via application.PutService before the
// tool resolves it.
//
// The fields map to spec docs/agent_tools/tool_file_read_spec.md §5:
//   - MaxBytes      → FR-FR-05 (default 256 KB)
//   - MaxTokens     → FR-FR-06 (default 25000)
//   - DefaultLines  → FR-FR-03 (default 2000)
//   - TokensPerChar → token-estimator divisor (chars/divisor ≈ tokens)
//
// All four fields are intentionally simple ints — no remote killswitch,
// no experiment infrastructure (FR-FR-07's full precedence chain is
// out of scope for MVP, deferred to future config work).
type FileReadConfig struct {
	// MaxBytes is the maximum file size accepted before any read is
	// attempted. Files larger than this are rejected with an error
	// suggesting offset+limit. Default: 256 KB.
	MaxBytes int

	// MaxTokens is the maximum estimated token count of the formatted
	// content returned to the LLM. Files producing more than this many
	// tokens are rejected after the read with an error suggesting
	// offset+limit. Default: 25000.
	MaxTokens int

	// DefaultLines is the default line count read when the caller does
	// not specify a limit. Default: 2000.
	DefaultLines int

	// TokensPerChar is the divisor used by the token estimator
	// (estimated_tokens = len(text) / TokensPerChar). 4 is the standard
	// chars-per-token rule of thumb for English-heavy text. Default: 4.
	TokensPerChar int
}

// DefaultConfig returns a *FileReadConfig populated with the spec
// defaults. Used by the DI factory and as a starting point for tests
// that want to override only one or two fields.
func DefaultConfig() *FileReadConfig {
	return &FileReadConfig{
		MaxBytes:      262144, // 256 KB
		MaxTokens:     25000,
		DefaultLines:  2000,
		TokensPerChar: 4,
	}
}

// EstimateTokens applies the configured chars-per-token ratio. The
// estimate is intentionally pessimistic-on-the-low-side: a small
// TokensPerChar (e.g. 3) will overcount tokens, a large one (e.g. 5)
// will undercount. The default of 4 is the conventional middle ground.
func (c *FileReadConfig) EstimateTokens(text string) int {
	if c.TokensPerChar <= 0 {
		return len(text) / 4
	}
	return len(text) / c.TokensPerChar
}

// FileReadConfigExtension registers *FileReadConfig as a simple DI
// service backed by DefaultConfig(). Embedding binaries that want to
// override the limits can register a custom instance before this
// extension runs, or replace the service after Run() via
// application.PutService.
var FileReadConfigExtension = application.DeclareSimpleService[*FileReadConfig](
	func(_ context.Context) (*FileReadConfig, error) {
		return DefaultConfig(), nil
	},
)
