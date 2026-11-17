// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/strategies"
	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

// perpetual waits for `tick.<run>`, counts it, waits again — for ever.
type perpetual struct{}

var perpetualHandled atomic.Int64

func (perpetual) Name() string                           { return "perpetual" }
func (perpetual) InitialState(engine.Event) engine.State { return engine.State{Name: "init"} }
func (perpetual) Step(_ context.Context, st *engine.State, ev engine.Event) (engine.ActionFunc, error) {
	n := 0
	if m, ok := st.Data.(map[string]any); ok {
		switch v := m["handled"].(type) {
		case int:
			n = v
		case float64:
			n = int(v)
		}
	}
	runID := st.RunID
	initial := st.Name == "init"
	return func(context.Context) (*engine.StepResult, error) {
		if !initial {
			n++
			perpetualHandled.Add(1)
		}
		return &engine.StepResult{NextState: "waiting", Data: map[string]any{"handled": n}, Wait: &engine.WaitSpec{
			Mode: engine.WaitAny, Conditions: []engine.WaitCondition{{Topic: "tick." + runID, Label: "tick"}}}}, nil
	}, nil
}

type perpetualFactory struct{}

func (perpetualFactory) Name() string { return "perpetual" }
func (perpetualFactory) CreateMachine(context.Context, any) (engine.Machine, error) {
	return perpetual{}, nil
}

func dirSize(t *testing.T, dir string) (files int, bytes int64) {
	t.Helper()
	_ = filepath.Walk(dir, func(_ string, fi os.FileInfo, err error) error {
		if err == nil && !fi.IsDir() {
			files++
			bytes += fi.Size()
		}
		return nil
	})
	return files, bytes
}

// A run that lives for ever stays bounded: after many events its checkpoint,
// its registry entry and its run store are as small as after 100, and its
// session journal stays within its rotation. 1 000 events by default (a
// checkpoint is written per event: ~6 ms on Windows); KTW_LONG_TESTS=1 runs
// 10 000.
func TestAPerpetualRunStaysBounded(t *testing.T) {
	if testing.Short() {
		t.Skip("many events")
	}
	more := int64(900)
	if os.Getenv("KTW_LONG_TESTS") == "1" {
		more = 9900
	}
	oldMax := engine.SetMaxJournalBytesForTest(64 << 10)
	t.Cleanup(func() { engine.SetMaxJournalBytesForTest(oldMax) })
	dir := scratchDir(t)
	svc := durableService(t, dir)
	_ = engine.MachineDomain.Types.Contribute(strategies.TypeDef[engine.MachineFactory]{DefMeta: strategies.DefMeta{Name: "perpetual"}, Impl: perpetualFactory{}})
	svc.tasks["perpetual"] = &taskapi.Task{Name: "perpetual", Provider: "test", Type: "perpetual"}
	perpetualHandled.Store(0)

	r, err := svc.Run(context.Background(), taskapi.TaskRunRequest{TaskName: "perpetual", Group: "g"})
	if err != nil {
		t.Fatal(err)
	}
	eventually(t, "waiting", func() bool { return status(t, svc, r.RunID) == taskapi.RunStatusWaiting })
	send := func(n int64) {
		target := perpetualHandled.Load() + n
		for i := int64(0); i < n; i++ {
			_ = svc.channel.Bus().Publish("tick."+r.RunID, i)
		}
		deadline := time.Now().Add(2 * time.Minute)
		for perpetualHandled.Load() < target && time.Now().Before(deadline) {
			time.Sleep(10 * time.Millisecond)
		}
		if got := perpetualHandled.Load(); got < target {
			t.Fatalf("handled %d events; want %d", got, target)
		}
		eventually(t, "waiting again", func() bool { return status(t, svc, r.RunID) == taskapi.RunStatusWaiting })
	}

	send(100)
	cp100, _ := os.Stat(svc.runs.checkpointPath(r.RunID))
	runFiles100, _ := dirSize(t, dir)
	send(more)
	cp10k, err := os.Stat(svc.runs.checkpointPath(r.RunID))
	if err != nil {
		t.Fatal(err)
	}
	runFiles10k, _ := dirSize(t, dir)
	if cp10k.Size() > cp100.Size()+16 {
		t.Errorf("checkpoint %d bytes after %d events, %d after 100; want it bounded", cp10k.Size(), 100+more, cp100.Size())
	}
	if runFiles10k != runFiles100 {
		t.Errorf("run store holds %d files after %d events, %d after 100", runFiles10k, 100+more, runFiles100)
	}
	if n := svc.reg.len(); n != 1 {
		t.Errorf("registry holds %d entries; want the one run", n)
	}
	files, bytes := dirSize(t, filepath.Join(svc.sessionsDir, r.SessionID))
	if files > 6 || bytes > 6*(64<<10)+64<<10 {
		t.Errorf("session journal: %d files, %d bytes; want it within its rotation", files, bytes)
	}
	_, _ = svc.Stop(context.Background(), taskapi.StopRequest{RunID: r.RunID})
}

// A kept-alive perpetual run handles events, its process dies, and after the
// restart the same run goes on handling events from where it was — no second
// run beside it.
func TestAPerpetualKeptAliveRunGoesOnAfterARestart(t *testing.T) {
	ctx := context.Background()
	dir := scratchDir(t)
	_ = engine.MachineDomain.Types.Contribute(strategies.TypeDef[engine.MachineFactory]{DefMeta: strategies.DefMeta{Name: "perpetual"}, Impl: perpetualFactory{}})
	declare := func(s *Service) {
		s.tasks["perpetual"] = &taskapi.Task{Name: "perpetual", Provider: "test", Type: "perpetual"}
		s.keepAlive = newKeepAlive([]taskapi.KeepAliveTask{{Task: "perpetual", Group: "g"}})
	}
	perpetualHandled.Store(0)
	first := durableService(t, dir)
	declare(first)
	first.keepAliveSweep(ctx)
	id := runsOf(t, first, "perpetual")[0].RunID
	tick := func(s *Service, n int) {
		for i := 0; i < n; i++ {
			eventually(t, "waiting", func() bool { return status(t, s, id) == taskapi.RunStatusWaiting })
			before := perpetualHandled.Load()
			_ = s.channel.Bus().Publish("tick."+id, i)
			eventually(t, "handled", func() bool { return perpetualHandled.Load() > before })
		}
		eventually(t, "waiting", func() bool { return status(t, s, id) == taskapi.RunStatusWaiting })
	}
	tick(first, 3)
	orphan(t, first.runs, id) // its process dies

	second := durableService(t, dir)
	declare(second)
	sctx, cancel := context.WithCancel(ctx)
	defer cancel()
	swept := make(chan struct{})
	go second.ServeRestores(sctx, func() { close(swept) })
	<-swept
	if runs := runsOf(t, second, "perpetual"); len(runs) != 1 || runs[0].RunID != id {
		t.Fatalf("runs after the restart = %+v; want the same run", runs)
	}
	tick(second, 2)
	cp, err := second.runs.Load(id)
	if err != nil {
		t.Fatal(err)
	}
	if m, _ := cp.State.Data.(map[string]any); m == nil || m["handled"] != float64(5) {
		t.Errorf("state after the restart = %+v; want 5 events handled (3 before, 2 after)", cp.State.Data)
	}
}
