// SPDX-License-Identifier: Apache-2.0

package sqlstore

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// : the hub keeps the store's snapshot generations - hourly for
// 24, daily for 14 - each verified, the other snapshots left alone.

// TestTheHourlyGenerationsAreKeptAndPruned drives Hourly on a fake clock over
// more than fifteen days and finds exactly 24 hourly and 14 daily verified
// generations left, the migration's snapshot untouched.
func TestTheHourlyGenerationsAreKeptAndPruned(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	filled(t, dir)
	st := open(t, dir)
	defer st.Close()
	migrated, err := st.Snapshot(ctx, SnapshotMigrate)
	if err != nil {
		t.Fatal(err)
	}

	start := time.Date(2026, 3, 1, 0, 30, 0, 0, time.UTC)
	end := start.Add(16 * 24 * time.Hour)
	var newest string
	// Every six hours for the first days, then every hour for the last two:
	// the pruning sees both the daily and the hourly generations.
	for now := start; !now.After(end); {
		got, err := st.Hourly(ctx, now)
		if err != nil {
			t.Fatalf("hourly snapshot at %s: %v", now, err)
		}
		newest = got
		if end.Sub(now) > 48*time.Hour {
			now = now.Add(6 * time.Hour)
		} else {
			now = now.Add(time.Hour)
		}
	}

	// A second call within the same hour answers the same snapshot.
	again, err := st.Hourly(ctx, end.Add(20*time.Minute))
	if err != nil || again != newest {
		t.Fatalf("hourly within the same hour = %s, %v; want the kept %s", again, err, newest)
	}

	gens, err := generationsOf(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(gens) != KeepHourly+KeepDaily {
		t.Fatalf("%d generations kept, want %d hourly + %d daily", len(gens), KeepHourly, KeepDaily)
	}
	if gens[len(gens)-1].path != newest {
		t.Fatalf("the newest generation is %s, want %s", gens[len(gens)-1].path, newest)
	}
	hourly := gens[len(gens)-KeepHourly:]
	for i := 1; i < len(hourly); i++ {
		if d := hourly[i].at.Sub(hourly[i-1].at); d != time.Hour {
			t.Fatalf("hourly generations %s and %s are %s apart, want an hour", hourly[i-1].path, hourly[i].path, d)
		}
	}
	days := map[string]bool{}
	for _, g := range gens[:len(gens)-KeepHourly] {
		day := g.at.Format("2006-01-02")
		if days[day] {
			t.Fatalf("two daily generations for %s", day)
		}
		days[day] = true
	}
	for _, g := range gens {
		if err := QuickCheckFile(ctx, g.path); err != nil {
			t.Fatalf("%s fails quick_check: %v", g.path, err)
		}
	}
	if _, err := os.Stat(migrated); err != nil {
		t.Fatalf("the migration's snapshot was pruned: %v", err)
	}
	if got := snapshotsOf(t, dir); len(got) != KeepHourly+KeepDaily+1 {
		t.Fatalf("%d snapshots, want the generations and the migration's", len(got))
	}
}

// TestAFailedHourlySnapshotLeavesNothing: a copy failing its quick_check is
// deleted and no generation is kept.
func TestAFailedHourlySnapshotLeavesNothing(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	filled(t, dir)
	page, size := rootPage(t, dir, "todos")
	was := snapshotCopied
	snapshotCopied = func(path string) {
		f, err := os.OpenFile(path, os.O_RDWR, 0)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if _, err := f.WriteAt([]byte(strings.Repeat("\xa5", 64)), (page-1)*size); err != nil {
			t.Fatal(err)
		}
	}
	defer func() { snapshotCopied = was }()
	st := open(t, dir)
	defer st.Close()
	if _, err := st.Hourly(ctx, time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC)); err == nil || !strings.Contains(err.Error(), "quick_check") {
		t.Fatalf("hourly snapshot of a damaged copy = %v, want refused by its quick_check", err)
	}
	if got := snapshotsOf(t, dir); len(got) != 0 {
		t.Fatalf("a damaged hourly copy was kept: %v", got)
	}
	if entries, _ := os.ReadDir(filepath.Join(dir, SnapshotDir)); len(entries) != 0 {
		t.Fatalf("files left in %s: %d", SnapshotDir, len(entries))
	}
}
