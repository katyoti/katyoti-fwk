// SPDX-License-Identifier: Apache-2.0

package contrib

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"path"
	"strings"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	"github.com/katyoti/katyoti-fwk/internal/core/marshal"
	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
)

// DiscoverTasksFrom reads the tasks of folders the catalog does not carry:
// the task service's ExtraDirs, execution-only. Every task a binary ships is
// run from the catalog (CatalogTask), never from this walk. Two
// layouts are supported per loader root:
//
//   - Folder task: `<root>/<name>/TASK.md` — the canonical form. The folder
//     may contain companion files (scripts, templates, references) that
//     belong to that task and are NOT discovered as separate tasks.
//   - Flat task: `<root>/<name>.md` (or any `*.md` file other than TASK.md
//     under root). Convenient for short or simple tasks that don't need
//     a folder. Subfolders without a TASK.md are organisational — the
//     walk recurses into them and discovers flat tasks inside.
//
// Each task gets `Provider` set from the loader's Provider field. The
// returned map is keyed by URI (`provider:name`) so multiple providers
// can ship a task with the same bare name without collision.
//
// Logs a WARN when bare-name collisions occur across providers — used
// by callers to surface ambiguity hints.
func DiscoverTasksFrom(loaders []taskapi.TaskLoader) map[string]*taskapi.Task {
	tasks, _ := DiscoverTasksReporting(loaders)
	return tasks
}

// DiscoverTasksReporting is the same discovery, plus the files it could not
// read as tasks.
//
// One implementation, two callers with different needs: the runtime (its
// ExtraDirs) discards the drops and carries on — a malformed task must cost only itself — while
// packaging refuses on them, because a pack that ships without the task it was
// built to carry is worse than a pack that does not ship.
func DiscoverTasksReporting(loaders []taskapi.TaskLoader) (map[string]*taskapi.Task, []contribapi.Drop) {
	var drops []contribapi.Drop
	drop := func(file, reason string) {
		slog.Warn("task not published: file could not be read as a task",
			"file", file, "reason", reason)
		drops = append(drops, contribapi.Drop{Path: file, Kind: "task", Reason: reason})
	}
	tasks := make(map[string]*taskapi.Task)
	bareNameProviders := make(map[string][]string) // bareName → providers shipping it
	for _, tl := range loaders {
		for _, entry := range collectTaskFiles(tl) {
			data, err := tl.Loader.ReadFile(entry)
			if err != nil {
				drop(entry, "read: "+err.Error())
				continue
			}
			t, err := Parse(data)
			if err != nil {
				drop(entry, err.Error())
				continue
			}
			t.Provider = tl.Provider
			t.Source = data
			if err := readSchemaFiles(tl.Loader, entry, t); err != nil {
				drop(entry, err.Error())
				continue
			}
			tasks[t.URI()] = t
			bareNameProviders[t.Name] = append(bareNameProviders[t.Name], tl.Provider)
		}
	}
	// Surface ambiguity: a bare name resolves only when unique.
	for name, providers := range bareNameProviders {
		if len(providers) > 1 {
			slog.Warn("task name collision across providers",
				"name", name, "providers", providers,
				"hint", "use full URI (provider:name) to disambiguate")
		}
	}
	return tasks, drops
}

// collectTaskFiles returns the list of paths inside one TaskLoader's root
// that should be parsed as tasks. Implements both the folder-task and
// flat-task layouts (see DiscoverTasksFrom) plus companion-file safety.
//
// Algorithm:
//  1. Walk every file under tl.Root that ends in `.md`.
//  2. Folder tasks: any file named `TASK.md`. Record their parent directory
//     as "claimed" — every file inside it (recursively) belongs to the
//     folder task and must not be discovered as a flat task.
//  3. Flat tasks: any other `*.md` file whose path does NOT live inside
//     a claimed folder.
func collectTaskFiles(tl taskapi.TaskLoader) []string {
	allFiles := tl.Loader.Walk()

	// Compute the path prefix for entries belonging to this loader's root.
	// "" or "." means "everything"; otherwise we filter to children of the root.
	var rootPrefix string
	switch tl.Root {
	case "", ".":
		rootPrefix = ""
	default:
		rootPrefix = strings.TrimSuffix(tl.Root, "/") + "/"
	}

	var folderTasks []string
	var candidateFlat []string
	claimedFolders := make(map[string]bool)

	for _, f := range allFiles {
		if rootPrefix != "" && !strings.HasPrefix(f, rootPrefix) {
			continue
		}
		if !strings.HasSuffix(f, ".md") {
			continue
		}
		if path.Base(f) == "TASK.md" {
			folderTasks = append(folderTasks, f)
			claimedFolders[path.Dir(f)] = true
		} else {
			candidateFlat = append(candidateFlat, f)
		}
	}

	// Companion-file safety: drop any candidate flat task whose directory is
	// (or is a descendant of) a claimed folder-task root.
	flatTasks := make([]string, 0, len(candidateFlat))
	for _, f := range candidateFlat {
		dir := path.Dir(f)
		claimed := false
		for d := dir; d != "" && d != "." && d != "/"; d = path.Dir(d) {
			if claimedFolders[d] {
				claimed = true
				break
			}
		}
		if !claimed {
			flatTasks = append(flatTasks, f)
		}
	}

	out := make([]string, 0, len(folderTasks)+len(flatTasks))
	out = append(out, folderTasks...)
	out = append(out, flatTasks...)
	return out
}

// ── List / Get / library facade ─────────────────────────────────────────

// The suffixes of a task's I/O contract files, beside its file: the entry's
// name with its extension replaced (`summarise.input.dsl.yaml` beside
// `summarise.md`, `TASK.input.dsl.yaml` beside a folder's TASK.md). The task
// kind declares the same suffixes for the catalog (its companions).
const (
	InputSchemaSuffix  = ".input.dsl.yaml"
	OutputSchemaSuffix = ".output.dsl.yaml"
)

// readSchemaFiles reads a task's I/O contracts from their files beside it,
// when the file declares none inline (the old form, still read).
func readSchemaFiles(ldr loader.Loader, entry string, t *taskapi.Task) error {
	stem := strings.TrimSuffix(entry, path.Ext(entry))
	for _, s := range []struct {
		suffix string
		into   *json.RawMessage
	}{{InputSchemaSuffix, &t.InputSchema}, {OutputSchemaSuffix, &t.OutputSchema}} {
		if len(*s.into) > 0 {
			continue
		}
		raw, err := ldr.ReadFile(stem + s.suffix)
		if err != nil || len(raw) == 0 {
			continue // no contract file: the task declares none
		}
		var tree any
		if err := marshal.Unmarshal(raw, marshal.YAML, &tree); err != nil {
			return fmt.Errorf("%s%s: %w", stem, s.suffix, err)
		}
		b, err := json.Marshal(tree)
		if err != nil {
			return fmt.Errorf("%s%s: %w", stem, s.suffix, err)
		}
		*s.into = b
	}
	return nil
}
