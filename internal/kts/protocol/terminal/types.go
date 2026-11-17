// SPDX-License-Identifier: Apache-2.0

package terminal

// Message is the terminal protocol envelope. Every message in a session's
// history has this structure. Content is a discriminated union by type field.
type Message struct {
	ID        string         `json:"id"`
	SessionID string         `json:"sessionId"`
	SourceRun string         `json:"sourceRun,omitempty"` // run id of the delegate that produced this message; lets the GUI filter the unified chat to one node in the delegation tree. Empty = the root run.
	SeqNo     int            `json:"seqNo"`
	Role      string         `json:"role"`      // "user" | "assistant" | "system" | "tool"
	Timestamp string         `json:"timestamp"` // ISO 8601
	Content   MessageContent `json:"content"`
	Metadata  *MsgMetadata   `json:"metadata,omitempty"`
}

// MsgMetadata carries optional context for a message.
type MsgMetadata struct {
	Display      *DisplayData `json:"display,omitempty"`
	TaskID       string       `json:"taskId,omitempty"`
	ParentTaskID string       `json:"parentTaskId,omitempty"`
}

// Session represents a chat thread with an agent. Sessions have a parent
// chain (for delegation) and link to the task system via taskId.
type Session struct {
	ID              string `json:"id"`
	ParentSessionID string `json:"parentSessionId,omitempty"` // null = root
	Name            string `json:"name"`
	Status          string `json:"status"` // "running" | "done" | "error" | "stopped"
	TaskID          string `json:"taskId"`
	ParentTaskID    string `json:"parentTaskId,omitempty"`
	MessageCount    int    `json:"messageCount"`
	StartedAt       string `json:"startedAt"`
}

// Room is the top-level container that bridges KTSP (system) and the
// terminal protocol (user). A room holds sessions, manages connected
// terminals, and routes the translator's output to connected UIs.
type Room struct {
	ID       string    `json:"id"`
	Sessions []Session `json:"sessions"`
}
