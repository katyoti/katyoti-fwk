// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"log/slog"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/internal/sqlstore"
)

// THE EXPORT RUNS BY ITSELF - the serving process looks at
// the store every autoExportCheck and exports (Export, every project) when it
// moved: autoExportChanges changes since the last export, autoExportEvery
// passed with a change, or the agents' pause - a to-do was claimed at the last
// look and none is now (the last claimed to-do reported or released). The
// sharded tree (export_tree.go) makes such an export write only what changed.
// When the process stops serving (its context ends) a last export runs if
// anything moved.

// The thresholds of the export that runs by itself; variables, a test sets
// its own.
var (
	autoExportCheck   = 30 * time.Second
	autoExportEvery   = 5 * time.Minute
	autoExportChanges = int64(200)
)

// autoExport is what the loop remembers between two looks.
type autoExport struct {
	exported bool          // an export ran (or was tried) since the process started
	mark     sqlstore.Mark // the store's mark at that export
	at       time.Time     // when
	claimed  int64         // the to-dos claimed at the last look
}

// due says whether the store at mark, now, calls for an export, and why ("" not).
func (a *autoExport) due(mark sqlstore.Mark, now time.Time) string {
	paused := a.claimed > 0 && mark.Claimed == 0
	a.claimed = mark.Claimed
	if !a.exported {
		return "start"
	}
	changes := mark.Changes(a.mark)
	switch {
	case changes == 0:
		return ""
	case changes >= autoExportChanges:
		return "changes"
	case paused:
		return "agents' pause"
	case now.Sub(a.at) >= autoExportEvery:
		return "interval"
	}
	return ""
}

// done records an export run on the store at mark.
func (a *autoExport) done(mark sqlstore.Mark, now time.Time) {
	a.exported, a.mark, a.at = true, mark, now
}

// marker is a store that tells how far it moved: the SQLite store.
type marker interface {
	Mark(ctx context.Context) (sqlstore.Mark, error)
}

var _ marker = (*sqlstore.Store)(nil)

// RunAutoExport is the serving process's export loop: it returns at once on
// the file store (its to-dos are committed files), else when ctx ends, after a
// last export if anything moved.
func RunAutoExport(ctx context.Context) {
	svc, err := ServiceIn(ctx)
	if err != nil {
		slog.Debug("scheduler: no automatic export, no scheduler", "error", err)
		return
	}
	m, ok := innerOf(svc.store).(marker)
	if !ok {
		return
	}
	var a autoExport
	look := func(ctx context.Context, last bool) {
		mark, err := m.Mark(ctx)
		if err != nil {
			slog.Warn("scheduler: the automatic export could not read the store", "error", err)
			return
		}
		why := a.due(mark, time.Now())
		if last && a.exported && mark.Changes(a.mark) > 0 {
			why = "stop"
		}
		if why == "" {
			return
		}
		a.done(mark, time.Now())
		out, err := svc.Export(ctx, api.ExportInput{})
		if err != nil {
			slog.Warn("scheduler: the automatic export failed", "why", why, "error", err)
			return
		}
		for _, r := range out.Refs {
			if r.Changed {
				slog.Info("scheduler: exported", "why", why, "ref", r.Ref, "dir", r.Dir, "commit", r.Commit, "todos", r.Todos)
			}
			if r.PushError != "" {
				slog.Warn("scheduler: the export ref was not pushed", "ref", r.Ref, "dir", r.Dir, "remote", r.Remote, "error", r.PushError)
			}
		}
	}
	for {
		look(ctx, false)
		select {
		case <-ctx.Done():
			look(context.WithoutCancel(ctx), true)
			return
		case <-time.After(autoExportCheck):
		}
	}
}
