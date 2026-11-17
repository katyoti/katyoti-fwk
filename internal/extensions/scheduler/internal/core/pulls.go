// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"sync"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	profileapi "github.com/katyoti/katyoti-fwk/internal/extensions/profile/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// THE PULL RECORD — the pulls an application was asked:
// who pulled, from which session, with which filter, open while a pull that
// waits still waits. The queue names, on each takeable to-do, the recorded
// pulls that would take it (Takers). Kept in memory, per application - lost at
// a restart, hence TakersSince - and bounded: the last maxPulls, an ended one
// dropped after pullKept. A pull by id is never recorded: it names its to-do.

const (
	maxPulls = 256
	pullKept = 24 * time.Hour
	// takerWindow is how long an ended pull still names its worker a taker:
	// the default lease, the time a worker takes to report and pull again.
	takerWindow = time.Duration(api.DefaultLeaseSeconds) * time.Second
)

// The outcomes of a recorded pull.
const (
	pullTook    = "took"
	pullNothing = "nothing"
	pullError   = "error"
)

// PullRecord is one application's record of its pulls: an application service
// (local declares it), shared by every Service the application builds.
type PullRecord struct {
	mu    sync.Mutex
	since time.Time
	now   func() time.Time
	next  int64
	pulls []recordedPull
}

// recordedPull is one pull: open while ended is zero.
type recordedPull struct {
	id      int64
	by      cwmapi.Actor
	session string
	filter  api.Filter
	auto    bool
	started time.Time
	ended   time.Time
	outcome string
}

// NewPullRecord is an empty record, started now.
func NewPullRecord(context.Context) (*PullRecord, error) { return newPullRecord(), nil }

func newPullRecord() *PullRecord {
	return &PullRecord{since: time.Now().UTC(), now: func() time.Time { return time.Now().UTC() }}
}

// SetClock replaces the record's clock (a test moves time past the taker
// window).
func (r *PullRecord) SetClock(now func() time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.now = now
}

// open records a pull starting now; its id ends it.
func (r *PullRecord) open(by cwmapi.Actor, session string, f api.Filter, auto bool) int64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.next++
	at := r.now()
	r.prune(at)
	r.pulls = append(r.pulls, recordedPull{id: r.next, by: by, session: session, filter: f, auto: auto, started: at})
	if over := len(r.pulls) - maxPulls; over > 0 {
		r.pulls = append(r.pulls[:0:0], r.pulls[over:]...)
	}
	return r.next
}

// end closes a recorded pull with its outcome.
func (r *PullRecord) end(id int64, outcome string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.pulls {
		if r.pulls[i].id == id {
			r.pulls[i].ended, r.pulls[i].outcome = r.now(), outcome
			return
		}
	}
}

// prune drops the ended pulls older than pullKept. Called under mu.
func (r *PullRecord) prune(at time.Time) {
	kept := r.pulls[:0]
	for _, p := range r.pulls {
		if p.ended.IsZero() || at.Sub(p.ended) <= pullKept {
			kept = append(kept, p)
		}
	}
	r.pulls = kept
}

// latest is the last pull of each actor and session, with the record's start
// and its now.
func (r *PullRecord) latest() ([]recordedPull, time.Time, time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	last := map[string]int{}
	var out []recordedPull
	for _, p := range r.pulls {
		key := p.by.Kind + "\x00" + p.by.ID + "\x00" + p.session
		if i, ok := last[key]; ok {
			out[i] = p
			continue
		}
		last[key] = len(out)
		out = append(out, p)
	}
	return out, r.since, r.now()
}

// pullRecordIn is the record of the application ctx carries; a context with
// no application (or none declared) gets a record of its own.
func pullRecordIn(ctx context.Context) *PullRecord {
	if r, err := application.UseService[*PullRecord](ctx); err == nil && r != nil {
		return r
	}
	return newPullRecord()
}

// outcomeOf is what ended a pull.
func outcomeOf(out api.PullOutput, err error) string {
	switch {
	case err != nil:
		return pullError
	case out.Todo != nil:
		return pullTook
	case out.Ended != "":
		return out.Ended
	}
	return pullNothing
}

// takers names, on each takeable entry, the recorded pulls that would take it
// - by the pull's own rules: its filter (For defaulted as pullIn does), the
// auto rule, its worker's right to take it (mayTake), a person's to-do kept
// for a person - and that are open now, ended within takerWindow, or older but
// whose session (its actor, when it names none) holds a live claim. The queue
// a narrower filter sees reserves modules differently: a taker is named on an
// entry this queue holds takeable.
func (s *Service) takers(ctx context.Context, q []api.QueueEntry, st queueState) (string, error) {
	pulls, since, now := s.pulls.latest()
	ms := newMemberships(ctx)
	for i := range q {
		e := &q[i]
		if !e.Takeable {
			continue
		}
		for _, p := range pulls {
			ok, err := wouldTake(p, e.Todo, st.modes, ms)
			if err != nil {
				return "", err
			}
			if !ok {
				continue
			}
			tk := api.Taker{By: p.by, Session: p.session}
			switch {
			case p.ended.IsZero():
				tk.Waiting = true
			case now.Sub(p.ended) <= takerWindow:
				tk.At = p.ended.Format(time.RFC3339)
			default:
				h := holdingOf(st.held, p)
				if h == "" {
					continue
				}
				tk.At, tk.Holds = p.ended.Format(time.RFC3339), h
			}
			e.Takers = append(e.Takers, tk)
		}
	}
	return since.Format(time.RFC3339), nil
}

// wouldTake reports a to-do a recorded pull would take, its module locks
// aside.
func wouldTake(p recordedPull, t api.Todo, modes map[string]string, ms *memberships) (bool, error) {
	f := p.filter
	if f.For == "" {
		f.For = api.ForAgent
		if profileapi.IsPerson(p.by) {
			f.For = ""
		}
	}
	if !matches(t, f) {
		return false, nil
	}
	if p.auto && (modeOf(modes, t) != api.ModeAuto || t.Start != api.StartAuto || t.For != api.ForAgent) {
		return false, nil
	}
	if t.For == api.ForPerson && !profileapi.IsPerson(p.by) {
		return false, nil
	}
	why, err := ms.mayTake(t, p.by)
	return why == "", err
}

// holdingOf is the to-do a pull's session holds - its actor's, when the pull
// names no session - "" when none.
func holdingOf(held []api.Todo, p recordedPull) string {
	for _, t := range held {
		switch {
		case p.session != "" && t.Claim.Session == p.session,
			p.session == "" && same(t.Claim.By, p.by):
			return t.ID
		}
	}
	return ""
}
