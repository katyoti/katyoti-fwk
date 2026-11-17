// SPDX-License-Identifier: Apache-2.0

// Package core is the panel server: it takes each display
// panel's events and inputs off the bus, appends them to the panel's log,
// folds them into its snapshot, coalesces them to the screens following it
// and prunes the ended panels after their type's retention.
package core

import (
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/panel"
	"github.com/katyoti/katyoti-fwk/internal/core/render"
	panelapi "github.com/katyoti/katyoti-fwk/internal/extensions/panelserver/api"
)

// fold takes one event, in its sequence order, into the snapshot: where the
// panel stands, and the events a screen folds to draw it, compacted so a
// screen folding them draws what it would have drawn folding the whole log
// (display-lib reducePanel): an ask is dropped once a later event ends it, a
// progress once a later one says how far, and an op on a node once a later
// replace or remove of that node supersedes it - a remove is never dropped,
// since a replace of a removed node does not bring it back.
func fold(s *panelapi.Snapshot, e panel.Event, at time.Time) {
	s.Seq = e.Seq
	s.Updated = at
	// The run sends nothing when it takes the person's answer: what it sends
	// next says it runs again (as reducePanel reads it).
	if s.State == panel.StateAsking && (e.Kind == panel.KindOps || e.Kind == panel.KindProgress) {
		s.State = panel.StateOpen
		s.Waiting = false
	}
	switch e.Kind {
	case panel.KindOpen:
		s.State = panel.StateOpen
		s.Opened = at
		if e.Open != nil {
			s.Type, s.Title, s.Owner, s.Parent, s.Object = e.Open.Type, e.Open.Title, e.Open.Owner, e.Open.Parent, e.Open.Object
		}
		s.Events = []panel.Event{e}
		return
	case panel.KindOps:
		s.Events = dropKinds(s.Events, panel.KindAsk)
		s.Events = supersede(s.Events, e.Ops)
	case panel.KindProgress:
		s.Events = dropKinds(s.Events, panel.KindAsk, panel.KindProgress)
	case panel.KindAsk:
		s.State = panel.StateAsking
		s.Waiting = true
		s.Events = dropKinds(s.Events, panel.KindAsk)
	case panel.KindComplete:
		s.State = panel.StateComplete
		s.Waiting = false
		s.Ended = at
		s.Events = dropKinds(s.Events, panel.KindAsk)
	case panel.KindClose:
		s.State = panel.StateClosed
		s.Waiting = false
		if s.Ended.IsZero() {
			s.Ended = at
		}
	}
	s.Events = append(s.Events, e)
}

// take records one input: an ask it answers no longer waits. The snapshot's
// sequence does not move - an input is no event out.
func take(s *panelapi.Snapshot, at time.Time) {
	s.Inputs++
	s.Updated = at
	if s.State == panel.StateAsking {
		s.Waiting = false
	}
}

// dropKinds removes the events of the kinds given.
func dropKinds(events []panel.Event, kinds ...panel.Kind) []panel.Event {
	out := events[:0]
	for _, e := range events {
		drop := false
		for _, k := range kinds {
			if e.Kind == k {
				drop = true
				break
			}
		}
		if !drop {
			out = append(out, e)
		}
	}
	return out
}

// supersede drops the earlier ops a later replace or remove of their node
// makes moot (replace, merge, append, fail on that node), and an ops event
// left with none.
func supersede(events []panel.Event, later []render.Op) []panel.Event {
	gone := map[string]bool{}
	for _, o := range later {
		if o.Target != "" && (o.Op == render.OpReplace || o.Op == render.OpRemove) {
			gone[o.Target] = true
		}
	}
	if len(gone) == 0 {
		return events
	}
	out := events[:0]
	for _, e := range events {
		if e.Kind != panel.KindOps {
			out = append(out, e)
			continue
		}
		var kept []render.Op
		for _, o := range e.Ops {
			if gone[o.Target] && o.Op != render.OpRemove && o.Op != render.OpComplete {
				continue
			}
			kept = append(kept, o)
		}
		if len(kept) == 0 {
			continue
		}
		e.Ops = kept
		out = append(out, e)
	}
	return out
}
