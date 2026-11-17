// SPDX-License-Identifier: Apache-2.0

package backup

import (
	"context"
	"log/slog"
	"path"
	"time"
)

// UniverseBackup is the backup job's configuration (UniverseBackup.yaml): the
// targets are the person's, the same for every universe they run. With no
// file the job is on and names no target: it copies nothing and the person is
// asked to name one.
type UniverseBackup struct {
	// On turns the job off when false; absent, it is on.
	On *bool `yaml:"on,omitempty" json:"on,omitempty"`
	// Local is the local target folder, off the system disk: every run.
	Local string `yaml:"local,omitempty" json:"local,omitempty"`
	// OffPC is the off-PC target folder: the day's first run.
	OffPC string `yaml:"off_pc,omitempty" json:"off_pc,omitempty"`
	// LocalOnly names the universes never copied off the PC (glob patterns).
	LocalOnly []string `yaml:"local_only,omitempty" json:"local_only,omitempty"`
	// Allow is the allow-list of the universe's configuration files;
	// DefaultConfigAllow when empty.
	Allow []string `yaml:"allow,omitempty" json:"allow,omitempty"`
	// SuggestedLocal is the folder the « Name the backup targets » form
	// offers while no local target is named: a stop-gap backup's own local
	// folder (its KATYOTI_BACKUP_LOCAL) when it runs beside the job.
	SuggestedLocal string `yaml:"suggested_local,omitempty" json:"suggested_local,omitempty" envkey:"KATYOTI_BACKUP_LOCAL"`
}

// Enabled answers whether the job runs: on unless set off.
func (c UniverseBackup) Enabled() bool { return c.On == nil || *c.On }

// NeedsTarget answers whether the job is on with no local target named: the
// question waiting on the person.
func (c UniverseBackup) NeedsTarget() bool { return c.Enabled() && c.Local == "" }

// IsLocalOnly answers whether a universe never goes off the PC.
func (c UniverseBackup) IsLocalOnly(universe string) bool {
	for _, p := range c.LocalOnly {
		if ok, _ := path.Match(p, universe); ok {
			return true
		}
	}
	return false
}

// Loop is the universe's backup job while the hub serves: a run at start when
// the current hour has no generation yet, then one at every tick (hourly).
type Loop struct {
	Universe  string // the universe's name
	ConfigDir string // the universe's folder the allow-list reads
	// Config reads UniverseBackup at each run, so a target named while the
	// hub serves is taken at the next tick.
	Config func(ctx context.Context) (UniverseBackup, error)
	// Snapshot answers the scheduler store's newest snapshot, "" when there
	// is none (the job then copies the configuration files only).
	Snapshot func(ctx context.Context) (string, error)
	Now      func() time.Time // the clock; time.Now when nil
	Tick     <-chan time.Time // the hourly ticks
	noTarget bool             // "no target" was logged once
}

// Run runs the job until ctx ends.
func (l *Loop) Run(ctx context.Context) {
	l.Once(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-l.Tick:
			l.Once(ctx)
		}
	}
}

// Once runs the job if it is due: on, a local target named, and no generation
// yet in the current hour. It answers the run's result and whether it ran.
func (l *Loop) Once(ctx context.Context) (Result, bool) {
	now := time.Now
	if l.Now != nil {
		now = l.Now
	}
	at := now().UTC()
	cfg, err := l.Config(ctx)
	if err != nil {
		slog.Warn("backup: the configuration cannot be read", "error", err)
		return Result{}, false
	}
	if !cfg.Enabled() {
		return Result{}, false
	}
	if cfg.Local == "" {
		if !l.noTarget {
			l.noTarget = true
			slog.Info("backup: no target named, nothing copied", "universe", l.Universe)
		}
		return Result{}, false
	}
	l.noTarget = false
	gens, err := generations(cfg.Local, l.Universe)
	if err != nil {
		slog.Warn("backup: the local target cannot be read", "dir", cfg.Local, "error", err)
		return Result{}, false
	}
	if len(gens) > 0 && gens[0].at.Truncate(time.Hour).Equal(at.Truncate(time.Hour)) {
		return Result{}, false
	}
	snap := ""
	if l.Snapshot != nil {
		if snap, err = l.Snapshot(ctx); err != nil {
			slog.Warn("backup: no snapshot of the scheduler store, configuration only", "error", err)
			snap = ""
		}
	}
	res, err := Run(Job{
		Universe: l.Universe, ConfigDir: l.ConfigDir, Allow: cfg.Allow, Snapshot: snap,
		Local: cfg.Local, OffPC: cfg.OffPC, LocalOnly: cfg.IsLocalOnly(l.Universe), Now: at,
	})
	if err != nil {
		slog.Warn("backup: the run failed", "universe", l.Universe, "error", err)
		return res, false
	}
	slog.Info("backup: copied", "universe", l.Universe, "written", len(res.Written), "off_pc", res.OffPC, "pruned", len(res.Pruned))
	return res, true
}
