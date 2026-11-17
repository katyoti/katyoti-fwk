// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"slices"
	"sync"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	eventsapi "github.com/katyoti/katyoti-fwk/internal/extensions/events/api"
	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

// eventBridgeHook forwards, while a run waits, the external events it waits on
// (WaitCondition.EventURI — an event of another process, e.g. an inbox request
// updated) onto the in-process topic the executor listens to. Subscribed when
// the run suspends; KEPT while the run waits on the same events again (a
// recheck that found nothing — re-subscribing would drop an event published
// in between); replaced when it waits on others; ended with the run (the
// subscription lives on the run's context). A run restored after a restart
// suspends again through the same hook.
type eventBridgeHook struct {
	bus    engine.EventBus
	events func(context.Context) (eventsapi.EventService, error) // the EventService (tests replace it)
	fail   func(error)                                           // fails its run with a cause (nil: log only)

	mu   sync.Mutex
	uris []string // the external events subscribed, in condition order
	stop context.CancelFunc
}

// eventServiceOf resolves the EventService a bridge subscribes through (tests
// replace it).
var eventServiceOf = func(ctx context.Context) (eventsapi.EventService, error) {
	return application.UseService[eventsapi.EventService](ctx)
}

func newEventBridgeHook(bus engine.EventBus, fail func(error)) *eventBridgeHook {
	return &eventBridgeHook{bus: bus, events: eventServiceOf, fail: fail}
}

// cannotWait: the run waits on an event it can never be told — it fails with
// why (when its executor can fail it), else it is logged.
func (h *eventBridgeHook) cannotWait(runID string, err error) {
	if h.fail != nil {
		h.fail(err)
		return
	}
	slog.Warn("tasks: a run waits on an event it cannot be told", "run_id", runID, "err", err)
}

func (*eventBridgeHook) OnPreStep(context.Context, *engine.State, engine.Event) error         { return nil }
func (*eventBridgeHook) OnPostStep(context.Context, *engine.State, *engine.StepResult, error) {}
func (*eventBridgeHook) OnResume(context.Context, string, engine.Event)                       {}

func (h *eventBridgeHook) OnSuspend(ctx context.Context, runID string, w *engine.WaitSpec) {
	if w == nil || h.bus == nil {
		return
	}
	var uris []string
	for _, c := range w.Conditions {
		if c.EventURI != "" {
			uris = append(uris, c.EventURI)
		}
	}
	h.mu.Lock()
	same := h.stop != nil && slices.Equal(h.uris, uris)
	h.mu.Unlock()
	if same || len(uris) == 0 {
		return // already subscribed; or nothing external to wait on
	}
	ev, err := h.events(ctx)
	if err != nil {
		h.cannotWait(runID, fmt.Errorf("it waits on %v but no event service is reachable: %w", uris, err))
		return
	}
	h.reset()
	subCtx, cancel := context.WithCancel(ctx)
	h.mu.Lock()
	h.stop, h.uris = cancel, uris
	h.mu.Unlock()
	for _, c := range w.Conditions {
		if c.EventURI == "" {
			continue
		}
		topic := c.Topic
		if _, err := ev.Subscribe(subCtx, c.EventURI, "*", func(_ context.Context, payload json.RawMessage) {
			h.bus.Publish(topic, payload)
		}); err != nil {
			h.cannotWait(runID, fmt.Errorf("it waits on %s but it could not be subscribed: %w", c.EventURI, err))
			return
		}
	}
}

// reset drops the subscriptions of the previous wait.
func (h *eventBridgeHook) reset() {
	h.mu.Lock()
	stop := h.stop
	h.stop, h.uris = nil, nil
	h.mu.Unlock()
	if stop != nil {
		stop()
	}
}
