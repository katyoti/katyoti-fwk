// SPDX-License-Identifier: Apache-2.0

package tui

import (
	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
)

// Agent → TUI messages. These are sent from the agent goroutine
// to the Bubbletea program as custom tea.Msg types.

// StreamTextMsg carries a partial text delta from the LLM.
type StreamTextMsg struct {
	Text string
}

// StreamDoneMsg signals that the LLM response is complete.
type StreamDoneMsg struct {
	Message agentapi.Message
}

// ToolStartMsg signals that a tool execution has started.
type ToolStartMsg struct {
	Name    string
	ID      string
	Display *agentapi.ToolDisplay
	Input   string // raw JSON input for template rendering
}

// ToolDoneMsg signals that a tool execution has completed.
type ToolDoneMsg struct {
	Name    string
	ID      string
	Result  string
	IsErr   bool
	Display *agentapi.ToolDisplay
}

// AskUserRequestMsg and form-related messages are in prompter.go.

// TaskCompleteMsg signals that the agent has finished.
type TaskCompleteMsg struct {
	Result *agentapi.RunResult
}

// TaskSelectedMsg signals that the user selected a task from the picker.
type TaskSelectedMsg struct {
	Task *taskapi.Task
}

// ErrorMsg carries an error from the agent.
type ErrorMsg struct {
	Err error
}

func (e ErrorMsg) Error() string { return e.Err.Error() }
