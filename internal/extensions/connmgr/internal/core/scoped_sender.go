// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"encoding/json"
	"strings"

	cmapi "github.com/katyoti/katyoti-fwk/internal/extensions/connmgr/api"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp"
)

// busProvider returns the provider token a bus-routed sender must address for
// this spec: the explicit self-declared bus identity (SpawnSpec.Provider) when
// set, otherwise the connmgr-local Name (which equals the binary's app name for
// the common single-instance case).
func busProvider(spec cmapi.SpawnSpec) string {
	if spec.Provider != "" {
		return spec.Provider
	}
	return spec.Name
}

// localScopedSender binds a provider-routed sender to a single provider so
// callers can use the LOCAL ktsp.Sender path grammar (`slot/method`) that the
// Sender interface documents — regardless of transport.
//
// A connmgr connection is always bound to ONE sidecar, so the caller has no
// reason to repeat the provider in every path. The stdio session already
// speaks the local form (its ParsePath splits on '/'); the NATS/remote
// messagebus.Sender does NOT — its grammar requires a `<provider>:` prefix
// (messagebus/path.go), so an unqualified path errors with "missing ':'
// separator". This wrapper closes that gap for the bus transports: it
// qualifies an unqualified path with the bound provider before the inner
// sender sees it. A path that already carries a ':' is passed through
// untouched, so callers that still write the fully-qualified form keep
// working.
//
// Stdio senders are returned unwrapped (see attemptSpawn) — they need no
// qualification.
type localScopedSender struct {
	inner    ktsp.Sender
	provider string
}

// newLocalScopedSender wraps inner so it honours the local path contract for
// the given provider. An empty provider returns inner unchanged (nothing to
// bind to).
func newLocalScopedSender(inner ktsp.Sender, provider string) ktsp.Sender {
	if provider == "" || inner == nil {
		return inner
	}
	return &localScopedSender{inner: inner, provider: provider}
}

// qualify prepends the bound provider to an unqualified (`slot/method`) path.
// A path that already contains ':' is assumed provider-qualified and returned
// as-is.
func (s *localScopedSender) qualify(path string) string {
	if strings.ContainsRune(path, ':') {
		return path
	}
	return s.provider + ":" + path
}

func (s *localScopedSender) Call(ctx context.Context, path string, params any, opts ...ktsp.CallOption) (json.RawMessage, error) {
	return s.inner.Call(ctx, s.qualify(path), params, opts...)
}

func (s *localScopedSender) Notify(ctx context.Context, path string, params any, opts ...ktsp.CallOption) error {
	return s.inner.Notify(ctx, s.qualify(path), params, opts...)
}
