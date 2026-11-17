// SPDX-License-Identifier: Apache-2.0

package main

// Embedded shell sessions backed by a PTY (ConPTY on Windows). The
// frontend mounts an xterm.js Terminal and bridges keystrokes / output
// over Wails events:
//
//   Frontend (xterm) → onData → Wails App.WriteShellPTY(id, data)
//   Wails event "shell.<id>.data" ← read loop ← PTY ← spawned shell
//   Frontend xterm.write(data)  on event
//
// Lifecycle:
//   - StartShellPTY(shellId, cwd) → returns id, spawns shell with PTY,
//     starts a read goroutine. Emits "shell.<id>.data" chunks until exit.
//   - ResizeShellPTY(id, cols, rows) → resize the PTY's window.
//   - WriteShellPTY(id, data) → send stdin bytes to the shell.
//   - StopShellPTY(id) → close the PTY (sends SIGHUP-equivalent on Windows).
//
// Companion to gui-terminal-pty (todo to be filed). Ships PowerShell /
// cmd / pwsh / git-bash / wsl support out of the box (any shell from
// GetInstalledTerminals).

import (
	"context"
	"fmt"
	"log"
	"os"
	"sync"

	"github.com/aymanbagabas/go-pty"
	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

type ptySession struct {
	id        string
	pty       pty.Pty
	cmd       *pty.Cmd
	cancel    context.CancelFunc
	closeOnce sync.Once
}

// close is safe to call from multiple goroutines and is idempotent.
// Wraps cancel + Process.Kill + pty.Close in a sync.Once to avoid the
// Windows ConPTY double-close panic that crashed ktw-gui when both
// `StopShellPTY` and the read-loop's defer raced to clean up.
func (s *ptySession) close() {
	s.closeOnce.Do(func() {
		// Recover from any panic in close — defensive: PTY close on a
		// half-released ConPTY occasionally crashes; we swallow it here
		// rather than let it kill the host app.
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[shell] close panic recovered for %s: %v", s.id, r)
			}
		}()
		if s.cancel != nil {
			s.cancel()
		}
		if s.cmd != nil && s.cmd.Process != nil {
			_ = s.cmd.Process.Kill()
		}
		_ = s.pty.Close()
	})
}

var (
	ptySessionsMu sync.Mutex
	ptySessions   = map[string]*ptySession{}
)

// StartShellPTY spawns the shell identified by shellId with a PTY and
// starts a read loop that emits "shell.<id>.data" events. Returns the
// session id so the frontend can wire write/resize/stop calls.
//
// Wails-bound: window.go.main.App.StartShellPTY(shellId, cwd).
func (a *App) StartShellPTY(shellId string, cwd string) (string, error) {
	terminals := a.GetInstalledTerminals()
	var t *TerminalInfo
	for i := range terminals {
		if terminals[i].ID == shellId {
			t = &terminals[i]
			break
		}
	}
	if t == nil {
		return "", fmt.Errorf("shell %q not installed", shellId)
	}

	if cwd == "" {
		if wd, err := os.Getwd(); err == nil {
			cwd = wd
		}
	}
	if cwd != "" {
		if fi, err := os.Stat(cwd); err != nil || !fi.IsDir() {
			return "", fmt.Errorf("workspace %q is not a directory", cwd)
		}
	}

	ptyInst, err := pty.New()
	if err != nil {
		return "", fmt.Errorf("create pty: %w", err)
	}

	// Build the shell command. Some shells (cmd / powershell / pwsh) want
	// to stay interactive; for those add the appropriate "stay open"
	// switch. For bash/zsh/wsl the default behavior IS interactive.
	args := buildShellArgs(t)

	cmd := ptyInst.Command(t.Path, args...)
	cmd.Dir = cwd
	cmd.Env = append(os.Environ(),
		// Force xterm-compatible TERM so colors/ansi work in the iframe.
		"TERM=xterm-256color",
	)

	if err := cmd.Start(); err != nil {
		_ = ptyInst.Close()
		return "", fmt.Errorf("start shell: %w", err)
	}

	id := newTerminalID()
	ctx, cancel := context.WithCancel(context.Background())
	sess := &ptySession{id: id, pty: ptyInst, cmd: cmd, cancel: cancel}

	ptySessionsMu.Lock()
	ptySessions[id] = sess
	ptySessionsMu.Unlock()

	// Read loop — pump PTY output → Wails events. Wrapped in recover()
	// so any panic (e.g. EventsEmit during Wails shutdown, PTY EOF
	// races) doesn't crash the host process.
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[shell] read-loop panic recovered for %s: %v", id, r)
			}
		}()
		buf := make([]byte, 16*1024)
		defer func() {
			ptySessionsMu.Lock()
			delete(ptySessions, id)
			ptySessionsMu.Unlock()
			sess.close()
			if a.ctx != nil {
				safeEmit(a.ctx, fmt.Sprintf("shell.%s.exit", id), map[string]any{"id": id})
			}
		}()
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			n, err := ptyInst.Read(buf)
			if n > 0 && a.ctx != nil {
				safeEmit(a.ctx, fmt.Sprintf("shell.%s.data", id), map[string]any{
					"id":   id,
					"data": string(buf[:n]),
				})
			}
			if err != nil {
				return
			}
		}
	}()

	// Wait goroutine — when the shell exits, cancel the read loop.
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[shell] wait-loop panic recovered for %s: %v", id, r)
			}
		}()
		_ = cmd.Wait()
		cancel()
	}()

	return id, nil
}

// WriteShellPTY sends stdin bytes to the shell. xterm's onData fires
// for each keystroke; the frontend forwards immediately.
//
// Wails-bound: window.go.main.App.WriteShellPTY(id, data).
func (a *App) WriteShellPTY(id string, data string) error {
	ptySessionsMu.Lock()
	sess := ptySessions[id]
	ptySessionsMu.Unlock()
	if sess == nil {
		return fmt.Errorf("shell session %q not found", id)
	}
	_, err := sess.pty.Write([]byte(data))
	return err
}

// ResizeShellPTY tells the PTY about the xterm widget's dimensions.
// xterm's fit-addon recomputes cols/rows on container resize.
//
// Wails-bound: window.go.main.App.ResizeShellPTY(id, cols, rows).
func (a *App) ResizeShellPTY(id string, cols int, rows int) error {
	ptySessionsMu.Lock()
	sess := ptySessions[id]
	ptySessionsMu.Unlock()
	if sess == nil {
		return fmt.Errorf("shell session %q not found", id)
	}
	if cols <= 0 || rows <= 0 {
		return nil
	}
	return sess.pty.Resize(cols, rows)
}

// StopShellPTY terminates the shell + closes the PTY. Idempotent: the
// session's `closeOnce` ensures the underlying PTY isn't double-closed
// even if the read-loop's defer ran first (race between user clicking
// Close and the shell exiting on its own).
//
// Wails-bound: window.go.main.App.StopShellPTY(id).
func (a *App) StopShellPTY(id string) {
	ptySessionsMu.Lock()
	sess := ptySessions[id]
	ptySessionsMu.Unlock()
	if sess == nil {
		return
	}
	sess.close()
}

// safeEmit wraps wailsRuntime.EventsEmit with panic recovery — defensive
// for the late-shutdown window where the Wails runtime context might
// be partially torn down (firing into a closed event bus has been seen
// to panic in practice). Swallow the panic; the worst case is one
// missed event chunk on shell exit, which the host already handles via
// the .exit event from the deferred cleanup.
func safeEmit(ctx context.Context, name string, payload any) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[shell] safeEmit panic recovered (%s): %v", name, r)
		}
	}()
	wailsRuntime.EventsEmit(ctx, name, payload)
}

// buildShellArgs returns shell-specific flags for an interactive PTY
// session. Most shells default to interactive when stdin is a TTY (which
// the PTY provides), but a few need explicit switches.
func buildShellArgs(t *TerminalInfo) []string {
	switch t.ID {
	case "powershell", "pwsh":
		// -NoLogo skips the version banner; PowerShell stays interactive
		// when it sees a TTY on stdin (no need for -NoExit).
		return []string{"-NoLogo"}
	case "cmd":
		// cmd /K keeps the prompt open. Without it, cmd exits when its
		// stdin closes which is fine inside a PTY but /K is more reliable.
		return []string{"/K"}
	default:
		// bash / zsh / fish / wsl — default to interactive when on a TTY.
		return nil
	}
}
