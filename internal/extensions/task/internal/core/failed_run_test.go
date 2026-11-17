// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/strategies"
	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

// failingMachine ends at once with its output and a Failure - as an agent
// whose run failed does.
type failingMachine struct{}

func (failingMachine) Name() string                             { return "failing" }
func (failingMachine) InitialState(_ engine.Event) engine.State { return engine.State{Name: "init"} }
func (failingMachine) Step(_ context.Context, _ *engine.State, _ engine.Event) (engine.ActionFunc, error) {
	return func(_ context.Context) (*engine.StepResult, error) {
		return &engine.StepResult{NextState: "failed", Output: map[string]any{"Status": 3},
			Failure: &engine.Failure{Reason: "prompt is too long"}}, nil
	}, nil
}

type failingFactory struct{}

func (failingFactory) Name() string { return "failing" }
func (failingFactory) CreateMachine(_ context.Context, _ any) (engine.Machine, error) {
	return failingMachine{}, nil
}

// : a child (a background worker) whose machine ends with a
// Failure is failed, with the failure's reason - never done.
func TestARunEndingInErrorIsRecordedFailedBySpawner(t *testing.T) {
	_ = engine.MachineDomain.Types.Contribute(strategies.TypeDef[engine.MachineFactory]{
		DefMeta: strategies.DefMeta{Name: "failing"}, Strategy: "failing", Impl: failingFactory{},
	})
	bus := engine.NewMemoryEventBus()
	spawner := NewLocalSpawner(engine.NewChannel(bus))
	failedCh := make(chan string, 1)
	bus.Subscribe(engine.ChildFailedTopic("worker"), func(_ string, payload any) {
		if evt, ok := payload.(*engine.ChildFailedEvent); ok {
			select {
			case failedCh <- evt.Error:
			default:
			}
		}
	})
	doneCh := make(chan struct{}, 1)
	bus.Subscribe(engine.ChildDoneTopic("worker"), func(string, any) {
		select {
		case doneCh <- struct{}{}:
		default:
		}
	})
	if _, err := spawner.Spawn(context.Background(), engine.ChildSpec{Kind: "failing", Name: "worker"}); err != nil {
		t.Fatalf("spawn: %v", err)
	}
	select {
	case msg := <-failedCh:
		if msg != "prompt is too long" {
			t.Fatalf("the child failed with %q; want the Failure's reason", msg)
		}
	case <-doneCh:
		t.Fatal("a child whose machine ended with a Failure was told done")
	case <-time.After(3 * time.Second):
		t.Fatal("no child.failed within 3s")
	}
}
