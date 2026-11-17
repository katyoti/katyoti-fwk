// SPDX-License-Identifier: Apache-2.0

package backup

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// archives lists the sealed archives of a target (not their .sha256).
func archives(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".sha256") {
			out = append(out, e.Name())
		}
	}
	return out
}

func TestUniverseBackupIsOnWithNoFile(t *testing.T) {
	var none UniverseBackup
	if !none.Enabled() || !none.NeedsTarget() {
		t.Fatal("with no UniverseBackup.yaml the job is on and asks for a target")
	}
	off := false
	if (UniverseBackup{On: &off}).NeedsTarget() {
		t.Fatal("a job set off asks for nothing")
	}
	if (UniverseBackup{Local: "x"}).NeedsTarget() {
		t.Fatal("a named target asks for nothing")
	}
}

// The loop on a fake clock: nothing copied with no target; once a target is
// named, one run per hour (a second tick in the same hour copies nothing),
// the snapshot the source gives, off-PC only on the day's first run, never
// off-PC for a local-only universe.
func TestLoopRunsOncePerHourOnAFakeClock(t *testing.T) {
	root := t.TempDir()
	cfgDir := filepath.Join(root, "config")
	write(t, filepath.Join(cfgDir, "Univers.yaml"), "name: u\n")
	write(t, filepath.Join(cfgDir, ".env"), "SECRET=1\n")
	snap := filepath.Join(root, "store.db")
	write(t, snap, "sqlite")
	local, offpc := filepath.Join(root, "local"), filepath.Join(root, "offpc")

	cfg := UniverseBackup{}
	now := time.Date(2026, 11, 17, 6, 10, 0, 0, time.UTC)
	l := &Loop{
		Universe: "u", ConfigDir: cfgDir,
		Config:   func(context.Context) (UniverseBackup, error) { return cfg, nil },
		Snapshot: func(context.Context) (string, error) { return snap, nil },
		Now:      func() time.Time { return now },
	}
	ctx := context.Background()

	if _, ran := l.Once(ctx); ran || archives(t, local) != nil {
		t.Fatal("with no target the job copied something")
	}

	cfg = UniverseBackup{Local: local, OffPC: offpc}
	res, ran := l.Once(ctx)
	if !ran || !res.OffPC || len(res.Written) != 4 {
		t.Fatalf("first run of the day: ran=%v off-pc=%v written=%v", ran, res.OffPC, res.Written)
	}
	now = now.Add(20 * time.Minute)
	if _, ran := l.Once(ctx); ran {
		t.Fatal("a second run in the same hour")
	}
	now = now.Add(time.Hour)
	res, ran = l.Once(ctx)
	if !ran || res.OffPC {
		t.Fatalf("the next hour: ran=%v off-pc=%v", ran, res.OffPC)
	}
	if got := len(archives(t, local)); got != 4 {
		t.Fatalf("local holds %d archives, want 4 (two hours, snapshot and config)", got)
	}
	if got := len(archives(t, offpc)); got != 2 {
		t.Fatalf("off-PC holds %d archives, want 2 (the day's first run)", got)
	}
	for _, f := range []string{"u-20261117T061000Z.db.gz", "u-20261117T061000Z-config.tar.gz"} {
		if err := Verify(filepath.Join(local, f)); err != nil {
			t.Fatal(err)
		}
	}
	if names := tarNames(t, filepath.Join(local, "u-20261117T061000Z-config.tar.gz")); len(names) != 1 || names[0] != "Univers.yaml" {
		t.Fatalf("config archive holds %v", names)
	}

	// A local-only universe never goes off the PC, even on a new day.
	cfg.LocalOnly = []string{"u*"}
	now = now.Add(24 * time.Hour)
	if res, ran := l.Once(ctx); !ran || res.OffPC {
		t.Fatalf("local-only universe: ran=%v off-pc=%v", ran, res.OffPC)
	}
}

// With no snapshot (no source contributed yet) the job copies the
// configuration files only.
func TestLoopWithNoSnapshotCopiesConfigOnly(t *testing.T) {
	root := t.TempDir()
	cfgDir := filepath.Join(root, "config")
	write(t, filepath.Join(cfgDir, "Univers.yaml"), "name: u\n")
	local := filepath.Join(root, "local")
	l := &Loop{
		Universe: "u", ConfigDir: cfgDir,
		Config: func(context.Context) (UniverseBackup, error) { return UniverseBackup{Local: local}, nil },
		Now:    func() time.Time { return time.Date(2026, 11, 17, 6, 0, 0, 0, time.UTC) },
	}
	res, ran := l.Once(context.Background())
	if !ran || len(res.Written) != 1 || !strings.HasSuffix(res.Written[0], "-config.tar.gz") {
		t.Fatalf("ran=%v written=%v", ran, res.Written)
	}
}

// Run starts with a run and runs again at each tick, until its context ends.
func TestLoopRunRunsAtStartAndAtEachTick(t *testing.T) {
	root := t.TempDir()
	cfgDir := filepath.Join(root, "config")
	write(t, filepath.Join(cfgDir, "Univers.yaml"), "name: u\n")
	local := filepath.Join(root, "local")
	tick := make(chan time.Time)
	at := time.Date(2026, 11, 17, 6, 0, 0, 0, time.UTC)
	clock := make(chan time.Time, 2)
	clock <- at
	clock <- at.Add(time.Hour)
	l := &Loop{
		Universe: "u", ConfigDir: cfgDir, Tick: tick,
		Config: func(context.Context) (UniverseBackup, error) { return UniverseBackup{Local: local}, nil },
		Now:    func() time.Time { return <-clock },
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { l.Run(ctx); close(done) }()
	tick <- at.Add(time.Hour)
	cancel()
	<-done
	if got := len(archives(t, local)); got != 2 {
		t.Fatalf("local holds %d archives, want 2 (start and one tick)", got)
	}
}
