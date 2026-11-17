// SPDX-License-Identifier: Apache-2.0

package translator_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/eventbus"
	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/terminal"
	termadapter "github.com/katyoti/katyoti-fwk/internal/kts/protocol/terminal/adapter"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/terminal/room"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/terminal/translator"
)

// testHarness sets up a Room + Translator + connected TerminalTransport client.
type testHarness struct {
	bus    eventbus.EventBus
	room   *room.Room
	trans  *translator.Translator
	client *termadapter.Channel
	ch     <-chan json.RawMessage
}

func newHarness(t *testing.T) *testHarness {
	t.Helper()
	bus := eventbus.NewMemoryEventBus()
	r := room.NewRoom(t.TempDir(), nil)
	t.Cleanup(func() { r.Close() })

	sess, err := r.CreateSession("s1", "test", "task-1")
	if err != nil {
		t.Fatal(err)
	}

	server, client := termadapter.NewChannelPair(64)
	t.Cleanup(func() { server.Close() })

	ctx := context.Background()
	if err := r.Connect(ctx, server, sess.ID); err != nil {
		t.Fatal(err)
	}

	ch, err := client.Receive(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Drain the initial StateSyncEvent.
	recvOne(t, ch, 2*time.Second)

	tr := translator.New(bus, r, "s1")
	tr.Start()
	t.Cleanup(func() { tr.Stop() })

	return &testHarness{bus: bus, room: r, trans: tr, client: client, ch: ch}
}

func recvOne(t *testing.T, ch <-chan json.RawMessage, timeout time.Duration) json.RawMessage {
	t.Helper()
	select {
	case msg, ok := <-ch:
		if !ok {
			t.Fatal("channel closed")
		}
		return msg
	case <-time.After(timeout):
		t.Fatal("timeout waiting for message")
		return nil
	}
}

func recvType(t *testing.T, ch <-chan json.RawMessage, timeout time.Duration) string {
	t.Helper()
	raw := recvOne(t, ch, timeout)
	var probe struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		t.Fatalf("unmarshal type: %v", err)
	}
	return probe.Type
}

// ── Tests ─────────────────────────────────────────────────────────────────

func TestTranslator_TextStreaming(t *testing.T) {
	h := newHarness(t)

	// Emit text deltas.
	h.bus.Publish(agentapi.EventTextDelta, agentapi.TextDeltaEvent{
		EventBase: agentapi.EventBase{Type: agentapi.EventTextDelta},
		Text:      "Hello ",
		Turn:      1,
	})
	h.bus.Publish(agentapi.EventTextDelta, agentapi.TextDeltaEvent{
		EventBase: agentapi.EventBase{Type: agentapi.EventTextDelta},
		Text:      "world!",
		Turn:      1,
	})

	// Receive the two msg.delta events.
	if typ := recvType(t, h.ch, time.Second); typ != "msg.delta" {
		t.Errorf("expected msg.delta, got %q", typ)
	}
	if typ := recvType(t, h.ch, time.Second); typ != "msg.delta" {
		t.Errorf("expected msg.delta, got %q", typ)
	}

	// Emit LLM response → finalize text.
	h.bus.Publish(agentapi.EventLLMResponse, agentapi.LLMResponseEvent{
		EventBase: agentapi.EventBase{Type: agentapi.EventLLMResponse},
		Turn:      1,
	})

	// The LLM response also emits a cumulative "stats" event (token telemetry
	// for the status bar); skip it before checking the text lifecycle.
	typ := recvType(t, h.ch, time.Second)
	if typ == "stats" {
		typ = recvType(t, h.ch, time.Second)
	}
	if typ != "msg.new" {
		t.Errorf("expected msg.new, got %q", typ)
	}
	if typ := recvType(t, h.ch, time.Second); typ != "msg.done" {
		t.Errorf("expected msg.done, got %q", typ)
	}

	// Verify the text was persisted.
	history, _ := h.room.GetHistory("s1")
	if len(history) != 1 {
		t.Fatalf("expected 1 message in history, got %d", len(history))
	}
	var content terminal.TextContent
	if err := history[0].Content.As(&content); err != nil {
		t.Fatal(err)
	}
	if content.Text != "Hello world!" {
		t.Errorf("text = %q, want %q", content.Text, "Hello world!")
	}
}

func TestTranslator_ToolLifecycle(t *testing.T) {
	h := newHarness(t)

	// Tool call → running.
	h.bus.Publish(agentapi.EventToolCall, agentapi.ToolCallEvent{
		EventBase: agentapi.EventBase{Type: agentapi.EventToolCall},
		Tool:      "file-read",
		ID:        "tc-1",
		Input:     json.RawMessage(`{"file_path":"types.go"}`),
		Display:   &agentapi.ToolDisplay{Icon: "📄", Label: "Read"},
	})

	raw := recvOne(t, h.ch, time.Second)
	var newEvt terminal.MsgNewEvent
	if err := json.Unmarshal(raw, &newEvt); err != nil {
		t.Fatal(err)
	}
	if newEvt.Type != "msg.new" {
		t.Errorf("expected msg.new, got %q", newEvt.Type)
	}
	var tc terminal.ToolContent
	if err := newEvt.Msg.Content.As(&tc); err != nil {
		t.Fatal(err)
	}
	if tc.Status != "running" {
		t.Errorf("status = %q, want running", tc.Status)
	}
	if tc.Display == nil || tc.Display.Icon != "📄" {
		t.Error("display not resolved")
	}

	// Tool result → success.
	h.bus.Publish(agentapi.EventToolResult, agentapi.ToolResultEvent{
		EventBase:  agentapi.EventBase{Type: agentapi.EventToolResult},
		Tool:       "file-read",
		ID:         "tc-1",
		Result:     "120 lines",
		DurationMs: 45,
	})

	raw = recvOne(t, h.ch, time.Second)
	var updateEvt terminal.MsgUpdateEvent
	if err := json.Unmarshal(raw, &updateEvt); err != nil {
		t.Fatal(err)
	}
	if updateEvt.Type != "msg.update" {
		t.Errorf("expected msg.update, got %q", updateEvt.Type)
	}
	if err := updateEvt.Content.As(&tc); err != nil {
		t.Fatal(err)
	}
	if tc.Status != "success" {
		t.Errorf("status = %q, want success", tc.Status)
	}
}

func TestTranslator_PromptAnswer(t *testing.T) {
	h := newHarness(t)

	// Prompt shown.
	h.bus.Publish(agentapi.EventPromptShown, agentapi.PromptShownEvent{
		EventBase:  agentapi.EventBase{Type: agentapi.EventPromptShown},
		Question:   "What is your name?",
		PromptType: "text",
	})

	raw := recvOne(t, h.ch, time.Second)
	var newEvt terminal.MsgNewEvent
	json.Unmarshal(raw, &newEvt)
	var pc terminal.PromptContent
	newEvt.Msg.Content.As(&pc)
	if pc.Answered {
		t.Error("expected answered=false")
	}
	if pc.Question != "What is your name?" {
		t.Errorf("question = %q", pc.Question)
	}

	// User answers.
	h.bus.Publish(agentapi.EventUserInput, agentapi.UserInputEvent{
		EventBase: agentapi.EventBase{Type: agentapi.EventUserInput},
		Answer:    "Alice",
	})

	raw = recvOne(t, h.ch, time.Second)
	var updateEvt terminal.MsgUpdateEvent
	json.Unmarshal(raw, &updateEvt)
	updateEvt.Content.As(&pc)
	if !pc.Answered {
		t.Error("expected answered=true")
	}
	if pc.Answer != "Alice" {
		t.Errorf("answer = %q, want Alice", pc.Answer)
	}
}

func TestTranslator_Complete(t *testing.T) {
	h := newHarness(t)

	h.bus.Publish(agentapi.EventComplete, agentapi.CompleteEvent{
		EventBase: agentapi.EventBase{Type: agentapi.EventComplete},
		Status:    "completed",
		Summary:   "All done",
		Turns:     5,
		Duration:  3 * time.Second,
	})

	raw := recvOne(t, h.ch, time.Second)
	var newEvt terminal.MsgNewEvent
	json.Unmarshal(raw, &newEvt)
	var sc terminal.SystemContent
	newEvt.Msg.Content.As(&sc)
	if sc.Level != "success" {
		t.Errorf("level = %q, want success", sc.Level)
	}

	// Session status should be updated.
	sess, ok := h.room.GetSession("s1")
	if !ok {
		t.Fatal("session not found")
	}
	if sess.Status != "done" {
		t.Errorf("session status = %q, want done", sess.Status)
	}
}

func TestTranslator_DisplayResolution(t *testing.T) {
	h := newHarness(t)

	// Tool with template.
	h.bus.Publish(agentapi.EventToolCall, agentapi.ToolCallEvent{
		EventBase: agentapi.EventBase{Type: agentapi.EventToolCall},
		Tool:      "file-read",
		ID:        "tc-tmpl",
		Input:     json.RawMessage(`{"file_path":"main.go","offset":10}`),
		Display:   &agentapi.ToolDisplay{Icon: "📄", Label: "Read", Template: "{{.file_path}}"},
	})

	raw := recvOne(t, h.ch, time.Second)
	var newEvt terminal.MsgNewEvent
	json.Unmarshal(raw, &newEvt)
	var tc terminal.ToolContent
	newEvt.Msg.Content.As(&tc)
	if tc.Display == nil {
		t.Fatal("expected display data")
	}
	if tc.Display.Title != "main.go" {
		t.Errorf("title = %q, want main.go", tc.Display.Title)
	}

	// Tool without display → fallback.
	h.bus.Publish(agentapi.EventToolCall, agentapi.ToolCallEvent{
		EventBase: agentapi.EventBase{Type: agentapi.EventToolCall},
		Tool:      "unknown_tool",
		ID:        "tc-fallback",
		Input:     json.RawMessage(`{"query":"test"}`),
	})

	raw = recvOne(t, h.ch, time.Second)
	json.Unmarshal(raw, &newEvt)
	newEvt.Msg.Content.As(&tc)
	if tc.Display == nil {
		t.Fatal("expected fallback display")
	}
	if tc.Display.Label != "unknown_tool" {
		t.Errorf("label = %q, want unknown_tool", tc.Display.Label)
	}
}

// ── ms08 task-engine bridge tests ─────────────────────────────────────────

// recvUntilType reads from ch until an event of the given type appears, or
// times out. Used by the team.* tests because the translator may also emit
// agent.* events on the same channel during the test.
func recvUntilType(t *testing.T, ch <-chan json.RawMessage, want string, timeout time.Duration) json.RawMessage {
	t.Helper()
	deadline := time.After(timeout)
	for {
		select {
		case msg, ok := <-ch:
			if !ok {
				t.Fatal("channel closed")
			}
			var probe struct {
				Type string `json:"type"`
			}
			_ = json.Unmarshal(msg, &probe)
			if probe.Type == want {
				return msg
			}
		case <-deadline:
			t.Fatalf("timeout waiting for event of type %q", want)
			return nil
		}
	}
}

func TestTranslator_BridgesWorkflowStarted(t *testing.T) {
	h := newHarness(t)

	h.bus.Publish(engine.WorkflowStartedTopic("run-abc"), &engine.WorkflowStartedEvent{
		RunID: "run-abc",
		Name:  "demo-magentic",
	})

	raw := recvUntilType(t, h.ch, "team.workflow_started", 2*time.Second)
	var got struct {
		Type  string `json:"type"`
		RunID string `json:"runId"`
		Name  string `json:"name"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.RunID != "run-abc" {
		t.Errorf("runId = %q, want run-abc", got.RunID)
	}
	if got.Name != "demo-magentic" {
		t.Errorf("name = %q, want demo-magentic", got.Name)
	}
}

func TestTranslator_BridgesChildStartedWithParent(t *testing.T) {
	h := newHarness(t)

	h.bus.Publish(engine.ChildStartedTopic("alice"), &engine.ChildStartedEvent{
		ChildName:   "alice",
		ChildKind:   "agent",
		RunID:       "run-child-1",
		ParentRunID: "run-parent-1",
	})

	raw := recvUntilType(t, h.ch, "team.child_started", 2*time.Second)
	var got struct {
		Type        string `json:"type"`
		RunID       string `json:"runId"`
		ParentRunID string `json:"parentRunId"`
		Name        string `json:"name"`
		Kind        string `json:"kind"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Name != "alice" {
		t.Errorf("name = %q, want alice", got.Name)
	}
	if got.RunID != "run-child-1" {
		t.Errorf("runId = %q, want run-child-1", got.RunID)
	}
	if got.ParentRunID != "run-parent-1" {
		t.Errorf("parentRunId = %q, want run-parent-1", got.ParentRunID)
	}
	if got.Kind != "agent" {
		t.Errorf("kind = %q, want agent", got.Kind)
	}
}

func TestTranslator_BridgesAgentMessage(t *testing.T) {
	h := newHarness(t)

	h.bus.Publish(engine.AgentMessageTopic("bob"), &engine.AgentMessageEvent{
		From: "alice",
		To:   "bob",
		Text: "hello bob",
	})

	raw := recvUntilType(t, h.ch, "team.message", 2*time.Second)
	var got struct {
		Type    string `json:"type"`
		From    string `json:"from"`
		To      string `json:"to"`
		Team    string `json:"team"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.From != "alice" || got.To != "bob" {
		t.Errorf("from/to = %q/%q, want alice/bob", got.From, got.To)
	}
	if got.Team != "" {
		t.Errorf("team should be empty for direct message, got %q", got.Team)
	}
	if got.Message != "hello bob" {
		t.Errorf("message = %q", got.Message)
	}
}

// TestTranslator_BridgesTopLevelAgentSession verifies that a top-level agent
// run (no parent) emits BOTH team.workflow_started and team.child_started so
// the GUI Team tab shows it as a card. Without this bridge, single-agent
// tasks (`type: agent` / `type: task`) wouldn't appear in the Team tab at
// all because they don't go through the spawner that emits child.* topics.
func TestTranslator_BridgesTopLevelAgentSession(t *testing.T) {
	h := newHarness(t)

	h.bus.Publish(agentapi.EventSessionStart, agentapi.SessionStartEvent{
		EventBase: agentapi.EventBase{
			Type:    agentapi.EventSessionStart,
			Emitter: "run-top-1",
		},
		Name: "hello-world",
		// ParentRunID empty → top-level
	})

	// Should emit BOTH team.workflow_started and team.child_started.
	wfRaw := recvUntilType(t, h.ch, "team.workflow_started", 2*time.Second)
	var wf struct {
		Type  string `json:"type"`
		RunID string `json:"runId"`
		Name  string `json:"name"`
	}
	_ = json.Unmarshal(wfRaw, &wf)
	if wf.RunID != "run-top-1" || wf.Name != "hello-world" {
		t.Errorf("workflow_started = %+v", wf)
	}

	childRaw := recvUntilType(t, h.ch, "team.child_started", 2*time.Second)
	var child struct {
		Type        string `json:"type"`
		RunID       string `json:"runId"`
		ParentRunID string `json:"parentRunId"`
		Name        string `json:"name"`
		Kind        string `json:"kind"`
	}
	_ = json.Unmarshal(childRaw, &child)
	if child.RunID != "run-top-1" {
		t.Errorf("child_started.runId = %q", child.RunID)
	}
	if child.ParentRunID != "" {
		t.Errorf("child_started.parentRunId = %q, want empty (top-level)", child.ParentRunID)
	}
	if child.Name != "hello-world" {
		t.Errorf("child_started.name = %q", child.Name)
	}
	if child.Kind != "agent" {
		t.Errorf("child_started.kind = %q, want 'agent'", child.Kind)
	}
}

// TestTranslator_BridgesAgentCompleteToTeamDone verifies that when a
// top-level agent run completes, BOTH team.child_done and team.workflow_done
// fire. The agent's name is recovered from the session_start map.
func TestTranslator_BridgesAgentCompleteToTeamDone(t *testing.T) {
	h := newHarness(t)

	// Start the agent first so its name is registered.
	h.bus.Publish(agentapi.EventSessionStart, agentapi.SessionStartEvent{
		EventBase: agentapi.EventBase{Type: agentapi.EventSessionStart, Emitter: "run-x"},
		Name:      "task-x",
	})
	_ = recvUntilType(t, h.ch, "team.workflow_started", 2*time.Second)
	_ = recvUntilType(t, h.ch, "team.child_started", 2*time.Second)

	// Now complete it.
	h.bus.Publish(agentapi.EventComplete, agentapi.CompleteEvent{
		EventBase: agentapi.EventBase{Type: agentapi.EventComplete, Emitter: "run-x"},
		Status:    "completed",
		Summary:   "all done",
	})

	doneRaw := recvUntilType(t, h.ch, "team.child_done", 3*time.Second)
	var done struct {
		Type   string         `json:"type"`
		RunID  string         `json:"runId"`
		Name   string         `json:"name"`
		Output map[string]any `json:"output"`
	}
	_ = json.Unmarshal(doneRaw, &done)
	if done.RunID != "run-x" {
		t.Errorf("child_done.runId = %q", done.RunID)
	}
	if done.Name != "task-x" {
		t.Errorf("child_done.name = %q (name not recovered from session_start map)", done.Name)
	}
	if summary, _ := done.Output["summary"].(string); summary != "all done" {
		t.Errorf("child_done.output.summary = %v", done.Output)
	}

	// Top-level → should ALSO emit team.workflow_done.
	wfRaw := recvUntilType(t, h.ch, "team.workflow_done", 2*time.Second)
	var wf struct {
		Type  string `json:"type"`
		RunID string `json:"runId"`
	}
	_ = json.Unmarshal(wfRaw, &wf)
	if wf.RunID != "run-x" {
		t.Errorf("workflow_done.runId = %q", wf.RunID)
	}
}

// A run that fails outside any agent (it could not start) reaches the client
// as a system error message carrying the reason.
func TestReportRunErrorReachesTheClient(t *testing.T) {
	h := newHarness(t)
	h.trans.ReportRunError("terminal: run task: unknown key(s)")
	raw := recvUntilType(t, h.ch, "msg.new", 2*time.Second)
	var ev struct {
		Msg struct {
			Role    string `json:"role"`
			Content struct {
				Level   string `json:"level"`
				Message string `json:"message"`
			} `json:"content"`
		} `json:"msg"`
	}
	if err := json.Unmarshal(raw, &ev); err != nil {
		t.Fatal(err)
	}
	if ev.Msg.Role != "system" || ev.Msg.Content.Level != "error" || ev.Msg.Content.Message != "terminal: run task: unknown key(s)" {
		t.Fatalf("got %s", raw)
	}
}

// The session's conversation is its chat panel, the one
// feed a chat draws a run's artifacts from: the terminal
// protocol carries the panel - its open and the replace of the artifact's
// node at the mount, one replace per patch, one at the completion - and no
// other artifact event.
func TestTranslator_CarriesTheChatPanel(t *testing.T) {
	h := newHarness(t)
	if h.trans.ChatPanelID() != "" {
		t.Fatal("a chat panel before any artifact")
	}
	h.bus.Publish(agentapi.EventUIArtifactMount, agentapi.UIArtifactMountEvent{ArtifactID: "f1", Kind: "form", Value: map[string]any{"name": "A"}})
	h.bus.Publish(agentapi.EventUIArtifactPatch, agentapi.UIArtifactPatchEvent{ArtifactID: "f1", Ops: []any{
		map[string]any{"kind": "replace", "path": "/name", "value": "Ada"},
	}})
	h.bus.Publish(agentapi.EventUIArtifactComplete, agentapi.UIArtifactCompleteEvent{ArtifactID: "f1"})

	var got []terminal.PanelEvent
	var others []string
	deadline := time.After(2 * time.Second)
	for len(got) < 4 {
		select {
		case raw := <-h.ch:
			var probe struct {
				Type string `json:"type"`
			}
			_ = json.Unmarshal(raw, &probe)
			if probe.Type != "panel.event" {
				others = append(others, probe.Type)
				continue
			}
			var pe terminal.PanelEvent
			if err := json.Unmarshal(raw, &pe); err != nil {
				t.Fatal(err)
			}
			got = append(got, pe)
		case <-deadline:
			t.Fatalf("panel events %d of 4 (others %v)", len(got), others)
		}
	}
	id := h.trans.ChatPanelID()
	for i, pe := range got {
		if pe.Event.Panel != id || pe.Event.Seq != int64(i+1) {
			t.Fatalf("event %d: panel %s seq %d, want %s seq %d", i, pe.Event.Panel, pe.Event.Seq, id, i+1)
		}
	}
	if got[0].Event.Kind != "open" || got[3].Event.Ops[0].Target != "artifact:f1" {
		t.Fatalf("events = %+v", got)
	}
	art, _ := got[2].Event.Ops[0].Node["artifact"].(map[string]any)
	if v, _ := art["value"].(map[string]any); v["name"] != "Ada" {
		t.Fatalf("the patch's replace carries %v", art)
	}
	if len(others) != 0 {
		t.Fatalf("the artifact also came as %v: a chat has one feed, the chat panel", others)
	}
	select {
	case raw := <-h.ch:
		t.Fatalf("an artifact event beside the chat panel's: %s", raw)
	case <-time.After(100 * time.Millisecond):
	}
}
