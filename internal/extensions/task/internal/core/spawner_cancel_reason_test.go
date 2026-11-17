// SPDX-License-Identifier: Apache-2.0

package core_test

import (
	"context"
	"strings"
	"testing"
	"time"

	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
	core "github.com/katyoti/katyoti-fwk/internal/extensions/task/internal/core"
)

// A workflow's child cancelled with a reason (its parent's heartbeat stopped,
// a sibling failed) fails saying why — it failed "context canceled".
func TestACancelledChildSaysWhy(t *testing.T) {
	registerSimulatorForGapTests(t)
	bus := engine.NewMemoryEventBus()
	spawner := core.NewLocalSpawner(engine.NewChannel(bus))
	failed := make(chan string, 1)
	bus.Subscribe(engine.ChildFailedTopic("why-target"), func(_ string, payload any) {
		if e, ok := payload.(*engine.ChildFailedEvent); ok {
			failed <- e.Error
		}
	})
	h, err := spawner.Spawn(context.Background(), engine.ChildSpec{
		Kind: "simulator", Name: "why-target",
		Config: map[string]any{"duration_ms": 5000, "output": "never"},
	})
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)
	if err := spawner.Cancel(h.RunID, "its parent's heartbeat stopped"); err != nil {
		t.Fatal(err)
	}
	select {
	case why := <-failed:
		if !strings.Contains(why, "heartbeat stopped") {
			t.Errorf("child failed with %q; want the reason it was cancelled", why)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("the cancelled child never failed")
	}
}
