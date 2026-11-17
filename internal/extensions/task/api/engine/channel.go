// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"context"
	"slices"
	"sync"
)

// channelKey is the context key for storing the shared Channel.
type channelKey struct{}

// WithChannel stores a Channel in the context.
// Used by TaskService to make the shared Channel available to factories.
func WithChannel(ctx context.Context, ch *Channel) context.Context {
	return context.WithValue(ctx, channelKey{}, ch)
}

// ChannelFrom retrieves the Channel from context. Returns nil if not set.
func ChannelFrom(ctx context.Context) *Channel {
	ch, _ := ctx.Value(channelKey{}).(*Channel)
	return ch
}

// runIDKey is the context key for the executing run's id.
type runIDKey struct{}

// WithRunID stamps the executing run's id onto the context. The executor sets
// this for every machine action so a strategy without a loop (OneShot:
// simulator, program) can still resolve its own run id — e.g. the comms service
// needs it to suspend/route a child's question.
func WithRunID(ctx context.Context, runID string) context.Context {
	if runID == "" {
		return ctx
	}
	return context.WithValue(ctx, runIDKey{}, runID)
}

// RunIDFromContext retrieves the executing run's id, or "" if unset.
func RunIDFromContext(ctx context.Context) string {
	id, _ := ctx.Value(runIDKey{}).(string)
	return id
}

// Channel is the unified event routing layer. It sits between EventBus and
// per-executor MachineInboxes, managing subscriptions and routing events.
//
// Key responsibilities:
//   - Route events from EventBus to matching executor inboxes
//   - Collect undeliverable events in the Dead Event Queue
//   - Dispatch outgoing events (emit buffer → EventBus)
//
// Phase 1: local EventBus only. Phase 2: KTSP bridge for remote routing.
type Channel struct {
	bus EventBus
	mu  sync.RWMutex
	reg map[string]*inboxEntry // runID → inbox + unsub functions
}

// inboxEntry tracks a registered executor's inbox and its EventBus subscriptions.
type inboxEntry struct {
	inbox  *MachineInbox
	unsubs []func() // one per WaitCondition
	topics []string // the subscribed topics, one per WaitCondition
}

// NewChannel creates a channel backed by the given EventBus.
func NewChannel(bus EventBus) *Channel {
	if bus == nil {
		bus = NopEventBus{}
	}
	return &Channel{
		bus: bus,
		reg: make(map[string]*inboxEntry),
	}
}

// Register adds an executor's inbox to the channel for event delivery.
// Must be called before Subscribe.
func (c *Channel) Register(runID string, inbox *MachineInbox) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.reg[runID] = &inboxEntry{inbox: inbox}
}

// Unregister removes an executor's inbox and cancels all its EventBus subscriptions.
func (c *Channel) Unregister(runID string) {
	c.mu.Lock()
	entry, ok := c.reg[runID]
	if ok {
		delete(c.reg, runID)
	}
	c.mu.Unlock()

	if ok {
		for _, unsub := range entry.unsubs {
			if unsub != nil {
				unsub()
			}
		}
	}
}

// Wait registers a per-executor WaitSpec for a runID, creating EventBus
// subscriptions that route matching events to the registered inbox. The
// suspension terminates when the WaitSpec matches; the returned unsub
// cancels the subscriptions. Renamed from Channel.Subscribe in D9 to
// disambiguate from EventBus.Subscribe (which is long-lived pub/sub):
// Channel.Wait is the suspend-until-matched primitive that backs every
// `Wait:` in a StepResult. The two were named the same pre-D9 and call
// sites confused which lifecycle they were getting.
func (c *Channel) Wait(runID string, spec *WaitSpec) func() {
	c.mu.RLock()
	entry, ok := c.reg[runID]
	c.mu.RUnlock()
	if !ok || spec == nil {
		return func() {}
	}

	// The same topics as the wait before (a recheck waiting again): the live
	// subscriptions are kept — tearing them down and subscribing again would
	// drop an event published in between. Only the conditions are updated.
	topics := make([]string, len(spec.Conditions))
	for i, cond := range spec.Conditions {
		topics[i] = cond.Topic
	}
	c.mu.RLock()
	same := len(entry.unsubs) > 0 && slices.Equal(entry.topics, topics)
	kept := entry.unsubs
	c.mu.RUnlock()
	if same {
		entry.inbox.Retarget(spec)
		return func() {
			for _, unsub := range kept {
				if unsub != nil {
					unsub()
				}
			}
		}
	}

	// Cancel previous subscriptions
	for _, unsub := range entry.unsubs {
		if unsub != nil {
			unsub()
		}
	}

	// Update inbox subscription filters
	entry.inbox.UpdateSubs(spec)

	// Create EventBus subscriptions
	unsubs := make([]func(), len(spec.Conditions))
	for idx, cond := range spec.Conditions {
		topic := cond.Topic
		inbox := entry.inbox
		unsub := c.bus.Subscribe(topic, func(t string, payload any) {
			inbox.Push(Event{
				Type:    t,
				Payload: payload,
			})
		})
		entry.inbox.SetUnsub(idx, unsub)
		unsubs[idx] = unsub
	}

	c.mu.Lock()
	entry.unsubs = unsubs
	entry.topics = topics
	c.mu.Unlock()

	return func() {
		for _, unsub := range unsubs {
			if unsub != nil {
				unsub()
			}
		}
	}
}

// EndWait ends runID's wait: its subscriptions are removed and the events
// still queued for it dropped.
func (c *Channel) EndWait(runID string) {
	c.mu.Lock()
	entry, ok := c.reg[runID]
	if ok {
		entry.unsubs = nil
		entry.topics = nil
	}
	c.mu.Unlock()
	if !ok {
		return
	}
	entry.inbox.UpdateSubs(nil) // unsubscribes them (the inbox holds the same funcs)
	entry.inbox.Drop()
}

// Dispatch publishes events through the EventBus. Called by the executor
// during the commit phase to make emitted events visible.
func (c *Channel) Dispatch(events []Event) {
	for i := range events {
		_ = c.bus.Publish(events[i].Type, events[i].Payload)
	}
}

// Bus returns the underlying EventBus for direct access when needed
// (e.g., SessionRecorder subscriptions that bypass Channel routing).
func (c *Channel) Bus() EventBus {
	return c.bus
}
