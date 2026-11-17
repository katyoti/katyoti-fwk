// SPDX-License-Identifier: Apache-2.0

package core

import (
	"strings"
	"testing"
)

// A config still holding `${{env:KEY}}` is refused by the key's name, not left
// to fail in the provider on the literal.
func TestAModelConfigWithAnUnsetKeyNamesIt(t *testing.T) {
	type cfg struct {
		APIKey  string
		BaseURL string
		Headers map[string]string
	}
	err := UnsetKeys(cfg{APIKey: "sk", BaseURL: "${{env:ANTHROPIC_FOUNDRY_BASE_URL}}", Headers: map[string]string{"x": "${{ env:OTHER }}"}})
	if err == nil || !strings.Contains(err.Error(), "ANTHROPIC_FOUNDRY_BASE_URL") || !strings.Contains(err.Error(), "OTHER") {
		t.Fatalf("UnsetKeys = %v; want both keys named", err)
	}
	if err := UnsetKeys(&cfg{APIKey: "sk", BaseURL: "https://example"}); err != nil {
		t.Fatalf("a resolved config was refused: %v", err)
	}
}
