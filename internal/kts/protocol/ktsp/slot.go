// SPDX-License-Identifier: Apache-2.0

package ktsp

import (
	"context"
	"encoding/json"
	"sort"
	"sync"

	"github.com/katyoti/katyoti-fwk/internal/core/meta"
)

// Handler processes a KTSP method call and returns a result.
type Handler func(ctx context.Context, params json.RawMessage) (json.RawMessage, error)

// Slot is one (typeName, instance) binding inside an Endpoint.
//
// Identity decomposition (Phase A of the slot-rename + ServiceDef-link
// initiative, see modules/framework/kts/):
//
//   - typeName    = wire-routing key, shared across every instance of
//     the same logical service (e.g. "crud-service").
//     This is the token the bus subject grammar names
//     <slottype> in `…slot.<provider>.<typeName>.<instance>.<method>`.
//
//   - instance    = the disambiguating NAME of this particular binding
//     within (provider, typeName). DefaultInstance for
//     singletons (skills, tasks, ktsp introspection,
//     session manager); a real connection / profile /
//     tenant name for multi-instance services.
//
//   - typeDef     = optional reference to a *meta.ServiceDef carrying
//     the canonical contract (method schemas, descriptions,
//     version). When typeDef is non-nil, multiple instances
//     of the same type share ONE *ServiceDef pointer — no
//     per-instance schema duplication. Phase C of the same
//     initiative makes typeDef the single source of truth
//     for method I/O type references.
//
// Inline-built slots (the built-in ktsp introspection slot, the session
// manager's session/registry/journal slots, the engine's agent/tool
// slots) pass typeDef=nil and rely on the explicit typeName for routing.
type Slot struct {
	typeName    string // wire-routing key + capability contract identifier
	instance    string // instance name (DefaultInstance for singletons)
	description string
	typeDef     *meta.ServiceDef // optional contract reference
	tags        SlotTags

	mu   sync.RWMutex
	defs map[string]*MethodDef
}

// NewSlot creates a slot with explicit identifiers. Pass DefaultInstance
// for `instance` when the slot is a singleton (one binding under this
// type).
//
// Capability discovery: callers that want to expose a "this slot
// fulfills the X contract" marker set typeName to the contract
// identifier itself (e.g. typeName="user_prompter" for TUI prompter
// bindings, typeName="agent" for engine agent slots). There is no
// separate categorical-marker dimension — typeName IS the capability
// contract, and `Session.FindByType` / `<provider>:interface/<name>`
// both key off the same string.
func NewSlot(typeName, instance, description string, tags SlotTags) *Slot {
	if instance == "" {
		// Loud rather than silent: every slot binding names its instance
		// explicitly. Callers wanting a singleton pass DefaultInstance.
		panic("ktsp.NewSlot: instance is required (use ktsp.DefaultInstance for singletons)")
	}
	return &Slot{
		typeName:    typeName,
		instance:    instance,
		description: description,
		tags:        tags,
		defs:        make(map[string]*MethodDef),
	}
}

// NewSlotForService creates a slot bound to a *meta.ServiceDef. The
// slot's typeName comes from def.ResolvedBusName(), description from
// def.Description().
//
// Use this from the publish framework + AutoKTSPContainer so every
// instance of the same service type shares one *ServiceDef pointer.
func NewSlotForService(def *meta.ServiceDef, instance string, tags SlotTags) *Slot {
	if def == nil {
		panic("ktsp.NewSlotForService: ServiceDef is required (use NewSlot for inline slots)")
	}
	if instance == "" {
		panic("ktsp.NewSlotForService: instance is required (use ktsp.DefaultInstance for singletons)")
	}
	return &Slot{
		typeName:    def.ResolvedBusName(),
		instance:    instance,
		description: def.Description(),
		typeDef:     def,
		tags:        tags,
		defs:        make(map[string]*MethodDef),
	}
}

// TypeName returns the wire-routing key. Shared across every instance
// of the same logical service / contract.
func (s *Slot) TypeName() string { return s.typeName }

// Instance returns the disambiguating name of this particular binding.
// DefaultInstance for singletons.
func (s *Slot) Instance() string { return s.instance }

// Description returns the slot description.
func (s *Slot) Description() string { return s.description }

// ServiceDef returns the optional contract reference. Nil for inline
// slots (ktsp introspection, session manager, engine slots) that
// don't carry a published *meta.ServiceDef.
func (s *Slot) ServiceDef() *meta.ServiceDef { return s.typeDef }

// Tags returns the slot's tags.
func (s *Slot) Tags() SlotTags { return s.tags }

// Add registers a method from a MethodDef.
// This is the only way to register methods.
func (s *Slot) Add(def *MethodDef) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.defs[def.name] = def
}

// Handle dispatches a method call to the registered handler.
// Returns ErrMethodNotFound if the method is not registered.
func (s *Slot) Handle(ctx context.Context, method string, params json.RawMessage) (json.RawMessage, error) {
	s.mu.RLock()
	def, ok := s.defs[method]
	s.mu.RUnlock()
	if !ok {
		return nil, ErrMethodNotFound(method)
	}
	return def.handler(ctx, params)
}

// Methods returns the list of registered method names.
func (s *Slot) Methods() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	methods := make([]string, 0, len(s.defs))
	for name := range s.defs {
		methods = append(methods, name)
	}
	return methods
}

// Def returns the MethodDef for a method, or nil if not found.
func (s *Slot) Def(method string) *MethodDef {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.defs[method]
}

// Defs returns all registered MethodDefs.
func (s *Slot) Defs() []*MethodDef {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*MethodDef, 0, len(s.defs))
	for _, d := range s.defs {
		result = append(result, d)
	}
	return result
}

// Info returns a SlotInfo description of this slot.
//
// Phase A: SlotInfo.Name continues to carry the wire-routing key
// (TypeName) — the wire shape stays unchanged so manifest readers don't
// break. Phase B will split into (Type, Instances) when the
// contribution-provider lands.
func (s *Slot) Info() SlotInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()
	methods := make([]MethodInfo, 0, len(s.defs))
	for _, def := range s.defs {
		methods = append(methods, def.Info())
	}
	sort.Slice(methods, func(i, j int) bool {
		return methods[i].Name < methods[j].Name
	})
	return SlotInfo{
		Name:        s.typeName,
		Description: s.description,
		// SlotType mirrors typeName — Phase A collapses the categorical
		// marker into the wire-routing key. The field is retained on
		// SlotInfo for downstream consumers (manifest builder, sidecar
		// discovery, ktw-gui ktsp browser) that key contract lookups
		// off it; they continue to work as long as typeName carries the
		// contract identifier (e.g. "user_prompter", "agent", "skill").
		SlotType: SlotType(s.typeName),
		Tags:     s.tags,
		Methods:  methods,
	}
}
