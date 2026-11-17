// SPDX-License-Identifier: Apache-2.0

package supervisor

import (
	"context"
	"encoding/json"
	"testing"

	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
	"github.com/katyoti/katyoti-fwk/internal/kts/subjects"
)

// oneSubBus records the subject subscribed and keeps its handler.
type oneSubBus struct {
	mbapi.MessageBus
	subject string
	handler mbapi.Handler
}

func (b *oneSubBus) Subscribe(_ context.Context, subject string, h mbapi.Handler, _ ...mbapi.SubOpt) (mbapi.Subscription, error) {
	b.subject, b.handler = subject, h
	return nil, nil
}

// msg is a bus message carrying data.
type msg struct {
	mbapi.Message
	data []byte
}

func (m msg) Data() []byte { return m.data }

// The GUI watches a universe's run-lifecycle subject — the one the task
// service publishes on — and tells the App a run of that universe changed,
// with the event as told: the scheduler console moves on it.
func TestTheGUIHearsAUniversesRunsChange(t *testing.T) {
	m := &Master{}
	var heard []string
	var events []json.RawMessage
	m.SetRunsEventHandler(func(u string, e json.RawMessage) { heard = append(heard, u); events = append(events, e) })
	bus := &oneSubBus{}
	if _, err := m.watchRuns(bus, "alpha"); err != nil {
		t.Fatal(err)
	}
	if bus.subject != subjects.RunLifecycleSubject("local", "alpha") {
		t.Fatalf("subscribed %q; want the universe's run-lifecycle subject", bus.subject)
	}
	bus.handler(context.Background(), nil)
	if len(heard) != 1 || heard[0] != "alpha" {
		t.Errorf("heard %v; want the universe told", heard)
	}
	told := engine.RunLifecycleEvent{RunID: "run-1", Transition: engine.RunWaiting}
	b, _ := json.Marshal(told)
	bus.handler(context.Background(), msg{data: b})
	var got engine.RunLifecycleEvent
	if len(events) != 2 || json.Unmarshal(events[1], &got) != nil || got.RunID != "run-1" || got.Transition != engine.RunWaiting {
		t.Errorf("event forwarded %s; want the run's transition as told", events[len(events)-1])
	}
}
