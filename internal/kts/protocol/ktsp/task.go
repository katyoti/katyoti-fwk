// SPDX-License-Identifier: Apache-2.0

package ktsp

// TaskState represents the lifecycle state of an async task.
type TaskState string

const (
	TaskRunning   TaskState = "running"
	TaskCompleted TaskState = "completed"
	TaskFailed    TaskState = "failed"
	TaskCancelled TaskState = "cancelled"
)

// IsTerminal returns true if the task has reached a final state.
func (s TaskState) IsTerminal() bool {
	return s == TaskCompleted || s == TaskFailed || s == TaskCancelled
}

// TaskProgress reports progress on a running async task.
type TaskProgress struct {
	Percent int    `json:"percent,omitempty"` // 0-100
	Message string `json:"message,omitempty"`
}

// TaskInfo is the status of an async task (returned by task/status).
type TaskInfo struct {
	TaskID   string        `json:"taskId"`
	State    TaskState     `json:"state"`
	Progress *TaskProgress `json:"progress,omitempty"`
}

// ContentBlock is a polymorphic content unit in KTSP responses.
// Distinct from llm.ContentBlock — KTSP blocks are for inter-service
// communication and support additional types (image, file).
type ContentBlock struct {
	Type     string `json:"type"`               // "text", "image", "file"
	Text     string `json:"text,omitempty"`     // type="text"
	Data     string `json:"data,omitempty"`     // type="image" (base64-encoded)
	URI      string `json:"uri,omitempty"`      // type="file"
	MimeType string `json:"mimeType,omitempty"` // type="image" or "file"
}

// TaskResult is the final result of an async task (returned by task/result).
type TaskResult struct {
	TaskID  string         `json:"taskId"`
	State   TaskState      `json:"state"`
	Content []ContentBlock `json:"content,omitempty"` // when completed
	Error   *RPCError      `json:"error,omitempty"`   // when failed
}
