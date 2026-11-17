// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/strategies"
	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

// panickerMachine blows up inside its first action — stands in for any
// machine implementation panicking mid-run.
type panickerMachine struct{}

func (panickerMachine) Name() string { return "panicker" }
func (panickerMachine) InitialState(_ engine.Event) engine.State {
	return engine.State{Name: "init"}
}
func (panickerMachine) Step(_ context.Context, _ *engine.State, _ engine.Event) (engine.ActionFunc, error) {
	return func(_ context.Context) (*engine.StepResult, error) {
		panic("boom-spawner-hygiene")
	}, nil
}

type panickerFactory struct{}

func (panickerFactory) Name() string { return "panicker" }
func (panickerFactory) CreateMachine(_ context.Context, _ any) (engine.Machine, error) {
	return panickerMachine{}, nil
}

// TestSpawner_PanickedChild_PublishesFailed — a panicking child machine must
// not take down the process (an unrecovered goroutine panic is fatal) and
// must still emit child.<name>.failed so a waiting parent workflow resumes
// instead of blocking until its ctx timeout.
func TestSpawner_PanickedChild_PublishesFailed(t *testing.T) {
	_ = engine.MachineDomain.Types.Contribute(strategies.TypeDef[engine.MachineFactory]{
		DefMeta:  strategies.DefMeta{Name: "panicker"},
		Strategy: "panicker",
		Impl:     panickerFactory{},
	})

	bus := engine.NewMemoryEventBus()
	channel := engine.NewChannel(bus)
	spawner := NewLocalSpawner(channel)

	failedCh := make(chan string, 1)
	bus.Subscribe(engine.ChildFailedTopic("boomer"), func(_ string, payload any) {
		if evt, ok := payload.(*engine.ChildFailedEvent); ok {
			select {
			case failedCh <- evt.Error:
			default:
			}
		}
	})

	_, err := spawner.Spawn(context.Background(), engine.ChildSpec{
		Kind: "panicker",
		Name: "boomer",
	})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}

	select {
	case errMsg := <-failedCh:
		if !strings.Contains(errMsg, "panicked") {
			t.Errorf("failed event error = %q, want it to mention the panic", errMsg)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no child.failed event within 2s — the panicking child vanished without its lifecycle event")
	}
}
