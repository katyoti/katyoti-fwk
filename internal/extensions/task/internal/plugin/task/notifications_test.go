// SPDX-License-Identifier: Apache-2.0

package task

import (
	"strings"
	"testing"

	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
)

func TestFormatNotifications_Empty(t *testing.T) {
	out := formatNotifications(nil)
	// Header still written (caller should not call with empty list, but be safe).
	if !strings.Contains(out, "Background task updates") {
		t.Errorf("missing header: %q", out)
	}
}

func TestFormatNotifications_Single(t *testing.T) {
	notifs := []taskapi.TaskNotification{
		{TaskID: "t-1", TaskName: "research", Status: taskapi.TaskCompleted, Summary: "found 5 papers"},
	}
	out := formatNotifications(notifs)
	if !strings.Contains(out, "[completed]") {
		t.Errorf("missing status: %q", out)
	}
	if !strings.Contains(out, "research") {
		t.Errorf("missing task name: %q", out)
	}
	if !strings.Contains(out, "found 5 papers") {
		t.Errorf("missing summary: %q", out)
	}
	if !strings.Contains(out, "t-1") {
		t.Errorf("missing task id: %q", out)
	}
}

func TestFormatNotifications_WithError(t *testing.T) {
	notifs := []taskapi.TaskNotification{
		{TaskID: "t-2", TaskName: "flaky", Status: taskapi.TaskFailed, Error: "timeout after 30s"},
	}
	out := formatNotifications(notifs)
	if !strings.Contains(out, "[failed]") {
		t.Errorf("missing failed status: %q", out)
	}
	if !strings.Contains(out, "error: timeout after 30s") {
		t.Errorf("missing error text: %q", out)
	}
}

func TestFormatNotifications_Multiple(t *testing.T) {
	notifs := []taskapi.TaskNotification{
		{TaskID: "t-1", TaskName: "a", Status: taskapi.TaskCompleted},
		{TaskID: "t-2", TaskName: "b", Status: taskapi.TaskFailed, Error: "oops"},
		{TaskID: "t-3", TaskName: "c", Status: taskapi.TaskKilled},
	}
	out := formatNotifications(notifs)
	lines := strings.Split(strings.TrimSpace(out), "\n")
	// Header + 3 items = 4 lines.
	if len(lines) != 4 {
		t.Errorf("lines = %d, want 4. out=%q", len(lines), out)
	}
}
