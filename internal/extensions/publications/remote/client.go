// SPDX-License-Identifier: Apache-2.0

// Package remote is the bus-backed PublishersService client: a consumer binary
// registers it so its callers resolve the PublishersService the
// standard way (application.UseService) and each method round-trips to the
// binary hosting the real service (ktw-agents, publications/local) over the
// message bus. It imports only the publications api.
//
// HOST vs CONSUMER: a binary either hosts the service (publications/local) or
// consumes it (RemoteExtension here) - never both.
package remote

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
	corereflect "github.com/katyoti/katyoti-fwk/internal/core/reflect"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/publications/api"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp/messagebus"
)

// slot is the named-slot prefix of the PublishersService ktw-agents publishes.
const slot = "ktw-agents:publishers/"

// timeouts are the deadlines the methods declare: a consumer's handler context
// arrives with none, and the bus would fall back to its short default.
var timeouts = func() map[string]time.Duration {
	m := map[string]time.Duration{}
	for name, def := range api.PublishersServiceDef.Methods {
		if def.TimeoutS > 0 {
			m[corereflect.ToKebab(name)] = time.Duration(def.TimeoutS) * time.Second
		}
	}
	return m
}()

type busClient struct{}

var _ api.PublishersService = busClient{}

func (busClient) Connectors(ctx context.Context, in api.ConnectorsInput) (api.ConnectorsOutput, error) {
	return send[api.ConnectorsOutput](ctx, "connectors", in)
}

func (busClient) Publish(ctx context.Context, in api.PublishRequest) (api.PublishOutput, error) {
	return send[api.PublishOutput](ctx, "publish", in)
}

func (busClient) NewPublicationPage(ctx context.Context, in api.NewPublicationPageInput) (api.PublicationPageOutput, error) {
	return send[api.PublicationPageOutput](ctx, "new-publication-page", in)
}

func (busClient) PublicationNew(ctx context.Context, in api.PublicationNewInput) (api.PublicationNewOutput, error) {
	return send[api.PublicationNewOutput](ctx, "publication-new", in)
}

// send calls one method over the bus and decodes its answer.
func send[O any](ctx context.Context, method string, in any) (O, error) {
	var zero O
	bus, err := application.UseService[mbapi.MessageBus](ctx)
	if err != nil {
		return zero, fmt.Errorf("publications client: message bus unavailable: %w", err)
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
	if d, ok := timeouts[method]; ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, d)
		defer cancel()
	}
	raw, err := messagebus.NewSender(bus, tenantID, universeID).Call(ctx, slot+method, in)
	if err != nil {
		return zero, fmt.Errorf("%s%s: %w", slot, method, err)
	}
	var out O
	if err := json.Unmarshal(raw, &out); err != nil {
		return zero, fmt.Errorf("decode %s%s answer: %w", slot, method, err)
	}
	return out, nil
}

// RemoteExtension registers the bus-backed client as the in-process
// PublishersService of a consumer binary.
func RemoteExtension() application.Extension {
	return application.DeclareSimpleService[api.PublishersService](
		func(_ context.Context) (api.PublishersService, error) { return busClient{}, nil },
	)
}
