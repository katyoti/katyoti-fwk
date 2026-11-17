// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/render"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// ONE QUEUE PER CHANGE: « À faire »'s page and the
// rail's badge read the same queue, and computing it is the page's cost. A
// count asked while the queue is being computed - for the page or for
// another count - takes that computation's answer instead of starting a
// second one; a count asked after it ended computes again. The page never
// joins a computation already running - it may have started before the
// person's last answer - it starts its own, which the counts asked meanwhile
// join. A re-read of the page (the same view, or asked fresh) computes
// again, so it sees the latest state (an answer the person just gave); a
// click (another view) is answered from the page's last queue while it is
// fresh and no object of the universe changed since it began (queryReuse,
// QueueChanged).

// queueComputations counts the whole queues computed (tests).
var queueComputations atomic.Int64

// queueRun is one computation of the whole queue, in flight or ended.
type queueRun struct {
	done chan struct{}
	base queueBase
	err  error
}

// queueRuns are the computations in flight, by universe and language.
type queueRuns struct {
	mu      sync.Mutex
	running map[string]*queueRun
	// pages is the page's last read, by universe and language
	// (sharedPersonQueue).
	pages map[string]pageRead
}

var sharedQueue = &queueRuns{running: map[string]*queueRun{}}

// sharedPersonQueue answers the person's queue as PersonQueue does; join
// takes a computation of the same universe and language in flight when there
// is one (a count), else - and always without join (the page) - it computes
// the queue, the counts asked meanwhile joining it. The page asked with
// another view than its last read (a tile, a kind chip, a focus: a click)
// answers from that read's queue while it is younger than queryReuse and
// nothing changed since it began; asked fresh or with the same view (a
// re-read: after an action, on a change) it computes again.
func sharedPersonQueue(ctx context.Context, in api.PersonQueueInput, join bool) (api.PersonQueueOutput, error) {
	if join {
		base, err := sharedQueueBase(ctx, in.Language, true)
		if err != nil {
			return api.PersonQueueOutput{}, err
		}
		return scopedQueue(base, in), nil
	}
	key, view := queueKey(ctx, in.Language), pageView(in)
	if !in.Fresh {
		if base, ok := sharedQueue.reuse(key, view, queueChanges.Load(), time.Now()); ok {
			return scopedQueue(base, in), nil
		}
	}
	start, mark := time.Now(), queueChanges.Load()
	base, err := sharedQueueBase(ctx, in.Language, false)
	if err != nil {
		return api.PersonQueueOutput{}, err
	}
	sharedQueue.keep(key, view, base, start, mark)
	return scopedQueue(base, in), nil
}

// queueChanges counts the changes of the universe the process has seen:
// every CWM object event and every move of the scheduler's to-dos
// (plan.changed) - QueueChanged. A kept read answers a click only
// while it is unchanged since the read began, so no reader - another window,
// a pane's « À faire » panel, the mobile companion - is ever shown a queue
// read before a change.
var queueChanges atomic.Uint64

// QueueChanged says an object of the universe or a to-do changed: every page
// read kept so far no longer answers a click. The CWM observer calls it on
// each event, and the scheduler's plan.changed on each move of a to-do.
func QueueChanged() { queueChanges.Add(1) }

// queryReuse is how long a page's queue answers the page's clicks
// : a click on a tile or a chip only shows another
// part of the same queue, and computing it again cost the person seconds per
// click. What changed meanwhile reaches the screen by its re-read (the same
// view: computed again), so a click shows at most this old a queue.
const queryReuse = 20 * time.Second

// pageView is what the page shows of the queue: its scope, its filters, the
// item it shows first.
func pageView(in api.PersonQueueInput) string {
	return strings.Join([]string{in.Target, in.ID, in.Focus, in.Action, in.Kind}, "|")
}

// pageRead is the page's last read of a queue: the queue, when its
// computation began, the universe's change mark then, the view it was last
// shown in.
type pageRead struct {
	base queueBase
	at   time.Time
	mark uint64
	view string
}

// reuse answers the queue of the page's last read under key when view is
// another (a click), that queue is younger than queryReuse and the universe
// is unchanged since it began (mark); the read is then shown in view.
func (s *queueRuns) reuse(key, view string, mark uint64, now time.Time) (queueBase, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	last, ok := s.pages[key]
	if !ok || last.mark != mark || last.view == view || now.Sub(last.at) >= queryReuse {
		return queueBase{}, false
	}
	last.view = view
	s.pages[key] = last
	return last.base, true
}

// keep records the page's read of the queue under key, its computation begun
// at at under the change mark mark; an older one never replaces a later.
func (s *queueRuns) keep(key, view string, base queueBase, at time.Time, mark uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pages == nil {
		s.pages = map[string]pageRead{}
	}
	if last, ok := s.pages[key]; ok && last.at.After(at) {
		return
	}
	s.pages[key] = pageRead{base: base, at: at, mark: mark, view: view}
}

// sharedQueueBase is the whole queue before its scope, computed or joined as
// sharedPersonQueue does: a reader needing several scopes of it (the deck's
// Needs you tile, its Needs tab and their badge) reads it once.
func sharedQueueBase(ctx context.Context, language string, join bool) (queueBase, error) {
	return sharedQueue.run(ctx, queueKey(ctx, language), join, func() (queueBase, error) {
		return readQueueBase(withReadCache(ctx), language)
	})
}

// queueKey names whose queue a computation is: the universe's application,
// its project, the language its texts are in.
func queueKey(ctx context.Context, language string) string {
	return fmt.Sprintf("%p|%s|%s", application.AppFromContext(ctx), application.ProjectFromContext(ctx), language)
}

// run joins the computation in flight under key (join), else computes it,
// counted as the one to join until it ends. A joined computation given up by
// its caller (its context ended) is computed again for a caller still there.
func (s *queueRuns) run(ctx context.Context, key string, join bool, compute func() (queueBase, error)) (queueBase, error) {
	if join {
		s.mu.Lock()
		r, ok := s.running[key]
		s.mu.Unlock()
		if ok {
			select {
			case <-r.done:
				if !errors.Is(r.err, context.Canceled) && !errors.Is(r.err, context.DeadlineExceeded) || ctx.Err() != nil {
					return r.base, r.err
				}
			case <-ctx.Done():
				return queueBase{}, ctx.Err()
			}
		}
	}
	r := &queueRun{done: make(chan struct{})}
	s.mu.Lock()
	s.running[key] = r // the latest computation is the one a count joins
	s.mu.Unlock()
	defer func() {
		close(r.done)
		s.mu.Lock()
		if s.running[key] == r {
			delete(s.running, key)
		}
		s.mu.Unlock()
	}()
	r.base, r.err = compute()
	return r.base, r.err
}

// selfContained lays each form's ui block over its inline schema here, as
// the object pane's ResolvePanel would: « À faire » answers a tree its host
// draws without a second round trip carrying the whole page back
// . A tree naming a schema by reference - which only
// the object pane reads - is answered as it is, for the host to resolve.
func selfContained(tree any) any {
	raw, err := json.Marshal(tree)
	if err != nil {
		return tree
	}
	var generic any
	if err := json.Unmarshal(raw, &generic); err != nil {
		return tree
	}
	out, err := render.ResolveComponents(generic, render.ComponentResolver{})
	if err != nil {
		return tree
	}
	return out
}
