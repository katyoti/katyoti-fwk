// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// overrideGoldenFile records, byte for byte, the override object and the
// layer files the overwrites retirement writes: the migration is frozen, so
// any change of what it writes is seen here (KTW_OVERRIDE_GOLDEN_UPDATE=1
// re-records it, a deliberate act).
const overrideGoldenFile = "testdata/override-objects.golden"

// The overwrites retirement writes the same override object and layer files,
// byte for byte, as recorded.
func TestTheOverwritesRetirementWritesTheRecordedObjects(t *testing.T) {
	svc, project, _ := overwritesFixture(t)
	if _, err := svc.Migrate(stdctx.Background(), api.MigrateInput{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	mod := filepath.Join(project, "modules", "sys-contributions")
	var paths []string
	files := map[string]string{}
	_ = filepath.WalkDir(mod, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(mod, p)
		rel = filepath.ToSlash(rel)
		if !strings.Contains(rel, "override") && !strings.HasPrefix(rel, "files/") {
			return nil
		}
		b, _ := os.ReadFile(p)
		paths = append(paths, rel)
		files[rel] = strings.ReplaceAll(string(b), "\r\n", "\n")
		return nil
	})
	sort.Strings(paths)
	var sb strings.Builder
	for _, p := range paths {
		sb.WriteString("=== " + p + "\n" + files[p])
		if !strings.HasSuffix(files[p], "\n") {
			sb.WriteString("\n")
		}
	}
	got := sb.String()
	if len(paths) == 0 {
		t.Fatal("the migration wrote no override object")
	}
	if os.Getenv("KTW_OVERRIDE_GOLDEN_UPDATE") != "" {
		if err := os.WriteFile(overrideGoldenFile, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(overrideGoldenFile)
	if err != nil {
		t.Fatalf("%s: %v (KTW_OVERRIDE_GOLDEN_UPDATE=1 records it)", overrideGoldenFile, err)
	}
	if got != strings.ReplaceAll(string(want), "\r\n", "\n") {
		t.Errorf("the migration's output drifted from %s:\n--- got\n%s--- want\n%s", overrideGoldenFile, got, want)
	}
}
