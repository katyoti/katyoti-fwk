// SPDX-License-Identifier: Apache-2.0

package api

import (
	"strings"
	"testing"
)

func TestLanguagePrompt(t *testing.T) {
	fr := LanguagePrompt("fr")
	if !strings.Contains(fr, "French (fr)") || !strings.HasPrefix(fr, "<person_language>") {
		t.Errorf("fr block = %q", fr)
	}
	for _, lang := range SupportedLanguages {
		if LanguagePrompt(lang) == "" {
			t.Errorf("supported language %q has no block", lang)
		}
	}
	if LanguagePrompt("") != "" || LanguagePrompt("de") != "" {
		t.Error("an empty or unsupported language has a block")
	}
}
