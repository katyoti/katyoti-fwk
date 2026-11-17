// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"errors"
	"os/exec"
	"slices"
	"testing"
	"time"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// FINDING TO-DOS — what a worker grepped the to-do files for
// is answered by the verbs: Search across every module's scheduled and done
// to-dos, List's done ones filtered by step, labels and time, and one to-do's
// History - the store's revisions, then its file's commits in git.

func ids(todos []api.Todo) []string {
	out := []string{}
	for _, t := range todos {
		out = append(out, t.ID)
	}
	return out
}

// worked pulls a to-do by id as by and reports it: the test owner collects it
// (archives it to done).
func worked(t *testing.T, ctx context.Context, svc api.TodoService, id string, by cwmapi.Actor, summary string) {
	t.Helper()
	if _, err := svc.Pull(ctx, api.PullInput{By: by, ID: id}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Report(ctx, api.ReportInput{ID: id, Result: api.Result{Status: api.StatusSuccess, Summary: summary, By: by}}); err != nil {
		t.Fatal(err)
	}
}

func TestSearchFindsToDosByWordsRequestTargetAndActor(t *testing.T) {
	ctx, _, svc, _ := world(t)
	a := put(t, ctx, svc, api.Todo{Module: "billing", Title: "implement the invoice export", Request: "billing.RQ-001", Target: "billing.F-001"})
	b := put(t, ctx, svc, api.Todo{Module: "shipping", Title: "review the parcel labels", Request: "shipping.RQ-002", Target: "shipping.F-002"})
	c := put(t, ctx, svc, api.Todo{Module: "shipping", Title: "analyse", Request: "shipping.RQ-003", Target: "shipping.F-003"})
	worked(t, ctx, svc, c.ID, bob, "the carrier quota is the blocker")

	search := func(in api.SearchInput) []string {
		t.Helper()
		out, err := svc.Search(ctx, in)
		if err != nil {
			t.Fatal(err)
		}
		return ids(out.Todos)
	}
	for _, tc := range []struct {
		name string
		in   api.SearchInput
		want []string
	}{
		{"words in a title, any case", api.SearchInput{Text: "INVOICE"}, []string{a.ID}},
		{"words in a done one's result", api.SearchInput{Text: "carrier quota"}, []string{c.ID}},
		{"a request", api.SearchInput{Request: "shipping.RQ-002"}, []string{b.ID}},
		{"a target", api.SearchInput{Target: "billing.F-001"}, []string{a.ID}},
		{"who reported it", api.SearchInput{Actor: bob.ID}, []string{c.ID}},
		{"every filter matches", api.SearchInput{Text: "analyse", Actor: alice.ID}, []string{}},
		{"words no to-do has", api.SearchInput{Text: "nowhere at all"}, []string{}},
		{"one module", api.SearchInput{Text: "e", Module: "billing"}, []string{a.ID}},
	} {
		if got := search(tc.in); !slices.Equal(got, tc.want) {
			t.Errorf("%s: %v; want %v", tc.name, got, tc.want)
		}
	}
	held := put(t, ctx, svc, api.Todo{Module: "billing", Title: "held"})
	if _, err := svc.Pull(ctx, api.PullInput{By: alice, ID: held.ID}); err != nil {
		t.Fatal(err)
	}
	if got := search(api.SearchInput{Actor: alice.ID}); !slices.Equal(got, []string{held.ID}) {
		t.Errorf("who holds it: %v; want %s", got, held.ID)
	}
	if _, err := svc.Search(ctx, api.SearchInput{}); !errors.Is(err, cwmapi.ErrInvalidInput) {
		t.Errorf("a search with no filter: %v; want refused", err)
	}
}

func TestListFiltersDoneToDosByStepLabelsAndTime(t *testing.T) {
	ctx, _, svc, _ := world(t)
	impl := put(t, ctx, svc, api.Todo{Module: "billing", Title: "implement", Request: "billing.RQ-001", Labels: []string{"increment:IN-1", "release:R"}, Work: map[string]any{"step": "implement"}})
	again := put(t, ctx, svc, api.Todo{Module: "billing", Title: "implement again", Request: "billing.RQ-001", Labels: []string{"increment:IN-1"}, Work: map[string]any{"step": "implement (again)"}})
	rev := put(t, ctx, svc, api.Todo{Module: "shipping", Title: "review", Request: "shipping.RQ-002", Labels: []string{"increment:IN-2"}, Work: map[string]any{"step": "review"}})
	open := put(t, ctx, svc, api.Todo{Module: "billing", Title: "still open", Labels: []string{"increment:IN-1"}, Work: map[string]any{"step": "implement"}})
	for _, td := range []api.Todo{impl, again, rev} {
		worked(t, ctx, svc, td.ID, alice, "done")
	}
	list := func(in api.ListInput) []string {
		t.Helper()
		out, err := svc.List(ctx, in)
		if err != nil {
			t.Fatal(err)
		}
		got := ids(out.Todos)
		slices.Sort(got)
		return got
	}
	sorted := func(s ...string) []string { slices.Sort(s); return s }
	for _, tc := range []struct {
		name string
		in   api.ListInput
		want []string
	}{
		{"every done one", api.ListInput{Done: true}, sorted(impl.ID, again.ID, rev.ID)},
		{"a module", api.ListInput{Done: true, Module: "shipping"}, []string{rev.ID}},
		{"a request", api.ListInput{Done: true, Request: "billing.RQ-001"}, sorted(impl.ID, again.ID)},
		{"a step, done again counted", api.ListInput{Done: true, Step: "implement"}, sorted(impl.ID, again.ID)},
		{"labels, every one carried", api.ListInput{Done: true, Labels: []string{"increment:IN-1", "release:R"}}, []string{impl.ID}},
		{"since a past date", api.ListInput{Done: true, Since: "2000-01-01"}, sorted(impl.ID, again.ID, rev.ID)},
		{"since a later time", api.ListInput{Done: true, Since: time.Now().Add(time.Hour).UTC().Format(time.RFC3339)}, []string{}},
		{"the scheduled ones of a step", api.ListInput{Step: "implement"}, []string{open.ID}},
	} {
		if got := list(tc.in); !slices.Equal(got, tc.want) {
			t.Errorf("%s: %v; want %v", tc.name, got, tc.want)
		}
	}
	if _, err := svc.List(ctx, api.ListInput{Done: true, Since: "yesterday"}); !errors.Is(err, cwmapi.ErrInvalidInput) {
		t.Errorf("since yesterday: %v; want refused (RFC 3339 or a date)", err)
	}
}

func TestHistoryGivesTheStoresRevisionsThenTheGitArchive(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	ctx, cwm, svc, _ := world(t)
	space := planning(t, ctx, cwm, "billing")
	td := put(t, ctx, svc, api.Todo{Module: "billing", Title: "implement"})

	// Outside a repository: the store's one revision, no archive.
	h, err := svc.History(ctx, api.HistoryInput{ID: td.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(h.Revisions) != 1 || h.Revisions[0].From != api.FromStore || h.Revisions[0].Todo.State != api.StateReady {
		t.Fatalf("history outside git: %+v; want the store's one revision, ready", h.Revisions)
	}

	gitIn := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", space, "-c", "user.name=probe", "-c", "user.email=probe@example.invalid"}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	gitIn("init", "--quiet")
	gitIn("add", "-A")
	gitIn("commit", "--quiet", "-m", "put")
	worked(t, ctx, svc, td.ID, alice, "implemented")
	gitIn("add", "-A")
	gitIn("commit", "--quiet", "-m", "reported and archived")

	h, err = svc.History(ctx, api.HistoryInput{ID: td.ID})
	if err != nil {
		t.Fatal(err)
	}
	var from, paths, states []string
	for _, r := range h.Revisions {
		from = append(from, r.From)
		paths = append(paths, r.Path)
		states = append(states, r.Todo.State+"/"+r.Todo.Archived)
	}
	if !slices.Equal(from, []string{api.FromStore, api.FromGit, api.FromGit}) {
		t.Fatalf("history sources %v; want the store, then two commits", from)
	}
	if paths[1] != "todo/"+td.ID[len("billing."):]+".yaml" || paths[2] != "done/"+td.ID[len("billing."):]+".yaml" {
		t.Errorf("history paths %v; want todo/ then done/", paths)
	}
	if h.Revisions[1].Todo.State != api.StateReady || h.Revisions[1].Todo.Archived != "" || h.Revisions[2].Todo.Archived == "" || h.Revisions[2].Commit == "" || h.Revisions[2].Author != "probe" {
		t.Errorf("history states %v (%+v); want ready first, archived last, each with its commit and author", states, h.Revisions[2])
	}
	if _, err := svc.History(ctx, api.HistoryInput{ID: "billing.TD-nowhere"}); !errors.Is(err, cwmapi.ErrNotFound) {
		t.Errorf("history of no to-do: %v; want not found", err)
	}
}
