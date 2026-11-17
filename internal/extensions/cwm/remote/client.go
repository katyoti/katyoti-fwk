// SPDX-License-Identifier: Apache-2.0

// Package remote is the bus-backed CWMService for binaries that do not host
// CWM: every method is one round-trip to ktw-agents, which does. A sidecar
// wires RemoteExtension and its services resolve CWMService the standard way
// — application.UseService[cwmapi.CWMService](ctx) — without knowing the
// service is elsewhere.
//
// The methods are GENERATED from the interface (zz_client.go, `go generate`);
// TestGeneratedClientIsCurrent refuses a stale file.
//
// HOST vs CONSUMER: a binary either hosts CWM (cwm/local) or consumes it
// (RemoteExtension). Never both.
package remote

//go:generate go run ./gen

import (
	"context"

	"github.com/katyoti/katyoti-fwk/internal/appcore/busclient"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// Host is the binary that publishes CWMService.
const Host = "ktw-agents"

// New returns the client addressing CWM on its host.
func New() Client { return NewClient(busclient.New(Host, cwmapi.CWMServiceDef)) }

// RemoteExtension registers the client as this binary's CWMService.
func RemoteExtension() application.Extension {
	return application.DeclareSimpleService[cwmapi.CWMService](
		func(context.Context) (cwmapi.CWMService, error) { return New(), nil },
	)
}
