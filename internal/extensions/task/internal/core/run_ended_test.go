// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"testing"
	"time"

	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

// A run's end is published on its own topic: whoever waits on that run as a
// machine (a delegating parent) is woken by it.
func TestARunsEndIsPublishedOnItsTopic(t *testing.T) {
	svc, _ := newServiceForLifecycleTest(t)
	bus, ok := svc.channel.Bus().(*engine.MemoryEventBus)
	if !ok {
		t.Fatalf("bus %T", svc.channel.Bus())
	}
	topics := make(chan string, 64)
	unsub := bus.SubscribeAny(func(topic string, _ any) {
		select {
		case topics <- topic:
		default:
		}
	})
	defer unsub()
	r, err := svc.Run(context.Background(), taskapi.TaskRunRequest{TaskName: "uc-130-task"})
	if err != nil {
		t.Fatal(err)
	}
	want := engine.RunEndedTopic(r.RunID)
	deadline := time.After(3 * time.Second)
	for {
		select {
		case topic := <-topics:
			if topic == want {
				return
			}
		case <-deadline:
			t.Fatalf("no end published on %s", want)
		}
	}
}
