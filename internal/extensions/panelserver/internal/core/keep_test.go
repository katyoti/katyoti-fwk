// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/panel"
	"github.com/katyoti/katyoti-fwk/internal/core/render"
	panelapi "github.com/katyoti/katyoti-fwk/internal/extensions/panelserver/api"
)

// moduleKeeper stands for CWM: it writes each outcome document in a module
// folder of its own, as the hub's keeper writes it in the object's module.
type moduleKeeper struct {
	dir string

	mu    sync.Mutex
	calls []string // object/panel, one per write
	fail  bool
}

func (k *moduleKeeper) keep(_ context.Context, object string, o panelapi.Outcome) (string, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.fail {
		return "", errors.New("the object's module cannot be written")
	}
	doc, err := OutcomeDocument(o)
	if err != nil {
		return "", err
	}
	path := DocumentPath(o.Snapshot.Panel)
	full := filepath.Join(k.dir, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
		return "", err
	}
	if err := os.WriteFile(full, doc, 0o600); err != nil {
		return "", err
	}
	k.calls = append(k.calls, object+"/"+o.Snapshot.Panel)
	return path, nil
}

func (k *moduleKeeper) written() []string {
	k.mu.Lock()
	defer k.mu.Unlock()
	return append([]string(nil), k.calls...)
}

// moduleFiles lists every file of the module folder.
func moduleFiles(t *testing.T, dir string) []string {
	t.Helper()
	var out []string
	_ = filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			rel, _ := filepath.Rel(dir, p)
			out = append(out, filepath.ToSlash(rel))
		}
		return nil
	})
	return out
}

const (
	keptType = "ktw-test:/panel-type/kept"
	fullType = "ktw-test:/panel-type/full"
	noneType = "ktw-test:/panel-type/plain"
)

func typeKeeps(_ context.Context, typ string) string {
	switch typ {
	case keptType:
		return panel.KeepOutcome
	case fullType:
		return panel.KeepFull
	case noneType:
		return panel.KeepNone
	}
	return ""
}

// A panel type that keeps its outcome leaves, when the panel ends, its final
// state and the person's inputs as a document on the object the run worked
// on; a panel that keeps nothing writes nothing in the
// module, and the panel's log stays in the server's folder.
func TestAKeptPanelLeavesItsOutcomeOnItsObject(t *testing.T) {
	ctx := context.Background()
	setup := func(t *testing.T) (*Server, *moduleKeeper, string, *clock) {
		t.Helper()
		dir := t.TempDir()
		k := &moduleKeeper{dir: t.TempDir()}
		c := &clock{t: time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)}
		s, err := New(Options{Dir: dir, Now: c.now, KeepOf: typeKeeps, Keep: k.keep})
		if err != nil {
			t.Fatal(err)
		}
		return s, k, dir, c
	}
	open := func(p *events, typ, object string) panel.Event {
		return p.next(panel.Event{Kind: panel.KindOpen, Open: &panel.Open{Type: typ, Title: "Publication", Owner: "run-1", Object: object, Answer: render.Answer{Tree: tree()}}})
	}
	// run drives a panel to its end: an ask the person answers, a progress,
	// a complete, then a close.
	run := func(t *testing.T, s *Server, typ, object string) string {
		t.Helper()
		p := &events{t: t, tr: panel.NewTrack("")}
		mustTake(t, s, open(p, typ, object))
		mustTake(t, s, p.next(panel.Event{Kind: panel.KindAsk, Ask: &panel.Ask{Question: "Overwrite the pages?", Actions: []panel.AskAction{{Name: "yes"}, {Name: "no"}}}}))
		if err := s.Input(panel.Input{Panel: p.tr.ID, Kind: panel.InputAction, Name: "yes", By: "jdoe"}); err != nil {
			t.Fatal(err)
		}
		mustTake(t, s, p.next(panel.Event{Kind: panel.KindProgress, Progress: &panel.Progress{Label: "pages", Current: 3, Total: 3}}))
		mustTake(t, s, p.next(panel.Event{Kind: panel.KindComplete, Complete: &panel.Complete{Status: "ok", Result: map[string]any{"published": 3}}}))
		mustTake(t, s, p.next(panel.Event{Kind: panel.KindClose}))
		s.WaitKeeps()
		return p.tr.ID
	}

	t.Run("a kept panel writes one document on its object", func(t *testing.T) {
		s, k, dir, _ := setup(t)
		id := run(t, s, keptType, "wiki.F-012")
		if got := k.written(); len(got) != 1 || got[0] != "wiki.F-012/"+id {
			t.Fatalf("outcomes written %v, want one, on wiki.F-012", got)
		}
		files := moduleFiles(t, k.dir)
		if len(files) != 1 || files[0] != "panels/"+id+".md" {
			t.Fatalf("the module holds %v, want the one document panels/%s.md", files, id)
		}
		doc, _ := os.ReadFile(filepath.Join(k.dir, "panels", id+".md"))
		for _, want := range []string{"panel: " + id, "object: wiki.F-012", "status: ok", "keep: outcome",
			"# Publication", "## Result", `"published": 3`, "## Progress", "pages 3 of 3",
			"## The person's inputs", "action - yes - by jdoe", "## Final view"} {
			if !strings.Contains(string(doc), want) {
				t.Fatalf("the document lacks %q:\n%s", want, doc)
			}
		}
		if strings.Contains(string(doc), "## Log") {
			t.Fatalf("an outcome document carries the whole log:\n%s", doc)
		}
		sn, _ := s.Snapshot(ctx, panelapi.SnapshotInput{Panel: id})
		if sn.Kept != "panels/"+id+".md" || sn.Object != "wiki.F-012" {
			t.Fatalf("snapshot kept %q on %q, want the document on its object", sn.Kept, sn.Object)
		}
		// The log stays in the server's folder, never in the module.
		if _, err := os.Stat(filepath.Join(dir, id+".jsonl")); err != nil {
			t.Fatalf("the panel's log is not in the server's folder: %v", err)
		}
		// Read again after a restart, it is not written twice.
		s2, err := New(Options{Dir: dir, KeepOf: typeKeeps, Keep: k.keep})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s2.Prune(ctx, time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC)); err != nil {
			t.Fatal(err)
		}
		s2.WaitKeeps()
		if got := k.written(); len(got) != 1 {
			t.Fatalf("after a restart the outcome was written again: %v", got)
		}
	})

	t.Run("a panel that keeps nothing writes nothing in the module", func(t *testing.T) {
		s, k, _, _ := setup(t)
		run(t, s, noneType, "wiki.F-012")
		run(t, s, "", "wiki.F-012")
		run(t, s, keptType, "") // kept, but no object to keep it on
		if got := k.written(); len(got) != 0 {
			t.Fatalf("outcomes written %v, want none", got)
		}
		if files := moduleFiles(t, k.dir); len(files) != 0 {
			t.Fatalf("the module holds %v, want nothing", files)
		}
	})

	t.Run("a type that keeps it all leaves its whole log", func(t *testing.T) {
		s, k, _, _ := setup(t)
		id := run(t, s, fullType, "wiki.F-012")
		doc, _ := os.ReadFile(filepath.Join(k.dir, "panels", id+".md"))
		if !strings.Contains(string(doc), "keep: full") || !strings.Contains(string(doc), "## Log") || !strings.Contains(string(doc), `"kind":"ask"`) {
			t.Fatalf("a full keep lacks its log:\n%s", doc)
		}
	})

	t.Run("the person keeps any panel", func(t *testing.T) {
		s, k, _, _ := setup(t)
		ended := run(t, s, noneType, "wiki.F-012")
		out, err := s.Keep(ctx, panelapi.KeepInput{Panel: ended})
		if err != nil || !out.Pending {
			t.Fatalf("keep an ended panel: %+v, %v", out, err)
		}
		s.WaitKeeps()
		if got := k.written(); len(got) != 1 || got[0] != "wiki.F-012/"+ended {
			t.Fatalf("outcomes written %v, want the ended panel's", got)
		}
		out, err = s.Keep(ctx, panelapi.KeepInput{Panel: ended})
		if err != nil || out.Kept != "panels/"+ended+".md" {
			t.Fatalf("keep it again: %+v, %v - want its document", out, err)
		}
		// Asked while it runs, it is written when it ends.
		p := &events{t: t, tr: panel.NewTrack("")}
		mustTake(t, s, open(p, noneType, "wiki.F-013"))
		if out, err := s.Keep(ctx, panelapi.KeepInput{Panel: p.tr.ID}); err != nil || !out.Pending {
			t.Fatalf("keep a running panel: %+v, %v", out, err)
		}
		s.WaitKeeps()
		if got := k.written(); len(got) != 1 {
			t.Fatalf("a running panel's outcome written before it ended: %v", got)
		}
		mustTake(t, s, p.next(panel.Event{Kind: panel.KindComplete, Complete: &panel.Complete{Status: "ok"}}))
		s.WaitKeeps()
		if got := k.written(); len(got) != 2 || got[1] != "wiki.F-013/"+p.tr.ID {
			t.Fatalf("outcomes written %v, want the running panel's once it ended", got)
		}
		// No object: nowhere to keep it.
		q := &events{t: t, tr: panel.NewTrack("")}
		mustTake(t, s, open(q, noneType, ""))
		if _, err := s.Keep(ctx, panelapi.KeepInput{Panel: q.tr.ID}); err == nil {
			t.Fatal("keeping a panel that names no object is taken")
		}
	})

	t.Run("an outcome its object could not take is tried again", func(t *testing.T) {
		s, k, _, c := setup(t)
		k.fail = true
		id := run(t, s, keptType, "wiki.F-012")
		if got := k.written(); len(got) != 0 {
			t.Fatalf("written %v while the module could not be", got)
		}
		k.mu.Lock()
		k.fail = false
		k.mu.Unlock()
		c.add(time.Hour)
		if _, err := s.Prune(ctx, c.now()); err != nil {
			t.Fatal(err)
		}
		s.WaitKeeps()
		if got := k.written(); len(got) != 1 || got[0] != "wiki.F-012/"+id {
			t.Fatalf("after the next prune written %v, want the outcome", got)
		}
	})
}
