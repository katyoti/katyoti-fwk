// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/extensions/displaybridge/api"
)

// schemaBridge implements api.SchemaBridge by forwarding GetWidgetURL to
// bun's /api/widget-url handler. bun owns URL composition because it
// knows its own port and mints the per-session token.
//
// **2026-05-22 schema-realign cleanup**: the package used to expose 13
// methods (Parse, Validate, ListTypes, codegen, etc.) as Phase-1 stubs of
// refined Architecture C. None ever had a backend; all had zero callers.
// made the Go side canonical for
// everything that wasn't inherently bun-side, so the forwarders became
// unreachable dead code and were removed. Only `GetWidgetURL` remains
// because URL+token composition is genuinely a bun responsibility.
//
// The supervisor still owns bun's lifetime (lazy-spawn on first call,
// SIGINT on Close). The schemaCache + sseSubscriber are retained for
// observability (the SSE subscriber's reconnect-loop still proves bun is
// reachable + alive), even though no forwarder uses the cache anymore.
// A follow-up may strip them too once the dead-code analysis confirms
// nothing else reads them.
type schemaBridge struct {
	client     *httpClient
	cache      *schemaCache
	sse        *sseSubscriber      // nil if SSE not configured (tests skipping it)
	supervisor *SupervisorEndpoint // nil for tests with custom endpoints
}

// New constructs the published SchemaBridge. Wires a SupervisorEndpoint
// that owns the bun process; NOTHING is spawned here. The first call that
// needs bun (GetWidgetURL) starts it — port allocation, session token,
// healthz probe — and later calls reuse the running process, so a binary
// that boots the bridge but never renders a display runs no bun.
//
// The SSE subscriber starts after that first successful start, never
// before: its reconnect loop would otherwise be the first user and spawn
// bun at boot.
//
// The ctx is the application-scope context: it lives as long as the
// universe core process. Bun's lifetime is bounded by it — context
// cancellation tears bun down via SIGINT.
func New(ctx context.Context) (api.SchemaBridge, error) {
	homePath := application.Env().HomeDir()
	return newWithSupervisor(NewSupervisorEndpoint(BunSpec(), homePath, ctx)), nil
}

// newWithSupervisor builds the bridge over a supervisor endpoint and hooks
// the SSE subscriber's start onto the endpoint's first successful start.
func newWithSupervisor(endpoint *SupervisorEndpoint) *schemaBridge {
	cache := newSchemaCache(schemaCacheCapacity)
	sse := newSSESubscriber(endpoint, cache)
	endpoint.onStarted = sse.Start
	return &schemaBridge{
		client:     newHTTPClient(endpoint),
		cache:      cache,
		sse:        sse,
		supervisor: endpoint,
	}
}

// Close terminates the SSE subscriber, then closes the supervisor (which
// SIGINTs bun + waits up to 5s for graceful exit). Safe to call multiple
// times. The publish framework invokes this on application shutdown via
// the io.Closer convention.
func (b *schemaBridge) Close() error {
	if b.sse != nil {
		b.sse.Stop()
	}
	if b.supervisor != nil {
		_ = b.supervisor.Close()
	}
	return nil
}

// newWithEndpoint builds a schemaBridge with a custom EndpointProvider.
// Used by tests (point at httptest.NewServer) and by future supervisor
// integration code that swaps the static EnvEndpoint for a live
// supervisor-backed provider.
//
// Does NOT start the SSE subscriber — tests that need SSE behavior call
// b.sse.Start() explicitly. This keeps non-SSE tests fast and free of
// background goroutines spinning on a 404 /sse/schema-changed endpoint.
func newWithEndpoint(endpoint EndpointProvider, opts ...httpClientOpt) *schemaBridge {
	cache := newSchemaCache(schemaCacheCapacity)
	return &schemaBridge{
		client: newHTTPClient(endpoint, opts...),
		cache:  cache,
		sse:    newSSESubscriber(endpoint, cache),
	}
}

// --- The only live slot ---

// GetWidgetURL forwards to POST /api/widget-url. Idempotent (read-only on
// bun's side — composes URL + reads the per-session token). bun's
// response embeds the token as a `?token=<...>` query param on the URL.
func (b *schemaBridge) GetWidgetURL(ctx context.Context, in api.GetWidgetURLInput) (api.WidgetURLOutput, error) {
	if in.Kind == "" {
		return api.WidgetURLOutput{}, fmt.Errorf("GetWidgetURL: kind is required")
	}
	var out api.WidgetURLOutput
	if err := b.client.PostJSON(ctx, "/api/widget-url", in, &out); err != nil {
		return api.WidgetURLOutput{}, fmt.Errorf("GetWidgetURL [%s]: %w", in.Kind, err)
	}
	return out, nil
}
