// SPDX-License-Identifier: Apache-2.0

package patch_ui_component

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

// patch-ui-component keeps its name and its ops; on the run's chat panel
// each call is one replace of the artifact's node, its value
// patched by the ops - by node id and by pointer alike.
func TestPatchUIComponentIsOneReplaceOnTheChatPanel(t *testing.T) {
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
	_ = bus.Publish(agentapi.EventUIArtifactMount, agentapi.UIArtifactMountEvent{ArtifactID: "dash", Kind: "page",
		DSLTree: map[string]any{"kind": "vbox", "children": []any{map[string]any{"kind": "kpi", "id": "kpi-open", "label": "Open", "value": 3}}}})
	events = nil

	out, err := executePatch(ctx, PatchUIComponentInput{ArtifactID: "dash", Ops: []PatchOp{
		{Op: "merge", Target: "kpi-open", Value: map[string]any{"value": 7}},
		{Op: "add", Path: "/children/-", Value: map[string]any{"kind": "text", "id": "note", "content": "updated"}},
	}})
	if err != nil || !out.Success {
		t.Fatalf("patch: %v %+v", err, out)
	}
	if len(events) != 1 || events[0].Kind != panel.KindOps || len(events[0].Ops) != 1 {
		t.Fatalf("want one ops event with one op, got %+v", events)
	}
	op := events[0].Ops[0]
	if op.Op != render.OpReplace || op.Target != chatpanel.NodeID("dash") {
		t.Fatalf("want the replace of the artifact's node, got %s of %s", op.Op, op.Target)
	}
	kids := chat.Artifact("dash").DSLTree.(map[string]any)["children"].([]any)
	if len(kids) != 2 || kids[0].(map[string]any)["value"] != float64(7) || kids[1].(map[string]any)["id"] != "note" {
		t.Fatalf("patched tree = %v", chat.Artifact("dash").DSLTree)
	}
}
