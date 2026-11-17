// SPDX-License-Identifier: Apache-2.0

package wkflow

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/panel"
	panelapi "github.com/katyoti/katyoti-fwk/internal/extensions/panelserver/api"
	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

// panelLog is a fake PanelService: the inputs the panel server took, logged
// after a sequence, as History answers them.
type panelLog struct {
	panelapi.PanelService
	mu      sync.Mutex
	records []panelapi.Record
	reads   int
}

func (l *panelLog) take(in panel.Input, after int64) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.records = append(l.records, panelapi.Record{At: time.Now(), Input: &in, After: after})
}

func (l *panelLog) History(_ context.Context, in panelapi.HistoryInput) (panelapi.HistoryOutput, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.reads++
	out := panelapi.HistoryOutput{Records: []panelapi.Record{}}
	for _, r := range l.records {
		if r.Input != nil && r.Input.Panel == in.Panel && r.After >= in.From {
			out.Records = append(out.Records, r)
		}
	}
	return out, nil
}

func (l *panelLog) readCount() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.reads
}

func withPanelLog(t *testing.T, svc panelapi.PanelService) {
	t.Helper()
	prev := resolvePanelService
	resolvePanelService = func(context.Context) (panelapi.PanelService, error) {
		if svc == nil {
			return nil, errors.New("no panel service")
		}
		return svc, nil
	}
	t.Cleanup(func() { resolvePanelService = prev })
}

// askSeqOf answers the sequence of the panel's ask.
func askSeqOf(t *testing.T, rig *panelRig) int64 {
	t.Helper()
	for _, e := range rig.events() {
		if e.Kind == panel.KindAsk {
			return e.Seq
		}
	}
	t.Fatal("no ask went out")
	return 0
}

// An answer given in the panel while its run was down - the panel server
// logged it, no run listened - reaches the run when it is restored: the
// restored ask rechecks, reads the panel's log after its ask and takes the
// input, with no input published again.
func TestAnAnswerGivenWhileTheRunWasDownReachesTheRestoredRun(t *testing.T) {
	log := &panelLog{}
	withPanelLog(t, log)
	store := engine.NewMemoryStateStore()
	bus := engine.NewMemoryEventBus()
	channel := engine.NewChannel(bus)
	ctx := withLocalSpawner(channel)
	rig := &panelRig{}
	first := engine.NewExecutor(panelMachine(t, ctx), channel, store)
	first.SetPanelPublisher(rig.publisher)
	first.SetRunID("run-panel-down")
	runCtx, stop := context.WithCancel(ctx)
	stopped := make(chan struct{})
	go func() { _, _ = first.Run(runCtx, engine.Event{Type: "start"}); close(stopped) }()
	id := rig.waitAsk(t)
	stop() // the process dies while the panel asks
	<-stopped

	// The person answers while no run listens: only the panel server takes it.
	// An input older than the ask is no answer to it.
	log.take(panel.Input{Panel: id, Kind: panel.InputAction, Name: "no", By: "early"}, askSeqOf(t, rig)-1)
	log.take(panel.Input{Panel: id, Kind: panel.InputAction, Name: "yes", By: "jdoe"}, askSeqOf(t, rig))

	bus2 := engine.NewMemoryEventBus()
	channel2 := engine.NewChannel(bus2)
	ctx2 := withLocalSpawner(channel2)
	second := engine.NewExecutor(panelMachine(t, ctx2), channel2, store)
	second.SetPanelPublisher(rig.publisher)
	resCtx, cancel := context.WithTimeout(ctx2, 3*time.Second)
	defer cancel()
	res, err := second.ResumeWaiting(resCtx, "run-panel-down")
	if err != nil {
		t.Fatalf("ResumeWaiting: %v", err)
	}
	if res == nil || res.Error != nil {
		t.Fatalf("the restored run did not end with the logged answer: %+v", res)
	}
	got, _ := res.Output.(map[string]any)["pub"].(map[string]any)
	if got["name"] != "yes" || got["by"] != "jdoe" {
		t.Fatalf("results.pub = %#v, want the answer logged after the ask", res.Output)
	}
	evs := rig.events()
	if last := evs[len(evs)-1]; last.Kind != panel.KindComplete || last.Panel != id {
		t.Fatalf("after the restore: %+v", last)
	}
}

// With nothing in the panel's log, the restored ask waits again - and an
// input then given live still ends it; a PanelService that cannot be reached
// leaves the ask waiting the same.
func TestARestoredAskWithNoLoggedAnswerWaitsOn(t *testing.T) {
	for name, svc := range map[string]panelapi.PanelService{"empty log": &panelLog{}, "no panel service": nil} {
		t.Run(name, func(t *testing.T) {
			withPanelLog(t, svc)
			store := engine.NewMemoryStateStore()
			bus := engine.NewMemoryEventBus()
			channel := engine.NewChannel(bus)
			ctx := withLocalSpawner(channel)
			rig := &panelRig{}
			first := engine.NewExecutor(panelMachine(t, ctx), channel, store)
			first.SetPanelPublisher(rig.publisher)
			first.SetRunID("run-panel-wait")
			runCtx, stop := context.WithCancel(ctx)
			stopped := make(chan struct{})
			go func() { _, _ = first.Run(runCtx, engine.Event{Type: "start"}); close(stopped) }()
			id := rig.waitAsk(t)
			stop()
			<-stopped

			bus2 := engine.NewMemoryEventBus()
			channel2 := engine.NewChannel(bus2)
			ctx2 := withLocalSpawner(channel2)
			second := engine.NewExecutor(panelMachine(t, ctx2), channel2, store)
			second.SetPanelPublisher(rig.publisher)
			resCtx, cancel := context.WithTimeout(ctx2, 5*time.Second)
			defer cancel()
			done := make(chan *engine.RunResult, 1)
			go func() { res, _ := second.ResumeWaiting(resCtx, "run-panel-wait"); done <- res }()
			select {
			case res := <-done:
				t.Fatalf("the ask ended with no answer: %+v", res)
			case <-time.After(150 * time.Millisecond):
			}
			if l, ok := svc.(*panelLog); ok && l.readCount() == 0 {
				t.Fatal("the restored ask never read its panel's log")
			}
			answer(bus2, id)
			res := <-done
			if got, _ := res.Output.(map[string]any)["pub"].(map[string]any); got["name"] != "yes" {
				t.Fatalf("results.pub = %#v", res.Output)
			}
		})
	}
}

// A bus reconnection wakes a waiting ask with a recheck (core.RecheckWaits
// publishes a non-input on its topic): the ask reads the log and takes an
// answer the person gave while the bus was down.
func TestABusRecheckTakesTheAnswerLoggedWhileTheBusWasDown(t *testing.T) {
	log := &panelLog{}
	withPanelLog(t, log)
	bus := engine.NewMemoryEventBus()
	channel := engine.NewChannel(bus)
	ctx := withLocalSpawner(channel)
	rig := &panelRig{}
	exec := engine.NewExecutor(panelMachine(t, ctx), channel, nil)
	exec.SetPanelPublisher(rig.publisher)
	runCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	done := make(chan *engine.RunResult, 1)
	go func() { res, _ := exec.Run(runCtx, engine.Event{Type: "start"}); done <- res }()
	id := rig.waitAsk(t)
	time.Sleep(50 * time.Millisecond)
	log.take(panel.Input{Panel: id, Kind: panel.InputAction, Name: "yes", By: "jdoe"}, askSeqOf(t, rig))
	bus.Publish(engine.ExternalEventTopic(engine.PanelInputURI(id)), map[string]any{"recheck": "the bus reconnected"})
	res := <-done
	if res == nil {
		t.Fatal("no result")
	}
	if got, _ := res.Output.(map[string]any)["pub"].(map[string]any); got["name"] != "yes" || got["by"] != "jdoe" {
		t.Fatalf("results.pub = %#v", res.Output)
	}
}
