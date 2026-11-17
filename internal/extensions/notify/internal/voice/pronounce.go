// SPDX-License-Identifier: Apache-2.0

package voice

import (
	"context"
	"log/slog"
	"regexp"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
)

// TTSPronunciationsExtension registers TTSPronunciations as a DI service (config: TTSPronunciations.yaml).
var TTSPronunciationsExtension = application.DeclareService[*TTSPronunciations, TTSPronunciationsConfig](
	func(_ context.Context, cfg *TTSPronunciationsConfig) (*TTSPronunciations, error) {
		return NewTTSPronunciations(cfg.Rules), nil
	},
)

type compiledRule struct {
	pattern     *regexp.Regexp
	replacement string
}

// TTSPronunciations applies pronunciation substitution rules to text.
// Created from pre-loaded rules — the caller handles config loading.
type TTSPronunciations struct {
	rules []compiledRule
}

// NewTTSPronunciations creates a TTSPronunciations from pre-loaded rules.
func NewTTSPronunciations(rawRules []PronunciationRule) *TTSPronunciations {
	d := &TTSPronunciations{}
	for _, rule := range rawRules {
		if rule.Term == "" || rule.Phonetic == "" {
			continue
		}
		escaped := regexp.QuoteMeta(rule.Term)
		patternStr := `(?i)(?:^|(\W))` + escaped + `(?:(\W)|$)`
		re, err := regexp.Compile(patternStr)
		if err != nil {
			slog.Warn("failed to compile pronunciation rule", "term", rule.Term, "error", err)
			continue
		}
		d.rules = append(d.rules, compiledRule{
			pattern:     re,
			replacement: rule.Phonetic,
		})
	}
	return d
}

// Apply applies all pronunciation rules to the text.
func (d *TTSPronunciations) Apply(text string) string {
	for _, rule := range d.rules {
		text = rule.pattern.ReplaceAllStringFunc(text, func(match string) string {
			submatch := rule.pattern.FindStringSubmatch(match)
			prefix := ""
			suffix := ""
			if len(submatch) > 1 {
				prefix = submatch[1]
			}
			if len(submatch) > 2 {
				suffix = submatch[2]
			}
			return prefix + rule.replacement + suffix
		})
	}
	return text
}

var sanitizePatterns = []struct {
	pattern *regexp.Regexp
	replace string
}{
	{regexp.MustCompile(`(?i)<script[^>]*>.*?</script>`), ""},
	{regexp.MustCompile(`\.\.\/`), ""},
	{regexp.MustCompile(`\*\*`), ""},
	{regexp.MustCompile(`\*`), ""},
	{regexp.MustCompile("`([^`]*)`"), "$1"},
	{regexp.MustCompile(`(?m)^#{1,6}\s+`), ""},
	{regexp.MustCompile(`[;&|><$\\]`), ""},
}

// ValidateInput validates and sanitizes text input for TTS.
// Returns (ok, errorMessage, sanitizedText).
func ValidateInput(value string, maxLength int) (bool, string, string) {
	if maxLength <= 0 {
		maxLength = 500
	}
	if len(value) == 0 {
		return false, "input is empty", ""
	}
	if len(value) > maxLength {
		return false, "input too long", ""
	}
	sanitized := SanitizeForSpeech(value, maxLength)
	if len(sanitized) == 0 {
		return false, "input is empty after sanitization", ""
	}
	return true, "", sanitized
}

// SanitizeForSpeech removes markdown and dangerous content from text.
func SanitizeForSpeech(text string, maxLen int) string {
	if maxLen <= 0 {
		maxLen = 500
	}
	for _, sp := range sanitizePatterns {
		text = sp.pattern.ReplaceAllString(text, sp.replace)
	}
	text = strings.TrimSpace(text)
	if len(text) > maxLen {
		text = text[:maxLen]
	}
	return text
}
