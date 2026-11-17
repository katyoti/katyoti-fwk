// SPDX-License-Identifier: Apache-2.0

package core

import (
	"strings"
	"testing"

	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
)

// TestBgManager_RejectsZeroMaxDurationS verifies that background spawns
// without a wall-clock budget are rejected at Register time (R6 — closes
// the unbounded-spawn resource exhaustion risk).
func TestBgManager_RejectsZeroMaxDurationS(t *testing.T) {
	svc, _ := newServiceForLifecycleTest(t)
	bg := svc.bgMgr.(*bgManager)

	// Zero is rejected.
	_, err := bg.Register("parent", taskapi.TaskRunRequest{
		TaskName: "uc-130-task",
	})
	if err == nil {
		t.Fatal("Register with MaxDurationS=0 should be rejected")
	}
	if !strings.Contains(err.Error(), "MaxDurationS") {
		t.Errorf("error %q should mention MaxDurationS", err.Error())
	}

	// Negative is rejected.
	_, err = bg.Register("parent", taskapi.TaskRunRequest{
		TaskName:     "uc-130-task",
		MaxDurationS: -1,
	})
	if err == nil {
		t.Fatal("Register with MaxDurationS<0 should be rejected")
	}

	// Positive is accepted.
	taskID, err := bg.Register("parent", taskapi.TaskRunRequest{
		TaskName:     "uc-130-task",
		MaxDurationS: 60,
	})
	if err != nil {
		t.Errorf("Register with MaxDurationS=60 should be accepted, got %v", err)
	}
	if taskID == "" {
		t.Error("Register returned empty taskID")
	}
}
