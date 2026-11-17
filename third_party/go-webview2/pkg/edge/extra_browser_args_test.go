//go:build windows

package edge

import "testing"

// The program's extra arguments follow the framework's own; none, the
// framework's alone.
func TestExtraBrowserArgsFollowTheFrameworksOwn(t *testing.T) {
	ExtraBrowserArgs = nil
	if got := browserArguments([]string{"--disable-gpu"}); got != "--disable-gpu" {
		t.Fatalf("no extra: %q", got)
	}
	ExtraBrowserArgs = []string{"--remote-debugging-port=9222"}
	defer func() { ExtraBrowserArgs = nil }()
	if got := browserArguments([]string{"--disable-gpu"}); got != "--disable-gpu --remote-debugging-port=9222" {
		t.Fatalf("with extra: %q", got)
	}
}
