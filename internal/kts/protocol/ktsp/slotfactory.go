// SPDX-License-Identifier: Apache-2.0

package ktsp

import (
	"fmt"
	"sync"

	"gopkg.in/yaml.v3"
)

// SlotTypeFactory creates slots of a specific type from typed config.
// Factories are registered at boot time, after runtime deps are available.
// Each factory captures its own dependencies at registration time.
type SlotTypeFactory interface {
	// TypeName returns the slot type identifier (e.g. "agent", "skill", "user_prompter").
	TypeName() string

	// NewConfig returns a pointer to a zero-value config struct for this slot type.
	// The registry uses it to unmarshal YAML before calling CreateSlot.
	NewConfig() any

	// CreateSlot creates a slot from a typed config.
	// The config parameter is the same pointer returned by NewConfig(), populated by YAML unmarshaling.
	CreateSlot(name string, config any) (*Slot, error)
}

// SlotTypeRegistry holds registered slot type factories.
// It is the central lookup for creating slots declaratively from config.
type SlotTypeRegistry struct {
	mu        sync.RWMutex
	factories map[string]SlotTypeFactory
}

// NewSlotTypeRegistry creates an empty registry.
func NewSlotTypeRegistry() *SlotTypeRegistry {
	return &SlotTypeRegistry{
		factories: make(map[string]SlotTypeFactory),
	}
}

// Register adds a factory to the registry.
// Returns an error if a factory with the same type name is already registered.
func (r *SlotTypeRegistry) Register(factory SlotTypeFactory) error {
	typeName := factory.TypeName()
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.factories[typeName]; exists {
		return fmt.Errorf("slot type %q already registered", typeName)
	}
	r.factories[typeName] = factory
	return nil
}

// Get returns the factory for the given type name.
func (r *SlotTypeRegistry) Get(typeName string) (SlotTypeFactory, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	f, ok := r.factories[typeName]
	return f, ok
}

// CreateSlot looks up the factory for typeName, unmarshals the YAML config
// into the factory's typed config struct, and creates the slot.
func (r *SlotTypeRegistry) CreateSlot(typeName, name string, raw yaml.Node) (*Slot, error) {
	factory, ok := r.Get(typeName)
	if !ok {
		return nil, fmt.Errorf("unknown slot type %q", typeName)
	}
	config := factory.NewConfig()
	if err := raw.Decode(config); err != nil {
		return nil, fmt.Errorf("decode config for slot type %q: %w", typeName, err)
	}
	return factory.CreateSlot(name, config)
}
