// SPDX-License-Identifier: Apache-2.0

package task

import (
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/task/api/presets"
)

// Migrated 2026-05-07 from in-package configOverrideFromModeWithParent calls
// to presets.BuildChildOverride per the OD-001 spawn-policy refactor.

func TestConfigOverride_InjectsIsolationMode(t *testing.T) {
	// Teammate preset has IsolationContextIsolated → should surface as isolationMode=context_isolated.
	override := presets.BuildChildOverride(presets.ModeTeammate, presets.ModeTeamLeader)
	if override == nil {
		t.Fatal("teammate mode should produce override with isolation")
	}
	iso, ok := override["isolationMode"].(string)
	if !ok {
		t.Fatalf("isolationMode missing or wrong type: %v", override["isolationMode"])
	}
	if iso != "context_isolated" {
		t.Errorf("isolationMode = %q, want context_isolated", iso)
	}
}

func TestConfigOverride_DoesNotInjectInProcessDefault(t *testing.T) {
	// Worker preset has IsolationInProcess → no isolationMode override needed.
	override := presets.BuildChildOverride(presets.ModeWorker, "")
	if override == nil {
		return // worker has no restrictions at all — override is nil
	}
	if _, ok := override["isolationMode"]; ok {
		t.Errorf("worker should not inject isolationMode (default is in_process), got %v", override["isolationMode"])
	}
}

func TestConfigOverride_InjectsRemoteIsolation(t *testing.T) {
	// Remote preset has IsolationRemote → should surface.
	override := presets.BuildChildOverride(presets.ModeRemote, "")
	if override == nil {
		t.Fatal("remote mode should produce override")
	}
	iso, ok := override["isolationMode"].(string)
	if !ok || iso != "remote" {
		t.Errorf("isolationMode = %v, want remote", override["isolationMode"])
	}
}

func TestConfigOverride_ForkStaysInProcess(t *testing.T) {
	// Fork mode preset has IsolationInProcess — should not inject isolationMode.
	override := presets.BuildChildOverride(presets.ModeFork, "")
	if override == nil {
		return
	}
	if v, ok := override["isolationMode"]; ok {
		t.Errorf("fork preset should not inject isolationMode (in_process default), got %v", v)
	}
}
