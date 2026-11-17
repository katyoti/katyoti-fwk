// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/internal/sqlstore"
)

// : todos import snapshots the store before it restores the
// first ref - one checked copy per import - and an import with no ref to read
// leaves none.
func TestAnImportIsSnapshottedFirst(t *testing.T) {
	ctx := context.Background()
	repoDir := gitRepo(t)
	rootsAre(t, map[string]string{"repo-1": repoDir})
	src := openSQL(t, t.TempDir(), "repo-1")
	fill(t, ctx, src)

	dir := t.TempDir()
	st := openSQL(t, dir, "repo-1")
	if _, err := NewOver(st).Import(ctx, api.ImportInput{}); err != nil {
		t.Fatal(err)
	}
	if got, _ := sqlstore.Snapshots(dir); len(got) != 0 {
		t.Fatalf("an import with no ref to read left snapshots: %v", got)
	}

	if _, err := NewOver(src).Export(ctx, api.ExportInput{}); err != nil {
		t.Fatal(err)
	}
	if _, err := NewOver(st).Import(ctx, api.ImportInput{}); err != nil {
		t.Fatal(err)
	}
	got, err := sqlstore.Snapshots(dir)
	if err != nil || len(got) != 1 || !strings.HasSuffix(got[0], "-"+sqlstore.SnapshotImport+".db") || filepath.Dir(got[0]) != filepath.Join(dir, sqlstore.SnapshotDir) {
		t.Fatalf("snapshots after an import = %v (%v), want one import snapshot", got, err)
	}
	if err := sqlstore.QuickCheckFile(ctx, got[0]); err != nil {
		t.Fatalf("the import's snapshot fails quick_check: %v", err)
	}
}
