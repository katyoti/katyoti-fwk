// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	orchapi "github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/backupjob"
	"github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/internal/sqlstore"
)

// : a real run of todos migrate snapshots the store first - one
// checked copy per run - and a dry run leaves none.
func TestARealMigrationIsSnapshottedFirstADryRunIsNot(t *testing.T) {
	dir := t.TempDir()
	ctx, _, todos, _ := filesWorld(t, dir)
	sched := envOf(t, ctx).SchedulerDir()
	snaps := func() []string {
		t.Helper()
		got, err := sqlstore.Snapshots(sched)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}

	migrate(t, ctx, todos, api.MigrateInput{DryRun: true})
	if got := snaps(); len(got) != 0 {
		t.Fatalf("a dry run with no store left snapshots: %v", got)
	}
	migrate(t, ctx, todos, api.MigrateInput{})
	got := snaps()
	if len(got) != 1 || !strings.HasSuffix(got[0], "-"+sqlstore.SnapshotMigrate+".db") {
		t.Fatalf("snapshots after a real run = %v, want one migrate snapshot", got)
	}
	if err := sqlstore.QuickCheckFile(context.Background(), got[0]); err != nil {
		t.Fatalf("the migration's snapshot fails quick_check: %v", err)
	}
	migrate(t, ctx, todos, api.MigrateInput{DryRun: true})
	if again := snaps(); len(again) != 1 {
		t.Fatalf("a dry run on the store left a snapshot: %v", again)
	}
	migrate(t, ctx, todos, api.MigrateInput{})
	if again := snaps(); len(again) != 2 {
		t.Fatalf("snapshots after a second real run = %v, want two", again)
	}
}

// : the scheduler answers its store's newest hourly snapshot -
// taken first when the hour has none - and gives it to the universe backup job
// through the backup's snapshot point.
func TestTheBackupJobGetsTheStoresHourlySnapshot(t *testing.T) {
	dir := t.TempDir()
	ctx, _, todos, _ := selectionWorldOf(t, dir, true, []string{"billing"}, nil, backupjob.Extension())
	if !storeOf(t, ctx).SQLite() {
		t.Fatal("want the SQLite store")
	}
	out, err := todos.Snapshot(ctx, api.SnapshotInput{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(out.Path, "-"+sqlstore.SnapshotHourly+".db") {
		t.Fatalf("snapshot = %q, want an hourly snapshot", out.Path)
	}
	if err := sqlstore.QuickCheckFile(context.Background(), out.Path); err != nil {
		t.Fatalf("the hourly snapshot fails quick_check: %v", err)
	}
	var sources []orchapi.BackupSnapshotSource
	for _, s := range application.UseExtensions[orchapi.BackupSnapshotSource](ctx) {
		if s.Name == "scheduler" {
			sources = append(sources, s)
		}
	}
	if len(sources) != 1 {
		t.Fatalf("the scheduler's backup snapshot sources = %d, want one", len(sources))
	}
	got, err := sources[0].Newest(ctx)
	if err != nil || got != out.Path {
		t.Fatalf("the backup's newest snapshot = %q, %v; want the hour's %q", got, err, out.Path)
	}
	if snaps, _ := sqlstore.Snapshots(envOf(t, ctx).SchedulerDir()); len(snaps) != 1 {
		t.Fatalf("snapshots = %v, want the one of the hour", snaps)
	}
}
