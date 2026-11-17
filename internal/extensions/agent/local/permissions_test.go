// SPDX-License-Identifier: Apache-2.0

package local

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/agent/api/permrules"
)

// ── permissionProfiles in-memory behavior ──────────────────────────────
//
// The Active/AddSession*/List/Default flow is testable by constructing
// the struct directly without going through the DI extension. This
// covers session forking, shadowing, and persistence-off behavior.

func newTestService(t *testing.T, defaultProfile string, persist bool) *permissionProfiles {
	t.Helper()
	dir := t.TempDir()
	wsRule := permrules.MustParseRule("file_read.workspace_path:**")
	planDeny := permrules.MustParseRule("file_write:**")
	return &permissionProfiles{
		settings: PermissionSettings{
			DefaultProfile:         defaultProfile,
			PersistAlwaysDecisions: persist,
		},
		base: map[string]*permrules.Profile{
			"workspace-edit": {
				Name:       "workspace-edit",
				AllowRules: []permrules.Rule{wsRule},
			},
			"plan-mode": {
				Name:       "plan-mode",
				DenyRules:  []permrules.Rule{planDeny},
				AllowRules: []permrules.Rule{permrules.MustParseRule("file_read:**")},
			},
		},
		userProfilesDir: dir,
	}
}

func TestPermissionProfiles_Active_DefaultFallback(t *testing.T) {
	pp := newTestService(t, "workspace-edit", false)
	got := pp.Active("")
	if got == nil || got.Name != "workspace-edit" {
		t.Errorf("empty name should fall back to default; got %v", got)
	}
}

func TestPermissionProfiles_Active_NamedProfile(t *testing.T) {
	pp := newTestService(t, "workspace-edit", false)
	got := pp.Active("plan-mode")
	if got == nil || got.Name != "plan-mode" {
		t.Errorf("got %v, want plan-mode", got)
	}
	if len(got.DenyRules) != 1 {
		t.Errorf("DenyRules len = %d, want 1", len(got.DenyRules))
	}
}

func TestPermissionProfiles_Active_UnknownNameFallsBackToDefault(t *testing.T) {
	pp := newTestService(t, "workspace-edit", false)
	got := pp.Active("does-not-exist")
	if got == nil || got.Name != "workspace-edit" {
		t.Errorf("unknown name should fall back to default; got %v", got)
	}
}

func TestPermissionProfiles_Active_ReturnsSameCloneOnReentry(t *testing.T) {
	pp := newTestService(t, "workspace-edit", false)
	first := pp.Active("workspace-edit")
	first.AddSessionAllow(permrules.MustParseRule("bash:**"))

	second := pp.Active("workspace-edit")
	if second != first {
		t.Error("repeated Active() with same name should return the cached clone")
	}
	if len(second.AllowRules) != 2 {
		t.Errorf("session add should persist across calls, got %d rules", len(second.AllowRules))
	}
}

func TestPermissionProfiles_AddSessionAllow_ShadowsBase(t *testing.T) {
	pp := newTestService(t, "workspace-edit", false)
	pp.Active("workspace-edit") // initialize the clone

	pp.AddSessionAllow(permrules.MustParseRule("*"))

	prof := pp.Active("workspace-edit")
	// First match should be the wildcard, not the workspace_path rule.
	ok, rule := prof.AllowMatches("bash", map[string]string{"command": "anything"})
	if !ok {
		t.Fatal("expected match")
	}
	if rule.ToolName != "*" {
		t.Errorf("first match = %+v, want * (session add at front)", rule)
	}
}

func TestPermissionProfiles_NoBase_ReturnsNil(t *testing.T) {
	pp := &permissionProfiles{
		settings: PermissionSettings{DefaultProfile: "missing"},
		base:     map[string]*permrules.Profile{},
	}
	if got := pp.Active("anything"); got != nil {
		t.Errorf("got %v, want nil when no profiles loaded", got)
	}
}

func TestPermissionProfiles_PersistAlwaysDecisions_RoundTrip(t *testing.T) {
	pp := newTestService(t, "workspace-edit", true)
	pp.Active("workspace-edit")
	pp.AddSessionAllow(permrules.MustParseRule("file_read.absolute_path:/home/user/**"))

	path := filepath.Join(pp.userProfilesDir, sessionAdditionsFile)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected %s to exist after persist: %v", path, err)
	}

	// Parse the persisted file with LoadProfile to confirm it's valid.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	profile, err := permrules.LoadProfile("_session-additions", data)
	if err != nil {
		t.Fatalf("persisted file failed to reload: %v", err)
	}
	// The persisted file mirrors the active.AllowRules — at minimum
	// the new rule should be present.
	found := false
	for _, r := range profile.AllowRules {
		if r.Field == "absolute_path" && r.ValueGlob == "/home/user/**" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("persisted file missing the new rule, got: %+v", profile.AllowRules)
	}
}

func TestPermissionProfiles_PersistDisabled_NoFile(t *testing.T) {
	pp := newTestService(t, "workspace-edit", false)
	pp.Active("workspace-edit")
	pp.AddSessionAllow(permrules.MustParseRule("file_read.absolute_path:/home/user/**"))

	path := filepath.Join(pp.userProfilesDir, sessionAdditionsFile)
	if _, err := os.Stat(path); err == nil {
		t.Errorf("file should NOT exist when persist disabled")
	}
}

func TestPermissionProfiles_List(t *testing.T) {
	pp := newTestService(t, "workspace-edit", false)
	got := pp.List()
	if len(got) != 2 {
		t.Errorf("len = %d, want 2 (got: %v)", len(got), got)
	}
}
