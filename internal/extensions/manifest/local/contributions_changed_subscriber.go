// SPDX-License-Identifier: Apache-2.0

package local

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	"github.com/katyoti/katyoti-fwk/internal/kts/subjects"

	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
)

// SubscribeContributionsChanged installs a NATS subscription on the
// universe-scoped `contributions-changed` subject. Each received event
// triggers `onEvent` after a debounce window — bursty edits (e.g. a
// multi-file save firing 3 fsnotify events that the producer publishes
// in quick succession) collapse into a single onEvent call.
//
// Wired by ktw-agents (and ktw-gui in a follow-up) so SkillManager /
// catalog views refresh without polling. The returned Subscription is
// the caller's responsibility to Drain at shutdown.
//
// Errors during event JSON-decode are logged and ignored — a malformed
// payload should not silence the subscription.
//
// `debounce` is the quiet window before firing; 100 ms is the typical
// value (covers the publisher's 50 ms watcher debounce + transport
// jitter). Zero or negative disables debouncing (fires per event).
func SubscribeContributionsChanged(
	ctx context.Context,
	bus mbapi.MessageBus,
	tenantID, universeID string,
	debounce time.Duration,
	onEvent func(contribapi.ContributionsChangedEvent),
) (mbapi.Subscription, error) {
	if bus == nil {
		return nil, fmt.Errorf("contributions subscriber: nil MessageBus")
	}
	if onEvent == nil {
		return nil, fmt.Errorf("contributions subscriber: nil onEvent")
	}

	subject := subjects.ContributionsChangedSubject(tenantID, universeID)

	var (
		mu      sync.Mutex
		timer   *time.Timer
		pending contribapi.ContributionsChangedEvent
	)
	fire := func() {
		mu.Lock()
		ev := pending
		mu.Unlock()
		onEvent(ev)
	}

	handler := func(_ context.Context, msg mbapi.Message) {
		var ev contribapi.ContributionsChangedEvent
		if err := json.Unmarshal(msg.Data(), &ev); err != nil {
			slog.Warn("contributions subscriber: decode payload failed", "error", err, "subject", subject)
			return
		}
		mu.Lock()
		pending = ev
		if debounce <= 0 {
			mu.Unlock()
			onEvent(ev)
			return
		}
		if timer != nil {
			timer.Stop()
		}
		timer = time.AfterFunc(debounce, fire)
		mu.Unlock()
	}

	sub, err := bus.Subscribe(ctx, subject, handler)
	if err != nil {
		return nil, fmt.Errorf("contributions subscriber: subscribe %s: %w", subject, err)
	}
	return sub, nil
}
