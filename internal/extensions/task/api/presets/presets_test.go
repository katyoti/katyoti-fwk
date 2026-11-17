// SPDX-License-Identifier: Apache-2.0

package presets

import "testing"

func TestResolvePreset_KnownModes(t *testing.T) {
	tests := []struct {
		mode            Mode
		expectSched     SchedulingMode
		expectHistory   HistoryMode
		expectIsolation IsolationMode
	}{
		{ModeWorker, SchedulingForeground, HistoryNone, IsolationInProcess},
		{ModeBackground, SchedulingBackground, HistoryNone, IsolationInProcess},
		{ModeFork, SchedulingBackground, HistoryFullFork, IsolationInProcess},
		{ModeIsolated, SchedulingBackground, HistoryNone, IsolationInProcess},
		{ModeForkIsolated, SchedulingBackground, HistoryFullFork, IsolationInProcess},
		{ModeCoordinator, SchedulingForeground, HistoryNone, IsolationInProcess},
		{ModeTeamLeader, SchedulingForeground, HistoryNone, IsolationInProcess},
		{ModeTeammate, SchedulingBackground, HistoryNone, IsolationContextIsolated},
		{ModeRemote, SchedulingBackground, HistoryNone, IsolationRemote},
	}

	for _, tt := range tests {
		cfg, err := ResolvePreset(tt.mode, nil)
		if err != nil {
			t.Errorf("ResolvePreset(%q): %v", tt.mode, err)
			continue
		}
		if cfg.Scheduling.Mode != tt.expectSched {
			t.Errorf("%s Scheduling = %q, want %q", tt.mode, cfg.Scheduling.Mode, tt.expectSched)
		}
		if cfg.Context.History != tt.expectHistory {
			t.Errorf("%s History = %q, want %q", tt.mode, cfg.Context.History, tt.expectHistory)
		}
		if cfg.Isolation.Mode != tt.expectIsolation {
			t.Errorf("%s Isolation = %q, want %q", tt.mode, cfg.Isolation.Mode, tt.expectIsolation)
		}
	}
}

func TestResolvePreset_UnknownMode(t *testing.T) {
	_, err := ResolvePreset("unknown-mode", nil)
	if err == nil {
		t.Error("expected error for unknown mode")
	}
}

func TestResolvePreset_EmptyModeDefaultsToWorker(t *testing.T) {
	cfg, err := ResolvePreset("", nil)
	if err != nil {
		t.Fatalf("empty mode: %v", err)
	}
	if cfg.Scheduling.Mode != SchedulingForeground {
		t.Errorf("empty mode should default to worker (foreground), got %q", cfg.Scheduling.Mode)
	}
}

func TestResolvePreset_IsolatedHasWorktree(t *testing.T) {
	cfg, err := ResolvePreset(ModeIsolated, nil)
	if err != nil {
		t.Fatalf("ResolvePreset: %v", err)
	}
	if cfg.Context.Workspace.Mode != WorkspaceDedicated {
		t.Errorf("isolated Workspace.Mode = %q, want dedicated", cfg.Context.Workspace.Mode)
	}
	if cfg.Context.Workspace.Strategy != "git_worktree" {
		t.Errorf("isolated Workspace.Strategy = %q, want git_worktree", cfg.Context.Workspace.Strategy)
	}
}

// No mode declares a permission mode that is not built: the agent factory
// refuses a child declaring "bubble", so a mode carrying it never starts.
func TestNoModeDeclaresAnUnbuiltPermissionMode(t *testing.T) {
	for mode, cfg := range presetsByMode {
		if cfg.Permissions.PermissionMode == PermissionBubble {
			t.Errorf("mode %s declares permission mode bubble, which no child can start with", mode)
		}
	}
}

func TestResolvePreset_CoordinatorHasMessaging(t *testing.T) {
	cfg, err := ResolvePreset(ModeCoordinator, nil)
	if err != nil {
		t.Fatalf("ResolvePreset: %v", err)
	}
	if !cfg.AgentComms.Messaging {
		t.Error("coordinator should have Messaging enabled")
	}
	if cfg.UserComms.Display != DisplayStreaming {
		t.Errorf("coordinator Display = %q, want streaming", cfg.UserComms.Display)
	}
}

// TestResolvePreset_InputDefaults pins the UserComms.Input default per
// preset after ms-cwm-v2-providers US-16 (2026-06-08). Every
// system-spawned preset defaults to InputInbox so ask_user calls land
// in the Inbox node and suspend the task; teammate routes via its
// leader's channel; remote stays InputNone (no user channel by
// design).
func TestResolvePreset_InputDefaults(t *testing.T) {
	cases := []struct {
		mode Mode
		want InputMode
	}{
		{ModeWorker, InputInbox},
		{ModeBackground, InputInbox},
		{ModeFork, InputInbox},
		{ModeIsolated, InputInbox},
		{ModeForkIsolated, InputInbox},
		{ModeCoordinator, InputInbox},
		{ModeTeamLeader, InputInbox},
		{ModeTeammate, InputViaLeader},
		{ModeRemote, InputNone},
	}
	for _, tc := range cases {
		t.Run(string(tc.mode), func(t *testing.T) {
			cfg, err := ResolvePreset(tc.mode, nil)
			if err != nil {
				t.Fatalf("ResolvePreset(%q): %v", tc.mode, err)
			}
			if cfg.UserComms.Input != tc.want {
				t.Errorf("%s UserComms.Input = %q, want %q",
					tc.mode, cfg.UserComms.Input, tc.want)
			}
		})
	}
}

// TestResolvePreset_InputInboxOverride confirms the resolver applies
// caller overrides on the UserComms.Input field — same merge semantics
// as the other dimensions.
func TestResolvePreset_InputInboxOverride(t *testing.T) {
	overrides := &SpawnConfig{
		UserComms: UserCommsConfig{Input: InputDirect},
	}
	cfg, err := ResolvePreset(ModeWorker, overrides)
	if err != nil {
		t.Fatalf("ResolvePreset: %v", err)
	}
	if cfg.UserComms.Input != InputDirect {
		t.Errorf("override didn't apply; got %q, want direct", cfg.UserComms.Input)
	}
}

// TestEffectiveUserCommsInput_AutoDetection pins the auto-detection
// rule: explicit override wins; else the preset default flows through
// (the chat preset's default IS InputDirect — no special-case flag).
func TestEffectiveUserCommsInput_AutoDetection(t *testing.T) {
	cases := []struct {
		name             string
		presetDefault    InputMode
		explicitOverride InputMode
		want             InputMode
	}{
		{"chat preset default direct", InputDirect, "", InputDirect},
		{"headless worker", InputInbox, "", InputInbox},
		{"remote stays silent", InputNone, "", InputNone},
		{"teammate via leader", InputViaLeader, "", InputViaLeader},
		{"explicit override wins over preset", InputInbox, InputDirect, InputDirect},
		{"explicit override can silence chat", InputDirect, InputNone, InputNone},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := EffectiveUserCommsInput(tc.presetDefault, tc.explicitOverride)
			if got != tc.want {
				t.Errorf("EffectiveUserCommsInput(%q, %q) = %q, want %q",
					tc.presetDefault, tc.explicitOverride, got, tc.want)
			}
		})
	}
}

// TestChatPreset pins the chat mode's load-bearing dimension values:
// foreground scheduling (the session lives as long as the chat tab)
// and direct UserComms (HumanGate prompts render inline).
func TestChatPreset(t *testing.T) {
	cfg, err := ResolvePreset(ModeChat, nil)
	if err != nil {
		t.Fatalf("ResolvePreset(chat): %v", err)
	}
	if cfg.Scheduling.Mode != SchedulingForeground {
		t.Errorf("chat scheduling = %q, want foreground", cfg.Scheduling.Mode)
	}
	if cfg.UserComms.Input != InputDirect {
		t.Errorf("chat UserComms.Input = %q, want direct", cfg.UserComms.Input)
	}
}

func TestResolvePreset_OverridesApplied(t *testing.T) {
	overrides := &SpawnConfig{
		Context: ContextConfig{
			Model: "gpt-5",
		},
		Permissions: PermissionsConfig{
			Tools: []string{"file-read", "file-write"},
		},
	}

	cfg, err := ResolvePreset(ModeWorker, overrides)
	if err != nil {
		t.Fatalf("ResolvePreset: %v", err)
	}

	// Override values applied:
	if cfg.Context.Model != "gpt-5" {
		t.Errorf("Model = %q, want gpt-5 (override)", cfg.Context.Model)
	}
	if len(cfg.Permissions.Tools) != 2 {
		t.Errorf("Tools = %v, want [file_read file_write] (override)", cfg.Permissions.Tools)
	}

	// Non-override values kept from preset:
	if cfg.Scheduling.Mode != SchedulingForeground {
		t.Errorf("Scheduling.Mode = %q, should keep preset value", cfg.Scheduling.Mode)
	}
	if cfg.Isolation.Mode != IsolationInProcess {
		t.Errorf("Isolation.Mode = %q, should keep preset value", cfg.Isolation.Mode)
	}
}

func TestResolvePreset_WorkspaceOverride(t *testing.T) {
	overrides := &SpawnConfig{
		Context: ContextConfig{
			Workspace: WorkspaceConfig{
				BaseDir:   "work/research/",
				OutputDir: "work/research/output/",
			},
		},
	}
	cfg, err := ResolvePreset(ModeWorker, overrides)
	if err != nil {
		t.Fatalf("ResolvePreset: %v", err)
	}
	if cfg.Context.Workspace.BaseDir != "work/research/" {
		t.Errorf("BaseDir = %q", cfg.Context.Workspace.BaseDir)
	}
	// Preset defaults preserved:
	if cfg.Context.Workspace.Mode != WorkspaceShared {
		t.Errorf("Workspace.Mode = %q, should keep preset 'shared'", cfg.Context.Workspace.Mode)
	}
}

func TestKnownModes(t *testing.T) {
	modes := KnownModes()
	if len(modes) != 10 {
		t.Errorf("KnownModes() returned %d modes, want 10", len(modes))
	}
	// Verify all canonical modes are present.
	expected := []Mode{ModeWorker, ModeBackground, ModeFork, ModeIsolated, ModeForkIsolated, ModeCoordinator, ModeTeamLeader, ModeTeammate, ModeRemote, ModeChat}
	for _, exp := range expected {
		if !IsKnownMode(exp) {
			t.Errorf("IsKnownMode(%q) = false, want true", exp)
		}
	}
}

func TestPromptTemplate_AllModesHaveTemplate(t *testing.T) {
	for _, mode := range KnownModes() {
		template := PromptTemplate(mode)
		if template == "" {
			t.Errorf("PromptTemplate(%q) is empty", mode)
		}
	}
}

func TestPromptTemplate_UnknownMode(t *testing.T) {
	if PromptTemplate("unknown") != "" {
		t.Error("PromptTemplate for unknown mode should return empty string")
	}
}

func TestPromptTemplate_ForkHasBoilerplate(t *testing.T) {
	tmpl := PromptTemplate(ModeFork)
	if !contains(tmpl, "FORK_CONTEXT") {
		t.Errorf("fork template missing FORK_CONTEXT tag: %q", tmpl)
	}
}

func TestMustResolvePreset_PanicsOnUnknown(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("MustResolvePreset should have panicked on unknown mode")
		}
	}()
	MustResolvePreset("unknown-mode")
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && indexOf(s, substr) >= 0
}

func indexOf(s, substr string) int {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
