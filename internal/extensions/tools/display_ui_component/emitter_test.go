// SPDX-License-Identifier: Apache-2.0

package display_ui_component

import (
	"context"
	"sync"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/eventbus"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

// runLoop is a loop that only knows its run id — all the stream plugin asks.
type runLoop struct {
	agentapi.LoopContext
	id string
}

func (l runLoop) RunID() string { return l.id }

// Every event the stream plugin publishes carries the run that produced it.
// A chat's translator shows only its own runs' events; an event with no
// emitter reaches every open chat — one chat's deck landed in another's pane.
func TestStreamedEventsCarryTheirRun(t *testing.T) {
	bus := eventbus.NewMemoryEventBus()
	app := application.New(config.NewConfigService(loader.NewMapLoader(nil)),
		application.DeclareSimpleService[engine.EventBus](func(context.Context) (engine.EventBus, error) { return bus, nil }),
	)
	if err := app.Run(); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	emitters := map[string]string{}
	for _, topic := range []string{agentapi.EventUIArtifactMount, agentapi.EventUIArtifactPatch, agentapi.EventUIArtifactComplete} {
		topic := topic
		bus.Subscribe(topic, func(_ string, p any) {
			if e, ok := p.(agentapi.AgentEvent); ok {
				mu.Lock()
				emitters[topic] = e.GetEmitter()
				mu.Unlock()
			}
		})
	}

	p := newStreamPlugin()
	p.ctx = application.WithApplication(context.Background(), app)
	loop := runLoop{id: "run-abc"}
	chunk := func(typ, input string) {
		if err := p.onStream(loop, agentapi.StreamChunk{Type: typ, ToolName: "display-ui-component", ToolID: "t1", ToolInput: input}); err != nil {
			t.Fatal(err)
		}
	}
	chunk("tool_use_start", "")
	chunk("tool_use_delta", `{"artifactId": "d1", "kind": "deck", "data": "---\ntitle: T\n---\n\n# One`)
	chunk("tool_use_delta", `\n\n---\n\n## Two"}`)
	chunk("tool_use_done", "")

	mu.Lock()
	defer mu.Unlock()
	for _, topic := range []string{agentapi.EventUIArtifactMount, agentapi.EventUIArtifactPatch, agentapi.EventUIArtifactComplete} {
		if got := emitters[topic]; got != "run-abc" {
			t.Errorf("%s emitter = %q, want run-abc", topic, got)
		}
	}
}
