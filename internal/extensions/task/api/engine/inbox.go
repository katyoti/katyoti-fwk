// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"context"
	"path"
	"sync"
	"time"
)

// MachineInbox is a per-executor ordered event queue with subscription-based routing.
// Events from EventBus are pushed into the inbox only if they match an active subscription.
// The executor calls Get() to block until the WaitSpec is satisfied, and Pop() on commit.
type MachineInbox struct {
	mu        sync.Mutex
	queue     []Event       // ordered FIFO
	subs      []inboxSub    // active subscriptions
	collected []Event       // for WaitAll: events collected so far
	allMet    map[int]Event // for WaitAll: the conditions met so far, by condition
	paused    bool          // true during Step execution — events buffered
	notify    chan struct{} // signals new event arrival
}

type inboxSub struct {
	cond  WaitCondition
	unsub func() // EventBus unsubscribe function
}

// NewMachineInbox creates an inbox.
func NewMachineInbox() *MachineInbox {
	return &MachineInbox{
		notify: make(chan struct{}, 1),
	}
}

// Push adds an event to the inbox. Called by EventBus subscription handlers.
// Thread-safe. If inbox is paused, events are still queued (buffered).
func (i *MachineInbox) Push(event Event) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.queue = append(i.queue, event)
	// Non-blocking signal
	select {
	case i.notify <- struct{}{}:
	default:
	}
}

// Get blocks until the WaitSpec is satisfied or context is cancelled.
// Returns the matched events:
//   - WaitAll: all collected events (one per condition)
//   - WaitAny: single matched event
//   - WaitCustom: single matched event (caller re-calls Get for more)
//
// A spec that names no mode is refused (ErrWaitWithoutMode).
func (i *MachineInbox) Get(ctx context.Context, spec *WaitSpec) ([]Event, error) {
	if spec == nil {
		return nil, nil
	}
	if !spec.Mode.Valid() {
		return nil, ErrWaitWithoutMode
	}

	var timer *time.Timer
	var timeoutCh <-chan time.Time
	if spec.Timeout > 0 {
		timer = time.NewTimer(spec.Timeout)
		defer timer.Stop()
		timeoutCh = timer.C
	}

	for {
		// Check for matches in current queue
		if matched, ok := i.tryMatch(spec); ok {
			return matched, nil
		}

		// Wait for new event, timeout, or cancellation
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-timeoutCh:
			label := spec.TimeoutLabel
			if label == "" {
				label = "timeout"
			}
			return []Event{{Type: "timeout", Label: label}}, nil
		case <-i.notify:
			// New event arrived — re-check
		}
	}
}

// tryMatch checks the queue for events satisfying the WaitSpec.
// Returns matched events and true if satisfied, or nil and false.
func (i *MachineInbox) tryMatch(spec *WaitSpec) ([]Event, bool) {
	i.mu.Lock()
	defer i.mu.Unlock()

	switch spec.Mode {
	case WaitAny:
		for idx, evt := range i.queue {
			for _, cond := range spec.Conditions {
				if matchesCondition(evt, cond) {
					evt.Label = cond.Label
					// Move to collected, remove from queue
					i.collected = []Event{evt}
					i.queue = append(i.queue[:idx], i.queue[idx+1:]...)
					return i.collected, true
				}
			}
		}
		return nil, false

	case WaitAll:
		// Each condition met once, by condition (not by label: conditions
		// without labels are distinct conditions); delivered in condition
		// order when all are.
		if i.allMet == nil {
			i.allMet = map[int]Event{}
		}
		remaining := i.queue[:0]
		for _, evt := range i.queue {
			matched := false
			for ci, cond := range spec.Conditions {
				if _, done := i.allMet[ci]; done {
					continue // already satisfied
				}
				if matchesCondition(evt, cond) {
					evt.Label = cond.Label
					i.allMet[ci] = evt
					matched = true
					break
				}
			}
			if !matched {
				remaining = append(remaining, evt)
			}
		}
		i.queue = remaining
		if len(i.allMet) == len(spec.Conditions) {
			out := make([]Event, len(spec.Conditions))
			for ci := range spec.Conditions {
				out[ci] = i.allMet[ci]
			}
			i.collected, i.allMet = out, nil
			result := make([]Event, len(out))
			copy(result, out)
			return result, true
		}
		return nil, false

	case WaitCustom:
		// Deliver first matching event — machine decides what to do
		for idx, evt := range i.queue {
			for _, cond := range spec.Conditions {
				if matchesCondition(evt, cond) {
					evt.Label = cond.Label
					i.collected = []Event{evt}
					i.queue = append(i.queue[:idx], i.queue[idx+1:]...)
					return i.collected, true
				}
			}
		}
		return nil, false
	}

	return nil, false
}

// Drop removes every queued event (its wait ended).
func (i *MachineInbox) Drop() {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.queue = nil
	i.collected = nil
	i.allMet = nil
}

// Pop removes consumed events. Called during commit.
func (i *MachineInbox) Pop() {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.collected = nil
	i.allMet = nil
}

// Pause stops delivering events to Get(). Events still queue but Get() won't see them.
// Used during Step execution to ensure no events arrive mid-step.
func (i *MachineInbox) Pause() {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.paused = true
}

// Resume allows Get() to see queued events again.
func (i *MachineInbox) Resume() {
	i.mu.Lock()
	i.paused = false
	i.mu.Unlock()
	// Signal in case events arrived while paused
	select {
	case i.notify <- struct{}{}:
	default:
	}
}

// UpdateSubs replaces the active subscriptions. Called after each step
// when the WaitSpec changes. Returns the unsubscribe functions from EventBus.
// The caller (executor) is responsible for subscribing to EventBus and calling Push.
func (i *MachineInbox) UpdateSubs(spec *WaitSpec) {
	i.mu.Lock()
	defer i.mu.Unlock()
	// Cancel old subscriptions
	for _, sub := range i.subs {
		if sub.unsub != nil {
			sub.unsub()
		}
	}
	i.subs = nil
	i.collected = nil
	i.allMet = nil
	if spec == nil {
		return
	}
	// Record conditions (actual EventBus subscription is done by executor)
	for _, cond := range spec.Conditions {
		i.subs = append(i.subs, inboxSub{cond: cond})
	}
}

// Retarget replaces the conditions of the current subscriptions — same topics,
// one per index — keeping their EventBus subscriptions and the queued events.
func (i *MachineInbox) Retarget(spec *WaitSpec) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.collected = nil
	i.allMet = nil
	for idx, cond := range spec.Conditions {
		if idx < len(i.subs) {
			i.subs[idx].cond = cond
		}
	}
}

// SetUnsub stores the EventBus unsubscribe function for a condition index.
func (i *MachineInbox) SetUnsub(idx int, unsub func()) {
	i.mu.Lock()
	defer i.mu.Unlock()
	if idx < len(i.subs) {
		i.subs[idx].unsub = unsub
	}
}

// matchesCondition checks if an event matches a WaitCondition.
func matchesCondition(evt Event, cond WaitCondition) bool {
	// Topic match (supports glob via path.Match)
	if matched, _ := path.Match(cond.Topic, evt.Type); !matched {
		return false
	}
	// Property filter (all must match)
	if len(cond.Filter) > 0 {
		props, ok := evt.Payload.(map[string]any)
		if !ok {
			return false
		}
		for k, v := range cond.Filter {
			pv, exists := props[k]
			if !exists {
				return false
			}
			if str, ok := pv.(string); !ok || str != v {
				return false
			}
		}
	}
	return true
}
