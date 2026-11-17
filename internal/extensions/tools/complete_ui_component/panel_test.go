// SPDX-License-Identifier: Apache-2.0

package complete_ui_component

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

// complete-ui-component keeps its name; on the run's chat panel
// it replaces the artifact's node with its status complete.
func TestCompleteUIComponentCompletesTheArtifactsNode(t *testing.T) {
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
	_ = bus.Publish(agentapi.EventUIArtifactMount, agentapi.UIArtifactMountEvent{ArtifactID: "f1", Kind: "form", Value: map[string]any{"name": "Ada"}})
	events = nil

	if out, err := executeComplete(ctx, CompleteUIComponentInput{ArtifactID: "f1"}); err != nil || !out.Success {
		t.Fatalf("complete: %v %+v", err, out)
	}
	if len(events) != 1 || len(events[0].Ops) != 1 {
		t.Fatalf("want one ops event with one op, got %+v", events)
	}
	op := events[0].Ops[0]
	art, _ := op.Node["artifact"].(map[string]any)
	if op.Op != render.OpReplace || op.Target != chatpanel.NodeID("f1") || art["status"] != chatpanel.StatusComplete {
		t.Fatalf("want the artifact's node replaced complete, got %s of %s: %v", op.Op, op.Target, op.Node)
	}
}
