// SPDX-License-Identifier: Apache-2.0

package local

import (
	"context"
	"log/slog"

	"github.com/katyoti/katyoti-fwk/internal/appcore/hubclient"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
	ktsapi "github.com/katyoti/katyoti-fwk/internal/kts/api"
)

// contributeRestore adds the serving hook that restores the runs a previous
// process left: once the process serves its universe, its waiting runs resume
// and the others are reported interrupted. Only then — a one-shot command
// must never pick up the universe's runs.
func contributeRestore(init *application.ApplicationInit) {
	init.AddContribute("task", ".restore", func(app *application.Application) error {
		if err := application.Contribute(app, ktsapi.ServingHook{Name: "task-restore", Start: restoreRuns, First: true}); err != nil {
			slog.Warn("tasks: no serving hooks in this binary — the runs a previous process left are not restored", "err", err)
		}
		return nil
	})
}

// restoreRuns asks this process's TaskService to restore the runs a previous
// process left, and to look again while it serves (taskcore.Service.ServeRestores).
func restoreRuns(ctx context.Context) {
	svc, err := application.UseService[taskapi.TaskService](ctx)
	if err != nil {
		slog.Warn("tasks: restore: no task service", "err", err)
		return
	}
	// Events published while the bus was down never arrive: on every
	// reconnect the waits that recheck themselves look again.
	if hub, err := application.UseService[*hubclient.HubClient](ctx); err == nil && hub != nil {
		if rw, ok := svc.(interface{ RecheckWaits() }); ok {
			hub.OnReconnect(rw.RecheckWaits)
		}
	}
	// The other serving hooks (the planner) start once the runs a previous
	// process left are decided.
	if r, ok := svc.(interface {
		ServeRestores(context.Context, func())
	}); ok {
		r.ServeRestores(ctx, func() { ktsapi.ServingReady(ctx) })
	}
}
