// SPDX-License-Identifier: Apache-2.0

package runtime

import "sync/atomic"

// I3 — permission-denial audit emitter.
//
// SpawnPermissionHook + WorkspacePathHook need to publish a structured
// event on every Deny so the recorder can capture it (audit trail per
// I4). The hook's OnPreToolExec signature is fixed by the agentapi
// PluginHooks contract — we can't thread a bus through it without a
// breaking change to every plugin in the codebase. Instead, use a
// package-level emitter that the agent factory wires at boot from the
// EventBus service.
//
// The emitter is an `atomic.Pointer` so SetAuditEmitter is goroutine-safe
// for the wire-up case (called once during Application bootstrap) and
// the hot path is a lock-free pointer load.

// AuditEvent is the minimal shape an emitter accepts. We deliberately
// avoid pulling in the full engine types here to keep the runtime
// package's dependency graph clean — the agent factory adapts engine
// types to this shape at wire-up time.
type AuditEvent struct {
	Hook        string // "spawn_permission" / "workspace_path" / "url_allowlist"
	RunID       string
	Tool        string
	Reason      string
	DeniedValue string // bounded; path / URL / tool kind — never full tool input
}

// AuditEmitter publishes audit events to the bus. Implementations adapt
// the AuditEvent → engine.ChildPermissionDeniedEvent and publish on
// engine.ChildPermissionDeniedTopic.
type AuditEmitter interface {
	EmitDeny(AuditEvent)
}

// auditEmitter holds the active emitter. atomic.Pointer for lock-free
// reads on the hot path; the rare write is at bootstrap.
var auditEmitter atomic.Pointer[AuditEmitter]

// SetAuditEmitter wires the active emitter. Called once at agent factory
// bootstrap with an adapter that publishes to the engine EventBus. nil
// resets to a no-op (useful for tests that want to suppress audit noise).
func SetAuditEmitter(e AuditEmitter) {
	if e == nil {
		auditEmitter.Store(nil)
		return
	}
	auditEmitter.Store(&e)
}

// emitAuditDeny is the hot-path helper called by hooks on every Deny.
// Lock-free pointer load; no-op when no emitter is wired.
func emitAuditDeny(evt AuditEvent) {
	ptr := auditEmitter.Load()
	if ptr == nil {
		return
	}
	(*ptr).EmitDeny(evt)
}
