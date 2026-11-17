// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	journalapi "github.com/katyoti/katyoti-fwk/internal/extensions/journal/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// A RUNTIME ROW FOLLOWS THE CONTENT IT NAMES. A to-do names
// CWM objects - its target, its request, the object a blocked report waits on -
// and a wait names the request it waits on. The names point one way: content
// never names a row. Git is written first; the store reacts after, with no
// transaction across the two:
//   - an object moved: every row still naming its old id names the new one;
//   - an object split with its source gone, or archived: the open rows naming
//     it end (archived, a note saying why); a held or reported row keeps its
//     claim and its result, collected by its owner as any other;
//   - an event the store missed (a process down, a pull of git): the tick asks
//     the journal where a name that no longer resolves went and does the same;
//     a name with no record is flagged once, never deleted.
//
// Every reaction writes only a row that still names the old id: an event
// delivered twice writes nothing the second time.

// Follow reacts to one CWM lifecycle event: a move re-points the rows naming
// the object, a split that drops its source and an archive end them. Any other
// event is nothing to it. It answers what it changed.
func (s *Service) Follow(ctx context.Context, ev cwmapi.EventEnvelope) ([]string, error) {
	segs := strings.Split(ev.Topic, ".")
	if len(segs) < 3 || segs[len(segs)-3] != "object" { // ...object.<token>.<action>
		return nil, nil
	}
	switch segs[len(segs)-1] {
	case journalapi.ActionMoved:
		var p cwmapi.ObjectMovedEventPayload
		if err := json.Unmarshal(ev.Payload, &p); err != nil || p.FromID == "" || p.ObjectID == "" || p.FromID == p.ObjectID {
			return nil, err
		}
		return s.repoint(ctx, p.FromID, p.ObjectID)
	case journalapi.ActionSplit:
		var p cwmapi.ObjectSplitEventPayload
		if err := json.Unmarshal(ev.Payload, &p); err != nil || p.ObjectID == "" || p.SourceKept {
			return nil, err
		}
		return s.end(ctx, p.ObjectID, fmt.Sprintf("ended: %s was split into %s", p.ObjectID, strings.Join(p.Successors, ", ")))
	case journalapi.ActionArchived:
		var p cwmapi.ObjectArchivedEventPayload
		if err := json.Unmarshal(ev.Payload, &p); err != nil || p.ObjectID == "" {
			return nil, err
		}
		return s.end(ctx, p.ObjectID, "ended: "+p.ObjectID+" was archived")
	}
	return nil, nil
}

// namesObject reports a to-do naming id: as its target, its request or the
// object its blocked report waits on.
func namesObject(t api.Todo, id string) bool {
	return t.Target == id || t.Request == id || heldOnObject(t) == id
}

// heldOnObject is the object a blocked report's wait names, "" when it waits
// on no object.
func heldOnObject(t api.Todo) string {
	if t.Held == nil || t.Held.Kind != api.HeldObject {
		return ""
	}
	return t.Held.Target
}

// repoint makes every row still naming from name to.
func (s *Service) repoint(ctx context.Context, from, to string) ([]string, error) {
	all, _, err := s.scheduled(ctx, "", false)
	if err != nil {
		return nil, err
	}
	var out []string
	var errs []error
	for _, t := range all {
		if !namesObject(t, from) {
			continue
		}
		changed := false
		_, err := s.store.Update(ctx, t.ID, func(t *api.Todo) error {
			if t.Target == from {
				t.Target, changed = to, true
			}
			if t.Request == from {
				t.Request, changed = to, true
			}
			if heldOnObject(*t) == from {
				t.Held.Target, changed = to, true
			}
			if !changed {
				return errNothingToWrite
			}
			return nil
		})
		switch {
		case errors.Is(err, errNothingToWrite), errors.Is(err, cwmapi.ErrNotFound):
		case err != nil:
			errs = append(errs, fmt.Errorf("%s: %w", t.ID, err))
		default:
			out = append(out, t.ID+": "+from+" -> "+to)
		}
	}
	waits, err := s.waitsOn(ctx, from)
	if err != nil {
		errs = append(errs, err)
	}
	for _, w := range waits {
		w.On = to
		if err := s.store.PutWait(ctx, w); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", w.ID, err))
			continue
		}
		out = append(out, w.ID+": "+from+" -> "+to)
	}
	return out, errors.Join(errs...)
}

// end ends the open rows naming id - a held or reported to-do keeps its claim
// and its result, for its owner to collect - and the waits on it.
func (s *Service) end(ctx context.Context, id, note string) ([]string, error) {
	all, _, err := s.scheduled(ctx, "", false)
	if err != nil {
		return nil, err
	}
	var out []string
	var errs []error
	for _, t := range all {
		if t.Target != id && t.Request != id {
			continue
		}
		_, err := s.store.Archive(ctx, t.ID, func(t *api.Todo) error {
			if (t.Target != id && t.Request != id) || held(*t) || t.State == api.StateReported {
				return errNothingToWrite
			}
			t.Archived, t.Note = now(), note
			return nil
		})
		switch {
		case errors.Is(err, errNothingToWrite), errors.Is(err, cwmapi.ErrNotFound):
		case err != nil:
			errs = append(errs, fmt.Errorf("%s: %w", t.ID, err))
		default:
			out = append(out, t.ID+": "+note)
		}
	}
	waits, err := s.waitsOn(ctx, id)
	if err != nil {
		errs = append(errs, err)
	}
	for _, w := range waits {
		if err := s.store.RemoveWait(ctx, w.ID); err != nil && !errors.Is(err, cwmapi.ErrNotFound) {
			errs = append(errs, fmt.Errorf("%s: %w", w.ID, err))
			continue
		}
		out = append(out, w.ID+": "+note)
	}
	return out, errors.Join(errs...)
}

// errNothingToWrite stops a rewrite that would change nothing: the row was
// re-pointed or ended already (a replayed event).
var errNothingToWrite = errors.New("nothing to write")

// waitsOn is every wait whose request is on, each naming its module by its
// reference: a wait written back (repoint) lands in its own project.
func (s *Service) waitsOn(ctx context.Context, on string) ([]api.Wait, error) {
	mods, err := modules(ctx)
	if err != nil {
		return nil, err
	}
	var out []api.Wait
	for _, m := range mods {
		got, err := s.store.ListWaits(ctx, m)
		if err != nil {
			return nil, err
		}
		for _, w := range got {
			w.Module = m
			if w.On == on {
				out = append(out, w)
			}
		}
	}
	return out, nil
}

// flagged is the names already flagged by this process (per workspace): a name the journal
// has no record of is said once, not at every tick.
var flagged sync.Map

// catchUp follows the moves, splits and archives the store missed: each name
// an open row carries that no longer resolves is looked up in the journal - a
// move re-points it, a split that dropped its source or an archive ends it; a
// name the journal has no record of is flagged, its rows kept.
func (s *Service) catchUp(ctx context.Context, rep *api.TickReport) {
	cwm, err := cwmOf(ctx)
	if err != nil {
		rep.Errors = append(rep.Errors, err.Error())
		return
	}
	names, err := s.names(ctx)
	if err != nil {
		rep.Errors = append(rep.Errors, err.Error())
		return
	}
	for _, name := range names {
		if _, err := cwm.ObjectResolve(ctx, cwmapi.ObjectResolveInput{Ref: name}); err == nil || !errors.Is(err, cwmapi.ErrNotFound) {
			continue
		}
		key := fmt.Sprintf("%p|%s", cwm, name)
		if _, seen := flagged.Load(key); seen {
			continue // said once; the journal is not asked again every tick
		}
		got, err := s.followRecord(ctx, name)
		if err != nil {
			rep.Errors = append(rep.Errors, err.Error())
		}
		if got == nil && err == nil {
			flagged.Store(key, true)
			msg := "to-dos name " + name + ", which no longer resolves and the journal has no record of: kept as they are"
			rep.Flagged = append(rep.Flagged, msg)
			slog.Warn("scheduler: " + msg)
			continue
		}
		rep.Followed = append(rep.Followed, got...)
	}
}

// names is every object name the open rows carry, each once.
func (s *Service) names(ctx context.Context) ([]string, error) {
	all, _, err := s.scheduled(ctx, "", false)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for _, t := range all {
		for _, n := range []string{t.Target, t.Request, heldOnObject(t)} {
			if n != "" {
				seen[n] = true
			}
		}
	}
	mods, err := modules(ctx)
	if err != nil {
		return nil, err
	}
	for _, m := range mods {
		waits, err := s.store.ListWaits(ctx, m)
		if err != nil {
			return nil, err
		}
		for _, w := range waits {
			if w.On != "" {
				seen[w.On] = true
			}
		}
	}
	out := make([]string, 0, len(seen))
	for n := range seen {
		out = append(out, n)
	}
	sort.Strings(out)
	return out, nil
}

// followRecord applies what the journal says became of name: nil, nil when it
// has no record of a move, a split dropping it, or an archive.
func (s *Service) followRecord(ctx context.Context, name string) ([]string, error) {
	journal, err := application.UseService[journalapi.JournalService](ctx)
	if err != nil {
		return nil, nil // no journal in this process: nothing to follow from
	}
	got, err := journal.Query(ctx, journalapi.QueryInput{ObjectID: name,
		Actions: []string{journalapi.ActionMoved, journalapi.ActionSplit, journalapi.ActionArchived}})
	if err != nil {
		return nil, fmt.Errorf("the journal of %s: %w", name, err)
	}
	for _, r := range got.Records { // newest first
		switch {
		case r.Action == journalapi.ActionMoved && r.FromID == name && r.ID != name:
			out, err := s.repoint(ctx, name, r.ID)
			return nonNil(out), err
		case r.Action == journalapi.ActionSplit && r.ID == name && r.SourceKept != nil && !*r.SourceKept:
			out, err := s.end(ctx, name, fmt.Sprintf("ended: %s was split into %s", name, strings.Join(r.Successors, ", ")))
			return nonNil(out), err
		case r.Action == journalapi.ActionArchived && r.ID == name:
			out, err := s.end(ctx, name, "ended: "+name+" was archived")
			return nonNil(out), err
		}
	}
	return nil, nil
}

// nonNil tells "followed, nothing left to change" from "no record".
func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// Follow is the scheduler's reaction to a CWM lifecycle event, for its
// observer (local): what it changed is logged, a failure warned.
func Follow(ctx context.Context, ev cwmapi.EventEnvelope) {
	svc, err := ServiceIn(ctx)
	if err != nil {
		slog.Warn("scheduler: rows naming a moved, split or archived object not followed; the tick catches up", "topic", ev.Topic, "error", err)
		return
	}
	got, err := svc.Follow(ctx, ev)
	if err != nil {
		slog.Warn("scheduler: rows naming a moved, split or archived object not all followed; the tick catches up", "topic", ev.Topic, "error", err)
	}
	if len(got) > 0 {
		slog.Info("scheduler: rows followed their object", "topic", ev.Topic, "rows", got)
	}
}
