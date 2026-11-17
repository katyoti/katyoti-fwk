// SPDX-License-Identifier: Apache-2.0

// Package testing is the fake Claude: an in-process peer that plays the
// Claude Code side of the stream-json protocol over pipes, so the driver's
// tests run without starting a process. A test scripts it: read what the host
// wrote, write what Claude Code would answer, end with an exit code.
package testing

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"time"
)

// Fake is a scripted Claude Code. It satisfies the driver's Conn.
type Fake struct {
	hostR *io.PipeReader // what the host writes, read by the script
	hostW *io.PipeWriter
	outR  *io.PipeReader // what the script writes, read by the host
	outW  *io.PipeWriter
	errR  *io.PipeReader
	errW  *io.PipeWriter
	sc    *bufio.Scanner

	mu   sync.Mutex
	code int
	done chan struct{}
	once sync.Once
}

// New is a fake Claude Code ready to be scripted.
func New() *Fake {
	f := &Fake{code: -1, done: make(chan struct{})}
	f.hostR, f.hostW = io.Pipe()
	f.outR, f.outW = io.Pipe()
	f.errR, f.errW = io.Pipe()
	f.sc = bufio.NewScanner(f.hostR)
	f.sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	return f
}

// Conn side (the host's view).

func (f *Fake) Stdin() io.WriteCloser { return f.hostW }
func (f *Fake) Stdout() io.Reader     { return f.outR }
func (f *Fake) Stderr() io.Reader     { return f.errR }
func (f *Fake) Done() <-chan struct{} { return f.done }

func (f *Fake) ExitCode() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.code
}

// Shutdown is the host closing stdin: the fake exits 0, as Claude Code does on
// end of input, unless the script already ended it.
func (f *Fake) Shutdown(time.Duration) {
	_ = f.hostW.Close()
	f.Exit(0)
}

// Script side (Claude Code's view).

// Read returns the next frame the host wrote, decoded; io.EOF when the host
// closed stdin.
func (f *Fake) Read() (map[string]any, error) {
	for f.sc.Scan() {
		line := f.sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal(line, &m); err != nil {
			return nil, fmt.Errorf("fake claude: the host wrote a non-JSON line: %q", line)
		}
		return m, nil
	}
	if err := f.sc.Err(); err != nil {
		return nil, err
	}
	return nil, io.EOF
}

// Write writes a frame as Claude Code would.
func (f *Fake) Write(v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = f.outW.Write(append(b, '\n'))
	return err
}

// WriteLine writes a raw line (a diagnostic, or a malformed frame).
func (f *Fake) WriteLine(s string) error {
	_, err := f.outW.Write([]byte(s + "\n"))
	return err
}

// Stderr writes a line on stderr.
func (f *Fake) StderrLine(s string) { _, _ = f.errW.Write([]byte(s + "\n")) }

// Exit ends the fake with code: its output closes, then it is done.
func (f *Fake) Exit(code int) {
	f.once.Do(func() {
		f.mu.Lock()
		f.code = code
		f.mu.Unlock()
		_ = f.outW.Close()
		_ = f.errW.Close()
		_ = f.hostR.Close()
		close(f.done)
	})
}

// Frames Claude Code writes.

// InitializeOK answers the host's initialize with a subscription account.
func InitializeOK(requestID string) map[string]any {
	return ControlSuccess(requestID, map[string]any{
		"commands": []any{}, "models": []any{},
		"account": map[string]any{"email": "person@example.com", "subscriptionType": "max",
			"tokenSource": "claude.ai", "apiKeySource": "none", "apiProvider": "firstParty"},
		"claude_code_version": "2.1.296",
	})
}

// InitializeAPIKey answers initialize with an account billed by an API key.
func InitializeAPIKey(requestID string) map[string]any {
	return ControlSuccess(requestID, map[string]any{
		"account": map[string]any{"apiKeySource": "ANTHROPIC_API_KEY", "apiProvider": "firstParty"},
	})
}

// ControlSuccess answers a host control request.
func ControlSuccess(requestID string, response any) map[string]any {
	return map[string]any{"type": "control_response", "response": map[string]any{
		"subtype": "success", "request_id": requestID, "response": response}}
}

// SystemInit starts a turn.
func SystemInit(sessionID, apiKeySource string) map[string]any {
	return map[string]any{"type": "system", "subtype": "init", "session_id": sessionID, "cwd": "/repo",
		"model": "claude-test", "permissionMode": "auto", "apiKeySource": apiKeySource,
		"claude_code_version": "2.1.296", "capabilities": []string{"interrupt_receipt_v1"}}
}

// SessionState is system/session_state_changed.
func SessionState(state string) map[string]any {
	return map[string]any{"type": "system", "subtype": "session_state_changed", "state": state}
}

// AssistantText is an assistant text block.
func AssistantText(text string) map[string]any {
	return map[string]any{"type": "assistant", "message": map[string]any{"role": "assistant",
		"content": []any{map[string]any{"type": "text", "text": text}}}, "parent_tool_use_id": nil}
}

// AssistantToolUse is an assistant tool call.
func AssistantToolUse(id, name string, input any) map[string]any {
	return map[string]any{"type": "assistant", "message": map[string]any{"role": "assistant",
		"content": []any{map[string]any{"type": "tool_use", "id": id, "name": name, "input": input}}}}
}

// ToolResult is the user frame carrying a tool's result.
func ToolResult(toolUseID, content string, isError bool) map[string]any {
	return map[string]any{"type": "user", "message": map[string]any{"role": "user",
		"content": []any{map[string]any{"type": "tool_result", "tool_use_id": toolUseID, "content": content, "is_error": isError}}}}
}

// CanUseTool asks the host a permission.
func CanUseTool(requestID, tool string, input any, suggestions ...any) map[string]any {
	req := map[string]any{"subtype": "can_use_tool", "tool_name": tool, "input": input, "tool_use_id": "toolu_" + requestID,
		"title": "Claude wants to run " + tool}
	if len(suggestions) > 0 {
		req["permission_suggestions"] = suggestions
	}
	return map[string]any{"type": "control_request", "request_id": requestID, "request": req}
}

// Cancel withdraws a request Claude Code sent.
func Cancel(requestID string) map[string]any {
	return map[string]any{"type": "control_cancel_request", "request_id": requestID}
}

// Result ends a turn.
func Result(sessionID, text string, isError bool, cost float64) map[string]any {
	return map[string]any{"type": "result", "subtype": "success", "is_error": isError, "result": text,
		"num_turns": 1, "total_cost_usd": cost, "session_id": sessionID}
}

// Request is the "request" object of a host control_request, and its id.
func Request(m map[string]any) (id string, req map[string]any) {
	id, _ = m["request_id"].(string)
	req, _ = m["request"].(map[string]any)
	return id, req
}

// RemoteControlOK answers the host's remote_control with a session URL.
func RemoteControlOK(requestID, url string) map[string]any {
	return ControlSuccess(requestID, map[string]any{"session_url": url, "connect_url": "https://claude.ai/code?environment=",
		"environment_id": "", "bridge_epoch": 1})
}

// BridgeState is system/bridge_state.
func BridgeState(state string) map[string]any {
	return map[string]any{"type": "system", "subtype": "bridge_state", "state": state}
}
