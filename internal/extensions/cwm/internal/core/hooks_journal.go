// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/atomicio"
	"github.com/katyoti/katyoti-fwk/internal/core/cause"
	actionsapi "github.com/katyoti/katyoti-fwk/internal/extensions/actions/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/internal/storage"
)

// AN on_* HOOK IS NEVER SILENTLY LOST. The write is done when an on_* hook
// runs, so CWM owes its consequence: every call is JOURNALED before it is
// dispatched, in CWM's uncommitted state (`<.katyoti>/cwm/hooks/`, one JSON
// file per call — never the corpus), and leaves the journal only when its
// action answered.
//
//   - The process stops before the action ran (or no action service could run
//     it): the call stays `pending`; the next start replays it
//     (RunBootstrapHooks, once the service is reachable).
//   - The action fails: it is retried with a doubling backoff up to the
//     policy's attempts; then the call turns `failed`, stays in the journal,
//     and the object carries a `hook-failed` error (problemsOf) — red in the
//     tree, in its pane, in the Errors list. A failed call is not retried on
//     its own: the next success of the same hook on the same object clears it
//     (or removing its journal file).
//
// The window this cannot cover is between the object's rename and the
// journal's write — the journal is written right after the write, before the
// verb returns.

// hookRetryPolicy bounds how a failing on_* action is retried.
type hookRetryPolicy struct {
	Attempts   int
	Backoff    time.Duration
	MaxBackoff time.Duration
}

var defaultHookRetry = hookRetryPolicy{Attempts: 5, Backoff: time.Second, MaxBackoff: 30 * time.Second}

// The states of a journaled call.
const (
	hookPending = "pending"
	hookFailed  = "failed"
)

// hookJournalEntry is one on_* call owed for a write.
type hookJournalEntry struct {
	ID       string          `json:"id"`
	Point    api.HookPoint   `json:"point"`
	URI      string          `json:"uri"`
	ObjectID string          `json:"object"`
	Input    json.RawMessage `json:"input,omitempty"`
	State    string          `json:"state"`
	Attempts int             `json:"attempts"`
	Error    string          `json:"error,omitempty"`
	At       time.Time       `json:"at"`
	// Cause is what the action runs under: {hook, this entry's id, the
	// write's depth + 1} — kept here so a replay runs under the same one.
	Cause cause.Cause `json:"cause"`
}

// hookJournal is the pending and failed on_* calls, on disk and in memory.
// dir "" = memory only (a service with no workspace root).
type hookJournal struct {
	dir      string
	mu       sync.Mutex
	entries  map[string]*hookJournalEntry
	inflight map[string]bool
}

// hooks is the service's journal, loaded from disk on first use.
func (s *Service) hooks() *hookJournal {
	s.hookJournalOnce.Do(func() {
		j := &hookJournal{entries: map[string]*hookJournalEntry{}, inflight: map[string]bool{}}
		if s.Config != nil && s.Config.WorkspaceRoot != "" {
			j.dir = filepath.Join(storage.NewPaths(s.Config.WorkspaceRoot).CWMDataDir(), "hooks")
			j.load()
		}
		s.hookJournal = j
	})
	return s.hookJournal
}

func (s *Service) retryPolicy() hookRetryPolicy {
	if s.hookRetry.Attempts > 0 {
		return s.hookRetry
	}
	return defaultHookRetry
}

func (j *hookJournal) load() {
	ents, err := os.ReadDir(j.dir)
	if err != nil {
		return // no journal yet
	}
	for _, de := range ents {
		if de.IsDir() || !strings.HasSuffix(de.Name(), ".json") {
			continue
		}
		path := filepath.Join(j.dir, de.Name())
		raw, err := atomicio.ReadAtomic(path)
		var e hookJournalEntry
		if err == nil {
			err = json.Unmarshal(raw, &e)
		}
		if err != nil || e.ID == "" {
			slog.Warn("cwm: unreadable hook journal entry", "path", path, "err", err)
			continue
		}
		j.entries[e.ID] = &e
	}
}

func (j *hookJournal) path(id string) string { return filepath.Join(j.dir, id+".json") }

// put records e (memory and disk). Caller holds no lock.
func (j *hookJournal) put(e *hookJournalEntry) {
	j.mu.Lock()
	defer j.mu.Unlock()
	cp := *e
	j.entries[e.ID] = &cp
	if j.dir == "" {
		return
	}
	raw, err := json.MarshalIndent(&cp, "", "  ")
	if err == nil {
		err = atomicio.WriteAtomic(j.path(e.ID), raw, 0o644)
	}
	if err != nil {
		slog.Warn("cwm: hook journal write", "id", e.ID, "uri", e.URI, "object", e.ObjectID, "err", err)
	}
}

// done drops e, and every failed call of the same hook on the same object: a
// success supersedes them.
func (j *hookJournal) done(e *hookJournalEntry) {
	j.mu.Lock()
	defer j.mu.Unlock()
	delete(j.inflight, e.ID)
	for id, o := range j.entries {
		if id == e.ID || (o.State == hookFailed && o.ObjectID == e.ObjectID && o.URI == e.URI) {
			delete(j.entries, id)
			if j.dir != "" {
				_ = os.Remove(j.path(id))
			}
		}
	}
}

// claim marks a pending call as running in this process; false if it already
// is, or is not pending.
func (j *hookJournal) claim(id string) bool {
	j.mu.Lock()
	defer j.mu.Unlock()
	e, ok := j.entries[id]
	if !ok || e.State != hookPending || j.inflight[id] {
		return false
	}
	j.inflight[id] = true
	return true
}

func (j *hookJournal) release(id string) {
	j.mu.Lock()
	delete(j.inflight, id)
	j.mu.Unlock()
}

// pending is the calls still owed, oldest first.
func (j *hookJournal) pending() []hookJournalEntry {
	return j.filter(func(e *hookJournalEntry) bool { return e.State == hookPending })
}

// failuresOf is an object's failed calls, as the errors it carries.
func (j *hookJournal) failuresOf(objectID string) []api.ObjectDiagnostic {
	var out []api.ObjectDiagnostic
	for _, e := range j.filter(func(e *hookJournalEntry) bool { return e.State == hookFailed && e.ObjectID == objectID }) {
		desc := fmt.Sprintf("the %s hook %s failed after %d attempt(s): %s", e.Point, e.URI, e.Attempts, e.Error)
		if j.dir != "" {
			desc += " (journal: " + j.path(e.ID) + ")"
		}
		out = append(out, api.ObjectDiagnostic{Code: api.DiagnosticHookFailed,
			Label: fmt.Sprintf("%s hook %s failed", e.Point, e.URI), Description: desc})
	}
	return out
}

func (j *hookJournal) filter(keep func(*hookJournalEntry) bool) []hookJournalEntry {
	j.mu.Lock()
	var out []hookJournalEntry
	for _, e := range j.entries {
		if keep(e) {
			out = append(out, *e)
		}
	}
	j.mu.Unlock()
	sort.Slice(out, func(a, b int) bool { return out[a].ID < out[b].ID })
	return out
}

// newHookEntry is a pending call, its id sortable by time.
func newHookEntry(point api.HookPoint, uri, objectID string, input json.RawMessage) *hookJournalEntry {
	var b [4]byte
	_, _ = rand.Read(b[:])
	now := time.Now().UTC()
	return &hookJournalEntry{ID: fmt.Sprintf("%020d-%s", now.UnixNano(), hex.EncodeToString(b[:])),
		Point: point, URI: uri, ObjectID: objectID, Input: input, State: hookPending, At: now}
}

// dispatchHook runs a journaled call in the background, retrying per the
// policy; it leaves the journal on success and turns failed when the attempts
// run out. The action runs under the entry's cause — at the first dispatch
// and at a replay alike.
func (s *Service) dispatchHook(ctx stdctx.Context, actions actionsapi.ActionService, e hookJournalEntry) {
	j := s.hooks()
	if !j.claim(e.ID) {
		return
	}
	ctx = cause.WithCause(ctx, e.Cause)
	policy := s.retryPolicy()
	go func() {
		backoff := policy.Backoff
		for {
			_, err := actions.Run(ctx, actionsapi.RunInput{URI: e.URI, Input: e.Input})
			e.Attempts++
			if err == nil {
				j.done(&e)
				return
			}
			e.Error = err.Error()
			if e.Attempts >= policy.Attempts {
				e.State = hookFailed
				j.put(&e)
				j.release(e.ID)
				slog.Warn("cwm: hook failed", "point", e.Point, "uri", e.URI, "object", e.ObjectID, "attempts", e.Attempts, "err", err)
				return
			}
			j.put(&e) // the attempts so far, on disk
			time.Sleep(backoff)
			if backoff *= 2; backoff > policy.MaxBackoff {
				backoff = policy.MaxBackoff
			}
		}
	}()
}

// replayPendingHooks dispatches every call a previous process (or this one,
// without an action service) left pending.
func (s *Service) replayPendingHooks(ctx stdctx.Context, actions actionsapi.ActionService) {
	for _, e := range s.hooks().pending() {
		s.dispatchHook(ctx, actions, e)
	}
}
