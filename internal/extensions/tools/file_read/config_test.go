// SPDX-License-Identifier: Apache-2.0

package file_read

import "testing"

func TestDefaultConfig_MatchesSpec(t *testing.T) {
	c := DefaultConfig()
	if c.MaxBytes != 262144 {
		t.Errorf("MaxBytes = %d, want 262144 (256 KB per FR-FR-05)", c.MaxBytes)
	}
	if c.MaxTokens != 25000 {
		t.Errorf("MaxTokens = %d, want 25000 (FR-FR-06)", c.MaxTokens)
	}
	if c.DefaultLines != 2000 {
		t.Errorf("DefaultLines = %d, want 2000 (FR-FR-03)", c.DefaultLines)
	}
	if c.TokensPerChar != 4 {
		t.Errorf("TokensPerChar = %d, want 4", c.TokensPerChar)
	}
}

func TestFileReadConfig_EstimateTokens(t *testing.T) {
	c := DefaultConfig()
	cases := []struct {
		text string
		want int
	}{
		{"", 0},
		{"abcd", 1},     // 4 chars / 4 = 1
		{"abcdefgh", 2}, // 8 / 4 = 2
		{"a", 0},        // 1 / 4 = 0 (integer truncation, intentional)
	}
	for _, tc := range cases {
		if got := c.EstimateTokens(tc.text); got != tc.want {
			t.Errorf("EstimateTokens(%q) = %d, want %d", tc.text, got, tc.want)
		}
	}
}

func TestFileReadConfig_EstimateTokens_HandlesZeroDivisor(t *testing.T) {
	// A misconfigured TokensPerChar=0 must not divide-by-zero — fall back
	// to the chars/4 default. Defensive contract for tests that build a
	// partial config struct.
	c := &FileReadConfig{TokensPerChar: 0}
	got := c.EstimateTokens("abcdefgh")
	if got != 2 {
		t.Errorf("EstimateTokens with TokensPerChar=0 = %d, want 2 (fallback to /4)", got)
	}
}
