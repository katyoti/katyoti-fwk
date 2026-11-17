// SPDX-License-Identifier: Apache-2.0

// Package local wires concrete CRUD connections into the publishing
// pipeline. Each connection is a named instance on the shared
// `crud-service` slot (`<provider>:crud-service(<connection>)/<method>`).
// The one connection is `memory`, code-registered here: the in-process
// store behind the load-record / save-record / delete-record agent
// tools. Durable data is CWM objects, written through CWMService — no
// CRUD connection stands for it.
package local

import (
	"reflect"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/em"
	"github.com/katyoti/katyoti-fwk/internal/core/eventbus"
	"github.com/katyoti/katyoti-fwk/internal/core/publish"
	crudapi "github.com/katyoti/katyoti-fwk/internal/extensions/crud/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/crud/internal/types/memory"
)

// ConnectionNameMemory is the canonical instance name of the
// in-process memory connection. Address records as
// `memory:/<type>/<id>` (external form) →
// `<provider>:crud-service(memory)/<method>` (wire form).
const ConnectionNameMemory = "memory"

// MemoryConnectionExtension publishes the `memory` CRUDService
// connection — an in-process, per-universe-core record store. State
// lives for the lifetime of the hosting process; restarts clear it.
// Useful for demos, scratch data, and tests; durable data is CWM
// objects, not records on a CRUD connection.
func MemoryConnectionExtension() application.Extension {
	provider := func(_ *application.Application) (map[string]any, error) {
		return map[string]any{
			// Watch events stay process-local for v1 (NopEventBus) —
			// wire the real bus when a Watch consumer materialises.
			ConnectionNameMemory: memory.NewMemoryStore(ConnectionNameMemory, eventbus.NopEventBus{}),
		}, nil
	}

	ps := publish.PublishedService{
		Name:        "CRUDServiceMemory",
		Description: crudapi.CRUDServiceDef.Description(),
		Version:     crudapi.CRUDServiceDef.Version,
		ServiceType: reflect.TypeOf((*crudapi.CRUDService)(nil)).Elem(),
		Definition:  &crudapi.CRUDServiceDef,
	}
	publish.WithKTSPNamedInstances(provider)(&ps)

	return func(init *application.ApplicationInit) {
		init.AddRegisterPoints("crud", ".points", func(app *application.Application) error {
			if app.PointInfo(em.PointID[publish.PublishedService]()) == nil {
				return application.RegisterPoint(app, publish.NewPublishedServicesPoint())
			}
			return nil
		})
		init.AddContribute("crud", ".memory-connection", func(app *application.Application) error {
			return application.Contribute(app, ps)
		})
	}
}
