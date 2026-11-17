// SPDX-License-Identifier: Apache-2.0

package local

import (
	"context"
	"sync"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/extensions/agent/internal/runtime"
	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

// I3 — bus-backed audit emitter wiring.
//
// SpawnPermissionHook + WorkspacePathHook can't reach the EventBus
// directly (their hook signature is fixed by agentapi.PluginHooks). The
// runtime package exposes a package-level AuditEmitter setter; this
// adapter resolves the bus from the application context and bridges
// runtime.AuditEvent → engine.ChildPermissionDeniedEvent on
// engine.ChildPermissionDeniedTopic.
//
// wireAuditEmitter is idempotent + sync.Once-guarded. Called from
// CreateMachine on every machine creation; only the first call wires
// the bus. After wiring, subsequent calls are a cheap atomic load.

var wireOnce sync.Once

// busAuditEmitter adapts runtime.AuditEvent to the engine event/topic
// pair. Stateless; the bus reference is captured in the closure at
// wire-up time.
type busAuditEmitter struct {
	bus engine.EventBus
}

func (b *busAuditEmitter) EmitDeny(evt runtime.AuditEvent) {
	if b.bus == nil {
		return
	}
	b.bus.Publish(engine.ChildPermissionDeniedTopic(), &engine.ChildPermissionDeniedEvent{
		RunID:       evt.RunID,
		Hook:        evt.Hook,
		Tool:        evt.Tool,
		Reason:      evt.Reason,
		DeniedValue: evt.DeniedValue,
	})
}

// wireAuditEmitter resolves the bus from ctx and wires the adapter as
// the active runtime emitter. Idempotent — first call wins. Designed
// for use from CreateMachine where ctx is the application-bootstrapped
// context that has the bus service registered.
func wireAuditEmitter(ctx context.Context) {
	wireOnce.Do(func() {
		bus, _ := application.UseService[engine.EventBus](ctx)
		if bus == nil {
			return // tests without a bus skip wiring; audit calls become no-ops
		}
		runtime.SetAuditEmitter(&busAuditEmitter{bus: bus})
	})
}
