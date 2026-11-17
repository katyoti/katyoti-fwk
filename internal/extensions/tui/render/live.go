// SPDX-License-Identifier: Apache-2.0

package render

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"golang.org/x/term"

	"github.com/katyoti/katyoti-fwk/internal/core/progress"
	schemaapi "github.com/katyoti/katyoti-fwk/internal/core/schema/api"
)

// LiveRenderer is the terminal's live view of a long method: it implements
// progress.Emitter so a long-running method can push data snapshots, and it
// re-renders the rich screen (tree-table/table/tree/text — via the same
// Render()) IN PLACE on each snapshot. On a TTY it redraws by moving the cursor
// up over the previous frame and clearing; off a TTY it stays silent during
// updates and prints the result once via Finalize (so piped / --out text|json
// output is unchanged and clean).
//
// Frame emission is throttled (minInterval) so a fast-emitting method doesn't
// thrash the terminal; Finalize always paints the final state, covering any
// throttled-out last snapshot.
type LiveRenderer struct {
	ast *schemaapi.Type
	ctx RenderCtx
	out io.Writer

	mu           sync.Mutex
	lastLines    int
	lastAt       time.Time
	started      bool
	cursorHidden bool
	minInterval  time.Duration
	// size is the terminal's columns and rows (0 when unknown): a live frame
	// is drawn within them, since the cursor cannot move up past the top of
	// the window nor over rows a long line wrapped onto.
	size func() (cols, rows int)
}

var _ progress.Emitter = (*LiveRenderer)(nil)

// NewLiveRenderer builds a live renderer for the given output AST + render
// context, writing to out (typically cmd.OutOrStdout()).
func NewLiveRenderer(ast *schemaapi.Type, ctx RenderCtx, out io.Writer) *LiveRenderer {
	return &LiveRenderer{
		ast:         ast,
		ctx:         ctx,
		out:         out,
		minInterval: 120 * time.Millisecond,
		size:        termSize(out),
	}
}

// termSize reads out's terminal size when out is a terminal.
func termSize(out io.Writer) func() (int, int) {
	f, ok := out.(interface{ Fd() uintptr })
	if !ok {
		return func() (int, int) { return 0, 0 }
	}
	return func() (int, int) {
		c, r, err := term.GetSize(int(f.Fd()))
		if err != nil {
			return 0, 0
		}
		return c, r
	}
}

// Emit renders a snapshot in place. Off a TTY it is a no-op (the final frame is
// printed by Finalize). Snapshots arriving within minInterval of the last frame
// are dropped to avoid terminal thrash.
func (r *LiveRenderer) Emit(snapshot any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.ctx.IsTTY {
		return
	}
	now := time.Now()
	if r.started && now.Sub(r.lastAt) < r.minInterval {
		return
	}
	body, err := json.Marshal(snapshot)
	if err != nil {
		return
	}
	r.draw(body, false)
	r.lastAt = now
	r.started = true
}

// Finalize paints the terminal frame for the final result. On a TTY it redraws
// in place (covering a throttled-out last snapshot) and restores the cursor;
// off a TTY it prints the result once as a plain static render.
func (r *LiveRenderer) Finalize(body json.RawMessage) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.ctx.IsTTY {
		_ = Render(r.ast, body, r.ctx, r.out)
		return
	}
	r.draw(body, true)
	if r.cursorHidden {
		_, _ = io.WriteString(r.out, ansiShowCursor)
		r.cursorHidden = false
	}
}

// draw renders body into a buffer, then repaints it in place: move the cursor
// up over the previous frame, clear to end of screen, and print the new frame.
// A live frame taller than the window shows its last rows that fit (the cursor
// cannot move above the window's top, so a taller one would pile up frames);
// the final frame is printed whole. Caller holds r.mu.
func (r *LiveRenderer) draw(body json.RawMessage, final bool) {
	var buf bytes.Buffer
	if err := Render(r.ast, body, r.ctx, &buf); err != nil {
		return
	}
	frame := buf.String()
	cols, rows := r.size()
	if cols <= 0 {
		cols = r.ctx.Width
	}
	if !final && rows > 1 {
		frame = fitRows(frame, cols, rows-1)
	}
	if !r.cursorHidden {
		_, _ = io.WriteString(r.out, ansiHideCursor)
		r.cursorHidden = true
	}
	if r.lastLines > 0 {
		// Cursor up N lines, then clear from cursor to end of screen.
		_, _ = fmt.Fprintf(r.out, "\x1b[%dA\x1b[0J", r.lastLines)
	}
	_, _ = io.WriteString(r.out, frame)
	r.lastLines = countRows(frame, cols)
}

// fitRows is frame's last lines that take at most max rows on a cols-wide
// terminal.
func fitRows(frame string, cols, max int) string {
	if countRows(frame, cols) <= max {
		return frame
	}
	lines := strings.Split(strings.TrimSuffix(frame, "\n"), "\n")
	used, i := 0, len(lines)
	for i > 0 {
		n := lineRows(lines[i-1], cols)
		if used+n > max {
			break
		}
		used += n
		i--
	}
	return strings.Join(lines[i:], "\n") + "\n"
}

// countRows counts the physical rows a frame occupies on a cols-wide terminal
// (cols <= 0: no soft-wrap). A trailing-newline frame ends the cursor on a
// fresh row below the content; a frame without one leaves the cursor on the
// last content row.
func countRows(s string, cols int) int {
	if s == "" {
		return 0
	}
	n := 0
	for _, l := range strings.Split(strings.TrimSuffix(s, "\n"), "\n") {
		n += lineRows(l, cols)
	}
	return n
}

// lineRows is how many rows one line takes, wrapped at cols.
func lineRows(l string, cols int) int {
	w := displayWidth(l)
	if cols <= 0 || w <= cols {
		return 1
	}
	return (w + cols - 1) / cols
}

// countRenderedLines counts a frame's rows with no soft-wrap.
func countRenderedLines(s string) int { return countRows(s, 0) }

const (
	ansiHideCursor = "\x1b[?25l"
	ansiShowCursor = "\x1b[?25h"
)
