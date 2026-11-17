// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"log/slog"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// hourlier is a store that keeps hourly snapshot generations: the SQLite store.
type hourlier interface {
	Hourly(ctx context.Context, now time.Time) (string, error)
}

// snapshotClock is the time of an hourly snapshot: a test names its own.
var snapshotClock = time.Now

// Snapshot answers the newest hourly snapshot of the store - taken first when
// the current hour has none, the generations then pruned
// an empty path on the file store, which has none.
func (s *Service) Snapshot(ctx context.Context, _ api.SnapshotInput) (api.SnapshotOutput, error) {
	h, ok := innerOf(s.store).(hourlier)
	if !ok {
		return api.SnapshotOutput{}, nil
	}
	path, err := h.Hourly(ctx, snapshotClock())
	return api.SnapshotOutput{Path: path}, err
}

// NewestSnapshot is Snapshot for the scheduler of the application ctx carries:
// what the universe backup job copies.
func NewestSnapshot(ctx context.Context) (string, error) {
	svc, err := ServiceIn(ctx)
	if err != nil {
		return "", err
	}
	out, err := svc.Snapshot(ctx, api.SnapshotInput{})
	return out.Path, err
}

// RunHourlySnapshots keeps the generations while the process serves: one
// snapshot at start when the current hour has none, then a look every tick
// until ctx ends.
func RunHourlySnapshots(ctx context.Context, tick <-chan time.Time) {
	for {
		if _, err := NewestSnapshot(ctx); err != nil {
			slog.Warn("scheduler: the hourly snapshot of the store failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-tick:
		}
	}
}
