// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/gitcli"
	"github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/internal/sqlstore"
)

// : a clone that fetched a scheduler export newer than its
// store shows it as one item in « À faire »; the import runs only on the
// person's word. All on t.TempDir repositories.

func waitingOf(t *testing.T, ctx context.Context, st *sqlstore.Store) int {
	t.Helper()
	src := fetchedWaitingSource(func(context.Context) (*Service, error) { return NewOver(st), nil })
	items, err := src.Items(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range items {
		if it.Key != FetchedWaitingSourceName+":fetched" {
			t.Fatalf("item %+v, want the fetched export's", it)
		}
		if nodes := src.Controls(ctx, it, "fr"); len(nodes) != 1 || nodes[0]["label"] != "Importer" {
			t.Fatalf("controls = %v, want the one « Importer » button", nodes)
		}
	}
	return len(items)
}

func fetchExports(t *testing.T, dir string) {
	t.Helper()
	if _, err := gitcli.Run(dir, "config", "--add", "remote.origin.fetch", ExportRefSpec); err != nil {
		t.Fatal(err)
	}
	if _, err := gitcli.Run(dir, "fetch", "-q", "origin"); err != nil {
		t.Fatal(err)
	}
}

// A fresh clone fetched another clone's export: one item, nothing imported by
// itself; the import on the person's word brings the to-dos and the item goes.
// The clone that wrote the export sees no item.
func TestAFetchedExportWaitsOnTheImport(t *testing.T) {
	ctx := context.Background()
	bare := bareRemote(t)
	a, b := gitRepo(t), gitRepo(t)
	withRemote(t, a, bare)
	withRemote(t, b, bare)

	rootsAre(t, map[string]string{"repo-1": a})
	sa := openSQL(t, t.TempDir(), "repo-1")
	fill(t, ctx, sa)
	outA, err := NewOver(sa).Export(ctx, api.ExportInput{})
	if err != nil || !outA.Refs[0].Pushed {
		t.Fatalf("export = %+v, %v", outA, err)
	}
	if n := waitingOf(t, ctx, sa); n != 0 {
		t.Fatalf("the clone that wrote the export shows %d item(s), want none", n)
	}

	rootsAre(t, map[string]string{"repo-1": b})
	sb := openSQL(t, t.TempDir(), "repo-1")
	if n := waitingOf(t, ctx, sb); n != 0 {
		t.Fatalf("before the fetch: %d item(s), want none", n)
	}
	fetchExports(t, b)
	if n := waitingOf(t, ctx, sb); n != 1 {
		t.Fatalf("after the fetch: %d item(s), want one", n)
	}
	if held, _ := sb.Repositories(ctx); len(held) != 0 {
		t.Fatalf("the store took %v by itself, want nothing before the import", held)
	}
	in, err := NewOver(sb).Import(ctx, api.ImportInput{})
	if err != nil || len(in.Refs) != 1 || in.Refs[0].Todos == 0 {
		t.Fatalf("import = %+v, %v; want the fetched to-dos", in, err)
	}
	if n := waitingOf(t, ctx, sb); n != 0 {
		t.Fatalf("after the import: %d item(s), want none", n)
	}
}

// An export written over a fetched tip it has not seen (a rejected push
// rewritten over the remote's) keeps that tip: the item stays, and the import
// brings the other clone's to-dos.
func TestAnExportOverAFetchedTipKeepsItForTheImport(t *testing.T) {
	ctx := context.Background()
	bare := bareRemote(t)
	a, b := gitRepo(t), gitRepo(t)
	withRemote(t, a, bare)
	withRemote(t, b, bare)

	rootsAre(t, map[string]string{"repo-1": a})
	sa := openSQL(t, t.TempDir(), "repo-1")
	fill(t, ctx, sa)
	if _, err := NewOver(sa).Export(ctx, api.ExportInput{}); err != nil {
		t.Fatal(err)
	}

	rootsAre(t, map[string]string{"repo-1": b})
	sb := openSQL(t, t.TempDir(), "repo-1")
	if _, err := sb.Create(ctx, api.Todo{Module: "billing", Title: "other", State: api.StateReady, For: api.ForAgent}); err != nil {
		t.Fatal(err)
	}
	outB, err := NewOver(sb).Export(ctx, api.ExportInput{})
	if err != nil || !outB.Refs[0].Pushed {
		t.Fatalf("second clone's export = %+v, %v", outB, err)
	}
	if n := waitingOf(t, ctx, sb); n != 1 {
		t.Fatalf("after writing over the other clone's tip: %d item(s), want one", n)
	}
	in, err := NewOver(sb).Import(ctx, api.ImportInput{})
	if err != nil || len(in.Refs) != 1 || in.Refs[0].Todos == 0 {
		t.Fatalf("import = %+v, %v; want the other clone's to-dos", in, err)
	}
	if n := waitingOf(t, ctx, sb); n != 0 {
		t.Fatalf("after the import: %d item(s), want none", n)
	}
	if kept, _ := refTip(b, fetchedRef("repo-1")); kept != "" {
		t.Fatalf("the kept tip %s outlived the import", kept)
	}
}

// A ref exported before the seen mark existed, by the store that holds its
// rows, shows nothing.
func TestAnExportFromBeforeTheMarkIsSeen(t *testing.T) {
	ctx := context.Background()
	dir := gitRepo(t)
	rootsAre(t, map[string]string{"repo-1": dir})
	st := openSQL(t, t.TempDir(), "repo-1")
	fill(t, ctx, st)
	if _, err := NewOver(st).Export(ctx, api.ExportInput{}); err != nil {
		t.Fatal(err)
	}
	if _, err := gitcli.Run(dir, "update-ref", "-d", seenRef("repo-1")); err != nil {
		t.Fatal(err)
	}
	if n := waitingOf(t, ctx, st); n != 0 {
		t.Fatalf("%d item(s), want none", n)
	}
}
