// SPDX-License-Identifier: Apache-2.0

package core

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/marshal"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/journal/api"
)

// Transitions is one object's lifecycle history, oldest first: the journal's
// `transitioned` records about it (followed through its moves and splits,
// like History), for the lifecycle asked — the object's own, or a
// capability's. The object stores only its state.
//
// Past the journal's retention the records are pruned; when the window is
// full (its oldest day is as old as the retention), the object's own
// lifecycle before that day is read, best effort, from git: `git log
// --follow` on the object's file, each commit's state read from the file at
// that commit, a change of state being one move (at the commit's time, by its
// author, the commit's subject as the note). A capability's state is not read
// from git, and nothing is when the file is not under git.
func (s *Service) Transitions(ctx context.Context, in api.TransitionsInput) (api.TransitionsOutput, error) {
	if in.ID == "" {
		return api.TransitionsOutput{}, fmt.Errorf("%w: id required", cwmapi.ErrInvalidInput)
	}
	limit := in.Limit
	if limit <= 0 {
		limit = api.DefaultLimit
	}
	capability := in.Capability
	if capability == cwmapi.CapLifecycle {
		capability = ""
	}
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.TransitionsOutput{}, err
	}
	key, id := scopeOf(ctx, cwm, in.ID)
	follow := followed(id)
	got, err := s.query(ctx, key, api.QueryInput{Limit: limit}, func(r api.Record) bool {
		// Every record goes through follow (a move names the older id);
		// only the moves of this lifecycle are kept.
		return follow(r) && r.Action == api.ActionTransitioned && r.Capability == capability
	})
	if err != nil {
		return api.TransitionsOutput{}, err
	}
	out := api.TransitionsOutput{Entries: make([]cwmapi.LifecycleEntry, 0, len(got.Records)), Truncated: got.Truncated}
	for i := len(got.Records) - 1; i >= 0; i-- { // newest first → oldest first
		out.Entries = append(out.Entries, entryOf(got.Records[i]))
	}
	if capability != "" || got.Truncated {
		return out, nil
	}
	if before, full := s.windowFull(ctx, key); full {
		older := s.gitTransitions(ctx, qualify(key, id), before)
		if room := limit - len(out.Entries); len(older) > room {
			older, out.Truncated = older[len(older)-room:], true
		}
		out.Entries = append(older, out.Entries...)
	}
	return out, nil
}

// entryOf is a transitioned record as a lifecycle history entry.
func entryOf(r api.Record) cwmapi.LifecycleEntry {
	e := cwmapi.LifecycleEntry{
		From: r.FromState, To: r.ToState, Transition: r.Transition,
		At: r.At.UTC().Format(time.RFC3339), Note: r.Note, Forced: r.Forced, Calls: r.Calls,
	}
	if r.By != nil {
		e.By = *r.By
	}
	return e
}

// windowFull answers the oldest kept day of the journal of the project of key
// ("" = every project's), and whether the retention may have pruned older
// ones.
func (s *Service) windowFull(ctx context.Context, key string) (time.Time, bool) {
	cwm, err := cwmOf(ctx)
	if err != nil {
		return time.Time{}, false
	}
	days, err := listDays(ctx, cwm, key)
	if err != nil || len(days) == 0 {
		return time.Time{}, false
	}
	oldest := days[len(days)-1].date
	at, err := time.Parse("2006-01-02", oldest)
	if err != nil {
		return time.Time{}, false
	}
	cutoff := s.now().UTC().AddDate(0, 0, -s.retention).Format("2006-01-02")
	return at, oldest <= cutoff
}

// gitCommitLimit bounds how many commits of one file the fallback reads.
const gitCommitLimit = 200

// gitTransitions reads, best effort, the object's own lifecycle moves before
// `before` from git's log of its file. Any failure answers none.
func (s *Service) gitTransitions(ctx context.Context, id string, before time.Time) []cwmapi.LifecycleEntry {
	root, rel, ok := objectFile(ctx, id)
	if !ok {
		return nil
	}
	logOut, err := gitRun(ctx, root, "log", "--follow", "--name-only", "-n", fmt.Sprint(gitCommitLimit),
		"--before="+before.UTC().Format(time.RFC3339), "--format=%x1e%H%x1f%aI%x1f%an%x1f%s", "--", rel)
	if err != nil {
		return nil
	}
	type commit struct{ hash, at, author, subject, path string }
	var commits []commit // newest first
	for _, chunk := range strings.Split(string(logOut), "\x1e") {
		lines := strings.Split(strings.TrimSpace(chunk), "\n")
		head := strings.Split(lines[0], "\x1f")
		if len(head) != 4 || len(lines) < 2 {
			continue
		}
		path := strings.TrimSpace(lines[len(lines)-1])
		commits = append(commits, commit{hash: head[0], at: head[1], author: head[2], subject: head[3], path: path})
	}
	var out []cwmapi.LifecycleEntry
	prev := ""
	for i := len(commits) - 1; i >= 0; i-- { // oldest first
		c := commits[i]
		body, err := gitRun(ctx, root, "show", c.hash+":"+c.path)
		if err != nil {
			continue
		}
		state := lifecycleStateIn(body)
		if state == "" || state == prev {
			continue
		}
		at := c.at
		if t, err := time.Parse(time.RFC3339, c.at); err == nil {
			at = t.UTC().Format(time.RFC3339)
		}
		out = append(out, cwmapi.LifecycleEntry{From: prev, To: state, By: cwmapi.Actor{ID: c.author, Kind: cwmapi.ActorPerson}, At: at, Note: c.subject})
		prev = state
	}
	return out
}

// objectFile is the object's file as git names it: the project root and the
// file's path under it (slashed). ok is false when CWM cannot resolve it.
func objectFile(ctx context.Context, id string) (root, rel string, ok bool) {
	cwm, err := cwmOf(ctx)
	if err != nil {
		return "", "", false
	}
	res, err := cwm.ObjectResolve(ctx, cwmapi.ObjectResolveInput{Ref: id})
	if err != nil || res.Path == "" || res.ProjectRoot == "" {
		return "", "", false
	}
	r, err := filepath.Rel(res.ProjectRoot, res.Path)
	if err != nil {
		return "", "", false
	}
	return res.ProjectRoot, filepath.ToSlash(r), true
}

func gitRun(ctx context.Context, dir string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...).Output()
}

// lifecycleStateIn is the lifecycle state an object file stores: a YAML
// object file, or a document's frontmatter. "" when it stores none.
func lifecycleStateIn(body []byte) string {
	if fm, ok := frontmatterOf(body); ok {
		body = fm
	}
	var file struct {
		Capabilities map[string]map[string]any `yaml:"capabilities"`
	}
	if err := config.DecodeBytes(body, marshal.YAML, &file); err != nil {
		return ""
	}
	st, _ := file.Capabilities[cwmapi.CapLifecycle]["state"].(string)
	return st
}

// frontmatterOf is the frontmatter of a document file (`---` … `---`).
func frontmatterOf(body []byte) ([]byte, bool) {
	sc := bufio.NewScanner(bytes.NewReader(body))
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	if !sc.Scan() || strings.TrimSpace(sc.Text()) != "---" {
		return nil, false
	}
	var fm bytes.Buffer
	for sc.Scan() {
		if strings.TrimSpace(sc.Text()) == "---" {
			return fm.Bytes(), true
		}
		fm.WriteString(sc.Text())
		fm.WriteByte('\n')
	}
	return nil, false
}
