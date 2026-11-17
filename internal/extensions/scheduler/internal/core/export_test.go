// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"os"
	"reflect"
	"slices"
	"testing"
	"testing/fstest"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/gitcli"
	"github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/internal/sqlstore"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// : todos export and import write the scheduler's whole store -
// open and done to-dos, claims, waits and their revisions - to a git ref and
// read it back: after a wipe, the same records with the same revisions.

// gitRepo is a git repository with one commit and a file people work on.
func gitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, args := range [][]string{
		{"init", "-q"},
		{"config", "user.name", "test"},
		{"config", "user.email", "test@localhost"},
	} {
		if _, err := gitcli.Run(dir, args...); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(dir+"/README.md", []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"add", "README.md"}, {"commit", "-q", "-m", "first"}} {
		if _, err := gitcli.Run(dir, args...); err != nil {
			t.Fatal(err)
		}
	}
	// Work in progress: the export must leave it as it is.
	if err := os.WriteFile(dir+"/README.md", []byte("hello, edited\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// rootsAre makes the universe's projects the ones given for the test.
func rootsAre(t *testing.T, roots map[string]string) {
	t.Helper()
	was := repositoryRoots
	repositoryRoots = func(context.Context) (map[string]string, error) { return roots, nil }
	t.Cleanup(func() { repositoryRoots = was })
}

func porcelain(t *testing.T, dir string) string {
	t.Helper()
	out, err := gitcli.Run(dir, "status", "--porcelain", "--untracked-files=all")
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// fill puts in st an open to-do written three times, a claimed one with its
// ticket, a done one and a wait.
func fill(t *testing.T, ctx context.Context, st *sqlstore.Store) {
	t.Helper()
	open, err := st.Create(ctx, api.Todo{Module: "billing", Target: "billing.F-001", Request: "billing.F-001", Title: "invoice", Owner: "planification", For: api.ForAgent, State: api.StateReady, Key: "k1", Labels: []string{"increment:IN-1"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, note := range []string{"first", "second"} {
		if open, err = st.Update(ctx, open.ID, func(t *api.Todo) error { t.Note = note; return nil }); err != nil {
			t.Fatal(err)
		}
	}
	held, err := st.Create(ctx, api.Todo{Module: "billing", Target: "billing.F-002", Title: "ship", State: api.StateReady, For: api.ForAgent})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.Claim(ctx, held.ID, claimAs(alice, lease(600))); err != nil {
		t.Fatal(err)
	}
	done, err := st.Create(ctx, api.Todo{Module: "billing", Title: "closed", State: api.StateReady})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.Archive(ctx, done.ID, func(t *api.Todo) error { t.Archived = now(); return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := st.AddWait(ctx, "billing", "ledger.RQ-001", func(id string) api.Wait {
		return api.Wait{ID: id, Module: "billing", On: "ledger.RQ-001", What: "close"}
	}); err != nil {
		t.Fatal(err)
	}
}

// TestAnExportWipedAndImportedGivesBackTheSameRows: a store holding open and
// done to-dos, several revisions, a claim with its ticket and a wait is
// exported to its project's ref - git objects only, the working tree as it
// was - then wiped; the import into an empty store gives back every row and
// revision as they were, and the ticket sequence; an export of the same rows
// writes no commit; an import run again adds nothing.
func TestAnExportWipedAndImportedGivesBackTheSameRows(t *testing.T) {
	ctx := context.Background()
	repoDir := gitRepo(t)
	rootsAre(t, map[string]string{"repo-1": repoDir})
	st := openSQL(t, t.TempDir(), "repo-1")
	fill(t, ctx, st)
	before, err := st.Dump(ctx, "repo-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(before.Todos) != 3 || len(before.Revisions) < 7 || len(before.Waits) != 1 || before.Ticket == 0 {
		t.Fatalf("filled store = %d to-dos, %d revisions, %d waits, ticket %d", len(before.Todos), len(before.Revisions), len(before.Waits), before.Ticket)
	}
	tree := porcelain(t, repoDir)
	head, _ := gitcli.Run(repoDir, "rev-parse", "HEAD")

	out, err := NewOver(st).Export(ctx, api.ExportInput{})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Refs) != 1 || out.Refs[0].Ref != ExportRef("repo-1") || !out.Refs[0].Changed || out.Refs[0].Revisions != len(before.Revisions) {
		t.Fatalf("export = %+v", out)
	}
	if tip, _ := refTip(repoDir, ExportRef("repo-1")); tip != out.Refs[0].Commit {
		t.Fatalf("the ref points at %q, want %s", tip, out.Refs[0].Commit)
	}
	if got := porcelain(t, repoDir); got != tree {
		t.Fatalf("the export changed the working tree or the index:\n%s\nwant\n%s", got, tree)
	}
	if again, _ := gitcli.Run(repoDir, "rev-parse", "HEAD"); again != head {
		t.Fatal("the export moved HEAD")
	}
	if same, err := NewOver(st).Export(ctx, api.ExportInput{}); err != nil || same.Refs[0].Changed || same.Refs[0].Commit != out.Refs[0].Commit {
		t.Fatalf("an export of the same rows = %+v (%v), want no new commit", same, err)
	}

	// The wipe: a new, empty store.
	wiped := openSQL(t, t.TempDir(), "repo-1")
	got, err := NewOver(wiped).Import(ctx, api.ImportInput{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Refs) != 1 || got.Refs[0].Todos != 3 || got.Refs[0].Revisions != len(before.Revisions) || got.Refs[0].Waits != 1 {
		t.Fatalf("import = %+v", got)
	}
	after, err := wiped.Dump(ctx, "repo-1")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(after, before) {
		t.Fatalf("after export, wipe and import:\n%+v\nwant\n%+v", after, before)
	}
	for _, td := range before.Todos {
		a, _ := st.Revisions(ctx, td.ID)
		b, _ := wiped.Revisions(ctx, td.ID)
		if !reflect.DeepEqual(a, b) {
			t.Fatalf("history of %s:\n%+v\nwant\n%+v", td.ID, b, a)
		}
	}
	if again, err := NewOver(wiped).Import(ctx, api.ImportInput{}); err != nil || again.Refs[0].Todos != 0 || again.Refs[0].TodosPresent != 3 || again.Refs[0].Revisions != 0 {
		t.Fatalf("an import run again = %+v (%v), want nothing added", again, err)
	}
	// The ticket sequence came back: a new claim supersedes the imported one.
	var held sqlstore.TodoRow
	for _, r := range before.Todos {
		if r.Ticket != 0 {
			held = r
		}
	}
	if _, err := wiped.Update(ctx, held.ID, func(t *api.Todo) error { t.Claim.Until = passed(); return nil }); err != nil {
		t.Fatal(err)
	}
	next, err := wiped.Claim(ctx, held.ID, claimAs(bob, lease(60)))
	if err != nil || next.Claim.Ticket <= held.Ticket {
		t.Fatalf("a claim after the import = %+v (%v), want a ticket above %d", next.Claim, err, held.Ticket)
	}
}

// TestPruneRemovesADoneToDoOnlyOnceAnExportHoldsIt: a done to-do past its
// module's done_days is pruned on the SQLite store only after its project's
// export holds it - found in the ref; when the export cannot be written it is
// kept for a later prune.
func TestPruneRemovesADoneToDoOnlyOnceAnExportHoldsIt(t *testing.T) {
	ctx, cwm := cwmapp.New(t, cwmapp.Pack(fstest.MapFS{"object-types/module.yaml": {Data: []byte("name: module\ncapabilities: [module]\n")}}))
	cwmapp.Module(t, ctx, cwm, "billing")
	st := openSQL(t, t.TempDir(), "repo-1")
	old, err := st.Create(ctx, api.Todo{Module: "billing", Title: "long done", State: api.StateReady})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.Archive(ctx, old.ID, func(t *api.Todo) error {
		t.Archived = time.Now().UTC().AddDate(0, 0, -400).Format(time.RFC3339Nano)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	svc := NewOver(st)

	// No git repository at the project's folder: the export fails, it is kept.
	rootsAre(t, map[string]string{"repo-1": t.TempDir()})
	p, err := svc.Prune(ctx, api.PruneInput{})
	if err == nil || p.Removed != 0 || p.Kept != 1 {
		t.Fatalf("prune with a failing export = %+v (%v), want it kept and the error said", p, err)
	}
	if _, err := st.Get(ctx, old.ID); err != nil {
		t.Fatalf("kept: %v", err)
	}

	repoDir := gitRepo(t)
	rootsAre(t, map[string]string{"repo-1": repoDir})
	if p, err = svc.Prune(ctx, api.PruneInput{}); err != nil || p.Removed != 1 {
		t.Fatalf("prune = %+v (%v), want it removed", p, err)
	}
	dump, commit, err := readExport(repoDir, ExportRef("repo-1"))
	if err != nil || commit == "" {
		t.Fatalf("the ref after the prune: %q (%v)", commit, err)
	}
	if !slices.ContainsFunc(dump.Todos, func(r sqlstore.TodoRow) bool { return r.ID == old.ID && r.Done }) {
		t.Fatalf("the pruned to-do %s is not in the export: %+v", old.ID, dump.Todos)
	}
}

// TestExportNeedsTheSQLiteStore: on the file store the verbs are refused,
// saying why.
func TestExportNeedsTheSQLiteStore(t *testing.T) {
	if _, err := NewOver(testFileStore(t)).Export(context.Background(), api.ExportInput{}); err == nil {
		t.Fatal("export on the file store: want refused")
	}
}
