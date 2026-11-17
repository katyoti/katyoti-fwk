// SPDX-License-Identifier: Apache-2.0

// Package core implements EventService as a thin facade over the
// NATS MessageBus.
package core

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
	eventsapi "github.com/katyoti/katyoti-fwk/internal/extensions/events/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	scopectx "github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp/messagebus"
)

// Config holds wire-format configuration. Empty for now — the
// facade resolves its bus dependency lazily from context per the
// application/UseService pattern.
type Config struct{}

// Service is the EventService facade.
type Service struct{}

// New constructs the facade. Bus resolution is lazy (per-call) so the
// service can be published before the bus binding exists.
func New(_ context.Context, _ *Config) (eventsapi.EventService, error) {
	return &Service{}, nil
}

// Subscribe registers handler against the NATS subject the EventURI
// resolves to. Context cancellation unsubscribes the handler.
func (s *Service) Subscribe(ctx context.Context, uri, scopeID string, handler eventsapi.Handler) (eventsapi.Subscription, error) {
	subject, err := uriToSubject(ctx, uri, scopeID)
	if err != nil {
		return nil, fmt.Errorf("EventService.Subscribe: %w", err)
	}
	slog.Info("EventService: subscribing", "uri", uri, "subject", subject)
	bus, err := application.UseService[mbapi.MessageBus](ctx)
	if err != nil {
		return nil, fmt.Errorf("EventService.Subscribe: resolve MessageBus: %w", err)
	}
	// Adapt the EventService.Handler (payload-only) to the
	// MessageBus.Handler (full Message). We drop the headers / reply
	// here — facade callers think in payload terms, not NATS terms.
	wrapped := mbapi.Handler(func(hctx context.Context, msg mbapi.Message) {
		handler(hctx, json.RawMessage(msg.Data()))
	})
	mbsub, err := bus.Subscribe(ctx, subject, wrapped)
	if err != nil {
		return nil, fmt.Errorf("EventService.Subscribe: bus subscribe %q: %w", subject, err)
	}
	return &subscriptionAdapter{inner: mbsub}, nil
}

// Publish emits payload on the subject the EventURI resolves to.
// payload is JSON-encoded via the standard library.
func (s *Service) Publish(ctx context.Context, uri, scopeID string, payload any) error {
	subject, err := uriToSubject(ctx, uri, scopeID)
	if err != nil {
		return fmt.Errorf("EventService.Publish: %w", err)
	}
	bus, err := application.UseService[mbapi.MessageBus](ctx)
	if err != nil {
		return fmt.Errorf("EventService.Publish: resolve MessageBus: %w", err)
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("EventService.Publish: encode payload: %w", err)
	}
	if err := bus.Publish(ctx, subject, body); err != nil {
		return fmt.Errorf("EventService.Publish: bus publish %q: %w", subject, err)
	}
	return nil
}

// uriToSubject parses the URI as an EventURI and maps it to a NATS
// subject using tenant/universe IDs from the context plus the
// caller-supplied scopeID. In-process callers — the agent's inbox
// tool, for one — invoke Subscribe/Publish with a context that never
// passed through a bus Send/Notify, so it carries no scope; the
// canonical resolver (KTW_TENANT_ID / Univers.yaml name) fills the
// gap. Both halves matter: a hardcoded fallback here would drift
// from the publisher's tokens (e.g. subscriber on
// `universe.default`, publisher on `universe.acme`) and events
// would silently never match; empty tokens (ktw..universe..) make
// NATS reject the subject outright.
func uriToSubject(ctx context.Context, uri, scopeID string) (string, error) {
	eu, err := mbapi.ParseEventURI(uri)
	if err != nil {
		return "", err
	}
	tenantID := scopectx.TenantFromContext(ctx)
	universeID := scopectx.UniverseFromContext(ctx)
	if tenantID == "" || universeID == "" {
		fallbackTenant, fallbackUniverse := cenv.ResolveTenantUniverse(application.Env().UniverseDir())
		if tenantID == "" {
			tenantID = fallbackTenant
		}
		if universeID == "" {
			universeID = fallbackUniverse
		}
	}
	return eu.Subject(tenantID, universeID, scopeID), nil
}

// subscriptionAdapter narrows the MessageBus Subscription (Drain /
// Unsubscribe / IsValid) to EventService's single-method shape.
type subscriptionAdapter struct {
	inner mbapi.Subscription
}

func (a *subscriptionAdapter) Unsubscribe() error {
	if a.inner == nil {
		return nil
	}
	return a.inner.Unsubscribe()
}
