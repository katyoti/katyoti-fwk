// SPDX-License-Identifier: Apache-2.0

package sqlstore

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// SNAPSHOTS - a copy of the store taken before every change that rewrites it
// in bulk: a planning-file migration (todos migrate, a real run), an import
// from the export refs (todos import) and the schema steps an existing file
// runs at Open. The copy is SQLite's own (VACUUM INTO: consistent while
// other processes write), written under a temporary name, checked with PRAGMA
// quick_check and renamed into place only once the check passes: a copy that
// fails it is deleted, never kept, and the change it was to protect is not
// made. The snapshots sit beside the store, in SnapshotDir, named
// scheduler-<UTC time>-<reason>.db so the newest sorts last; the store is
// restored by copying the newest one over scheduler.db while no process holds
// the store (its -wal and -shm files removed). The hourly and daily
// generations (reason hourly) are written by Hourly and pruned by
// PruneGenerations (generations.go).

// snapshotStamp is the layout of the UTC time in a snapshot's name.
const snapshotStamp = "2006-01-02T150405.000000000Z"

// SnapshotDir is the folder, beside the store's file, the snapshots are kept in.
const SnapshotDir = "snapshots"

// Snapshot reasons: the change a snapshot is taken before.
const (
	SnapshotMigrate = "migrate"
	SnapshotImport  = "import"
	SnapshotSchema  = "schema"
)

// snapshotCopied runs between the copy and its check: a test damages the copy
// there.
var snapshotCopied = func(path string) {}

// Snapshot copies the store to SnapshotDir before a change named by reason and
// answers the kept copy's path. The copy is kept only once its quick_check
// passes; otherwise it is deleted and an error names the fault.
func (s *Store) Snapshot(ctx context.Context, reason string) (string, error) {
	return snapshot(ctx, s.db, filepath.Dir(s.path), reason, time.Now())
}

// snapshot writes the copy of db, stamped with the time at, under dir.
func snapshot(ctx context.Context, db *sql.DB, dir, reason string, at time.Time) (string, error) {
	folder := filepath.Join(dir, SnapshotDir)
	if err := os.MkdirAll(folder, 0o755); err != nil {
		return "", fmt.Errorf("the scheduler's store snapshot: %w", err)
	}
	stamp := at.UTC().Format(snapshotStamp)
	path := filepath.Join(folder, "scheduler-"+stamp+"-"+reason+".db")
	part := path + ".part"
	_ = os.Remove(part)
	if _, err := db.ExecContext(ctx, "VACUUM INTO ?", part); err != nil {
		_ = os.Remove(part)
		return "", fmt.Errorf("the scheduler's store snapshot before %s: VACUUM INTO %s: %w", reason, part, err)
	}
	snapshotCopied(part)
	if err := QuickCheckFile(ctx, part); err != nil {
		_ = os.Remove(part)
		return "", fmt.Errorf("the scheduler's store snapshot before %s fails its quick_check (%v): the copy is deleted and the %s is not run", reason, err, reason)
	}
	if err := os.Rename(part, path); err != nil {
		_ = os.Remove(part)
		return "", fmt.Errorf("the scheduler's store snapshot before %s: %w", reason, err)
	}
	return path, nil
}

// QuickCheckFile runs PRAGMA quick_check on a database file opened read-only:
// nil when its one row is "ok", else the first fault.
func QuickCheckFile(ctx context.Context, path string) error {
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path)+"?mode=ro")
	if err != nil {
		return err
	}
	defer db.Close()
	faults, err := pragmaCheck(ctx, db, "PRAGMA quick_check")
	if err != nil {
		return err
	}
	if len(faults) > 0 {
		return fmt.Errorf("%s", faults[0])
	}
	return nil
}

// Snapshots lists the snapshots kept beside the store of dir, oldest first.
func Snapshots(dir string) ([]string, error) {
	entries, err := os.ReadDir(filepath.Join(dir, SnapshotDir))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if n := e.Name(); !e.IsDir() && strings.HasPrefix(n, "scheduler-") && strings.HasSuffix(n, ".db") {
			out = append(out, filepath.Join(dir, SnapshotDir, n))
		}
	}
	sort.Strings(out)
	return out, nil
}
