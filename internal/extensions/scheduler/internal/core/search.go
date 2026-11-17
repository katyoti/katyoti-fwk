// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/gitcli"
	"github.com/katyoti/katyoti-fwk/internal/core/marshal"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// FINDING TO-DOS — what a worker grepped the to-do files for, through the
// verbs: List's filters (a step, labels, a time), Search
// across every module's scheduled and done to-dos, and History - the revisions
// the store keeps, then the commits of the to-do's file in the git archive.

// kept says whether t passes List's filters.
func kept(t api.Todo, in api.ListInput, since time.Time) bool {
	switch {
	case in.Request != "" && t.Request != in.Request,
		in.State != "" && t.State != in.State,
		in.Step != "" && stepOf(t) != in.Step,
		!carries(t, in.Labels),
		!since.IsZero() && lastAt(t).Before(since):
		return false
	}
	return true
}

// again is the mark of a step done again: "implement (again)".
var again = regexp.MustCompile(`\s*\(.*\)$`)

// stepOf is the owner's step of a to-do (work.step), a step done again counted
// as its step.
func stepOf(t api.Todo) string {
	s, _ := t.Work["step"].(string)
	return again.ReplaceAllString(s, "")
}

// carries says whether t carries every label.
func carries(t api.Todo, labels []string) bool {
	for _, l := range labels {
		if l = strings.TrimSpace(l); l == "" {
			continue
		}
		found := false
		for _, have := range t.Labels {
			if have == l {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// lastAt is a to-do's last moment: archived, else reported, else claimed,
// else made; the zero time when it names none.
func lastAt(t api.Todo) time.Time {
	at := t.Archived
	if at == "" && t.Result != nil {
		at = t.Result.At
	}
	if at == "" && t.Claim != nil {
		at = t.Claim.At
	}
	if at == "" {
		at = t.Made
	}
	got, _ := time.Parse(time.RFC3339Nano, at)
	return got
}

// sinceOf reads List's since: RFC 3339 or a date (its midnight UTC).
func sinceOf(s string) (time.Time, error) {
	if s = strings.TrimSpace(s); s == "" {
		return time.Time{}, nil
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t, nil
	}
	if t, err := time.Parse(time.DateOnly, s); err == nil {
		return t, nil
	}
	return time.Time{}, invalid("since %q is neither an RFC 3339 time nor a date YYYY-MM-DD", s)
}

// Search finds the to-dos of every module (or one), scheduled then done, that
// match every filter given.
func (s *Service) Search(ctx context.Context, in api.SearchInput) (api.TodosOutput, error) {
	text := strings.ToLower(strings.TrimSpace(in.Text))
	if text == "" && in.Request == "" && in.Target == "" && in.Actor == "" {
		return api.TodosOutput{}, invalid("search needs words (text) or a request or a target or an actor")
	}
	match := func(t api.Todo) bool {
		switch {
		case in.Request != "" && t.Request != in.Request,
			in.Target != "" && t.Target != in.Target,
			in.Actor != "" && !actedBy(t, in.Actor),
			text != "" && !strings.Contains(searchable(t), text):
			return false
		}
		return true
	}
	out := api.TodosOutput{Todos: []api.Todo{}}
	for _, done := range []bool{false, true} {
		all, _, err := s.scheduled(ctx, in.Module, done)
		if err != nil {
			return api.TodosOutput{}, err
		}
		var got []api.Todo
		for _, t := range all {
			if match(t) {
				got = append(got, t)
			}
		}
		if done {
			// The done ones, the most recently archived first.
			sort.SliceStable(got, func(i, j int) bool { return got[i].Archived > got[j].Archived })
		} else {
			sort.SliceStable(got, func(i, j int) bool { return before(got[i], got[j], "") })
		}
		out.Todos = append(out.Todos, got...)
	}
	return out, nil
}

// actedBy says whether actor holds or held t, reported it or was given it.
func actedBy(t api.Todo, actor string) bool {
	return (t.Claim != nil && t.Claim.By.ID == actor) ||
		(t.Result != nil && t.Result.By.ID == actor) ||
		(t.Assignee != nil && t.Assignee.ID == actor)
}

// searchable is the text words are found in, lower case.
func searchable(t api.Todo) string {
	parts := []string{t.ID, t.Title, t.Note}
	if t.Result != nil {
		parts = append(parts, t.Result.Summary, t.Result.Details)
	}
	return strings.ToLower(strings.Join(parts, "\n"))
}

// History reads a to-do's revisions: the store's, oldest first, then the
// commits of its file (todo/ then done/ of its module's planning space) in the
// git archive, oldest first. A to-do neither keeps is not found; a planning
// space outside a git repository has no archive.
func (s *Service) History(ctx context.Context, in api.HistoryInput) (api.HistoryOutput, error) {
	if in.ID == "" {
		return api.HistoryOutput{}, invalid("history needs the to-do's id")
	}
	out := api.HistoryOutput{ID: in.ID, Revisions: []api.Revision{}}
	revs, err := s.store.Revisions(ctx, in.ID)
	if err != nil && !errors.Is(err, cwmapi.ErrNotFound) {
		return api.HistoryOutput{}, err
	}
	for _, t := range revs {
		out.Revisions = append(out.Revisions, api.Revision{From: api.FromStore, Revision: t.Revision, Todo: t})
	}
	archived, err := gitArchive(ctx, in.ID)
	if err != nil {
		return api.HistoryOutput{}, err
	}
	out.Revisions = append(out.Revisions, archived...)
	if len(out.Revisions) == 0 {
		return api.HistoryOutput{}, fmt.Errorf("%w: to-do %s: neither its store nor the git archive keeps it", cwmapi.ErrNotFound, in.ID)
	}
	return out, nil
}

// gitArchive reads the commits of a to-do's file in the repository of its
// module's planning space, oldest first: each commit's version of the file
// (todo/ while scheduled, done/ once archived). None when git is not there or
// the space is in no repository.
func gitArchive(ctx context.Context, id string) ([]api.Revision, error) {
	module, err := moduleOfID(id)
	if err != nil {
		return nil, err
	}
	space, err := planningSpace(ctx, module)
	if err != nil {
		return nil, err
	}
	if _, err := exec.LookPath("git"); err != nil {
		return nil, nil
	}
	if top, err := gitcli.Run(space, "rev-parse", "--is-inside-work-tree"); err != nil || strings.TrimSpace(top) != "true" {
		return nil, nil
	}
	paths := []string{filepath.ToSlash(filepath.Join(todoDir, fileOf(id))), filepath.ToSlash(filepath.Join(doneDir, fileOf(id)))}
	log, err := gitcli.Run(space, append([]string{"log", "--reverse", "--format=%H%x1f%aI%x1f%an", "--"}, paths...)...)
	if err != nil {
		return nil, nil // a space no commit names yet
	}
	var out []api.Revision
	for _, line := range gitcli.Lines(log) {
		f := strings.Split(line, "\x1f")
		if len(f) != 3 {
			continue
		}
		for _, p := range paths {
			body, err := gitcli.Run(space, "show", f[0]+":./"+p)
			if err != nil {
				continue // not in this commit: moved, or removed
			}
			var t api.Todo // data, as the store reads it: no ${…} expansion
			if err := marshal.Unmarshal([]byte(body), marshal.YAML, &t); err != nil {
				return nil, fmt.Errorf("to-do %s at %s (%s): %w", id, f[0], p, err)
			}
			out = append(out, api.Revision{From: api.FromGit, Revision: t.Revision, Commit: f[0], At: f[1], Author: f[2], Path: p, Todo: t})
			break
		}
	}
	return out, nil
}
