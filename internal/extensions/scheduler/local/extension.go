// SPDX-License-Identifier: Apache-2.0

// Package local wires the scheduler into an application: its services
// (ktw-agents:todos/<Verb> and waits/<Verb>, their command lines), the point owners
// contribute their collectors and runners to, and the internal scheduler (its machine, and
// the serving hook that starts its run once the process serves).
package local

import (
	"context"
	"log/slog"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/em"
	"github.com/katyoti/katyoti-fwk/internal/core/publish"
	"github.com/katyoti/katyoti-fwk/internal/core/strategies"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	orchapi "github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
	schedcore "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/internal/core"
	ktsapi "github.com/katyoti/katyoti-fwk/internal/kts/api"
)

// Extension publishes the scheduler and the waits, and opens the collectors,
// runners, group-members and wait-namers points.
func Extension() application.Extension {
	// One store per application, chosen at its first use (SQLite in
	// SchedulerDir() on a universe with nothing to migrate, else the file
	// store), shared by the to-dos, the waits and the internal scheduler.
	store := application.DeclareSimpleService[*schedcore.SchedulerStore](schedcore.OpenSchedulerStore)
	// One pull record per application, in memory: the pulls the queue names
	// as takers of its takeable to-dos.
	pulls := application.DeclareSimpleService[*schedcore.PullRecord](schedcore.NewPullRecord)
	todos := publish.PublishSimpleService[api.TodoService](
		func(ctx context.Context) (api.TodoService, error) { return schedcore.ServiceIn(ctx) },
		publish.WithServiceDef(api.TodoServiceDef),
		publish.WithKTSPContainer(),
	)
	waits := publish.PublishSimpleService[api.WaitService](
		func(ctx context.Context) (api.WaitService, error) { return schedcore.WaitsIn(ctx) },
		publish.WithServiceDef(api.WaitServiceDef),
		publish.WithKTSPContainer(),
	)
	// A runtime row follows the content it names: a move
	// a split or an archive of an object re-points or ends the rows naming it.
	follow := cwmapi.ObserveEvents(cwmapi.EventObserver{Name: "scheduler-follow", Handle: schedcore.Follow})
	return func(init *application.ApplicationInit) {
		follow(init)
		init.AddRegisterPoints("scheduler", ".collectors", func(app *application.Application) error {
			return application.RegisterPoint(app, em.NewListPoint[api.Collector]())
		})
		init.AddRegisterPoints("scheduler", ".runners", func(app *application.Application) error {
			return application.RegisterPoint(app, em.NewListPoint[api.Runner]())
		})
		init.AddRegisterPoints("scheduler", ".group-members", func(app *application.Application) error {
			return application.RegisterPoint(app, em.NewListPoint[api.GroupMembers]())
		})
		init.AddRegisterPoints("scheduler", ".wait-namers", func(app *application.Application) error {
			return application.RegisterPoint(app, em.NewListPoint[api.WaitNamer]())
		})
		store(init)
		pulls(init)
		todos(init)
		waits(init)
		init.AddContribute("scheduler", ".machine", func(app *application.Application) error {
			// A fetched export newer than the store waits on the person's
			// import in « À faire ».
			if err := application.Contribute(app, schedcore.FetchedWaitingSource()); err != nil {
				slog.Debug("scheduler: no « À faire » in this binary, a fetched export is not shown there", "err", err)
			}
			// The universe backup job copies the store's newest hourly
			// snapshot.
			if err := application.Contribute(app, orchapi.BackupSnapshotSource{Name: "scheduler", Newest: schedcore.NewestSnapshot}); err != nil {
				slog.Debug("scheduler: no universe backup in this binary, the store's snapshot is not copied there", "err", err)
			}
			// A binary without the task engine runs no internal scheduler.
			if err := strategies.ContributeStrategy(app, schedcore.MachineStrategyDef); err != nil {
				slog.Debug("scheduler: no task engine in this binary, no internal scheduler", "err", err)
				return nil
			}
			if err := application.Contribute(app, ktsapi.ServingHook{Name: "scheduler", Start: start}); err != nil {
				slog.Debug("scheduler: no serving hooks in this binary, the internal scheduler does not start", "err", err)
			}
			return nil
		})
	}
}

// snapshotLook is how often the serving process looks whether the current
// hour has its snapshot.
const snapshotLook = 10 * time.Minute

// started is when this process started: a to-do reported before it was
// reported to a process that stopped.
var started = time.Now()

// start starts the internal scheduler's run and the automatic export once
// the process serves (a one-shot command never starts them).
func start(ctx context.Context) {
	// What a stopped process left reported is collected now — every module
	// manual or not: no tick would.
	if got := schedcore.CollectLeftReported(ctx, started); len(got) > 0 {
		slog.Info("scheduler: to-dos left reported by a stopped process collected", "todos", got)
	}
	// The export runs by itself while the process serves.
	go schedcore.RunAutoExport(ctx)
	// So do the store's hourly snapshot generations.
	go func() {
		t := time.NewTicker(snapshotLook)
		defer t.Stop()
		schedcore.RunHourlySnapshots(ctx, t.C)
	}()
	run, err := schedcore.StartScheduler(ctx)
	if err != nil {
		slog.Warn("scheduler: the internal scheduler did not start", "error", err)
		return
	}
	if run != "" {
		slog.Info("scheduler: the internal scheduler runs", "run", run)
	}
}

// ContributeCollector adds an owner's collector: what reads its reported
// to-dos. Called at init (Contribute or AppInit).
func ContributeCollector(app *application.Application, c api.Collector) error {
	return application.Contribute(app, c)
}

// ContributeRunner adds an owner's runner: how the internal scheduler runs its
// to-dos without the work-todo task. Called at init (Contribute or AppInit).
func ContributeRunner(app *application.Application, r api.Runner) error {
	return application.Contribute(app, r)
}

// ContributeGroupMembers adds an owner's holding lists: the members of the
// groups it lists, the only workers who pull their to-dos.
// Called at init (Contribute or AppInit).
func ContributeGroupMembers(app *application.Application, g api.GroupMembers) error {
	return application.Contribute(app, g)
}

// ContributeWaitNamer adds an owner's wait namer: the label, in the owner's
// words, a pull prefixes to the waiting line of one of its to-dos. Called at
// init (Contribute or AppInit).
func ContributeWaitNamer(app *application.Application, n api.WaitNamer) error {
	return application.Contribute(app, n)
}
