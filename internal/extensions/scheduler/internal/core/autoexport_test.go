// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/gitcli"
	"github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/internal/sqlstore"
)

// : the export runs by itself - every 5 minutes or 200
// changes, and at the agents' pause - and its tree is sharded, one file per
// to-do, so an export writes only what changed.

// TestAnExportOfOneChangedToDoWritesOnlyItsFile: the tree holds store.json,
// one file per to-do under todos/<module>/<month>/ and the module's waits;
// after one to-do changes, the next export's commit differs from the last one
// by that to-do's file alone, and the import of the sharded tree into an
// empty store gives back the same rows.
func TestAnExportOfOneChangedToDoWritesOnlyItsFile(t *testing.T) {
	ctx := context.Background()
	repoDir := gitRepo(t)
	rootsAre(t, map[string]string{"repo-1": repoDir})
	st := openSQL(t, t.TempDir(), "repo-1")
	fill(t, ctx, st)
	first, err := NewOver(st).Export(ctx, api.ExportInput{})
	if err != nil {
		t.Fatal(err)
	}
	c1 := first.Refs[0].Commit
	ls, err := gitcli.Run(repoDir, "ls-tree", "-r", "--name-only", c1)
	if err != nil {
		t.Fatal(err)
	}
	paths := gitcli.Lines(ls)
	todos := 0
	for _, p := range paths {
		if strings.HasPrefix(p, "todos/billing/") && strings.HasSuffix(p, ".json") {
			todos++
		}
	}
	if todos != 3 || !slices.Contains(paths, "store.json") || !slices.Contains(paths, "waits/billing.jsonl") {
		t.Fatalf("the export's tree = %v, want store.json, three to-do files and waits/billing.jsonl", paths)
	}

	before, _ := st.Dump(ctx, "repo-1")
	changed := before.Todos[0]
	if _, err := st.Update(ctx, changed.ID, func(t *api.Todo) error { t.Note = "moved"; return nil }); err != nil {
		t.Fatal(err)
	}
	second, err := NewOver(st).Export(ctx, api.ExportInput{})
	if err != nil || !second.Refs[0].Changed {
		t.Fatalf("export after a change = %+v (%v)", second, err)
	}
	diff, err := gitcli.Run(repoDir, "diff-tree", "-r", "--name-only", c1, second.Refs[0].Commit)
	if err != nil {
		t.Fatal(err)
	}
	if got := gitcli.Lines(diff); !reflect.DeepEqual(got, []string{todoPath(changed)}) {
		t.Fatalf("the second export changed %v, want only %s", got, todoPath(changed))
	}

	after, _ := st.Dump(ctx, "repo-1")
	wiped := openSQL(t, t.TempDir(), "repo-1")
	if _, err := NewOver(wiped).Import(ctx, api.ImportInput{}); err != nil {
		t.Fatal(err)
	}
	if got, _ := wiped.Dump(ctx, "repo-1"); !reflect.DeepEqual(got, after) {
		t.Fatalf("the sharded export imported:\n%+v\nwant\n%+v", got, after)
	}
}

// TestAnExportWrittenBeforeTheShardingIsStillImported: a ref whose tree holds
// the three whole files of an older export reads back into the same rows.
func TestAnExportWrittenBeforeTheShardingIsStillImported(t *testing.T) {
	ctx := context.Background()
	repoDir := gitRepo(t)
	rootsAre(t, map[string]string{"repo-1": repoDir})
	st := openSQL(t, t.TempDir(), "repo-1")
	fill(t, ctx, st)
	d, err := st.Dump(ctx, "repo-1")
	if err != nil {
		t.Fatal(err)
	}
	head, _ := encodeJSON(d)
	todos, _ := jsonLines(d.Todos)
	revs, _ := jsonLines(d.Revisions)
	waits, _ := jsonLines(d.Waits)
	var tree strings.Builder
	for _, f := range []struct {
		name string
		data []byte
	}{{exportRevisionsFile, revs}, {exportStoreFile, head}, {exportTodosFile, todos}, {exportWaitsFile, waits}} {
		blob, err := gitcli.RunInput(repoDir, f.data, "hash-object", "-w", "--stdin")
		if err != nil {
			t.Fatal(err)
		}
		tree.WriteString("100644 blob " + strings.TrimSpace(blob) + "\t" + f.name + "\n")
	}
	treeID, err := gitcli.RunInput(repoDir, []byte(tree.String()), "mktree")
	if err != nil {
		t.Fatal(err)
	}
	commit, err := gitcli.Run(repoDir, append(append([]string{}, katyotiCommitter...), "commit-tree", strings.TrimSpace(treeID), "-m", "old export")...)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := gitcli.Run(repoDir, "update-ref", ExportRef("repo-1"), strings.TrimSpace(commit)); err != nil {
		t.Fatal(err)
	}
	wiped := openSQL(t, t.TempDir(), "repo-1")
	if _, err := NewOver(wiped).Import(ctx, api.ImportInput{}); err != nil {
		t.Fatal(err)
	}
	if got, _ := wiped.Dump(ctx, "repo-1"); !reflect.DeepEqual(got, d) {
		t.Fatalf("the older export imported:\n%+v\nwant\n%+v", got, d)
	}
	// The next export over it writes the sharded tree.
	out, err := NewOver(wiped).Export(ctx, api.ExportInput{})
	if err != nil || !out.Refs[0].Changed {
		t.Fatalf("an export over the older one = %+v (%v), want a new commit", out, err)
	}
}

// TestTheExportIsDueOnChangesIntervalAndPause: the loop exports at its start,
// then only when the store moved - 200 changes at once, the agents' pause
// (claimed to-dos, then none) or 5 minutes since the last export - and never
// when nothing changed.
func TestTheExportIsDueOnChangesIntervalAndPause(t *testing.T) {
	t0 := time.Date(2026, 11, 17, 8, 0, 0, 0, time.UTC)
	base := sqlstore.Mark{Todos: 10, Revs: 40, Waits: 2}
	var a autoExport
	if why := a.due(base, t0); why != "start" {
		t.Fatalf("first look = %q, want start", why)
	}
	a.done(base, t0)
	if why := a.due(base, t0.Add(time.Hour)); why != "" {
		t.Fatalf("nothing changed = %q, want none", why)
	}
	few := base
	few.Revs += 199
	if why := a.due(few, t0.Add(time.Minute)); why != "" {
		t.Fatalf("199 changes within 5 minutes = %q, want none", why)
	}
	many := few
	many.Revs++
	if why := a.due(many, t0.Add(time.Minute)); why != "changes" {
		t.Fatalf("200 changes = %q, want changes", why)
	}
	one := base
	one.Revs++
	one.Claimed = 1
	if why := a.due(one, t0.Add(time.Minute)); why != "" {
		t.Fatalf("one change, a to-do claimed = %q, want none", why)
	}
	one.Claimed = 0
	if why := a.due(one, t0.Add(2*time.Minute)); why != "agents' pause" {
		t.Fatalf("the last claimed to-do reported = %q, want agents' pause", why)
	}
	if why := a.due(one, t0.Add(4*time.Minute)); why != "" {
		t.Fatalf("one change, 4 minutes = %q, want none", why)
	}
	if why := a.due(one, t0.Add(5*time.Minute)); why != "interval" {
		t.Fatalf("one change, 5 minutes = %q, want interval", why)
	}
}

// TestTheStoreMarkCountsChangesAndLiveClaims: the mark moves with a write and
// counts a claimed to-do while its lease holds.
func TestTheStoreMarkCountsChangesAndLiveClaims(t *testing.T) {
	ctx := context.Background()
	st := openSQL(t, t.TempDir(), "repo-1")
	m0, err := st.Mark(ctx)
	if err != nil {
		t.Fatal(err)
	}
	td, err := st.Create(ctx, api.Todo{Module: "billing", Title: "ship", State: api.StateReady, For: api.ForAgent})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.Claim(ctx, td.ID, claimAs(alice, lease(600))); err != nil {
		t.Fatal(err)
	}
	m1, err := st.Mark(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if m1.Changes(m0) < 2 || m1.Claimed != 1 {
		t.Fatalf("mark after a create and a claim = %+v (changes %d), want 2+ changes and one claimed", m1, m1.Changes(m0))
	}
}
