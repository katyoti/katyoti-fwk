// SPDX-License-Identifier: Apache-2.0

package sm

import (
	"encoding/json"
	"sync"
	"sync/atomic"
	"time"
)

// JournalEvent is an event emitted to the session journal.
type JournalEvent struct {
	Seq           uint64          `json:"seq"`
	SessionID     string          `json:"sessionId"`
	ContainerName string          `json:"containerName"`
	Timestamp     time.Time       `json:"timestamp"`
	Type          string          `json:"type"`
	Data          json.RawMessage `json:"data,omitempty"`
}

// Subscription receives journal events via a channel.
type Subscription struct {
	Events chan JournalEvent
	filter func(JournalEvent) bool
	done   chan struct{}
}

// Close stops the subscription.
func (sub *Subscription) Close() {
	close(sub.done)
}

// Journal aggregates events from all containers in a session.
// Subscribers receive events via channels with optional filtering.
type Journal struct {
	sessionID string
	seq       atomic.Uint64

	mu          sync.RWMutex
	subscribers []*Subscription
	closed      bool
}

// NewJournal creates a new journal for the given session.
func NewJournal(sessionID string) *Journal {
	return &Journal{
		sessionID: sessionID,
	}
}

// Emit records an event and fans it out to all subscribers.
func (j *Journal) Emit(containerName, eventType string, data json.RawMessage) uint64 {
	seq := j.seq.Add(1)

	event := JournalEvent{
		Seq:           seq,
		SessionID:     j.sessionID,
		ContainerName: containerName,
		Timestamp:     time.Now(),
		Type:          eventType,
		Data:          data,
	}

	j.mu.RLock()
	defer j.mu.RUnlock()

	for _, sub := range j.subscribers {
		if sub.filter != nil && !sub.filter(event) {
			continue
		}
		select {
		case sub.Events <- event:
		case <-sub.done:
			// subscriber closed, skip
		default:
			// channel full, drop event (non-blocking)
		}
	}

	return seq
}

// Subscribe creates a new subscription. Events matching the filter are sent
// to the returned channel. Pass nil filter to receive all events.
// Buffer size controls the channel capacity.
func (j *Journal) Subscribe(bufSize int, filter func(JournalEvent) bool) *Subscription {
	if bufSize <= 0 {
		bufSize = 64
	}
	sub := &Subscription{
		Events: make(chan JournalEvent, bufSize),
		filter: filter,
		done:   make(chan struct{}),
	}

	j.mu.Lock()
	j.subscribers = append(j.subscribers, sub)
	j.mu.Unlock()

	return sub
}

// Unsubscribe removes a subscription and closes its channel.
func (j *Journal) Unsubscribe(sub *Subscription) {
	j.mu.Lock()
	defer j.mu.Unlock()

	for i, s := range j.subscribers {
		if s == sub {
			j.subscribers = append(j.subscribers[:i], j.subscribers[i+1:]...)
			break
		}
	}
}

// Close shuts down the journal and all subscriptions.
func (j *Journal) Close() {
	j.mu.Lock()
	defer j.mu.Unlock()

	j.closed = true
	for _, sub := range j.subscribers {
		close(sub.Events)
	}
	j.subscribers = nil
}

// Seq returns the current sequence number.
func (j *Journal) Seq() uint64 {
	return j.seq.Load()
}
