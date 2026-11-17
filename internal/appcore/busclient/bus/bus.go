// SPDX-License-Identifier: Apache-2.0

// Package bus is the bus-backed busclient.SenderProvider: a request's Sender
// over the message bus, scoped to its tenant and universe, retrying through the
// hub so a lazy host is spawned on a first cold call.
//
// A binary that consumes another binary's service through a generated client
// wires Extension() once, beside the client's own extension.
package bus

import (
	"context"
	"errors"
	"fmt"

	"github.com/katyoti/katyoti-fwk/internal/appcore/busclient"
	"github.com/katyoti/katyoti-fwk/internal/appcore/hubclient"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	ktsapi "github.com/katyoti/katyoti-fwk/internal/kts/api"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp/messagebus"
)

type provider struct{}

// Sender builds the request's scoped Sender. Tenant and universe come from the
// ctx, else from the universe folder (the same resolution the publisher uses —
// divergent tokens would never match).
func (provider) Sender(ctx context.Context) (ktsapi.Sender, error) {
	b, err := application.UseService[mbapi.MessageBus](ctx)
	if err != nil {
		return nil, fmt.Errorf("message bus unavailable: %w", err)
	}
	tenantID := messagebus.TenantFromContext(ctx)
	universeID := messagebus.UniverseFromContext(ctx)
	if tenantID == "" || universeID == "" {
		ft, fu := cenv.ResolveTenantUniverse(application.Env().UniverseDir())
		if tenantID == "" {
			tenantID = ft
		}
		if universeID == "" {
			universeID = fu
		}
	}
	// The retry sender asks the hub to spawn a lazy host on a first cold call.
	return messagebus.NewRetrySender(messagebus.NewSender(b, tenantID, universeID), "ktw-hub"), nil
}

// Extension registers the bus-backed SenderProvider.
func Extension() application.Extension {
	return application.DeclareSimpleService[busclient.SenderProvider](
		func(context.Context) (busclient.SenderProvider, error) { return provider{}, nil },
	)
}

// RunningHubExtension declares a MessageBus for a binary run as a one-shot
// CLI: the RUNNING hub of the active universe (ModeReuse — a one-shot verb
// never spawns a universe). Served by the hub (`sc ktsp nats`), the binary is
// handed the hub's bus (PutService), which takes precedence over this factory.
// With no hub running, the error says what to do.
func RunningHubExtension() application.Extension {
	return application.DeclareSimpleService[mbapi.MessageBus](func(ctx context.Context) (mbapi.MessageBus, error) {
		env := application.Env()
		universe := "default"
		if m, err := cenv.LoadUniverseManifest(env.UniverseDir()); err == nil && m != nil && m.Name != "" {
			universe = m.Name
		}
		hc, err := hubclient.ConnectToHub(ctx, hubclient.ConnectOptions{DataDir: env.DataDir(), Mode: hubclient.ModeReuse})
		if errors.Is(err, hubclient.ErrNoHub) {
			return nil, fmt.Errorf("this command reaches universe %q through its hub and none is running — start the universe", universe)
		}
		if err != nil {
			return nil, fmt.Errorf("connect to universe %q's hub: %w", universe, err)
		}
		return hc.MessageBus(), nil
	})
}
