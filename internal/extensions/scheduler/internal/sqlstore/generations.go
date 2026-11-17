// SPDX-License-Identifier: Apache-2.0

package sqlstore

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// GENERATIONS - the hub keeps the store's snapshot generations by itself
// : Hourly takes a snapshot with reason hourly (VACUUM INTO
// kept only once its quick_check passes, like every snapshot) unless one of
// the current hour exists, then PruneGenerations keeps the 24 newest hourly
// generations and, of the older ones, the newest of each UTC day for the 14
// newest such days. Only the hourly snapshots are pruned: those taken before a
// migration, an import or a schema step are left alone.

// SnapshotHourly is the reason of the hub's hourly generations.
const SnapshotHourly = "hourly"

// Generations kept: the newest hourly ones, then one a day.
const (
	KeepHourly = 24
	KeepDaily  = 14
)

// generation is one hourly snapshot and the time its name carries.
type generation struct {
	path string
	at   time.Time
}

// generationsOf lists the hourly snapshots of dir's store, oldest first; a
// name whose time does not parse is not one of them.
func generationsOf(dir string) ([]generation, error) {
	all, err := Snapshots(dir)
	if err != nil {
		return nil, err
	}
	var out []generation
	for _, p := range all {
		name := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(p), "scheduler-"), ".db")
		stamp, reason, ok := cutLast(name, "-")
		if !ok || reason != SnapshotHourly {
			continue
		}
		at, err := time.Parse(snapshotStamp, stamp)
		if err != nil {
			continue
		}
		out = append(out, generation{path: p, at: at})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].at.Before(out[j].at) })
	return out, nil
}

// cutLast splits s around the last sep.
func cutLast(s, sep string) (string, string, bool) {
	i := strings.LastIndex(s, sep)
	if i < 0 {
		return s, "", false
	}
	return s[:i], s[i+len(sep):], true
}

// Hourly answers the path of the hourly snapshot of now's hour: the one kept
// when it exists, else a new one taken at now - then the generations are
// pruned. A copy failing its quick_check is deleted and nothing is kept.
func (s *Store) Hourly(ctx context.Context, now time.Time) (string, error) {
	dir := filepath.Dir(s.path)
	gens, err := generationsOf(dir)
	if err != nil {
		return "", fmt.Errorf("the scheduler's store snapshots: %w", err)
	}
	hour := now.UTC().Truncate(time.Hour)
	if n := len(gens); n > 0 && gens[n-1].at.Truncate(time.Hour).Equal(hour) {
		return gens[n-1].path, nil
	}
	path, err := snapshot(ctx, s.db, dir, SnapshotHourly, now)
	if err != nil {
		return "", err
	}
	if err := PruneGenerations(dir); err != nil {
		return path, err
	}
	return path, nil
}

// PruneGenerations deletes the hourly snapshots of dir's store past their
// keeping: the KeepHourly newest stay, then of the older ones the newest of
// each UTC day, for the KeepDaily newest days. Other snapshots are left alone.
func PruneGenerations(dir string) error {
	gens, err := generationsOf(dir)
	if err != nil {
		return err
	}
	days := map[string]bool{}
	for i := len(gens) - 1; i >= 0; i-- {
		if len(gens)-1-i < KeepHourly {
			continue
		}
		day := gens[i].at.UTC().Format("2006-01-02")
		if !days[day] && len(days) < KeepDaily {
			days[day] = true
			continue
		}
		if err := os.Remove(gens[i].path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("the scheduler's store snapshot %s: %w", gens[i].path, err)
		}
	}
	return nil
}
