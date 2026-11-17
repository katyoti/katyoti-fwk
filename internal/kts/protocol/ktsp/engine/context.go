// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"fmt"
	"log/slog"

	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp"
)

// EngineContext is the single dependency injection point for slot code.
// Slot handlers receive it at creation time and use it to access infrastructure.
// They never see Engine, SM, or transports directly.
type EngineContext interface {
	// ContainerID returns this Engine's UUID (used for routing identity).
	ContainerID() string

	// GetStub finds a Slot by path and populates target with a typed proxy.
	// path = "slot_name" (local) or "container_id/slot_name" (remote).
	// target must be a pointer to an interface type.
	// Validates that the target's methods match the slot's declared methods.
	GetStub(path string, target any) error

	// FindBySlotType discovers all slots of the given type across all containers.
	// Delegates to the SM for cross-container discovery.
	FindBySlotType(slotType ktsp.SlotType) ([]SlotRef, error)

	// Logger returns a structured logger scoped to this Engine.
	Logger() *slog.Logger

	// Config returns slot-specific configuration (from topology or extension config).
	Config() map[string]any
}

// engineContext is the concrete implementation of EngineContext.
type engineContext struct {
	engine *Engine
	config map[string]any
}

func newEngineContext(e *Engine, slotConfig map[string]any) *engineContext {
	return &engineContext{engine: e, config: slotConfig}
}

func (ec *engineContext) ContainerID() string {
	return ec.engine.id
}

func (ec *engineContext) GetStub(path string, target any) error {
	// TODO(urgent-stub-skeleton): implement stub resolution.
	// For now, return an error — this will be implemented in the stub/skeleton todo.
	return fmt.Errorf("GetStub not yet implemented (see urgent-stub-skeleton)")
}

func (ec *engineContext) FindBySlotType(slotType ktsp.SlotType) ([]SlotRef, error) {
	// TODO(urgent-session-manager): implement cross-container discovery via SM.
	return nil, fmt.Errorf("FindBySlotType not yet implemented (see urgent-session-manager)")
}

func (ec *engineContext) Logger() *slog.Logger {
	return ec.engine.logger
}

func (ec *engineContext) Config() map[string]any {
	return ec.config
}
