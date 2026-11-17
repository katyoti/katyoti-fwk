// SPDX-License-Identifier: Apache-2.0

package api

// ChildQuestion is one in-flight question a child agent raised through the
// bubble_to_parent / via_agent ask-user channel, awaiting its parent's answer.
// It is the question DTO carried by the comms service (Dimension 5); the
// routing/wait/wake substrate lives in internal/extensions/comms.
type ChildQuestion struct {
	// ID uniquely identifies the question; the parent answers by this id.
	ID string `json:"id"`
	// ChildTaskID is the child's task/loop id — the key the parent's answer
	// uses to Resume the suspended child via the SuspensionManager.
	ChildTaskID string `json:"childTaskId"`
	// ChildName is a human label for the child (task name) for display.
	ChildName string `json:"childName,omitempty"`
	// Question is the prompt text the child asked.
	Question string `json:"question"`
	// Options, when non-empty, are the select choices the child offered.
	Options []string `json:"options,omitempty"`
}
