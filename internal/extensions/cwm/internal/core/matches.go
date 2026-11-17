// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"crypto/sha256"
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
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/internal/storage"
)

// THE MATCHES OF THE ASYNCHRONOUS SUBSCRIPTIONS. An on_* entry that declares
// `async` is never run by CWM: at the write it becomes a MATCH (api.Match) —
// recorded in CWM's uncommitted state (`<.katyoti>/cwm/matches/`, one JSON file
// per match, named by a hash of its id) BEFORE the verb returns, then handed
// over in the background to the MatchTakers an extension contributed
// (api.TakeMatches). A match leaves the store only when a taker took it:
//
//   - no taker attached: it stays pending; attaching one, or the next start
//     (RunBootstrapHooks), hands it over;
//   - a taker fails: retried with the hook journal's doubling backoff; past
//     the attempts it stays pending until the next start;
//   - a pending match not yet handed over gets the next event of the same
//     subscription and key appended (coalescing): one reaction per object.
//
// The guards (api.ReactionDepthLimit, no self-reaction, accepts_caused) are
// applied when the match is made; what a reaction's write matches is staged
// in its outbox (outbox.go) rather than recorded here.

// matchStore is the matches not yet taken, on disk and in memory. dir "" =
// memory only (a service with no workspace root).
type matchStore struct {
	dir      string
	mu       sync.Mutex
	entries  map[string]*api.Match
	inflight map[string]bool
}

// matches is the service's match store, loaded from disk on first use.
func (s *Service) matches() *matchStore {
	s.matchStoreOnce.Do(func() {
		m := &matchStore{entries: map[string]*api.Match{}, inflight: map[string]bool{}}
		if s.Config != nil && s.Config.WorkspaceRoot != "" {
			m.dir = filepath.Join(storage.NewPaths(s.Config.WorkspaceRoot).CWMDataDir(), "matches")
			m.load()
		}
		s.matchStore = m
	})
	return s.matchStore
}

// stateFileName is the file name of a state entry keyed by id: a match or a
// reaction id holds `#`, `:` and `/`, which a file name cannot.
func stateFileName(id string) string {
	sum := sha256.Sum256([]byte(id))
	return hex.EncodeToString(sum[:16])
}

func (m *matchStore) path(id string) string {
	return filepath.Join(m.dir, stateFileName(id)+".json")
}

func (m *matchStore) load() {
	ents, err := os.ReadDir(m.dir)
	if err != nil {
		return // no match recorded yet
	}
	for _, de := range ents {
		if de.IsDir() || !strings.HasSuffix(de.Name(), ".json") {
			continue
		}
		p := filepath.Join(m.dir, de.Name())
		raw, err := atomicio.ReadAtomic(p)
		var e api.Match
		if err == nil {
			err = json.Unmarshal(raw, &e)
		}
		if err != nil || e.ID == "" {
			slog.Warn("cwm: unreadable match", "path", p, "err", err)
			continue
		}
		m.entries[e.ID] = &e
	}
}

// persist writes e to disk. Caller holds m.mu.
func (m *matchStore) persist(e *api.Match) {
	if m.dir == "" {
		return
	}
	raw, err := json.MarshalIndent(e, "", "  ")
	if err == nil {
		err = atomicio.WriteAtomic(m.path(e.ID), raw, 0o644)
	}
	if err != nil {
		slog.Warn("cwm: match write", "id", e.ID, "err", err)
	}
}

// record keeps in — appended to a pending match of the same subscription and
// key not being handed over, else as a new one — and returns the id of the
// match that holds it.
func (m *matchStore) record(in api.Match) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	var into *api.Match
	for id, e := range m.entries {
		if m.inflight[id] || !sameReactionSlot(*e, in) {
			continue
		}
		if into == nil || e.At.Before(into.At) || (e.At.Equal(into.At) && e.ID < into.ID) {
			into = e // the oldest, so the result does not depend on map order
		}
	}
	if into != nil {
		into.Events = append(into.Events, in.Events...)
		m.persist(into)
		return into.ID
	}
	cp := in
	m.entries[cp.ID] = &cp
	m.persist(&cp)
	return cp.ID
}

// sameReactionSlot reports two matches one reaction covers: the same
// subscription on the same key.
func sameReactionSlot(a, b api.Match) bool {
	return a.State == api.MatchPending && a.Subscription.ID == b.Subscription.ID && a.Key == b.Key
}

// claim marks a match as being handed over and returns it; false when it is
// gone or already being handed over.
func (m *matchStore) claim(id string) (api.Match, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.entries[id]
	if !ok || m.inflight[id] {
		return api.Match{}, false
	}
	m.inflight[id] = true
	cp := *e
	cp.Events = append([]api.MatchedEvent(nil), e.Events...)
	return cp, true
}

func (m *matchStore) release(id string) {
	m.mu.Lock()
	delete(m.inflight, id)
	m.mu.Unlock()
}

// taken forgets a match a taker took.
func (m *matchStore) taken(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.inflight, id)
	delete(m.entries, id)
	if m.dir != "" {
		_ = os.Remove(m.path(id))
	}
}

// pending is every match not taken, oldest first.
func (m *matchStore) pending() []api.Match {
	m.mu.Lock()
	out := make([]api.Match, 0, len(m.entries))
	for _, e := range m.entries {
		cp := *e
		cp.Events = append([]api.MatchedEvent(nil), e.Events...)
		out = append(out, cp)
	}
	m.mu.Unlock()
	sortMatches(out)
	return out
}

func sortMatches(ms []api.Match) {
	sort.Slice(ms, func(a, b int) bool {
		if !ms[a].At.Equal(ms[b].At) {
			return ms[a].At.Before(ms[b].At)
		}
		return ms[a].ID < ms[b].ID
	})
}

// matchSubscriptions is the matches one write makes on its applying async
// entries, under the cascade guards:
//
//   - the write's depth d (its cause's): none at d >= api.ReactionDepthLimit;
//   - a write caused by a reaction of subscription S never matches S;
//   - a caused write (d > 0) matches only an entry that accepts_caused.
func (s *Service) matchSubscriptions(ctx stdctx.Context, ev api.MatchedEvent, obj api.Object, subs []api.ResolvedHook) []api.Match {
	if len(subs) == 0 {
		return nil
	}
	depth := cause.DepthFrom(ctx)
	if depth >= api.ReactionDepthLimit {
		slog.Debug("cwm: write at the reaction depth limit matches no subscription", "object", obj.ID, "depth", depth)
		return nil
	}
	self := ""
	if c, ok := cause.CauseFrom(ctx); ok && c.Kind == cause.KindReaction {
		if rid, ok := api.ParseReactionID(c.ID); ok {
			self = rid.Subscription
		}
	}
	// The input and the key speak the hook vocabulary: the object and the
	// event (a raised event's payload included).
	var env map[string]any
	envOf := func() map[string]any {
		if env == nil {
			env = s.hookEnv(obj, eventEnv(&ev))
		}
		return env
	}
	var out []api.Match
	for _, h := range subs {
		spec := h.Async.WithDefaults()
		if h.Subscription == self {
			continue // a reaction never reacts to its own writes
		}
		if depth > 0 && !spec.AcceptsCaused {
			continue
		}
		ref := api.SubscriptionRef{ID: h.Subscription, Source: h.Source, Point: h.Point, URI: h.URI,
			Provider: h.Provider, Module: ev.Module, Priority: spec.Priority, Attempts: spec.Attempts, AcceptsCaused: spec.AcceptsCaused, Atomic: spec.Atomic}
		// A cross-module subscription's subscriber is the module object that
		// declared it, and its reactions write only the modules it declares
		// (the table's Writes, defaults filled).
		if subscriber, ok := strings.CutPrefix(h.Source, "module:"); ok {
			ref.Module = subscriber
			ref.Writes = append([]string(nil), h.Writes...)
		}
		if len(h.Input) > 0 {
			ref.ResolvedInput = interpolateObjectInput(hookExprs, h.Input, envOf())
		}
		key := obj.ID
		if spec.Key != api.DefaultAsyncKey {
			// An expression that resolves to nothing falls back to the object:
			// an empty key would make one reaction of every object's events.
			if v := interpolateObjectInput(hookExprs, map[string]any{"key": spec.Key}, envOf())["key"]; v != nil && fmt.Sprint(v) != "" {
				key = fmt.Sprint(v)
			}
		}
		out = append(out, api.Match{
			ID:           api.ReactionID{Subscription: h.Subscription, EventID: ev.EventID}.String(),
			Subscription: ref, Module: ev.Module, ObjectID: obj.ID, Key: key, Depth: depth,
			Cause: ev.Cause, Events: []api.MatchedEvent{ev}, At: ev.At, State: api.MatchPending,
		})
	}
	return out
}

// recordMatches records each match (before the verb returns) and hands it
// over in the background.
func (s *Service) recordMatches(ctx stdctx.Context, matches []api.Match) {
	for _, m := range matches {
		s.handOver(ctx, s.matches().record(m))
	}
}

// AttachMatchTakers attaches the takers extensions contributed
// (api.TakeMatches) and hands them every match pending — what this or a
// previous process recorded with no taker to take it. ctx carries the
// application each Take runs under.
func (s *Service) AttachMatchTakers(ctx stdctx.Context, takers []api.MatchTaker) {
	kept := make([]api.MatchTaker, 0, len(takers))
	for _, t := range takers {
		if t.Take != nil {
			kept = append(kept, t)
		}
	}
	sort.SliceStable(kept, func(a, b int) bool { return kept[a].Name < kept[b].Name })
	s.takersMu.Lock()
	s.takers = kept
	s.takersMu.Unlock()
	s.replayPendingMatches(ctx)
}

func (s *Service) matchTakers() []api.MatchTaker {
	s.takersMu.RLock()
	defer s.takersMu.RUnlock()
	return s.takers
}

// replayPendingMatches hands over every match pending.
func (s *Service) replayPendingMatches(ctx stdctx.Context) {
	for _, m := range s.matches().pending() {
		s.handOver(ctx, m.ID)
	}
}

// handOver gives a match to the takers in the background: the first that
// takes it takes it and the match is forgotten; when every one fails it is
// retried with a doubling backoff, then left pending for the next start. With
// no taker it stays pending.
func (s *Service) handOver(ctx stdctx.Context, id string) {
	takers := s.matchTakers()
	if len(takers) == 0 {
		return
	}
	store := s.matches()
	m, ok := store.claim(id)
	if !ok {
		return
	}
	policy := s.retryPolicy()
	go func() {
		backoff := policy.Backoff
		for attempt := 1; ; attempt++ {
			var err error
			for _, t := range takers {
				if err = t.Take(ctx, m); err == nil {
					store.taken(id)
					return
				}
			}
			if attempt >= policy.Attempts {
				store.release(id)
				slog.Warn("cwm: match kept for the next start — no taker took it", "match", id, "attempts", attempt, "err", err)
				return
			}
			time.Sleep(backoff)
			if backoff *= 2; backoff > policy.MaxBackoff {
				backoff = policy.MaxBackoff
			}
		}
	}()
}
