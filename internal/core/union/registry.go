// SPDX-License-Identifier: Apache-2.0

// Package union provides a generic discriminated union registry.
// It maps a string type discriminator to concrete Go types,
// enabling JSON deserialization into typed values.
//
// Usage:
//
//	var EventTypes = union.NewRegistry[Event]("type")
//
//	func init() {
//	    EventTypes.Register("click", ClickEvent{})
//	    EventTypes.Register("key", KeyEvent{})
//	}
//
//	event, err := EventTypes.Deserialize(jsonBytes)
//	switch e := event.(type) {
//	case ClickEvent: ...
//	case KeyEvent: ...
//	}
package union

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sync"
)

// Registry maps a string discriminator field to concrete Go types.
// T is the interface or base type that all registered types satisfy.
type Registry[T any] struct {
	typeField string // JSON field name used as discriminator (e.g., "type")
	mu        sync.RWMutex
	types     map[string]reflect.Type // discriminator value → concrete type
}

// NewRegistry creates a union registry.
// typeField is the JSON field name used as the discriminator (e.g., "type", "kind", "event").
func NewRegistry[T any](typeField string) *Registry[T] {
	return &Registry[T]{
		typeField: typeField,
		types:     make(map[string]reflect.Type),
	}
}

// Register adds a concrete type to the registry.
func (r *Registry[T]) Register(discriminator string, prototype T) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.types[discriminator] = reflect.TypeOf(prototype)
}

// Deserialize reads the discriminator field from raw JSON,
// looks up the concrete type, and unmarshals into it.
func (r *Registry[T]) Deserialize(raw json.RawMessage) (T, error) {
	var zero T

	// Extract discriminator value from JSON
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return zero, fmt.Errorf("union: unmarshal envelope: %w", err)
	}

	discRaw, ok := envelope[r.typeField]
	if !ok {
		return zero, fmt.Errorf("union: missing discriminator field %q", r.typeField)
	}

	var discriminator string
	if err := json.Unmarshal(discRaw, &discriminator); err != nil {
		return zero, fmt.Errorf("union: unmarshal discriminator %q: %w", r.typeField, err)
	}

	// Look up concrete type
	r.mu.RLock()
	concreteType, found := r.types[discriminator]
	r.mu.RUnlock()

	if !found {
		return zero, fmt.Errorf("union: unknown %s=%q", r.typeField, discriminator)
	}

	// Unmarshal into concrete type
	ptr := reflect.New(concreteType)
	if err := json.Unmarshal(raw, ptr.Interface()); err != nil {
		return zero, fmt.Errorf("union: unmarshal %s=%q: %w", r.typeField, discriminator, err)
	}

	result, ok := ptr.Elem().Interface().(T)
	if !ok {
		return zero, fmt.Errorf("union: %s=%q does not implement %T", r.typeField, discriminator, zero)
	}
	return result, nil
}

// Lookup returns the concrete type for a discriminator value, or nil.
func (r *Registry[T]) Lookup(discriminator string) (reflect.Type, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.types[discriminator]
	return t, ok
}

// Types returns all registered discriminator values.
func (r *Registry[T]) Types() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	keys := make([]string, 0, len(r.types))
	for k := range r.types {
		keys = append(keys, k)
	}
	return keys
}
