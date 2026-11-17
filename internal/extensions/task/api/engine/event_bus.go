// SPDX-License-Identifier: Apache-2.0

package engine

import "github.com/katyoti/katyoti-fwk/internal/core/eventbus"

// EventBus is re-exported from core/eventbus for backward compatibility.
// New code should import core/eventbus directly.
type EventBus = eventbus.EventBus

// MemoryEventBus is re-exported from core/eventbus.
type MemoryEventBus = eventbus.MemoryEventBus

// NopEventBus is re-exported from core/eventbus.
type NopEventBus = eventbus.NopEventBus

// NewMemoryEventBus creates an in-process event bus.
func NewMemoryEventBus() *MemoryEventBus {
	return eventbus.NewMemoryEventBus()
}
