// SPDX-License-Identifier: Apache-2.0

// Package backupjob runs the universe backup job on the binary that serves the
// universe and asks the person for its target in « À faire ».
package backupjob

import (
	"context"
	"log/slog"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/em"
	"github.com/katyoti/katyoti-fwk/internal/core/publish"
	inboxapi "github.com/katyoti/katyoti-fwk/internal/extensions/inbox/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/internal/backup"
	ktsapi "github.com/katyoti/katyoti-fwk/internal/kts/api"
)

// BackupWaitingSourceName is the backup job's source in « À faire ».
const BackupWaitingSourceName = "universe-backup"

// Extension runs the universe's backup job while the binary serves (a
// one-shot command never starts it): one run at start when the current hour
// has none, then one an hour, with the configuration of UniverseBackup.yaml
// and the snapshot a BackupSnapshotSource contributes to its point. With no
// target named it copies nothing and asks the person, in « À faire », to name
// one, its answer form writing the targets (ktw-agents:universe-backup/targets).
func Extension() application.Extension {
	return func(init *application.ApplicationInit) {
		publish.PublishSimpleService[UniverseBackupService](
			func(context.Context) (UniverseBackupService, error) { return envStore(), nil },
			publish.WithServiceDef(BackupServiceDef),
			publish.WithKTSPContainer(),
			publish.WithKTSPSlotTags(map[string]string{"cli": "false"}),
		)(init)
		init.AddRegisterPoints("orchestrator", ".backup-snapshot", func(app *application.Application) error {
			return application.RegisterPoint(app, em.NewListPoint[api.BackupSnapshotSource]())
		})
		init.AddContribute("orchestrator", ".backup", func(app *application.Application) error {
			if err := application.Contribute(app, ktsapi.ServingHook{Name: "universe-backup", Start: startBackup}); err != nil {
				slog.Debug("backup: no serving hooks in this binary, the backup job does not run", "err", err)
			}
			if err := application.Contribute(app, backupWaitingSource(backupConfig)); err != nil {
				slog.Debug("backup: no « À faire » in this binary, the missing target is not asked there", "err", err)
			}
			return nil
		})
	}
}

// backupConfig reads UniverseBackup.yaml at each call: the universe's file,
// else the global layer's (backupStore).
func backupConfig(ctx context.Context) (backup.UniverseBackup, error) {
	return envStore().Load(ctx)
}

// newestSnapshot asks the contributed snapshot sources, the first that has
// one; "" when none is contributed or none has a snapshot.
func newestSnapshot(ctx context.Context) (string, error) {
	for _, src := range application.UseExtensions[api.BackupSnapshotSource](ctx) {
		if src.Newest == nil {
			continue
		}
		p, err := src.Newest(ctx)
		if err != nil || p != "" {
			return p, err
		}
	}
	return "", nil
}

// startBackup starts the job's loop on the universe this binary serves.
func startBackup(ctx context.Context) {
	env := application.Env()
	if env == nil || env.UniverseName() == "" {
		slog.Debug("backup: no universe, the backup job does not run")
		return
	}
	t := time.NewTicker(time.Hour)
	l := &backup.Loop{
		Universe: env.UniverseName(), ConfigDir: env.ConfigDir(),
		Config: backupConfig, Snapshot: newestSnapshot, Tick: t.C,
	}
	go func() {
		defer t.Stop()
		l.Run(ctx)
	}()
}

// backupWaitingSource lists one question while the job is on and names no
// target: name the backup targets, answered in place by its form.
func backupWaitingSource(config func(context.Context) (backup.UniverseBackup, error)) inboxapi.WaitingSource {
	return inboxapi.WaitingSource{
		Name:     BackupWaitingSourceName,
		Controls: targetsControls(config),
		Items: func(ctx context.Context) ([]inboxapi.WaitingItem, error) {
			cfg, err := config(ctx)
			if err != nil || !cfg.NeedsTarget() {
				return nil, err
			}
			return []inboxapi.WaitingItem{{
				Key:   BackupWaitingSourceName + ":target",
				Kind:  inboxapi.KindProvide,
				Title: "Name the backup targets",
				Asks: "The universe backup job is on but has no target, so nothing is copied. " +
					"Name a local folder off the system disk and, if you want, an off-PC folder: they are written to UniverseBackup.yaml. " +
					"A stop-gap backup script already running (a local folder, rclone off the PC) keeps running until this job pushes off the PC itself.",
				Module: "univers", About: "univers", AnswerableBy: "person",
			}}, nil
		},
	}
}
