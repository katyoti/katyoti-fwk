// SPDX-License-Identifier: Apache-2.0

package core

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/gitcli"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/internal/sqlstore"
)

// THE EXPORT REF - the history and the backup the committed
// to-do files gave, for the SQLite store: each repository's rows (its project's
// uuid) are written to a ref of the git repository that holds the project,
// refs/katyoti/scheduler/<project uuid>, as git objects only (hash-object,
// mktree, commit-tree, a compare-and-swap update-ref) - never a file of the
// working tree or the index, which people and agents share. Each export that
// changes something is a commit over the last one; import reads the ref back
// into the store. A done to-do is pruned only once an export holds it.

// ExportRefPrefix is the refs the export writes, one per project.
const ExportRefPrefix = "refs/katyoti/scheduler/"

// ExportRef is the ref a project's rows are exported to.
func ExportRef(repository string) string { return ExportRefPrefix + repository }

// The files at the root of an export's tree: store.json, and the three
// whole files an export written before the sharding held (export_tree.go).
const (
	exportStoreFile     = "store.json"
	exportTodosFile     = "todos.jsonl"
	exportRevisionsFile = "revisions.jsonl"
	exportWaitsFile     = "waits.jsonl"
)

// dumper is a store whose whole content goes out and comes back: the SQLite
// store. The file store's to-dos are their committed files already.
type dumper interface {
	Repositories(ctx context.Context) ([]string, error)
	Dump(ctx context.Context, repository string) (sqlstore.Dump, error)
	Restore(ctx context.Context, d sqlstore.Dump) (sqlstore.Restored, error)
	Snapshot(ctx context.Context, reason string) (string, error)
}

var _ dumper = (*sqlstore.Store)(nil)

// repositoryRoots answers, for each project uuid the universe's modules belong
// to, the top level of the git repository holding the project ("" when it is
// in none). A variable: a test names its own.
var repositoryRoots = func(ctx context.Context) (map[string]string, error) {
	cwm, err := cwmOf(ctx)
	if err != nil {
		return nil, err
	}
	mods, err := modules(ctx)
	if err != nil {
		return nil, err
	}
	out, tops := map[string]string{}, map[string]string{}
	for _, m := range mods {
		got, err := cwm.ObjectResolve(ctx, cwmapi.ObjectResolveInput{Ref: m})
		if err != nil || got.ProjectID == "" {
			continue
		}
		if _, seen := out[got.ProjectID]; seen {
			continue
		}
		top, ok := tops[got.ProjectRoot]
		if !ok {
			top = gitTopLevel(got.ProjectRoot)
			tops[got.ProjectRoot] = top
		}
		out[got.ProjectID] = top
	}
	return out, nil
}

// gitTopLevel is the top level of the git repository dir is in, "" when none.
func gitTopLevel(dir string) string {
	if dir == "" {
		return ""
	}
	out, err := gitcli.Run(dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return ""
	}
	return filepath.Clean(filepath.FromSlash(strings.TrimSpace(out)))
}

func dumpOf(s *Service) (dumper, error) {
	d, ok := innerOf(s.store).(dumper)
	if !ok {
		return nil, invalid("export and import keep the SQLite store; this scheduler runs on the file store, whose to-dos are their committed files")
	}
	return d, nil
}

// Export writes each repository's rows to its export ref.
func (s *Service) Export(ctx context.Context, in api.ExportInput) (api.ExportOutput, error) {
	d, err := dumpOf(s)
	if err != nil {
		return api.ExportOutput{}, err
	}
	roots, err := repositoryRoots(ctx)
	if err != nil {
		return api.ExportOutput{}, err
	}
	repos, err := d.Repositories(ctx)
	if err != nil {
		return api.ExportOutput{}, err
	}
	out := api.ExportOutput{Refs: []api.ExportedRef{}}
	for _, repo := range repos {
		if in.Repository != "" && repo != in.Repository {
			continue
		}
		got, skip, err := exportOne(ctx, d, roots, repo)
		if err != nil {
			return out, err
		}
		if skip != nil {
			out.Skipped = append(out.Skipped, *skip)
			continue
		}
		out.Refs = append(out.Refs, got)
	}
	return out, nil
}

// exportOne exports one repository's rows - or says why it has no ref to go
// to (a row of no project, a project in no git repository).
func exportOne(ctx context.Context, d dumper, roots map[string]string, repo string) (api.ExportedRef, *api.SkippedRepository, error) {
	dump, err := d.Dump(ctx, repo)
	if err != nil {
		return api.ExportedRef{}, nil, err
	}
	dir, known := roots[repo]
	switch {
	case repo == "":
		return api.ExportedRef{}, &api.SkippedRepository{Todos: len(dump.Todos), Waits: len(dump.Waits), Reason: "rows of no project: no git repository keeps them"}, nil
	case !known:
		return api.ExportedRef{}, &api.SkippedRepository{Repository: repo, Todos: len(dump.Todos), Waits: len(dump.Waits), Reason: "no project of the universe has this uuid"}, nil
	case dir == "":
		return api.ExportedRef{}, &api.SkippedRepository{Repository: repo, Todos: len(dump.Todos), Waits: len(dump.Waits), Reason: "its project is in no git repository"}, nil
	}
	ref := ExportRef(repo)
	commit, changed, err := writeOwnExport(dir, repo, dump)
	if err != nil {
		return api.ExportedRef{}, nil, fmt.Errorf("export of %s to %s in %s: %w", repo, ref, dir, err)
	}
	// The ref leaves the machine to the private remote (export_push.go); a
	// push refused or failed is said, never an error: the local ref holds it.
	p := pushExport(dir, ref, commit, func() (string, error) {
		c, ch, err := writeOwnExport(dir, repo, dump)
		changed = changed || ch
		if err == nil {
			commit = c
		}
		return c, err
	})
	return api.ExportedRef{Repository: repo, Dir: dir, Ref: ref, Commit: commit, Changed: changed,
		Todos: len(dump.Todos), Revisions: len(dump.Revisions), Waits: len(dump.Waits),
		Remote: p.Remote, Pushed: p.Pushed, PushError: p.Err}, nil, nil
}

// Import reads each known project's export ref back into the store.
func (s *Service) Import(ctx context.Context, in api.ImportInput) (api.ImportOutput, error) {
	d, err := dumpOf(s)
	if err != nil {
		return api.ImportOutput{}, err
	}
	roots, err := repositoryRoots(ctx)
	if err != nil {
		return api.ImportOutput{}, err
	}
	var repos []string
	for repo, dir := range roots {
		if dir != "" && (in.Repository == "" || repo == in.Repository) {
			repos = append(repos, repo)
		}
	}
	sort.Strings(repos)
	if in.Repository != "" && len(repos) == 0 {
		return api.ImportOutput{}, fmt.Errorf("%w: repository %s: no project of the universe in a git repository has this uuid", cwmapi.ErrNotFound, in.Repository)
	}
	out := api.ImportOutput{Refs: []api.ImportedRef{}}
	snapped := false
	for _, repo := range repos {
		ref, dir := ExportRef(repo), roots[repo]
		// A fetched tip an export wrote over before it was imported is read
		// first (export_fetched.go), then the ref's tip.
		kept, err := refTip(dir, fetchedRef(repo))
		if err != nil {
			return out, err
		}
		tip, err := refTip(dir, ref)
		if err != nil {
			return out, err
		}
		if tip == "" && kept == "" {
			continue // never exported
		}
		imp := api.ImportedRef{Repository: repo, Dir: dir, Ref: ref, Commit: tip}
		var from []string
		if kept != "" {
			from = append(from, kept)
		}
		if tip != "" && tip != kept {
			from = append(from, tip)
		}
		for _, at := range from {
			dump, err := readExportAt(dir, at)
			if err != nil {
				return out, fmt.Errorf("import of %s from %s in %s: %w", repo, ref, dir, err)
			}
			if dump.Repository != repo {
				return out, fmt.Errorf("%w: %s in %s holds the rows of %q", cwmapi.ErrInvalidInput, ref, dir, dump.Repository)
			}
			if !snapped {
				// The import rewrites the store: a checked copy of it first, once.
				if _, err := d.Snapshot(ctx, sqlstore.SnapshotImport); err != nil {
					return out, err
				}
				snapped = true
			}
			got, err := d.Restore(ctx, dump)
			if err != nil {
				return out, err
			}
			imp.Todos += got.Todos.Imported
			imp.TodosPresent += got.Todos.Present
			imp.Revisions += got.Revisions
			imp.Waits += got.Waits.Imported
			imp.WaitsPresent += got.Waits.Present
		}
		if imp.Commit == "" {
			imp.Commit = kept
		}
		if err := markSeen(dir, repo, imp.Commit, true); err != nil {
			return out, err
		}
		out.Refs = append(out.Refs, imp)
	}
	return out, nil
}

// writeOwnExport writes this store's rows over the ref: a tip it has not seen
// is kept first for the import (export_fetched.go), the commit written is
// seen.
func writeOwnExport(dir, repo string, d sqlstore.Dump) (string, bool, error) {
	if err := keepUnseen(dir, repo); err != nil {
		return "", false, err
	}
	commit, changed, err := writeExport(dir, ExportRef(repo), d)
	if err != nil {
		return "", false, err
	}
	return commit, changed, markSeen(dir, repo, commit, false)
}

// --- the ref -----------------------------------------------------------------

// jsonLines is one JSON line per row.
func jsonLines[T any](rows []T) ([]byte, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	for _, r := range rows {
		if err := enc.Encode(r); err != nil {
			return nil, err
		}
	}
	return b.Bytes(), nil
}

// refTip is the commit ref points at, "" when there is none.
func refTip(dir, ref string) (string, error) {
	out, err := gitcli.Run(dir, "for-each-ref", "--format=%(objectname)", ref)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// katyotiCommitter is the identity of the export's commits: a machine ref,
// and a repository with no user configured still takes them.
var katyotiCommitter = []string{"-c", "user.name=KaTyoti", "-c", "user.email=katyoti@localhost"}

// writeExport writes a dump as a commit over the ref's tip and moves the ref
// to it by compare-and-swap: unchanged (the tip's commit, false) when the tip
// holds the same tree. Another writer moving the ref first is taken as the new
// parent, a few times.
func writeExport(dir, ref string, d sqlstore.Dump) (string, bool, error) {
	files, err := exportFiles(d)
	if err != nil {
		return "", false, err
	}
	for attempt := 0; attempt < 3; attempt++ {
		tip, err := refTip(dir, ref)
		if err != nil {
			return "", false, err
		}
		base := ""
		if tip != "" {
			old, err := gitcli.Run(dir, "rev-parse", tip+"^{tree}")
			if err != nil {
				return "", false, err
			}
			base = strings.TrimSpace(old)
		}
		treeID, err := writeTree(dir, files, base)
		if err != nil {
			return "", false, err
		}
		if treeID == base {
			return tip, false, nil
		}
		args := append(append([]string{}, katyotiCommitter...), "commit-tree", treeID, "-m", fmt.Sprintf("scheduler export: %d to-dos, %d revisions, %d waits", len(d.Todos), len(d.Revisions), len(d.Waits)))
		if tip != "" {
			args = append(args, "-p", tip)
		}
		commit, err := gitcli.Run(dir, args...)
		if err != nil {
			return "", false, err
		}
		commit = strings.TrimSpace(commit)
		if _, err := gitcli.Run(dir, "update-ref", "-m", "scheduler export", ref, commit, tip); err == nil {
			return commit, true, nil
		}
	}
	return "", false, fmt.Errorf("%w: %s moved by another writer at every attempt", cwmapi.ErrConflict, ref)
}

// readExport reads the dump the ref's tip holds, and its commit; "" when the
// ref does not exist.
func readExport(dir, ref string) (sqlstore.Dump, string, error) {
	tip, err := refTip(dir, ref)
	if err != nil || tip == "" {
		return sqlstore.Dump{}, "", err
	}
	d, err := readExportAt(dir, tip)
	if err != nil {
		return d, "", err
	}
	return d, tip, nil
}

// readExportAt reads the dump an export commit holds.
func readExportAt(dir, commit string) (sqlstore.Dump, error) {
	files, err := treeBlobs(dir, commit)
	if err != nil {
		return sqlstore.Dump{}, err
	}
	return dumpOfTree(files)
}

// readLines decodes one JSON line per row of a file of the tree.
func readLines[T any](read func(string) ([]byte, error), name string, into *[]T) error {
	raw, err := read(name)
	if err != nil {
		return err
	}
	sc := bufio.NewScanner(bytes.NewReader(raw))
	sc.Buffer(make([]byte, 0, 1<<20), 1<<30)
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		var r T
		if err := json.Unmarshal(line, &r); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		*into = append(*into, r)
	}
	if err := sc.Err(); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	return nil
}

// exportBeforePrune exports the repositories whose done to-dos a prune is
// about to remove, once each: the repositories it could keep are answered;
// one with no ref to go to (no project, no git repository) is answered too -
// nothing keeps it, it is pruned as before; one whose export failed is not.
func exportBeforePrune(ctx context.Context, d dumper, repos map[string]bool) (map[string]bool, error) {
	roots, err := repositoryRoots(ctx)
	if err != nil {
		return nil, err
	}
	ok := map[string]bool{}
	var errs []error
	for repo := range repos {
		if _, _, err := exportOne(ctx, d, roots, repo); err != nil {
			errs = append(errs, err)
			continue
		}
		ok[repo] = true
	}
	return ok, errors.Join(errs...)
}
