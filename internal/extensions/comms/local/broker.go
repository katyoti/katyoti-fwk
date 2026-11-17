// SPDX-License-Identifier: Apache-2.0

package commslocal

import (
	"sync"

	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
)

// askBroker shows a child's question to its parent: it carries the question
// UP to its parent (keyed by the parent's run id) for the parent's turn to
// surface, and lets the parent take it back to answer. The question itself
// is a human task - its to-do the record, its answer the
// task's result -; the broker keeps no answer. It ALSO wakes a parent that is
// blocked in Await — the broker is the authoritative `asked` signal, because it
// is hit on EVERY Post regardless of whether the child's engine Channel is
// reachable from its tool ctx (a delegated agent child's bubble may run with a
// nil Channel, so the engine-Channel wake is unreliable for `asked`; the
// bgManager's done/failed wake stays on the engine Channel where it IS reliable).
//
// Private to comms — the only door to it is commsapi.Service.
type askBroker struct {
	mu sync.Mutex
	// byID holds the full record (incl. ChildTaskID) until the parent Takes it
	// to answer — survives Drain so the answer can still Resume the child.
	byID map[string]agentapi.ChildQuestion
	// pending is the per-parent DISPLAY queue (question ids in arrival order),
	// drained by the OnTurnStart surfacing.
	pending map[string][]string
	// parentOf maps a question id to its parent run id, so Take can decrement
	// the outstanding count without the caller knowing the parent.
	parentOf map[string]string
	// outstanding counts posted-but-not-yet-answered questions per parent — the
	// "is there a question to react to" signal Await pre-checks (survives Drain;
	// reflects only UNANSWERED questions, so no double-answer).
	outstanding map[string]int
	// waiters holds per-parent notification channels; each is signaled (non-
	// blocking) on Post so a blocked Await returns `asked` immediately.
	waiters map[string][]chan struct{}
}

func newAskBroker() *askBroker {
	return &askBroker{
		byID:        map[string]agentapi.ChildQuestion{},
		pending:     map[string][]string{},
		parentOf:    map[string]string{},
		outstanding: map[string]int{},
		waiters:     map[string][]chan struct{}{},
	}
}

func (b *askBroker) Post(parentRunID string, q agentapi.ChildQuestion) {
	if parentRunID == "" || q.ID == "" {
		return
	}
	b.mu.Lock()
	b.byID[q.ID] = q
	b.pending[parentRunID] = append(b.pending[parentRunID], q.ID)
	b.parentOf[q.ID] = parentRunID
	b.outstanding[parentRunID]++
	waiters := append([]chan struct{}(nil), b.waiters[parentRunID]...)
	b.mu.Unlock()

	// Wake every parent blocked in Await on this parent id (non-blocking; the
	// channels are buffered, a full one already carries the signal).
	for _, w := range waiters {
		select {
		case w <- struct{}{}:
		default:
		}
	}
}

func (b *askBroker) Drain(parentRunID string) []agentapi.ChildQuestion {
	b.mu.Lock()
	defer b.mu.Unlock()
	ids := b.pending[parentRunID]
	delete(b.pending, parentRunID)
	out := make([]agentapi.ChildQuestion, 0, len(ids))
	for _, id := range ids {
		if q, ok := b.byID[id]; ok { // byID kept until Take
			out = append(out, q)
		}
	}
	return out
}

func (b *askBroker) Take(id string) (agentapi.ChildQuestion, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	q, ok := b.byID[id]
	if !ok {
		return agentapi.ChildQuestion{}, false
	}
	delete(b.byID, id)
	if p, ok := b.parentOf[id]; ok {
		delete(b.parentOf, id)
		if b.outstanding[p] > 0 {
			b.outstanding[p]--
		}
		if b.outstanding[p] == 0 {
			delete(b.outstanding, p)
		}
	}
	return q, true
}

// HasOutstanding reports whether parentRunID has a posted-but-unanswered
// question — the race-safe `asked` pre-check for Await (a question posted before
// Await subscribed).
func (b *askBroker) HasOutstanding(parentRunID string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.outstanding[parentRunID] > 0
}

// Subscribe registers a wake channel for parentRunID; it receives a signal on
// each subsequent Post for that parent. Returns the channel + an unsubscribe.
func (b *askBroker) Subscribe(parentRunID string) (<-chan struct{}, func()) {
	ch := make(chan struct{}, 4)
	b.mu.Lock()
	b.waiters[parentRunID] = append(b.waiters[parentRunID], ch)
	b.mu.Unlock()
	return ch, func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		ws := b.waiters[parentRunID]
		for i, w := range ws {
			if w == ch {
				b.waiters[parentRunID] = append(ws[:i], ws[i+1:]...)
				break
			}
		}
		if len(b.waiters[parentRunID]) == 0 {
			delete(b.waiters, parentRunID)
		}
	}
}
