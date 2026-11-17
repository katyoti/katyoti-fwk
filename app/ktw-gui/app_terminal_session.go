// SPDX-License-Identifier: Apache-2.0

package main

// In-app terminal-session bindings for the TERM tab's log-tail sources.
// Companion to app_terminal.go, which handles installed-shell discovery
// and external-window launching.
//
// What this file provides:
//   - GetCurrentLogPath() — exposes the ktw-gui log file path so the
//     frontend can show users where their logs land.
//   - StartLogTerminal(source) — spawns a tailer goroutine that streams
//     the chosen log source's content into the iframe via Wails events.
//   - StopLogTerminal(id) — kills the tailer goroutine.
//
// Sources supported in v1:
//   - "ktw-gui-log" — full ktw-gui log file
//
// Wire format: a Wails runtime event "terminal.<id>.append" emitted with
// the appended chunk string. The frontend's TerminalView subscribes once
// per terminal and appends to its scrollback. On stop, "terminal.<id>.end"
// is emitted so the frontend can mark the entry as terminated.

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// currentLogPath is set in main.go after openLogFile() resolves. Empty
// until then; the frontend gates the "ktw-gui-log" picker entry on it.
var currentLogPath string

// activeLogTerminals tracks running tailer goroutines so StopLogTerminal
// can cancel them. Keyed by id.
var (
	activeLogTerminalsMu sync.Mutex
	activeLogTerminals   = map[string]context.CancelFunc{}
)

// GetCurrentLogPath returns the absolute path of the ktw-gui log file
// for this session. Empty if the log file failed to open at startup.
//
// Wails-bound: window.go.main.App.GetCurrentLogPath().
func (a *App) GetCurrentLogPath() string {
	return currentLogPath
}

// StartLogTerminal opens a tailer for the given source and returns a
// stable terminal id. The tailer runs until StopLogTerminal(id) is
// called or ktw-gui exits. Tailer behaviour:
//
//   - Reads any existing content from the file as the initial snapshot
//     (capped to the last 256 KB so very long sessions don't flood the
//     event channel on first attach).
//   - Polls the file every 500 ms for new bytes; emits each appended
//     chunk as a "terminal.<id>.append" event with payload {chunk}.
//
// Wails-bound: window.go.main.App.StartLogTerminal(source).
func (a *App) StartLogTerminal(source string) (string, error) {
	if currentLogPath == "" {
		return "", fmt.Errorf("log file path unknown — startup logging failed")
	}
	if source != "ktw-gui-log" {
		return "", fmt.Errorf("unknown log source %q (expected ktw-gui-log)", source)
	}

	id := newTerminalID()
	ctx, cancel := context.WithCancel(context.Background())

	activeLogTerminalsMu.Lock()
	activeLogTerminals[id] = cancel
	activeLogTerminalsMu.Unlock()

	go tailLogFile(ctx, a.ctx, id, currentLogPath)

	return id, nil
}

// StopLogTerminal cancels the tailer goroutine for the given id. No-op
// if the id is unknown (e.g. already stopped).
//
// Wails-bound: window.go.main.App.StopLogTerminal(id).
func (a *App) StopLogTerminal(id string) {
	activeLogTerminalsMu.Lock()
	cancel, ok := activeLogTerminals[id]
	if ok {
		delete(activeLogTerminals, id)
	}
	activeLogTerminalsMu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// tailLogFile reads the existing tail of the file (capped) then polls
// for new bytes. Emits "terminal.<id>.append" / "terminal.<id>.end"
// Wails events.
func tailLogFile(ctx context.Context, wailsCtx context.Context, id string, path string) {
	const initialTailBytes = int64(256 * 1024)
	const pollInterval = 500 * time.Millisecond

	emit := func(eventName string, payload any) {
		if wailsCtx == nil {
			return
		}
		wailsRuntime.EventsEmit(wailsCtx, eventName, payload)
	}

	defer emit(fmt.Sprintf("terminal.%s.end", id), map[string]any{"id": id})

	f, err := os.Open(path)
	if err != nil {
		emit(fmt.Sprintf("terminal.%s.append", id), map[string]any{
			"id":    id,
			"chunk": fmt.Sprintf("error opening log: %v\n", err),
		})
		return
	}
	defer f.Close()

	// Seek to the cap point so we don't re-stream gigabytes of history.
	if fi, statErr := f.Stat(); statErr == nil && fi.Size() > initialTailBytes {
		_, _ = f.Seek(fi.Size()-initialTailBytes, 0)
		// Drop the partial first line so we start at a line boundary.
		br := bufio.NewReader(f)
		_, _ = br.ReadString('\n')
	}

	// Stream initial content + then poll forever.
	br := bufio.NewReader(f)
	flush := func() {
		var batch strings.Builder
		for {
			line, err := br.ReadString('\n')
			if line != "" {
				batch.WriteString(line)
			}
			if err != nil {
				break
			}
		}
		if batch.Len() > 0 {
			emit(fmt.Sprintf("terminal.%s.append", id), map[string]any{
				"id":    id,
				"chunk": batch.String(),
			})
		}
	}
	flush()

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			flush()
		}
	}
}

func newTerminalID() string {
	buf := make([]byte, 6)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("term-%d", time.Now().UnixNano())
	}
	return "term-" + hex.EncodeToString(buf)
}
