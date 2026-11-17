// SPDX-License-Identifier: Apache-2.0

package display_ui_component

import (
	"context"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/panel"
	"github.com/katyoti/katyoti-fwk/internal/core/render"
	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/terminal/chatpanel"
)

// display-ui-component keeps its name and its stream; on the run's chat
// panel the first parse opens the panel and shows the
// artifact as its node, each later parse replaces that node with the def
// streamed so far, and the end of the call completes it.
func TestDisplayUIComponentStreamsAsChatPanelEvents(t *testing.T) {
	app := application.New(application.Config())
	if err := app.Run(); err != nil {
		t.Fatal(err)
	}
	bus := engine.NewMemoryEventBus()
	application.PutService[engine.EventBus](app, "default", bus)
	ctx := application.WithApplication(context.Background(), app)

	chat := chatpanel.New("", "s1", "run-1")
	var events []panel.Event
	defer chatpanel.Follow(bus, chat, func(e panel.Event) { events = append(events, e) })()

	p := &streamPlugin{ctx: ctx, accums: map[string]*displayState{}}
	for _, c := range []agentapi.StreamChunk{
		{Type: "tool_use_start", ToolName: "display-ui-component", ToolID: "t1"},
		{Type: "tool_use_delta", ToolID: "t1", ToolInput: `{"artifactId": "who", "kind": "form", "data": {"name": "A`},
		{Type: "tool_use_delta", ToolID: "t1", ToolInput: `da"}}`},
		{Type: "tool_use_done", ToolID: "t1"},
	} {
		if err := p.onStream(nil, c); err != nil {
			t.Fatal(err)
		}
	}

	if len(events) < 4 || events[0].Kind != panel.KindOpen || events[0].Open.Type != chatpanel.Type {
		t.Fatalf("want the chat panel opened then its ops, got %+v", events)
	}
	if op := events[1].Ops[0]; op.Op != render.OpReplace || op.Target != chatpanel.ArtifactsNode {
		t.Fatalf("the mount: %s of %s", op.Op, op.Target)
	}
	for i, e := range events[2:] {
		if len(e.Ops) != 1 || e.Ops[0].Op != render.OpReplace || e.Ops[0].Target != chatpanel.NodeID("who") {
			t.Fatalf("event %d: want one replace of the artifact's node, got %+v", i+2, e)
		}
	}
	a := chat.Artifact("who")
	if v, _ := a.Value.(map[string]any); v["name"] != "Ada" || a.Status != chatpanel.StatusComplete {
		t.Fatalf("the artifact at the end: %+v", a)
	}
}
