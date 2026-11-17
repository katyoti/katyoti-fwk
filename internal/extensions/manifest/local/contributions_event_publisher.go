// SPDX-License-Identifier: Apache-2.0

package local

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	"github.com/katyoti/katyoti-fwk/internal/kts/subjects"

	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
)

// NewNATSContributionsEventPublisher returns a
// `contribapi.ContributionsEventPublisher` that emits each
// `ContributionsChangedEvent` to the universe-scoped subject
// `ktw.<tenant>.universe.<u>.event.contributions-changed` via the
// passed-in MessageBus.
//
// Wired by ktw-hub's bootstrap once the embedded NATS server is up and
// `mbapi.MessageBus` is registered. Other binaries (ktw-agents, ktw-gui)
// subscribe to the same subject via `subjects.ContributionsChangedSubject`
// and `bus.Subscribe(subject, handler)` — the publisher is purely a
// catalog → bus adapter.
//
// Publish failures are surfaced to the caller (CatalogImpl logs and
// continues per its non-fatal contract). A short publish-timeout
// keeps a slow bus from blocking the catalog's mutex window.
func NewNATSContributionsEventPublisher(bus mbapi.MessageBus, tenantID, universeID string) contribapi.ContributionsEventPublisher {
	return &natsContribsPublisher{
		bus:        bus,
		tenantID:   tenantID,
		universeID: universeID,
	}
}

type natsContribsPublisher struct {
	bus        mbapi.MessageBus
	tenantID   string
	universeID string
}

func (p *natsContribsPublisher) PublishContributionsChanged(ev contribapi.ContributionsChangedEvent) error {
	if p.bus == nil {
		return fmt.Errorf("contributions publisher: nil MessageBus")
	}
	payload, err := json.Marshal(ev)
	if err != nil {
		return fmt.Errorf("contributions publisher: marshal event: %w", err)
	}
	subject := subjects.ContributionsChangedSubject(p.tenantID, p.universeID)
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	if err := p.bus.Publish(ctx, subject, payload); err != nil {
		return fmt.Errorf("contributions publisher: publish %s: %w", subject, err)
	}
	return nil
}
