// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/atomicio"
	actionsapi "github.com/katyoti/katyoti-fwk/internal/extensions/actions/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/internal/storage"
)

// A REACTION'S OUTBOX (refined by the person 2026-10-07). A
// write whose cause is a reaction (cause kind `reaction`, its id the
// reaction's) hands nothing on: what it can TRIGGER — the async subscriptions
// it matches and the on_* hook calls it owes — is staged under the reaction,
// in CWM's uncommitted state (`<.katyoti>/cwm/outbox/<reaction>/staged.json`,
// the folder named by a hash of the reaction's id), coalesced per object (a
// match per subscription and key, a hook call per point, uri and object, the
// latest input winning). Its object event is NOT staged: it goes out at once,
// as every write's does (the journal, the GUI).
//
// The reaction's collector decides: OutboxRelease records the staged matches
// (handed over) and journals the staged hook calls (run) when the reaction
// succeeded; OutboxDrop forgets them when it is given up, listing the writes.
// Over failed attempts nothing happens: the outbox is held and accumulates, so
// a rerun that finds the state already right still delivers what a failed
// attempt changed.

// outboxEntry is one reaction's staged triggers.
type outboxEntry struct {
	Reaction string             `json:"reaction"`
	At       time.Time          `json:"at"`
	Writes   []api.MatchedEvent `json:"writes"`
	Matches  []api.Match        `json:"matches,omitempty"`
	Hooks    []hookJournalEntry `json:"hooks,omitempty"`
}

// outboxStore is every reaction's outbox, on disk and in memory. dir "" =
// memory only.
type outboxStore struct {
	dir     string
	mu      sync.Mutex
	entries map[string]*outboxEntry
}

// outbox is the service's outbox store, loaded from disk on first use.
func (s *Service) outbox() *outboxStore {
	s.outboxOnce.Do(func() {
		o := &outboxStore{entries: map[string]*outboxEntry{}}
		if s.Config != nil && s.Config.WorkspaceRoot != "" {
			o.dir = filepath.Join(storage.NewPaths(s.Config.WorkspaceRoot).CWMDataDir(), "outbox")
			o.load()
		}
		s.outboxStore = o
	})
	return s.outboxStore
}

const outboxFile = "staged.json"

func (o *outboxStore) folder(reaction string) string {
	return filepath.Join(o.dir, stateFileName(reaction))
}

func (o *outboxStore) load() {
	ents, err := os.ReadDir(o.dir)
	if err != nil {
		return
	}
	for _, de := range ents {
		if !de.IsDir() {
			continue
		}
		p := filepath.Join(o.dir, de.Name(), outboxFile)
		raw, err := atomicio.ReadAtomic(p)
		var e outboxEntry
		if err == nil {
			err = json.Unmarshal(raw, &e)
		}
		if err != nil || e.Reaction == "" {
			slog.Warn("cwm: unreadable outbox", "path", p, "err", err)
			continue
		}
		o.entries[e.Reaction] = &e
	}
}

// persist writes e to disk. Caller holds o.mu.
func (o *outboxStore) persist(e *outboxEntry) {
	if o.dir == "" {
		return
	}
	dir := o.folder(e.Reaction)
	raw, err := json.MarshalIndent(e, "", "  ")
	if err == nil {
		err = os.MkdirAll(dir, 0o755)
	}
	if err == nil {
		err = atomicio.WriteAtomic(filepath.Join(dir, outboxFile), raw, 0o644)
	}
	if err != nil {
		slog.Warn("cwm: outbox write", "reaction", e.Reaction, "err", err)
	}
}

// stage adds one write of a reaction: its event, its matches, its hook calls.
func (o *outboxStore) stage(reaction string, ev api.MatchedEvent, matches []api.Match, hooks []hookJournalEntry) {
	o.mu.Lock()
	defer o.mu.Unlock()
	e, ok := o.entries[reaction]
	if !ok {
		e = &outboxEntry{Reaction: reaction, At: ev.At}
		o.entries[reaction] = e
	}
	e.Writes = append(e.Writes, ev)
	for _, m := range matches {
		merged := false
		for i := range e.Matches {
			if sameReactionSlot(e.Matches[i], m) {
				e.Matches[i].Events = append(e.Matches[i].Events, m.Events...)
				merged = true
				break
			}
		}
		if !merged {
			e.Matches = append(e.Matches, m)
		}
	}
	for _, h := range hooks {
		merged := false
		for i := range e.Hooks {
			if prev := &e.Hooks[i]; prev.Point == h.Point && prev.URI == h.URI && prev.ObjectID == h.ObjectID {
				// One call per point, action and object: the latest input.
				prev.Input, prev.State, prev.Error = h.Input, h.State, h.Error
				merged = true
				break
			}
		}
		if !merged {
			e.Hooks = append(e.Hooks, h)
		}
	}
	o.persist(e)
}

// get is a copy of a reaction's outbox.
func (o *outboxStore) get(reaction string) (outboxEntry, bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	e, ok := o.entries[reaction]
	if !ok {
		return outboxEntry{}, false
	}
	return *e, true
}

// remove forgets a reaction's outbox.
func (o *outboxStore) remove(reaction string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	delete(o.entries, reaction)
	if o.dir != "" {
		_ = os.RemoveAll(o.folder(reaction))
	}
}

func (o *outboxStore) list() []api.OutboxSummary {
	o.mu.Lock()
	out := make([]api.OutboxSummary, 0, len(o.entries))
	for _, e := range o.entries {
		out = append(out, api.OutboxSummary{Reaction: e.Reaction, At: e.At,
			Writes: len(e.Writes), Matches: len(e.Matches), Hooks: len(e.Hooks)})
	}
	o.mu.Unlock()
	sort.Slice(out, func(a, b int) bool {
		if !out[a].At.Equal(out[b].At) {
			return out[a].At.Before(out[b].At)
		}
		return out[a].Reaction < out[b].Reaction
	})
	return out
}

// OutboxRelease hands on what a reaction's writes staged: each match recorded
// (coalesced with the pending ones) and handed over, each hook call journaled
// and run. Both are written before the outbox is removed: a stop in between
// hands something on twice, never loses it. An unknown reaction releases
// nothing.
func (s *Service) OutboxRelease(ctx stdctx.Context, in api.OutboxReleaseInput) (api.OutboxReleaseOutput, error) {
	if in.Reaction == "" {
		return api.OutboxReleaseOutput{}, fmt.Errorf("%w: OutboxRelease: reaction required", api.ErrInvalidInput)
	}
	if s.changeSetHeld(in.Reaction) {
		// An atomic reaction's writes are not written until its set is
		// committed: what they trigger is not released before (changeset.go).
		return api.OutboxReleaseOutput{}, fmt.Errorf("%w: OutboxRelease: the change set of %s is not committed — ChangeSetCommit first, or ChangeSetDiscard", api.ErrInvalidInput, in.Reaction)
	}
	e, ok := s.outbox().get(in.Reaction)
	if !ok {
		return api.OutboxReleaseOutput{}, nil
	}
	ids := make([]string, 0, len(e.Matches))
	for _, m := range e.Matches {
		ids = append(ids, s.matches().record(m))
	}
	j := s.hooks()
	for i := range e.Hooks {
		j.put(&e.Hooks[i])
	}
	s.outbox().remove(in.Reaction)
	detached := stdctx.WithoutCancel(ctx)
	for _, id := range ids {
		s.handOver(detached, id)
	}
	if actions, err := application.UseService[actionsapi.ActionService](ctx); err == nil && actions != nil {
		for _, h := range e.Hooks {
			if h.State == hookPending {
				s.dispatchHook(detached, actions, h)
			}
		}
	} else if len(e.Hooks) > 0 {
		slog.Warn("cwm: released hook calls kept for the next start — no action service", "reaction", in.Reaction, "hooks", len(e.Hooks))
	}
	return api.OutboxReleaseOutput{Matches: len(e.Matches), Hooks: len(e.Hooks)}, nil
}

// OutboxDrop forgets a given-up reaction's outbox and lists the writes it
// made: they landed, and nothing they would have triggered will run. Its open
// change set is dropped too: what it staged is listed as never written.
func (s *Service) OutboxDrop(_ stdctx.Context, in api.OutboxDropInput) (api.OutboxDropOutput, error) {
	if in.Reaction == "" {
		return api.OutboxDropOutput{}, fmt.Errorf("%w: OutboxDrop: reaction required", api.ErrInvalidInput)
	}
	var notWritten []api.ChangeSetWrite
	if held := s.dropChangeSet(s.changeSets(), in.Reaction); len(held) > 0 {
		notWritten = summaries(held)
	}
	e, ok := s.outbox().get(in.Reaction)
	if !ok {
		return api.OutboxDropOutput{Writes: []api.MatchedEvent{}, NotWritten: notWritten}, nil
	}
	s.outbox().remove(in.Reaction)
	if e.Writes == nil {
		e.Writes = []api.MatchedEvent{}
	}
	return api.OutboxDropOutput{Writes: e.Writes, NotWritten: notWritten}, nil
}

// OutboxList lists the outboxes held, oldest first.
func (s *Service) OutboxList(_ stdctx.Context, _ api.OutboxListInput) (api.OutboxListOutput, error) {
	return api.OutboxListOutput{Outboxes: s.outbox().list()}, nil
}
