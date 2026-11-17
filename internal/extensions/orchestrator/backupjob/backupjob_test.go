// SPDX-License-Identifier: Apache-2.0

package backupjob

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/render"

	"github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/internal/backup"
)

// One question while the job is on with no target; none once a target is
// named or the job is off.
func TestWaitingSourceAsksForATargetOnlyWhileNoneIsNamed(t *testing.T) {
	cfg := backup.UniverseBackup{}
	src := backupWaitingSource(func(context.Context) (backup.UniverseBackup, error) { return cfg, nil })
	ctx := context.Background()
	items, err := src.Items(ctx)
	if err != nil || len(items) != 1 || items[0].Title != "Name the backup targets" {
		t.Fatalf("no target: items=%v err=%v", items, err)
	}
	cfg.Local = t.TempDir()
	if items, _ := src.Items(ctx); len(items) != 0 {
		t.Fatalf("a target named: items=%v", items)
	}
	off := false
	cfg = backup.UniverseBackup{On: &off}
	if items, _ := src.Items(ctx); len(items) != 0 {
		t.Fatalf("job off: items=%v", items)
	}
}

// The item carries its answer form; answering it writes UniverseBackup.yaml -
// the global layer's when the universe holds none, the universe's when it
// does - and the item goes.
func TestAnsweringTheItemWritesTheTargetsAndTheItemGoes(t *testing.T) {
	ctx := context.Background()
	s := backupStore{global: t.TempDir(), universe: t.TempDir()}
	t.Setenv("KATYOTI_BACKUP_LOCAL", "")
	src := backupWaitingSource(s.Load)
	items, err := src.Items(ctx)
	if err != nil || len(items) != 1 {
		t.Fatalf("no target: items=%v err=%v", items, err)
	}
	if src.Controls == nil || len(src.Controls(ctx, items[0], "fr")) != 1 {
		t.Fatal("the item carries no answer form")
	}
	if res, _ := s.Targets(ctx, BackupTargetsInput{Local: "relative"}); res.OK {
		t.Fatal("a relative folder was taken")
	}
	local, off := filepath.Join(t.TempDir(), "backup"), filepath.Join(t.TempDir(), "drive")
	res, err := s.Targets(ctx, BackupTargetsInput{Local: local, OffPC: off})
	if err != nil || !res.OK {
		t.Fatalf("answer refused: %+v err=%v", res, err)
	}
	if _, err := os.Stat(filepath.Join(s.global, "UniverseBackup.yaml")); err != nil {
		t.Fatalf("the global UniverseBackup.yaml was not written: %v", err)
	}
	cfg, _ := s.Load(ctx)
	if cfg.Local != local || cfg.OffPC != off {
		t.Fatalf("read back: %+v", cfg)
	}
	if items, _ := src.Items(ctx); len(items) != 0 {
		t.Fatalf("the item stays once answered: %v", items)
	}

	// A universe holding its own file: it is the one written, its other
	// fields kept.
	if err := os.WriteFile(filepath.Join(s.universe, "UniverseBackup.yaml"), []byte("local_only: [\"client-*\"]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if items, _ := src.Items(ctx); len(items) != 1 {
		t.Fatalf("the universe's file replaces the global one: items=%v", items)
	}
	if res, _ := s.Targets(ctx, BackupTargetsInput{Local: local}); !res.OK {
		t.Fatalf("answer refused: %+v", res)
	}
	cfg, _ = s.Load(ctx)
	if cfg.Local != local || len(cfg.LocalOnly) != 1 {
		t.Fatalf("universe file read back: %+v", cfg)
	}
	if items, _ := src.Items(ctx); len(items) != 0 {
		t.Fatalf("the item stays once answered: %v", items)
	}
}

// The form offers the stop-gap's folder while no target is named.
func TestTheFormOffersTheSuggestedLocalFolder(t *testing.T) {
	ctx := context.Background()
	want := filepath.Join(t.TempDir(), "stop-gap")
	t.Setenv("KATYOTI_BACKUP_LOCAL", want)
	s := backupStore{global: t.TempDir()}
	src := backupWaitingSource(s.Load)
	items, _ := src.Items(ctx)
	if len(items) != 1 {
		t.Fatalf("items=%v", items)
	}
	form := src.Controls(ctx, items[0], "en")[0]
	if data, _ := form["data"].(render.Node); data["local"] != want {
		t.Fatalf("form data: %v", form["data"])
	}
}
