// SPDX-License-Identifier: Apache-2.0

package anchor

import (
	"strings"
	"testing"
)

func TestNormalizeWhitespace(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"hello world", "hello world"},
		{"  leading  ", "leading"},
		{"trailing   \n\t", "trailing"},
		{"multi\n\nline\t\tspaces", "multi line spaces"},
		{"", ""},
		{"   ", ""},
		{"a\nb\nc", "a b c"},
		{"  MUST  not  fold  case  ", "MUST not fold case"},
	}
	for _, tc := range cases {
		if got := normalizeText(tc.in); got != tc.want {
			t.Errorf("normalize(%q) = %q want %q", tc.in, got, tc.want)
		}
	}
}

func TestHashOfDeterministic(t *testing.T) {
	a := hashOf("hello world")
	b := hashOf("hello world")
	if a != b {
		t.Errorf("non-deterministic hash: %q vs %q", a, b)
	}
	if !strings.HasPrefix(a, "sha256:") {
		t.Errorf("missing sha256: prefix: %q", a)
	}
	if got := hashOf(""); !strings.HasPrefix(got, "sha256:") {
		t.Errorf("empty input hash missing prefix: %q", got)
	}
	if hashOf("hello") == hashOf("hello world") {
		t.Errorf("collision on distinct inputs")
	}
}
