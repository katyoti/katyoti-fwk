// SPDX-License-Identifier: Apache-2.0

package api

import "sync"

// Following a display panel's events.
//
// A panel's events go out on subjects.PanelOutSubject, each with the panel's
// sequence number; its input comes back on subjects.PanelInSubject. A screen
// that opens a panel late reads its snapshot (the panel's data after event
// Seq, from the panel server) and then follows the events from there: the
// ones the snapshot already holds are dropped, the next ones are delivered in
// order, once each - one that arrives early waits for those before it - and
// a gap, an event lost on the way, makes it read the snapshot again.
// PanelFollower is that rule, the same for every follower.

// PanelReorderWindow is how many events may arrive ahead of a missing one
// before the follower calls it a gap: NATS keeps one subscription's order,
// but a bus that hands each message its own goroutine does not.
const PanelReorderWindow = 16

// PanelFollower delivers a panel's events after a snapshot, in order, once
// each. It is safe for concurrent use.
type PanelFollower[E any] struct {
	mu     sync.Mutex
	seqOf  func(E) int64
	seq    int64
	resync bool
	held   map[int64]E
}

// NewPanelFollower follows from a snapshot taken after event snapshotSeq;
// seqOf reads an event's sequence number.
func NewPanelFollower[E any](snapshotSeq int64, seqOf func(E) int64) *PanelFollower[E] {
	return &PanelFollower[E]{seqOf: seqOf, seq: snapshotSeq, held: map[int64]E{}}
}

// Seq is the sequence number of the last event the follower delivered (or
// of its snapshot).
func (f *PanelFollower[E]) Seq() int64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.seq
}

// Offer takes one event as it arrives. It answers the events now ready, in
// order - none for one already delivered or held in the snapshot, none yet
// for one ahead of a missing one -, and resync true once more than
// PanelReorderWindow events wait on a missing one: the caller reads the
// snapshot again and calls Reset with its sequence. Events that arrive while
// it waits for the snapshot are kept and delivered after it.
func (f *PanelFollower[E]) Offer(e E) (ready []E, resync bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s := f.seqOf(e)
	if s <= f.seq {
		return nil, f.resync
	}
	f.held[s] = e
	if !f.resync {
		ready = f.drain(nil)
	}
	if len(f.held) > PanelReorderWindow {
		f.resync = true
	}
	return ready, f.resync
}

// Reset follows from a snapshot read again after event snapshotSeq: the held
// events it already covers are dropped and the next ones delivered.
func (f *PanelFollower[E]) Reset(snapshotSeq int64) []E {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.resync = false
	if snapshotSeq > f.seq {
		f.seq = snapshotSeq
	}
	for s := range f.held {
		if s <= f.seq {
			delete(f.held, s)
		}
	}
	return f.drain(nil)
}

func (f *PanelFollower[E]) drain(ready []E) []E {
	for {
		e, ok := f.held[f.seq+1]
		if !ok {
			return ready
		}
		delete(f.held, f.seq+1)
		f.seq++
		ready = append(ready, e)
	}
}
