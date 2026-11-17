// SPDX-License-Identifier: Apache-2.0

package local

import (
	"context"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
)

// Agent factory validation tests — the early-exit branches that fire BEFORE
// profileMgr/pluginMgr resolution. The validation is the L1 contract for
// the HistoryMode dimension: invalid combinations surface as clear errors.

// TestFactory_HistoryModeFullFork_RequiresInitialMessages (UC-072 negative)
// — HistoryMode "full_fork" without InitialMessages is rejected with a
// clear error citing the constraint.
func TestFactory_HistoryModeFullFork_RequiresInitialMessages(t *testing.T) {
	f := NewAgentMachineFactory(nil, nil)
	_, err := f.CreateMachine(context.Background(), &api.AgentRunRequest{
		HistoryMode: "full_fork",
		// InitialMessages intentionally empty
	})
	if err == nil {
		t.Fatal("expected error when HistoryMode=full_fork without InitialMessages")
	}
	if !strings.Contains(err.Error(), "full_fork") {
		t.Errorf("error %q should cite full_fork", err.Error())
	}
	if !strings.Contains(err.Error(), "InitialMessages") {
		t.Errorf("error %q should mention InitialMessages", err.Error())
	}
}

// TestFactory_HistoryModeSummary_Deferred (UC-073 — explicitly fail-deferred)
// — HistoryMode "summary" is not implemented. The factory surfaces a clear
// "not yet implemented" error so callers can detect the deferral.
func TestFactory_HistoryModeSummary_Deferred(t *testing.T) {
	f := NewAgentMachineFactory(nil, nil)
	_, err := f.CreateMachine(context.Background(), &api.AgentRunRequest{
		HistoryMode: "summary",
	})
	if err == nil {
		t.Fatal("expected error when HistoryMode=summary (not implemented)")
	}
	if !strings.Contains(err.Error(), "summary") {
		t.Errorf("error %q should cite summary", err.Error())
	}
	if !strings.Contains(err.Error(), "not yet implemented") {
		t.Errorf("error %q should indicate not-implemented status", err.Error())
	}
}

// TestFactory_PermissionModeBubble_Deferred (R9) — `bubble` permission mode
// is rejected at attach time. The historic no-op (silent allow) was the
// audit's medium-finding-2: a parent thinking it had set bubble actually
// granted full access. Until the ProtocolPermission round-trip lands as a
// tier-2 follow-up, the factory rejects loudly.
func TestFactory_PermissionModeBubble_Deferred(t *testing.T) {
	f := NewAgentMachineFactory(nil, nil)
	_, err := f.CreateMachine(context.Background(), &api.AgentRunRequest{
		SpawnPermissions: &api.SpawnPermissions{
			PermissionMode: "bubble",
		},
	})
	if err == nil {
		t.Fatal("expected error when PermissionMode=bubble (not implemented)")
	}
	if !strings.Contains(err.Error(), "bubble") {
		t.Errorf("error %q should cite bubble", err.Error())
	}
	if !strings.Contains(err.Error(), "not yet implemented") {
		t.Errorf("error %q should indicate not-implemented status", err.Error())
	}
}

// Isolation outside the process (a pane, a remote host) is not built: the
// factory refuses it rather than run the child in-process as if isolated.
func TestFactory_UnbuiltIsolationIsRefused(t *testing.T) {
	f := NewAgentMachineFactory(nil, nil)
	for _, mode := range []string{"pane", "remote"} {
		_, err := f.CreateMachine(context.Background(), &api.AgentRunRequest{IsolationMode: mode})
		if err == nil || !strings.Contains(err.Error(), "not built") {
			t.Errorf("isolation %q: %v; want it refused", mode, err)
		}
	}
}
