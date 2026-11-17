// SPDX-License-Identifier: Apache-2.0

package driver_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/extensions/claudecode/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/claudecode/internal/driver"
	cctest "github.com/katyoti/katyoti-fwk/internal/extensions/claudecode/testing"
)

func init() { driver.ShutdownGrace = 2 * time.Second }

// read is the fake's next frame, or the test fails.
func read(t *testing.T, f *cctest.Fake) map[string]any {
	t.Helper()
	m, err := f.Read()
	if err != nil {
		t.Errorf("fake claude: reading the host: %v", err)
		return nil
	}
	return m
}

// handshake plays initialize with a subscription account.
func handshake(t *testing.T, f *cctest.Fake) {
	t.Helper()
	m := read(t, f)
	id, req := cctest.Request(m)
	if m["type"] != "control_request" || req["subtype"] != "initialize" {
		t.Errorf("first frame = %v, want the initialize control request", m)
	}
	if _, ok := req["hooks"]; !ok {
		t.Errorf("initialize carries no hooks field: %v", req)
	}
	_ = f.Write(cctest.InitializeOK(id))
}

// waitState waits for the worker to reach s.
func waitState(t *testing.T, w *driver.Worker, s api.State) api.Worker {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if i := w.Info(); i.State == s {
			return i
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("worker state = %s, want %s (last error %q)", w.Info().State, s, w.Info().LastError)
	return api.Worker{}
}

// TestDriverTurn: handshake, a prompt, an assistant answer with a tool call
// and its result, the turn's result - the worker idles with the session id,
// the cost and the conversation as events.
func TestDriverTurn(t *testing.T) {
	f := cctest.New()
	w := driver.New(f, api.Worker{})
	go func() {
		handshake(t, f)
		m := read(t, f)
		msg, _ := m["message"].(map[string]any)
		if m["type"] != "user" || msg["content"] != "fix the test" {
			t.Errorf("prompt frame = %v", m)
		}
		_ = f.WriteLine("[SandboxDebug] a diagnostic line the host skips")
		_ = f.Write(cctest.SystemInit("sess-1", "none"))
		_ = f.Write(cctest.AssistantText("Looking at it."))
		_ = f.Write(cctest.AssistantToolUse("tu1", "Bash", map[string]any{"command": "go test ./..."}))
		_ = f.Write(cctest.ToolResult("tu1", "ok", false))
		_ = f.Write(map[string]any{"type": "keep_alive"})
		_ = f.Write(map[string]any{"type": "a_type_from_the_future", "x": 1})
		_ = f.Write(cctest.Result("sess-1", "Fixed.", false, 0.42))
		_ = f.Write(cctest.SessionState("idle"))
	}()
	if err := w.Start(context.Background(), "fix the test"); err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitState(t, w, api.StateIdle)
	deadline := time.Now().Add(5 * time.Second)
	for w.Info().Turns == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	i := w.Info()
	if i.ClaudeSessionID != "sess-1" || i.Turns != 1 || i.CostUSD != 0.42 || i.LastResult != "Fixed." {
		t.Errorf("info = %+v", i)
	}
	if i.Account == nil || i.Account.SubscriptionType != "max" || i.ClaudeVersion != "2.1.296" {
		t.Errorf("account/version not recorded: %+v", i)
	}
	kinds := map[api.EventKind]int{}
	for _, e := range w.History() {
		kinds[e.Kind]++
	}
	for _, k := range []api.EventKind{api.EventText, api.EventToolUse, api.EventToolResult, api.EventResult, api.EventFrame} {
		if kinds[k] == 0 {
			t.Errorf("no %s event in %v", k, kinds)
		}
	}
	f.Exit(0)
	waitState(t, w, api.StateFinished)
}

// TestDriverPermissionAllowAndDeny: a can_use_tool puts the worker in
// awaiting-permission with the request; allow for the session answers with
// the original input and the suggestions sent to the session; a deny answers
// with its message.
func TestDriverPermissionAllowAndDeny(t *testing.T) {
	f := cctest.New()
	w := driver.New(f, api.Worker{})
	answers := make(chan map[string]any, 2)
	go func() {
		handshake(t, f)
		read(t, f) // the prompt
		_ = f.Write(cctest.SystemInit("s", "none"))
		_ = f.Write(cctest.CanUseTool("perm-1", "Bash", map[string]any{"command": "rm -rf build"},
			map[string]any{"type": "addRules", "rules": []any{map[string]any{"toolName": "Bash", "ruleContent": "rm -rf build"}},
				"behavior": "allow", "destination": "localSettings"}))
		answers <- read(t, f)
		_ = f.Write(cctest.CanUseTool("perm-2", "Write", map[string]any{"file_path": "/etc/hosts"}))
		answers <- read(t, f)
		_ = f.Write(cctest.Result("s", "done", false, 0.1))
	}()
	if err := w.Start(context.Background(), "clean"); err != nil {
		t.Fatalf("Start: %v", err)
	}
	i := waitState(t, w, api.StateAwaitingPermission)
	if i.Pending == nil || i.Pending.RequestID != "perm-1" || i.Pending.ToolName != "Bash" {
		t.Fatalf("pending = %+v", i.Pending)
	}
	if err := w.Answer(api.AnswerInput{RequestID: "wrong", Behavior: api.BehaviorAllow}); err == nil {
		t.Errorf("an answer to a request not pending was accepted")
	}
	if err := w.Answer(api.AnswerInput{RequestID: "perm-1", Behavior: api.BehaviorAllow, Scope: api.ScopeSession}); err != nil {
		t.Fatalf("Answer allow: %v", err)
	}
	a := <-answers
	body := response(t, a, "perm-1")
	if body["behavior"] != "allow" {
		t.Errorf("allow answer = %v", body)
	}
	if in, _ := body["updatedInput"].(map[string]any); in["command"] != "rm -rf build" {
		t.Errorf("updatedInput = %v, want the original input", body["updatedInput"])
	}
	ups, _ := body["updatedPermissions"].([]any)
	if len(ups) != 1 || ups[0].(map[string]any)["destination"] != "session" {
		t.Errorf("updatedPermissions = %v, want the suggestion sent to the session", ups)
	}
	i = waitState(t, w, api.StateAwaitingPermission)
	if i.Pending == nil || i.Pending.RequestID != "perm-2" {
		t.Fatalf("second pending = %+v", i.Pending)
	}
	if err := w.Answer(api.AnswerInput{RequestID: "perm-2", Behavior: api.BehaviorDeny, Message: "not that file"}); err != nil {
		t.Fatalf("Answer deny: %v", err)
	}
	body = response(t, <-answers, "perm-2")
	if body["behavior"] != "deny" || body["message"] != "not that file" {
		t.Errorf("deny answer = %v", body)
	}
	waitState(t, w, api.StateIdle)
	w.Stop()
	if s := w.Info().State; s != api.StateFinished {
		t.Errorf("after Stop state = %s, want finished", s)
	}
}

func response(t *testing.T, m map[string]any, requestID string) map[string]any {
	t.Helper()
	r, _ := m["response"].(map[string]any)
	if m["type"] != "control_response" || r["subtype"] != "success" || r["request_id"] != requestID {
		t.Fatalf("answer frame = %v", m)
	}
	body, _ := r["response"].(map[string]any)
	return body
}

// TestDriverPermissionWithdrawn: a control_cancel_request withdraws the
// pending permission - the worker runs again, nothing to answer.
func TestDriverPermissionWithdrawn(t *testing.T) {
	f := cctest.New()
	w := driver.New(f, api.Worker{})
	go func() {
		handshake(t, f)
		read(t, f)
		_ = f.Write(cctest.CanUseTool("perm-9", "Edit", map[string]any{"file_path": "a.go"}))
		time.Sleep(50 * time.Millisecond)
		_ = f.Write(cctest.Cancel("perm-9"))
	}()
	if err := w.Start(context.Background(), "go"); err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitState(t, w, api.StateAwaitingPermission)
	i := waitState(t, w, api.StateRunning)
	if i.Pending != nil {
		t.Errorf("pending after withdrawal = %+v", i.Pending)
	}
	if err := w.Answer(api.AnswerInput{RequestID: "perm-9", Behavior: api.BehaviorAllow}); err == nil {
		t.Errorf("a withdrawn permission was answered")
	}
	f.Exit(0)
}

// TestDriverInterrupt: an interrupt is a control request; the turn's result
// brings the worker back to idle.
func TestDriverInterrupt(t *testing.T) {
	f := cctest.New()
	w := driver.New(f, api.Worker{})
	go func() {
		handshake(t, f)
		read(t, f)
		m := read(t, f)
		id, req := cctest.Request(m)
		if req["subtype"] != "interrupt" {
			t.Errorf("frame = %v, want an interrupt", m)
		}
		_ = f.Write(cctest.ControlSuccess(id, map[string]any{}))
		r := cctest.Result("s", "", false, 0)
		r["terminal_reason"] = "aborted_streaming"
		_ = f.Write(r)
	}()
	if err := w.Start(context.Background(), "a long task"); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := w.Interrupt(context.Background()); err != nil {
		t.Fatalf("Interrupt: %v", err)
	}
	waitState(t, w, api.StateIdle)
	f.Exit(0)
}

// TestDriverCrash: Claude Code exits non-zero without a result - the worker
// is crashed, its reason carrying the stderr tail; an unknown control request
// is answered with an error, never left hanging.
func TestDriverCrash(t *testing.T) {
	f := cctest.New()
	w := driver.New(f, api.Worker{})
	unknown := make(chan map[string]any, 1)
	go func() {
		handshake(t, f)
		read(t, f)
		_ = f.Write(map[string]any{"type": "control_request", "request_id": "h1", "request": map[string]any{"subtype": "hook_callback"}})
		unknown <- read(t, f)
		f.StderrLine("panic: something broke")
		time.Sleep(50 * time.Millisecond)
		f.Exit(3)
	}()
	if err := w.Start(context.Background(), "go"); err != nil {
		t.Fatalf("Start: %v", err)
	}
	m := <-unknown
	r, _ := m["response"].(map[string]any)
	if r["subtype"] != "error" || r["request_id"] != "h1" {
		t.Errorf("unknown control request answered %v, want an error", m)
	}
	i := waitState(t, w, api.StateCrashed)
	if !strings.Contains(i.LastError, "code 3") || !strings.Contains(i.LastError, "something broke") {
		t.Errorf("crash reason = %q", i.LastError)
	}
	if err := w.Send("again"); err == nil {
		t.Errorf("a crashed worker accepted a prompt")
	}
}

// TestDriverMalformedFrame: a '{' line that is not JSON ends the worker crashed.
func TestDriverMalformedFrame(t *testing.T) {
	f := cctest.New()
	w := driver.New(f, api.Worker{})
	go func() {
		handshake(t, f)
		read(t, f)
		_ = f.WriteLine("{not json")
		f.Exit(0)
	}()
	if err := w.Start(context.Background(), "go"); err != nil {
		t.Fatalf("Start: %v", err)
	}
	i := waitState(t, w, api.StateCrashed)
	if !strings.Contains(i.LastError, "malformed") {
		t.Errorf("reason = %q", i.LastError)
	}
}

// TestSubscriptionGuardRefusesAnAPIKeyAccount: an initialize account billed
// by an API key refuses the start - no prompt is written, the worker ends.
func TestSubscriptionGuardRefusesAnAPIKeyAccount(t *testing.T) {
	f := cctest.New()
	w := driver.New(f, api.Worker{})
	wrote := make(chan map[string]any, 1)
	go func() {
		m := read(t, f)
		id, _ := cctest.Request(m)
		_ = f.Write(cctest.InitializeAPIKey(id))
		if next, err := f.Read(); err == nil {
			wrote <- next
		}
		close(wrote)
	}()
	err := w.Start(context.Background(), "spend my key")
	if err == nil || !strings.Contains(err.Error(), "subscription") {
		t.Fatalf("Start = %v, want the subscription refusal", err)
	}
	if m, ok := <-wrote; ok {
		t.Errorf("the host wrote after the refusal: %v", m)
	}
	i := waitState(t, w, api.StateCrashed)
	if !strings.Contains(i.LastError, "subscription") {
		t.Errorf("reason = %q", i.LastError)
	}
}

// TestSubscriptionGuardStopsAnAPIKeyTurn: a turn whose system/init reports an
// API key stops the worker.
func TestSubscriptionGuardStopsAnAPIKeyTurn(t *testing.T) {
	f := cctest.New()
	w := driver.New(f, api.Worker{})
	go func() {
		handshake(t, f)
		read(t, f)
		_ = f.Write(cctest.SystemInit("s", "ANTHROPIC_API_KEY"))
	}()
	if err := w.Start(context.Background(), "go"); err != nil {
		t.Fatalf("Start: %v", err)
	}
	i := waitState(t, w, api.StateCrashed)
	if !strings.Contains(i.LastError, "subscription") {
		t.Errorf("reason = %q", i.LastError)
	}
}

// keep encoding/json used for readers of this file.
var _ = json.Marshal
