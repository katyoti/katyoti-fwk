// SPDX-License-Identifier: Apache-2.0

// Package protocol holds the wire shapes of the Claude Code stream-json
// protocol and its framing: newline-delimited JSON both ways. The shapes follow
// Anthropic's Python Agent SDK (MIT), the protocol's only specification
// (modules/claude/docs/claude-code-protocol.md). Unknown fields are ignored and
// unknown types kept raw, so a newer CLI never breaks the driver.
package protocol

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sync"
)

// MaxLine is the largest frame read: tool results and images exceed the SDKs'
// 1 MiB.
const MaxLine = 16 * 1024 * 1024

// Message types on stdout.
const (
	TypeSystem               = "system"
	TypeAssistant            = "assistant"
	TypeUser                 = "user"
	TypeResult               = "result"
	TypeStreamEvent          = "stream_event"
	TypeRateLimit            = "rate_limit_event"
	TypeControlRequest       = "control_request"
	TypeControlResponse      = "control_response"
	TypeControlCancelRequest = "control_cancel_request"
	TypeKeepAlive            = "keep_alive"
)

// System subtypes the driver reads.
const (
	SubtypeInit                = "init"
	SubtypeSessionStateChanged = "session_state_changed"
	SubtypeBridgeState         = "bridge_state"
)

// Control request subtypes.
const (
	SubtypeInitialize        = "initialize"
	SubtypeInterrupt         = "interrupt"
	SubtypeSetPermissionMode = "set_permission_mode"
	SubtypeCanUseTool        = "can_use_tool"
	SubtypeHookCallback      = "hook_callback"
	SubtypeMCPMessage        = "mcp_message"
	// SubtypeRemoteControl opens the session to claude.ai/code (Remote
	// Control). Undocumented: the request the VS Code extension sends.
	SubtypeRemoteControl = "remote_control"
)

// Envelope is the part of every frame the driver dispatches on.
type Envelope struct {
	Type      string `json:"type"`
	Subtype   string `json:"subtype,omitempty"`
	SessionID string `json:"session_id,omitempty"`
}

// SystemInit is system/init: the start of every turn.
type SystemInit struct {
	SessionID         string   `json:"session_id"`
	Cwd               string   `json:"cwd"`
	Model             string   `json:"model"`
	PermissionMode    string   `json:"permissionMode"`
	APIKeySource      string   `json:"apiKeySource"`
	ClaudeCodeVersion string   `json:"claude_code_version"`
	Capabilities      []string `json:"capabilities"`
}

// SessionStateChanged is system/session_state_changed.
type SessionStateChanged struct {
	State string `json:"state"` // idle, running, requires_action
}

// ContentBlock is one block of an assistant or user message.
type ContentBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Content   json.RawMessage `json:"content,omitempty"`
	IsError   bool            `json:"is_error,omitempty"`
}

// ChatMessage is an assistant or user frame.
type ChatMessage struct {
	Message struct {
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	} `json:"message"`
	ParentToolUseID *string `json:"parent_tool_use_id"`
}

// Blocks decodes the message content: a string is one text block.
func (m ChatMessage) Blocks() []ContentBlock {
	c := bytes.TrimSpace(m.Message.Content)
	if len(c) == 0 {
		return nil
	}
	if c[0] == '"' {
		var s string
		if json.Unmarshal(c, &s) == nil {
			return []ContentBlock{{Type: "text", Text: s}}
		}
		return nil
	}
	var bs []ContentBlock
	_ = json.Unmarshal(c, &bs)
	return bs
}

// Result ends a turn.
type Result struct {
	Subtype        string   `json:"subtype"`
	IsError        bool     `json:"is_error"`
	Result         string   `json:"result"`
	NumTurns       int      `json:"num_turns"`
	TotalCostUSD   float64  `json:"total_cost_usd"`
	SessionID      string   `json:"session_id"`
	TerminalReason string   `json:"terminal_reason,omitempty"`
	Errors         []string `json:"errors,omitempty"`
}

// RateLimitEvent reports the subscription's limits.
type RateLimitEvent struct {
	RateLimitInfo struct {
		Status   string `json:"status"` // allowed, allowed_warning, rejected
		ResetsAt any    `json:"resetsAt,omitempty"`
	} `json:"rate_limit_info"`
}

// ControlRequest is a request either side sends.
type ControlRequest struct {
	Type      string          `json:"type"`
	RequestID string          `json:"request_id"`
	Request   json.RawMessage `json:"request"`
}

// ControlCancel withdraws a request Claude Code sent.
type ControlCancel struct {
	RequestID string `json:"request_id"`
}

// ControlResponseBody is the inner answer of a control_response.
type ControlResponseBody struct {
	Subtype   string          `json:"subtype"` // success or error
	RequestID string          `json:"request_id"`
	Response  json.RawMessage `json:"response,omitempty"`
	Error     string          `json:"error,omitempty"`
}

// ControlResponse answers a control_request.
type ControlResponse struct {
	Type     string              `json:"type"`
	Response ControlResponseBody `json:"response"`
}

// CanUseTool is a permission Claude Code asks.
type CanUseTool struct {
	Subtype               string            `json:"subtype"`
	ToolName              string            `json:"tool_name"`
	Input                 json.RawMessage   `json:"input"`
	ToolUseID             string            `json:"tool_use_id,omitempty"`
	PermissionSuggestions []json.RawMessage `json:"permission_suggestions,omitempty"`
	BlockedPath           string            `json:"blocked_path,omitempty"`
	DecisionReason        string            `json:"decision_reason,omitempty"`
	Title                 string            `json:"title,omitempty"`
	Description           string            `json:"description,omitempty"`
}

// Account is the account of the initialize answer.
type Account struct {
	Email            string `json:"email"`
	Organization     string `json:"organization"`
	SubscriptionType string `json:"subscriptionType"`
	TokenSource      string `json:"tokenSource"`
	APIKeySource     string `json:"apiKeySource"`
	APIProvider      string `json:"apiProvider"`
}

// InitializeResponse is the answer to initialize.
type InitializeResponse struct {
	Account           *Account `json:"account"`
	ClaudeCodeVersion string   `json:"claude_code_version"`
}

// RemoteControlResponse answers remote_control.
type RemoteControlResponse struct {
	SessionURL string `json:"session_url"`
	ConnectURL string `json:"connect_url"`
}

// BridgeState is system/bridge_state: the Remote Control bridge.
type BridgeState struct {
	State string `json:"state"`
}

// UserMessage is a user message written to stdin.
type UserMessage struct {
	Type            string  `json:"type"`
	Message         userMsg `json:"message"`
	ParentToolUseID *string `json:"parent_tool_use_id"`
	SessionID       string  `json:"session_id"`
}

type userMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// NewUserMessage builds a text user message.
func NewUserMessage(text string) UserMessage {
	return UserMessage{Type: TypeUser, Message: userMsg{Role: "user", Content: text}}
}

// PermissionAllow and PermissionDeny are the answers to can_use_tool.
type PermissionAllow struct {
	Behavior           string            `json:"behavior"`
	UpdatedInput       json.RawMessage   `json:"updatedInput"`
	UpdatedPermissions []json.RawMessage `json:"updatedPermissions,omitempty"`
}

type PermissionDeny struct {
	Behavior  string `json:"behavior"`
	Message   string `json:"message"`
	Interrupt bool   `json:"interrupt,omitempty"`
}

// Reader reads frames from Claude Code's stdout.
type Reader struct{ sc *bufio.Scanner }

// NewReader reads newline-delimited frames from r.
func NewReader(r io.Reader) *Reader {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), MaxLine)
	return &Reader{sc: sc}
}

// Next returns the next frame. A line that does not start with '{' is
// diagnostic output and skipped; a '{' line that is not JSON is an error.
// io.EOF ends the stream.
func (r *Reader) Next() (json.RawMessage, Envelope, error) {
	for r.sc.Scan() {
		line := bytes.TrimSpace(r.sc.Bytes())
		if len(line) == 0 || line[0] != '{' {
			continue
		}
		var env Envelope
		if err := json.Unmarshal(line, &env); err != nil {
			return nil, Envelope{}, fmt.Errorf("protocol: malformed frame: %w", err)
		}
		frame := make(json.RawMessage, len(line))
		copy(frame, line)
		return frame, env, nil
	}
	if err := r.sc.Err(); err != nil {
		return nil, Envelope{}, err
	}
	return nil, Envelope{}, io.EOF
}

// Writer writes frames to Claude Code's stdin, one JSON object per line.
type Writer struct {
	mu sync.Mutex
	w  io.Writer
}

// NewWriter writes frames to w.
func NewWriter(w io.Writer) *Writer { return &Writer{w: w} }

// Write encodes v and writes it as one line.
func (w *Writer) Write(v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	b = append(b, '\n')
	w.mu.Lock()
	defer w.mu.Unlock()
	_, err = w.w.Write(b)
	return err
}
