// SPDX-License-Identifier: Apache-2.0

package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	"github.com/katyoti/katyoti-fwk/internal/extensions/agent/api/permrules"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	hgapi "github.com/katyoti/katyoti-fwk/internal/extensions/humangate/api"
	inboxapi "github.com/katyoti/katyoti-fwk/internal/extensions/inbox/api"
	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
	toolsapi "github.com/katyoti/katyoti-fwk/internal/extensions/tools/api"
)

// ── Fake HumanGate ──────────────────────────────────────────────────────

// fakeHumanGate is a programmable HumanGate for permission gate tests.
// PromptPermission returns the canned Decision (and SelectedRule if
// the test wants to exercise AllowAlways/DenyAlways).
//
// permissionCalls captures the requests seen so tests can assert on
// the message wording, suggestions, and tool name the kernel passed
// through from the policy.
type fakeHumanGate struct {
	decision     hgapi.PermissionDecision
	selectedRule string
	err          error

	permissionCalls []hgapi.PromptPermissionRequest
}

func (g *fakeHumanGate) PromptPermission(_ context.Context, in hgapi.PromptPermissionRequest) (hgapi.PromptPermissionResponse, error) {
	g.permissionCalls = append(g.permissionCalls, in)
	if g.err != nil {
		return hgapi.PromptPermissionResponse{Decision: hgapi.PermissionDecisionDenyOnce}, g.err
	}
	return hgapi.PromptPermissionResponse{Decision: g.decision, SelectedRule: g.selectedRule}, nil
}

// PromptText, PromptSelect, PromptConfirm are unused by the permission
// gate but required by the HumanGate interface.
func (g *fakeHumanGate) Prompt(_ context.Context, _ hgapi.PromptRequest) (hgapi.PromptResponse, error) {
	return hgapi.PromptResponse{}, errors.New("Prompt not used by permission gate")
}
func (g *fakeHumanGate) Capabilities() []string { return []string{"text", "select", "confirm"} }

// personTasks is the person answering a tool's permission human task
// : each human task started is one call of the fake's
// PromptPermission - the tool, the question and the rules it offers - and
// its decision is the human task's answer; an error ends it unanswered.
type personTasks struct {
	taskapi.TaskService
	gate    *fakeHumanGate
	answers map[string]taskapi.ResultResponse
}

var permissionDecisions = map[hgapi.PermissionDecision]string{
	hgapi.PermissionDecisionAllowOnce: inboxapi.PermissionAllowOnce, hgapi.PermissionDecisionAllowAlways: inboxapi.PermissionAllowAlways,
	hgapi.PermissionDecisionDenyOnce: inboxapi.PermissionDenyOnce, hgapi.PermissionDecisionDenyAlways: inboxapi.PermissionDenyAlways,
}

func (p *personTasks) Run(ctx context.Context, in taskapi.TaskRunRequest) (taskapi.TaskRunResponse, error) {
	var ht inboxapi.HumanTask
	raw, _ := json.Marshal(in.Input)
	_ = json.Unmarshal(raw, &ht)
	req := hgapi.PromptPermissionRequest{ToolName: strings.TrimSuffix(strings.TrimPrefix(ht.Title, "Allow "), "?"), Question: ht.Question}
	for _, r := range ht.Options {
		req.Suggestions = append(req.Suggestions, hgapi.PermissionSuggestion{Label: r, Rule: r})
	}
	run := fmt.Sprintf("ht-perm-%d", len(p.answers)+1)
	resp, err := p.gate.PromptPermission(ctx, req)
	if err != nil {
		p.answers[run] = taskapi.ResultResponse{RunID: run, Error: err.Error()}
	} else {
		p.answers[run] = taskapi.ResultResponse{RunID: run, Status: "completed", Output: inboxapi.HumanAnswer{
			Status: "success", By: cwmapi.Actor{ID: "jdoe", Kind: "person"},
			More: map[string]any{"decision": permissionDecisions[resp.Decision], "rule": resp.SelectedRule}}}
	}
	return taskapi.TaskRunResponse{RunID: run}, nil
}

func (p *personTasks) Result(_ context.Context, in taskapi.ResultRequest) (taskapi.ResultResponse, error) {
	return p.answers[in.RunID], nil
}

// putPerson registers the fake as the person answering human tasks.
func putPerson(app *application.Application, gate hgapi.HumanGate) {
	if g, ok := gate.(*fakeHumanGate); ok && g != nil {
		application.PutService[taskapi.TaskService](app, "default", &personTasks{gate: g, answers: map[string]taskapi.ResultResponse{}})
	}
}

// ── Test app helpers ────────────────────────────────────────────────────

// newAppCtxWithGate creates a minimal Application with a fake
// HumanGate registered, runs it through the bootstrap phases, and
// returns a context that resolves UseService[HumanGate] to the gate.
//
// If gate is nil, no HumanGate is registered — used to test the
// "Ask without HumanGate" failure path.
func newAppCtxWithGate(t *testing.T, gate hgapi.HumanGate) context.Context {
	t.Helper()

	cfgSvc := config.NewConfigService(loader.NewMapLoader(nil))
	app := application.New(cfgSvc)
	if err := app.Run(); err != nil {
		t.Fatalf("app.Run: %v", err)
	}
	if gate != nil {
		putPerson(app, gate)
	}
	return application.WithApplication(context.Background(), app)
}

// dispatcherWithTool builds a Dispatcher pre-loaded with a single
// synthetic tool wrapped via NewToolPlugin. The returned tool is the
// same instance the dispatcher routes to, so tests can read its
// callCount and gotInput.
func dispatcherWithTool(name string, pres toolsapi.PermissionResult) (*Dispatcher, *permissionTestTool) {
	tool := newPermTool(name, pres)
	d := NewDispatcher()
	d.RegisterPlugin(toolsapi.NewToolPlugin(tool))
	return d, tool
}

// ── Allow path ──────────────────────────────────────────────────────────

func TestApplyPermissionGate_Allow_PassesThrough(t *testing.T) {
	d, _ := dispatcherWithTool("t_allow", toolsapi.PermissionResult{Behavior: toolsapi.PermissionAllow})
	ctx := newAppCtxWithGate(t, nil)

	out := applyPermissionGate(ctx, d, "t_allow", json.RawMessage(`{"echo":"original"}`))
	if !out.allow {
		t.Fatalf("allow = false, want true (errMsg=%q)", out.errMsg)
	}
	if string(out.effectiveInput) != `{"echo":"original"}` {
		t.Errorf("effectiveInput = %q, want original input passthrough", out.effectiveInput)
	}
}

func TestApplyPermissionGate_Allow_WithModifiedInput(t *testing.T) {
	modified := json.RawMessage(`{"echo":"rewritten"}`)
	d, _ := dispatcherWithTool("t_mod", toolsapi.PermissionResult{
		Behavior:      toolsapi.PermissionAllow,
		ModifiedInput: modified,
	})
	ctx := newAppCtxWithGate(t, nil)

	out := applyPermissionGate(ctx, d, "t_mod", json.RawMessage(`{"echo":"original"}`))
	if !out.allow {
		t.Fatalf("allow = false, want true")
	}
	if string(out.effectiveInput) != `{"echo":"rewritten"}` {
		t.Errorf("effectiveInput = %q, want ModifiedInput substitution", out.effectiveInput)
	}
}

// ── Deny path ───────────────────────────────────────────────────────────

func TestApplyPermissionGate_Deny_WithMessage(t *testing.T) {
	d, _ := dispatcherWithTool("t_deny", toolsapi.PermissionResult{
		Behavior: toolsapi.PermissionDeny,
		Message:  "policy says no",
	})
	ctx := newAppCtxWithGate(t, nil)

	out := applyPermissionGate(ctx, d, "t_deny", json.RawMessage(`{}`))
	if out.allow {
		t.Fatal("allow = true, want false")
	}
	if out.errMsg != "policy says no" {
		t.Errorf("errMsg = %q, want %q", out.errMsg, "policy says no")
	}
}

func TestApplyPermissionGate_Deny_FallbackMessage(t *testing.T) {
	d, _ := dispatcherWithTool("t_deny_silent", toolsapi.PermissionResult{
		Behavior: toolsapi.PermissionDeny,
	})
	ctx := newAppCtxWithGate(t, nil)

	out := applyPermissionGate(ctx, d, "t_deny_silent", json.RawMessage(`{}`))
	if out.allow {
		t.Fatal("allow = true, want false")
	}
	if !strings.Contains(out.errMsg, "permission denied") {
		t.Errorf("errMsg = %q, want fallback message containing 'permission denied'", out.errMsg)
	}
}

// ── Ask path ────────────────────────────────────────────────────────────

func TestApplyPermissionGate_Ask_Confirmed(t *testing.T) {
	gate := &fakeHumanGate{decision: hgapi.PermissionDecisionAllowOnce}
	d, _ := dispatcherWithTool("t_ask_yes", toolsapi.PermissionResult{
		Behavior: toolsapi.PermissionAsk,
		Message:  "do the thing?",
	})
	ctx := newAppCtxWithGate(t, gate)

	out := applyPermissionGate(ctx, d, "t_ask_yes", json.RawMessage(`{"echo":"x"}`))
	if !out.allow {
		t.Fatalf("allow = false, want true after confirmed (errMsg=%q)", out.errMsg)
	}
	if string(out.effectiveInput) != `{"echo":"x"}` {
		t.Errorf("effectiveInput = %q, want original passthrough", out.effectiveInput)
	}
	if len(gate.permissionCalls) != 1 || !strings.HasPrefix(gate.permissionCalls[0].Question, "do the thing?") {
		t.Fatalf("the person was asked %v, want 1 permission starting 'do the thing?'", gate.permissionCalls)
	}
	if gate.permissionCalls[0].ToolName != "t_ask_yes" {
		t.Errorf("ToolName = %q, want t_ask_yes", gate.permissionCalls[0].ToolName)
	}
}

func TestApplyPermissionGate_Ask_Denied(t *testing.T) {
	gate := &fakeHumanGate{decision: hgapi.PermissionDecisionDenyOnce}
	d, _ := dispatcherWithTool("t_ask_no", toolsapi.PermissionResult{
		Behavior: toolsapi.PermissionAsk,
		Message:  "outside workspace",
	})
	ctx := newAppCtxWithGate(t, gate)

	out := applyPermissionGate(ctx, d, "t_ask_no", json.RawMessage(`{}`))
	if out.allow {
		t.Fatal("allow = true, want false after denial")
	}
	if !strings.Contains(out.errMsg, "the person denied t_ask_no") {
		t.Errorf("errMsg = %q, want the person's denial of the tool", out.errMsg)
	}
}

func TestApplyPermissionGate_Ask_HumanGateError_Denies(t *testing.T) {
	gate := &fakeHumanGate{err: errors.New("prompt failed")}
	d, _ := dispatcherWithTool("t_ask_err", toolsapi.PermissionResult{
		Behavior: toolsapi.PermissionAsk,
		Message:  "ok?",
	})
	ctx := newAppCtxWithGate(t, gate)

	out := applyPermissionGate(ctx, d, "t_ask_err", json.RawMessage(`{}`))
	if out.allow {
		t.Fatal("allow = true, want false (HumanGate error should fail closed)")
	}
	if !strings.Contains(out.errMsg, "ended without an answer: prompt failed") {
		t.Errorf("errMsg = %q, want the permission ended without an answer", out.errMsg)
	}
}

func TestApplyPermissionGate_Ask_FallbackQuestionWhenPolicyMessageBlank(t *testing.T) {
	gate := &fakeHumanGate{decision: hgapi.PermissionDecisionAllowOnce}
	d, _ := dispatcherWithTool("t_ask_blank", toolsapi.PermissionResult{
		Behavior: toolsapi.PermissionAsk,
		// Message intentionally blank — kernel should fall back.
	})
	ctx := newAppCtxWithGate(t, gate)

	_ = applyPermissionGate(ctx, d, "t_ask_blank", json.RawMessage(`{}`))
	if len(gate.permissionCalls) != 1 {
		t.Fatalf("HumanGate not called: %v", gate.permissionCalls)
	}
	if !strings.Contains(gate.permissionCalls[0].Question, "t_ask_blank") {
		t.Errorf("fallback question = %q, want substring 't_ask_blank'", gate.permissionCalls[0].Question)
	}
}

func TestApplyPermissionGate_Ask_ConfirmedWithModifiedInput(t *testing.T) {
	gate := &fakeHumanGate{decision: hgapi.PermissionDecisionAllowOnce}
	d, _ := dispatcherWithTool("t_ask_mod", toolsapi.PermissionResult{
		Behavior:      toolsapi.PermissionAsk,
		Message:       "ok?",
		ModifiedInput: json.RawMessage(`{"echo":"sanitized"}`),
	})
	ctx := newAppCtxWithGate(t, gate)

	out := applyPermissionGate(ctx, d, "t_ask_mod", json.RawMessage(`{"echo":"original"}`))
	if !out.allow {
		t.Fatalf("allow = false, want true")
	}
	if string(out.effectiveInput) != `{"echo":"sanitized"}` {
		t.Errorf("effectiveInput = %q, want ModifiedInput after Ask-confirmed", out.effectiveInput)
	}
}

// ── Passthrough path (not built) ────────────────────────────────────────

// A tool that asks to decide its own permission (not built) is denied, with
// the reason — it was taken for an allow.
func TestApplyPermissionGate_Passthrough_IsDenied(t *testing.T) {
	d, _ := dispatcherWithTool("t_pass", toolsapi.PermissionResult{
		Behavior: toolsapi.PermissionPassthrough,
	})
	ctx := newAppCtxWithGate(t, nil)

	out := applyPermissionGate(ctx, d, "t_pass", json.RawMessage(`{"echo":"x"}`))
	if out.allow || !strings.Contains(out.errMsg, "denied") {
		t.Fatalf("passthrough = %+v; want it denied, saying why", out)
	}
}

// ── Non-wrapper plugin (no Tool registered) ─────────────────────────────

func TestApplyPermissionGate_NoToolRegistered_FallsThroughAllow(t *testing.T) {
	// A bare dispatcher with no tools at all — simulates the case
	// where the named tool came from a non-wrapper plugin (MCP,
	// sidecar). The gate must NOT block; the existing plugin-side
	// machinery (or the tool itself) is responsible.
	d := NewDispatcher()
	ctx := newAppCtxWithGate(t, nil)

	out := applyPermissionGate(ctx, d, "remote_tool", json.RawMessage(`{}`))
	if !out.allow {
		t.Fatalf("allow = false, want true (no Tool → fall through)")
	}
}

// ── Rules system: profile deny + profile allow + always-decisions ──────

// fakeProfiles is a programmable permrules.Profiles implementation for
// gate tests. Uses an embedded Profile so the test can set up arbitrary
// allow/deny rules without going through the file loader.
type fakeProfiles struct {
	profile     *permrules.Profile
	defaultName string

	addedAllow []permrules.Rule
	addedDeny  []permrules.Rule
}

func (f *fakeProfiles) Active(_ string) *permrules.Profile { return f.profile }
func (f *fakeProfiles) Default() string                    { return f.defaultName }
func (f *fakeProfiles) AddSessionAllow(rule permrules.Rule) {
	f.addedAllow = append(f.addedAllow, rule)
	if f.profile != nil {
		f.profile.AddSessionAllow(rule)
	}
}
func (f *fakeProfiles) AddSessionDeny(rule permrules.Rule) {
	f.addedDeny = append(f.addedDeny, rule)
	if f.profile != nil {
		f.profile.AddSessionDeny(rule)
	}
}

// newAppCtxWithGateAndProfiles wires both the fake HumanGate and a
// fake permrules.Profiles into the application context, so the gate
// can resolve both via UseService.
func newAppCtxWithGateAndProfiles(t *testing.T, gate hgapi.HumanGate, profiles permrules.Profiles) context.Context {
	t.Helper()
	cfgSvc := config.NewConfigService(loader.NewMapLoader(nil))
	app := application.New(cfgSvc)
	if err := app.Run(); err != nil {
		t.Fatalf("app.Run: %v", err)
	}
	if gate != nil {
		putPerson(app, gate)
	}
	if profiles != nil {
		application.PutService[permrules.Profiles](app, "default", profiles)
	}
	return application.WithApplication(context.Background(), app)
}

func TestApplyPermissionGate_ProfileDeny_BlocksBeforeTool(t *testing.T) {
	// Profile has a deny rule for the tool. The kernel must block
	// BEFORE calling tool.CheckPermissions — even if the tool would
	// have allowed.
	profile := &permrules.Profile{
		Name:      "test",
		DenyRules: []permrules.Rule{permrules.MustParseRule("file-write:**")},
	}
	profiles := &fakeProfiles{profile: profile}

	tool := newPermTool("file-write", toolsapi.PermissionResult{Behavior: toolsapi.PermissionAllow})
	d := NewDispatcher()
	d.RegisterPlugin(toolsapi.NewToolPlugin(tool))

	ctx := newAppCtxWithGateAndProfiles(t, nil, profiles)
	out := applyPermissionGate(ctx, d, "file-write", json.RawMessage(`{}`))

	if out.allow {
		t.Fatal("allow = true, want false (profile deny should block)")
	}
	if !strings.Contains(out.errMsg, "blocked by permission profile") {
		t.Errorf("errMsg = %q, want substring 'blocked by permission profile'", out.errMsg)
	}
	// CRITICAL: tool.CheckPermissions must NOT have been called.
	if tool.callCount != 0 {
		t.Errorf("tool was called %d times, want 0 (profile deny short-circuits before tool)", tool.callCount)
	}
}

func TestApplyPermissionGate_ProfileAllow_SilencesAsk(t *testing.T) {
	// Profile has an allow rule that matches the tool's Ask. The
	// kernel must NOT call PromptPermission — the profile pre-approves
	// the call silently.
	profile := &permrules.Profile{
		Name:       "test",
		AllowRules: []permrules.Rule{permrules.MustParseRule("file-read.workspace_path:**")},
	}
	profiles := &fakeProfiles{profile: profile}
	gate := &fakeHumanGate{decision: hgapi.PermissionDecisionDenyOnce} // would deny if called

	d, _ := dispatcherWithTool("file-read", toolsapi.PermissionResult{
		Behavior:   toolsapi.PermissionAsk,
		Message:    "outside workspace",
		Dimensions: map[string]string{"workspace_path": "src/main.go"},
	})

	ctx := newAppCtxWithGateAndProfiles(t, gate, profiles)
	out := applyPermissionGate(ctx, d, "file-read", json.RawMessage(`{"workspace_path":"src/main.go"}`))

	if !out.allow {
		t.Fatalf("allow = false, want true (profile should silence Ask). errMsg=%q", out.errMsg)
	}
	if len(gate.permissionCalls) != 0 {
		t.Errorf("HumanGate was called %d times, want 0 (profile pre-approval should skip prompt)", len(gate.permissionCalls))
	}
}

func TestApplyPermissionGate_ProfileAllow_NoMatch_FallsThroughToPrompt(t *testing.T) {
	// Profile has allow rules but none match this Ask. Gate must
	// fall through to PromptPermission.
	profile := &permrules.Profile{
		Name:       "test",
		AllowRules: []permrules.Rule{permrules.MustParseRule("file-read.workspace_path:**")},
	}
	profiles := &fakeProfiles{profile: profile}
	gate := &fakeHumanGate{decision: hgapi.PermissionDecisionAllowOnce}

	// Ask with absolute_path — won't match the workspace_path allow rule.
	d, _ := dispatcherWithTool("file-read", toolsapi.PermissionResult{
		Behavior:   toolsapi.PermissionAsk,
		Message:    "outside workspace",
		Dimensions: map[string]string{"absolute_path": "/etc/hosts"},
	})

	ctx := newAppCtxWithGateAndProfiles(t, gate, profiles)
	out := applyPermissionGate(ctx, d, "file-read", json.RawMessage(`{"absolute_path":"/etc/hosts"}`))

	if !out.allow {
		t.Fatal("allow = false, want true after AllowOnce")
	}
	if len(gate.permissionCalls) != 1 {
		t.Errorf("HumanGate was called %d times, want 1 (profile didn't match → prompt)", len(gate.permissionCalls))
	}
}

func TestApplyPermissionGate_AllowAlways_AddsSessionAllow(t *testing.T) {
	profile := &permrules.Profile{Name: "test"}
	profiles := &fakeProfiles{profile: profile}
	gate := &fakeHumanGate{
		decision:     hgapi.PermissionDecisionAllowAlways,
		selectedRule: "file-read.absolute_path:/etc/**",
	}

	d, _ := dispatcherWithTool("file-read", toolsapi.PermissionResult{
		Behavior: toolsapi.PermissionAsk,
		Message:  "ok?",
	})

	ctx := newAppCtxWithGateAndProfiles(t, gate, profiles)
	out := applyPermissionGate(ctx, d, "file-read", json.RawMessage(`{}`))

	if !out.allow {
		t.Fatalf("allow = false, want true after AllowAlways")
	}
	if len(profiles.addedAllow) != 1 {
		t.Fatalf("expected 1 session-add, got %d", len(profiles.addedAllow))
	}
	if profiles.addedAllow[0].Raw != "file-read.absolute_path:/etc/**" {
		t.Errorf("session-added rule = %q, want %q", profiles.addedAllow[0].Raw, "file-read.absolute_path:/etc/**")
	}
}

func TestApplyPermissionGate_DenyAlways_AddsSessionDenyAndBlocks(t *testing.T) {
	profile := &permrules.Profile{Name: "test"}
	profiles := &fakeProfiles{profile: profile}
	gate := &fakeHumanGate{
		decision:     hgapi.PermissionDecisionDenyAlways,
		selectedRule: "file-read:**",
	}

	d, _ := dispatcherWithTool("file-read", toolsapi.PermissionResult{
		Behavior: toolsapi.PermissionAsk,
		Message:  "ok?",
	})

	ctx := newAppCtxWithGateAndProfiles(t, gate, profiles)
	out := applyPermissionGate(ctx, d, "file-read", json.RawMessage(`{}`))

	if out.allow {
		t.Fatal("allow = true, want false after DenyAlways")
	}
	if len(profiles.addedDeny) != 1 {
		t.Fatalf("expected 1 session-add deny, got %d", len(profiles.addedDeny))
	}
	if profiles.addedDeny[0].Raw != "file-read:**" {
		t.Errorf("session-added deny rule = %q", profiles.addedDeny[0].Raw)
	}
}

func TestApplyPermissionGate_AllowAlways_InvalidRule_StillAllowsThisCall(t *testing.T) {
	// SelectedRule from the prompt is malformed. The gate should
	// still honor the user's "allow once" intent for THIS call but
	// log a warning and not add anything to the session profile.
	profile := &permrules.Profile{Name: "test"}
	profiles := &fakeProfiles{profile: profile}
	gate := &fakeHumanGate{
		decision:     hgapi.PermissionDecisionAllowAlways,
		selectedRule: "this is not a valid rule",
	}

	d, _ := dispatcherWithTool("file-read", toolsapi.PermissionResult{
		Behavior: toolsapi.PermissionAsk,
		Message:  "ok?",
	})

	ctx := newAppCtxWithGateAndProfiles(t, gate, profiles)
	out := applyPermissionGate(ctx, d, "file-read", json.RawMessage(`{}`))

	if !out.allow {
		t.Fatalf("allow = false, want true (this call should still proceed)")
	}
	if len(profiles.addedAllow) != 0 {
		t.Errorf("expected 0 session-adds (rule was invalid), got %d", len(profiles.addedAllow))
	}
}

func TestApplyPermissionGate_PromptCarriesSuggestions(t *testing.T) {
	// Verify that the tool's Suggestions are forwarded to the gate's
	// PromptPermission request.
	suggestions := []toolsapi.PermissionSuggestion{
		{Label: "narrow", Rule: "file-read.absolute_path:/etc/foo"},
		{Label: "broad", Rule: "file-read.absolute_path:/etc/**"},
	}
	gate := &fakeHumanGate{decision: hgapi.PermissionDecisionAllowOnce}

	d, _ := dispatcherWithTool("file-read", toolsapi.PermissionResult{
		Behavior:    toolsapi.PermissionAsk,
		Message:     "ok?",
		Suggestions: suggestions,
	})

	ctx := newAppCtxWithGate(t, gate)
	_ = applyPermissionGate(ctx, d, "file-read", json.RawMessage(`{}`))

	if len(gate.permissionCalls) != 1 {
		t.Fatal("expected 1 prompt call")
	}
	got := gate.permissionCalls[0].Suggestions
	if len(got) != 2 {
		t.Fatalf("Suggestions len = %d, want 2", len(got))
	}
	if got[0].Rule != "file-read.absolute_path:/etc/foo" {
		t.Errorf("first suggestion = %+v", got[0])
	}
	if got[1].Rule != "file-read.absolute_path:/etc/**" {
		t.Errorf("second suggestion = %+v", got[1])
	}
}
