// SPDX-License-Identifier: Apache-2.0

package core

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/extensions/testharness/api"
)

// ProcessSession manages a single spawned process with captured I/O.
type ProcessSession struct {
	id        string
	cmd       *exec.Cmd
	stdin     io.WriteCloser
	startedAt time.Time

	mu     sync.RWMutex
	output []api.OutputLine
	seq    atomic.Int64

	done     chan struct{}
	exitCode *int
}

// NewProcessSession spawns a process and starts capturing output.
func NewProcessSession(id string, req api.RunRequest) (*ProcessSession, error) {
	cmd := exec.Command(req.Cmd, req.Args...)

	if req.Dir != "" {
		cmd.Dir = req.Dir
	}
	// Always pass current environment. Extra env vars override.
	cmd.Env = os.Environ()
	for _, e := range req.Env {
		cmd.Env = append(cmd.Env, e)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("stderr pipe: %w", err)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("stdin pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start process: %w", err)
	}

	s := &ProcessSession{
		id:        id,
		cmd:       cmd,
		stdin:     stdin,
		startedAt: time.Now(),
		done:      make(chan struct{}),
	}

	// Capture stdout and stderr in goroutines
	go s.capture("stdout", stdout)
	go s.capture("stderr", stderr)

	// Wait for process exit
	go s.waitExit()

	return s, nil
}

// capture reads from r and emits output lines. Complete lines (ending with \n)
// are emitted immediately. Partial lines (e.g., prompts like "[y/N]: " with no
// trailing newline) are flushed after a 500ms timeout so they appear in /output.
func (s *ProcessSession) capture(stream string, r io.Reader) {
	const flushTimeout = 500 * time.Millisecond

	buf := make([]byte, 4096)
	var partial bytes.Buffer

	emit := func(text string) {
		line := api.OutputLine{
			Seq:    s.seq.Add(1),
			Ts:     time.Now(),
			Stream: stream,
			Text:   text,
		}
		s.mu.Lock()
		s.output = append(s.output, line)
		s.mu.Unlock()
	}

	// readCh delivers read results from a background goroutine so we can
	// select on both incoming data and a flush timer.
	type readResult struct {
		n   int
		err error
	}
	ch := make(chan readResult, 1)

	go func() {
		for {
			n, err := r.Read(buf)
			ch <- readResult{n, err}
			if err != nil {
				return
			}
		}
	}()

	var timer *time.Timer
	for {
		var timerC <-chan time.Time
		if partial.Len() > 0 {
			if timer == nil {
				timer = time.NewTimer(flushTimeout)
			} else {
				timer.Reset(flushTimeout)
			}
			timerC = timer.C
		}

		select {
		case res := <-ch:
			if res.n > 0 {
				data := buf[:res.n]
				for len(data) > 0 {
					idx := bytes.IndexByte(data, '\n')
					if idx >= 0 {
						// Complete line: partial + data up to newline
						partial.Write(data[:idx])
						text := partial.String()
						// Strip trailing \r for Windows
						if len(text) > 0 && text[len(text)-1] == '\r' {
							text = text[:len(text)-1]
						}
						emit(text)
						partial.Reset()
						data = data[idx+1:]
					} else {
						// No newline — accumulate into partial
						partial.Write(data)
						data = nil
					}
				}
			}
			if res.err != nil {
				// Flush any remaining partial content
				if partial.Len() > 0 {
					emit(partial.String())
					partial.Reset()
				}
				return
			}

		case <-timerC:
			// Flush partial line after timeout (e.g., "[y/N]: " prompt)
			if partial.Len() > 0 {
				emit(partial.String())
				partial.Reset()
			}
		}
	}
}

func (s *ProcessSession) waitExit() {
	err := s.cmd.Wait()
	code := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			code = exitErr.ExitCode()
		} else {
			code = -1
		}
	}
	s.mu.Lock()
	s.exitCode = &code
	s.mu.Unlock()
	close(s.done)
}

// ID returns the session identifier.
func (s *ProcessSession) ID() string { return s.id }

// PID returns the process ID.
func (s *ProcessSession) PID() int { return s.cmd.Process.Pid }

// Running returns true if the process is still alive.
func (s *ProcessSession) Running() bool {
	select {
	case <-s.done:
		return false
	default:
		return true
	}
}

// Output returns all output lines with seq > since.
func (s *ProcessSession) Output(since int64) []api.OutputLine {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if since <= 0 {
		result := make([]api.OutputLine, len(s.output))
		copy(result, s.output)
		return result
	}

	// Binary search would be faster but output is append-only and seq is monotonic
	for i, line := range s.output {
		if line.Seq > since {
			result := make([]api.OutputLine, len(s.output)-i)
			copy(result, s.output[i:])
			return result
		}
	}
	return nil
}

// OutputLen returns the total number of captured lines.
func (s *ProcessSession) OutputLen() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.output)
}

// SendInput writes text to the process stdin.
func (s *ProcessSession) SendInput(text string) error {
	if !s.Running() {
		return fmt.Errorf("process not running")
	}
	_, err := io.WriteString(s.stdin, text)
	return err
}

// Kill terminates the process.
func (s *ProcessSession) Kill() error {
	if !s.Running() {
		return nil
	}
	// Try interrupt first
	if err := s.cmd.Process.Signal(os.Interrupt); err != nil {
		// Fallback to kill
		return s.cmd.Process.Kill()
	}
	// Wait up to 5 seconds for graceful exit
	select {
	case <-s.done:
		return nil
	case <-time.After(5 * time.Second):
		return s.cmd.Process.Kill()
	}
}

// Status returns the current session status.
func (s *ProcessSession) Status() api.SessionStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return api.SessionStatus{
		SessionID: s.id,
		Running:   s.Running(),
		PID:       s.PID(),
		ExitCode:  s.exitCode,
		OutputLen: len(s.output),
		StartedAt: s.startedAt.Format(time.RFC3339),
		Cmd:       s.cmd.Path,
	}
}

// Done returns a channel that closes when the process exits.
func (s *ProcessSession) Done() <-chan struct{} { return s.done }
