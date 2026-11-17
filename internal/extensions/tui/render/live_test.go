// SPDX-License-Identifier: Apache-2.0

package render

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestCountRenderedLines(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"", 0},
		{"a", 1},
		{"a\n", 1},
		{"a\nb\n", 2},
		{"a\nb", 2},
		{"a\nb\nc\n", 3},
	}
	for _, tc := range cases {
		if got := countRenderedLines(tc.in); got != tc.want {
			t.Errorf("countRenderedLines(%q) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

// TestLiveRenderer_TTYRedraw verifies the in-place redraw math: the first frame
// paints with no cursor-up; subsequent frames move up by the previous frame's
// line count and clear; Finalize restores the cursor.
func TestLiveRenderer_TTYRedraw(t *testing.T) {
	var buf bytes.Buffer
	ctx := RenderCtx{IsTTY: true, Width: 80}
	// nil AST + rich mode → JSON render (deterministic, 3 lines per {"n":X}).
	r := NewLiveRenderer(nil, ctx, &buf)
	r.minInterval = 0 // disable throttle so every Emit paints

	r.Emit(map[string]any{"n": 1})
	firstFrame := buf.String()
	if !strings.HasPrefix(firstFrame, ansiHideCursor) {
		t.Errorf("first frame should start by hiding the cursor; got %q", firstFrame)
	}
	if strings.Contains(strings.TrimPrefix(firstFrame, ansiHideCursor), "\x1b[") {
		t.Errorf("first frame must not emit a cursor-up (nothing to overwrite yet); got %q", firstFrame)
	}
	if r.lastLines != 3 {
		t.Fatalf("lastLines after first frame = %d, want 3 (pretty JSON of {n:1})", r.lastLines)
	}

	buf.Reset()
	r.Emit(map[string]any{"n": 2})
	second := buf.String()
	if !strings.Contains(second, "\x1b[3A\x1b[0J") {
		t.Errorf("second frame should move up 3 lines + clear; got %q", second)
	}

	buf.Reset()
	r.Finalize(json.RawMessage(`{"n":3}`))
	fin := buf.String()
	if !strings.Contains(fin, "\x1b[3A\x1b[0J") {
		t.Errorf("finalize should repaint in place (cursor-up 3 + clear); got %q", fin)
	}
	if !strings.HasSuffix(fin, ansiShowCursor) {
		t.Errorf("finalize should restore the cursor; got %q", fin)
	}
	if !strings.Contains(fin, `"n": 3`) {
		t.Errorf("finalize should render the final body; got %q", fin)
	}
}

// TestLiveRenderer_NonTTY verifies updates are silent off a TTY and Finalize
// prints the result once with no ANSI cursor control.
func TestLiveRenderer_NonTTY(t *testing.T) {
	var buf bytes.Buffer
	ctx := RenderCtx{IsTTY: false, Width: 80}
	r := NewLiveRenderer(nil, ctx, &buf)
	r.minInterval = 0

	r.Emit(map[string]any{"n": 1})
	r.Emit(map[string]any{"n": 2})
	if buf.Len() != 0 {
		t.Errorf("non-TTY updates should be silent; got %q", buf.String())
	}

	r.Finalize(json.RawMessage(`{"n":9}`))
	out := buf.String()
	if strings.Contains(out, "\x1b[") {
		t.Errorf("non-TTY finalize must not emit ANSI escapes; got %q", out)
	}
	if !strings.Contains(out, `"n": 9`) {
		t.Errorf("non-TTY finalize should print the result; got %q", out)
	}
}

// A live frame taller than the window, or with lines wider than it, still
// redraws in place: it is cut to the rows the window shows (the cursor cannot
// move above the window's top) and a wrapped line counts its rows; the final
// frame is printed whole.
func TestLiveRenderer_FrameTallerOrWiderThanTheWindow(t *testing.T) {
	var buf bytes.Buffer
	r := NewLiveRenderer(nil, RenderCtx{IsTTY: true, Width: 80}, &buf)
	r.minInterval = 0
	r.size = func() (int, int) { return 20, 5 }

	items := make([]int, 30) // pretty JSON: 32 lines
	r.Emit(map[string]any{"items": items})
	if r.lastLines > 4 {
		t.Fatalf("a live frame took %d rows of a 5-row window; want at most 4", r.lastLines)
	}
	buf.Reset()
	r.Emit(map[string]any{"items": items})
	if !strings.Contains(buf.String(), "\x1b[4A\x1b[0J") {
		t.Errorf("the second frame should move up the 4 rows drawn; got %q", buf.String())
	}
	buf.Reset()
	r.Finalize(json.RawMessage(`{"items":[0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0]}`))
	if n := strings.Count(buf.String(), "\n"); n < 32 {
		t.Errorf("the final frame should be printed whole (32 lines); got %d", n)
	}

	if got := countRows(strings.Repeat("x", 45)+"\n", 20); got != 3 {
		t.Errorf("a 45-column line on a 20-column window takes %d rows; want 3", got)
	}
}
