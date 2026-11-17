// SPDX-License-Identifier: Apache-2.0

package local

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
	coreschemaapi "github.com/katyoti/katyoti-fwk/internal/core/schema/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/schema/api"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp/messagebus"
)

// hubSchemaSlot is the named-slot prefix of the universe-wide SchemaService
// published by ktw-hub (the schema authority — see Extension()).
const hubSchemaSlot = "ktw-hub:schema/"

// busClient implements api.SchemaService by dispatching to the universe-wide
// SchemaService published by ktw-hub over the message bus. Consumer binaries
// register it (RemoteExtension) so in-process callers resolve SchemaService
// through application.UseService exactly like a local service — the bus
// round-trip is encapsulated here, NOT hand-rolled at every call site.
//
// Stateless: each method resolves the scoped bus sender from the request ctx
// (tenant/universe), so one shared instance serves every run.
type busClient struct{}

func (busClient) Parse(ctx context.Context, in coreschemaapi.ParseInput) (coreschemaapi.ParseOutput, error) {
	return schemaSend[coreschemaapi.ParseOutput](ctx, "Parse", in)
}

func (busClient) ParseFile(ctx context.Context, _ coreschemaapi.ParseFileInput) (coreschemaapi.ParseOutput, error) {
	return coreschemaapi.ParseOutput{}, fmt.Errorf("schema client: ParseFile is host-local only (not available over the bus)")
}

func (busClient) Validate(ctx context.Context, in coreschemaapi.ValidateInput) (coreschemaapi.ValidateOutput, error) {
	return schemaSend[coreschemaapi.ValidateOutput](ctx, "Validate", in)
}

func (busClient) RegisterDataType(_ context.Context, _ api.RegisterDataTypeInput) error {
	return fmt.Errorf("schema client: RegisterDataType is hub-internal (not callable by consumers)")
}

func (busClient) GetDataType(ctx context.Context, in api.GetDataTypeInput) (api.GetDataTypeOutput, error) {
	return schemaSend[api.GetDataTypeOutput](ctx, "GetDataType", in)
}

func (busClient) GetJSONSchema(ctx context.Context, in api.GetJSONSchemaInput) (api.GetJSONSchemaOutput, error) {
	return schemaSend[api.GetJSONSchemaOutput](ctx, "GetJSONSchema", in)
}

func (busClient) ValidateAgainst(ctx context.Context, in api.ValidateAgainstInput) (coreschemaapi.ValidateOutput, error) {
	return schemaSend[coreschemaapi.ValidateOutput](ctx, "ValidateAgainst", in)
}

func (busClient) GetDataTypeWithExtensions(ctx context.Context, in api.GetDataTypeWithExtensionsInput) (api.GetDataTypeOutput, error) {
	return schemaSend[api.GetDataTypeOutput](ctx, "GetDataTypeWithExtensions", in)
}

// schemaSend dispatches one SchemaService method on ktw-hub over the bus and
// decodes the reply into O. The single low-level bus seam for the client.
func schemaSend[O any](ctx context.Context, method string, in any) (O, error) {
	var zero O
	bus, err := application.UseService[mbapi.MessageBus](ctx)
	if err != nil {
		return zero, fmt.Errorf("schema client: message bus unavailable: %w", err)
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
	sender := messagebus.NewSender(bus, tenantID, universeID)
	raw, err := sender.Call(ctx, hubSchemaSlot+method, in)
	if err != nil {
		return zero, fmt.Errorf("%s%s: %w", hubSchemaSlot, method, err)
	}
	var out O
	if err := json.Unmarshal(raw, &out); err != nil {
		return zero, fmt.Errorf("decode %s%s response: %w", hubSchemaSlot, method, err)
	}
	return out, nil
}

// RemoteExtension registers the bus-backed SchemaService client as the
// in-process api.SchemaService for CONSUMER binaries (ktw-agents and friends)
// that do NOT host the real service. Callers then resolve it the standard way
// — application.UseService[api.SchemaService](ctx) — and the bus round-trip to
// ktw-hub stays encapsulated in the client.
//
// A binary is either the HOST (Extension(), which publishes the real service)
// or a CONSUMER (RemoteExtension()). Do not wire both in the same binary.
func RemoteExtension() application.Extension {
	return application.DeclareSimpleService[api.SchemaService](
		func(_ context.Context) (api.SchemaService, error) {
			return busClient{}, nil
		},
	)
}
