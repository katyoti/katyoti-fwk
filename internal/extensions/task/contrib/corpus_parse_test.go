// SPDX-License-Identifier: Apache-2.0

package contrib

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCorpusParse_AllInRepoTASKmdStrictGreen walks every in-repo TASK.md
// shipped from a sidecar's `embed/tasks/` directory and asserts the
// canonical strict parser (`Parse`) accepts it.
//
// Scope:
//   - Only `app/<sidecar>/embed/tasks/**/TASK.md` are loaded by the task
//     parser at runtime (per `tasklocal.ContributeTaskResources`).
//   - `embed/personas/**/TASK.md` files are persona artifacts loaded by
//     a separate parser (see internal/extensions/personas/) and are
//     intentionally excluded — they use different frontmatter shape.
//   - `modules/`, `test_files/`, and `internal/` TASK.md files are
//     test fixtures or charter scaffolding — excluded.
//
// Failure mode: a new TASK.md added under `app/*/embed/tasks/` that
// violates the canonical schema (unknown root key, missing required
// name/type, legacy flat shape) will fail this test, surfacing drift
// before it ships. Add the failing file to the migration checklist or
// fix the file's frontmatter.
func TestCorpusParse_AllInRepoTASKmdStrictGreen(t *testing.T) {
	root := findRepoRoot(t)

	var taskFiles []string
	err := filepath.Walk(filepath.Join(root, "app"), func(path string, info os.FileInfo, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil // a throwaway app rendered and removed by another test
			}
			return err
		}
		if info.IsDir() && (strings.HasPrefix(info.Name(), "_") || strings.HasPrefix(info.Name(), ".")) {
			return filepath.SkipDir // throwaway or hidden — never corpus
		}
		if info.IsDir() {
			return nil
		}
		// Accept BOTH layouts during/after the cwm-v2-providers US-15
		// task-folder reorg (2026-06-08):
		//  - Legacy folder-form: `<root>/<name>/TASK.md`
		//  - Flat form (new):    `<root>/<name>.md` (any *.md file
		//    under embed/tasks/ that isn't TASK.md itself)
		// Both shapes go through the same strict-parse contract.
		if !strings.HasSuffix(info.Name(), ".md") {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		relSlash := filepath.ToSlash(rel)
		if !strings.Contains(relSlash, "/embed/tasks/") {
			return nil
		}
		taskFiles = append(taskFiles, path)
		return nil
	})
	if err != nil {
		t.Fatalf("walk app/: %v", err)
	}

	if len(taskFiles) == 0 {
		t.Skip("no in-repo task files found under app/*/embed/tasks/")
	}

	t.Logf("parsing %d in-repo TASK.md files in strict mode", len(taskFiles))

	for _, f := range taskFiles {
		f := f
		rel, _ := filepath.Rel(root, f)
		t.Run(rel, func(t *testing.T) {
			data, err := os.ReadFile(f)
			if err != nil {
				t.Fatalf("read: %v", err)
			}
			task, err := Parse(data)
			if err != nil {
				t.Errorf("strict parse failed: %v\n(if this is a new legacy TASK.md, add to the migration sweep instead of disabling this test)", err)
				return
			}
			if task.Name == "" {
				t.Errorf("parsed task has empty Name")
			}
			if task.Type == "" {
				t.Errorf("parsed task has empty Type")
			}
		})
	}
}

// findRepoRoot walks up from cwd until it finds go.mod.
func findRepoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	dir := wd
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir || !strings.Contains(parent, string(filepath.Separator)) {
			t.Fatalf("repo root not found from %s", wd)
		}
		dir = parent
	}
}
