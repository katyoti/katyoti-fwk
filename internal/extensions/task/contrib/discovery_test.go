// SPDX-License-Identifier: Apache-2.0

package contrib

import (
	"sort"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
)

// minimal valid TASK.md frontmatter for fixtures.
func taskMD(name string) string {
	return "---\nname: " + name + "\ntype: simulator\ndescription: fixture\n---\nbody"
}

func TestDiscoverTasks_FolderForm(t *testing.T) {
	// Original layout: tasks/<name>/TASK.md.
	files := map[string]string{
		"tasks/foo/TASK.md": taskMD("foo"),
		"tasks/bar/TASK.md": taskMD("bar"),
	}
	got := DiscoverTasksFrom([]taskapi.TaskLoader{
		{Loader: loader.NewMapLoader(files), Root: "tasks", Provider: "test"},
	})
	assertNames(t, got, []string{"bar", "foo"})
}

func TestDiscoverTasks_FlatForm(t *testing.T) {
	// New layout: tasks/<name>.md (no folder per task).
	files := map[string]string{
		"tasks/foo.md": taskMD("foo"),
		"tasks/bar.md": taskMD("bar"),
	}
	got := DiscoverTasksFrom([]taskapi.TaskLoader{
		{Loader: loader.NewMapLoader(files), Root: "tasks", Provider: "test"},
	})
	assertNames(t, got, []string{"bar", "foo"})
}

func TestDiscoverTasks_OrganisationalFolderWithFlat(t *testing.T) {
	// Mixed layout: organisational subfolder containing flat tasks.
	files := map[string]string{
		"tasks/audits/codebase-audit.md":        taskMD("codebase-audit"),
		"tasks/audits/security/threat-model.md": taskMD("threat-model"),
		"tasks/quick-task.md":                   taskMD("quick-task"),
	}
	got := DiscoverTasksFrom([]taskapi.TaskLoader{
		{Loader: loader.NewMapLoader(files), Root: "tasks", Provider: "test"},
	})
	assertNames(t, got, []string{"codebase-audit", "quick-task", "threat-model"})
}

func TestDiscoverTasks_MixedLayouts(t *testing.T) {
	// All four layouts simultaneously: prove they coexist without collision.
	files := map[string]string{
		"tasks/legacy-folder/TASK.md":          taskMD("legacy-folder"), // (1) folder
		"tasks/quick-flat.md":                  taskMD("quick-flat"),    // (2) flat at root
		"tasks/category/nested-flat.md":        taskMD("nested-flat"),   // (3) flat inside org folder
		"tasks/category/nested-folder/TASK.md": taskMD("nested-folder"), // (4) folder inside org folder
	}
	got := DiscoverTasksFrom([]taskapi.TaskLoader{
		{Loader: loader.NewMapLoader(files), Root: "tasks", Provider: "test"},
	})
	assertNames(t, got, []string{
		"legacy-folder", "nested-flat", "nested-folder", "quick-flat",
	})
}

func TestDiscoverTasks_CompanionFilesIgnored(t *testing.T) {
	// A folder task may carry companion *.md files (templates, references).
	// They MUST NOT be discovered as separate flat tasks.
	files := map[string]string{
		"tasks/audit/TASK.md":             taskMD("audit"),
		"tasks/audit/template.md":         "this is a template, not a task",
		"tasks/audit/references/intro.md": "reference doc",
		"tasks/audit/references/notes.md": taskMD("rogue-companion"), // even if it parses, must be skipped
	}
	got := DiscoverTasksFrom([]taskapi.TaskLoader{
		{Loader: loader.NewMapLoader(files), Root: "tasks", Provider: "test"},
	})
	assertNames(t, got, []string{"audit"})
}

func TestDiscoverTasks_NestedOrganisationalFolders(t *testing.T) {
	// Folders without a TASK.md are organisational at any depth — recurse in.
	files := map[string]string{
		"tasks/a/b/c/d/deep-task.md": taskMD("deep-task"),
	}
	got := DiscoverTasksFrom([]taskapi.TaskLoader{
		{Loader: loader.NewMapLoader(files), Root: "tasks", Provider: "test"},
	})
	assertNames(t, got, []string{"deep-task"})
}

func TestDiscoverTasks_NonMDFilesSkipped(t *testing.T) {
	// Non-`.md` files must never be considered task candidates, even at root.
	files := map[string]string{
		"tasks/script.py":                   "import sys",
		"tasks/category/data.json":          `{"foo": 1}`,
		"tasks/category/legitimate-task.md": taskMD("legitimate-task"),
	}
	got := DiscoverTasksFrom([]taskapi.TaskLoader{
		{Loader: loader.NewMapLoader(files), Root: "tasks", Provider: "test"},
	})
	assertNames(t, got, []string{"legitimate-task"})
}

func TestDiscoverTasks_OutsideRootIgnored(t *testing.T) {
	// Files outside tl.Root must be ignored entirely.
	files := map[string]string{
		"tasks/foo.md":          taskMD("foo"),
		"skills/bar.md":         taskMD("bar-from-skills"), // should NOT appear
		"unrelated/baz/TASK.md": taskMD("baz-from-elsewhere"),
	}
	got := DiscoverTasksFrom([]taskapi.TaskLoader{
		{Loader: loader.NewMapLoader(files), Root: "tasks", Provider: "test"},
	})
	assertNames(t, got, []string{"foo"})
}

func TestDiscoverTasks_BadFrontmatterSkipped(t *testing.T) {
	// Malformed `.md` files must be silently skipped — they shouldn't crash
	// discovery or remove valid sibling tasks.
	files := map[string]string{
		"tasks/good.md":           taskMD("good"),
		"tasks/no-frontmatter.md": "just markdown content with no `---` block",
		"tasks/empty.md":          "",
	}
	got := DiscoverTasksFrom([]taskapi.TaskLoader{
		{Loader: loader.NewMapLoader(files), Root: "tasks", Provider: "test"},
	})
	assertNames(t, got, []string{"good"})
}

// assertNames sorts both lists and compares.
func assertNames(t *testing.T, got map[string]*taskapi.Task, want []string) {
	t.Helper()
	names := make([]string, 0, len(got))
	for _, task := range got {
		names = append(names, task.Name)
	}
	sort.Strings(names)
	sort.Strings(want)
	if len(names) != len(want) {
		t.Fatalf("got %d tasks (%v), want %d (%v)", len(names), names, len(want), want)
	}
	for i := range names {
		if names[i] != want[i] {
			t.Errorf("[%d] got %q, want %q", i, names[i], want[i])
		}
	}
}
