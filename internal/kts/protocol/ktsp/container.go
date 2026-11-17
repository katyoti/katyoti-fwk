// SPDX-License-Identifier: Apache-2.0

package ktsp

import (
	"context"
	"encoding/json"
	"sort"
	"sync"
)

// Container is the interface every KTSP container implements. A
// container hosts (slot, instance) pairs and dispatches requests to
// them. There is no implicit instance — callers always pass one;
// single-instance services use the `DefaultInstance` constant
// explicitly.
type Container interface {
	// Info returns the container's metadata including all slot descriptions.
	Info() ContainerInfo

	// Handle dispatches a request to (slot, instance, method). An
	// unknown instance returns ErrSlotNotFound with the
	// `slot(instance)` form in the error so clients can distinguish
	// "wrong connection name" from "wrong service".
	Handle(ctx context.Context, slot, instance, method string, params json.RawMessage) (json.RawMessage, error)
}

// DefaultInstance is the conventional instance name for slots that
// don't model multiple addressable copies (singletons: SkillService,
// TaskService, the session manager's `session` slot, the ktsp
// introspection slot, …). The constant is exported so caller code
// records the intent at the call site rather than burying it in a
// magic empty string — `endpoint.AddSlot(slot)`
// reads as "one instance, conventional name".
//
// The bus subject parser uses the same literal when a remote URI
// omits the instance token (e.g. `<provider>:skills/list` resolves to
// `<provider>:skills(default)/list`).
const DefaultInstance = "default"

// Endpoint hosts (slot type → instance → *Slot) bindings and the
// built-in `ktsp` introspection slot. Concurrent reads run under the
// RWMutex; mutations under the write lock.
//
// Instance dimension:
//
//	Two callers can publish the same slot TYPE (e.g. "crud-service")
//	under different INSTANCE names ("books-pg", "leanix-prod") with
//	different handler closures bound to different impls. The endpoint
//	dispatches by the full (slot, instance) pair at request time;
//	method shapes stay identical across instances of the same type.
type Endpoint struct {
	name        string
	description string
	version     string

	mu sync.RWMutex
	// slots[slotType][instance] → *Slot. The only dispatch map.
	slots map[string]map[string]*Slot
}

// NewEndpoint creates a new container with a built-in ktsp slot.
func NewEndpoint(name, description, version string) *Endpoint {
	c := &Endpoint{
		name:        name,
		description: description,
		version:     version,
		slots:       make(map[string]map[string]*Slot),
	}
	c.registerKTSPSlot()
	return c
}

// AddSlot registers a slot at (slot.TypeName(), slot.Instance()).
// The slot itself owns both identifiers — the constructor (NewSlot /
// NewSlotForService) sets them at build time. Multiple instances of
// the same type are supported: method shapes MUST match across
// instances (registration callers like PublishService /
// AutoKTSPContainer are responsible for binding consistent methods).
//
// The reserved "ktsp" type is silently ignored — the built-in
// introspection slot cannot be overridden.
func (c *Endpoint) AddSlot(slot *Slot) {
	if slot.TypeName() == "ktsp" {
		return // ktsp slot is built-in, cannot be overridden
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.slots[slot.TypeName()] == nil {
		c.slots[slot.TypeName()] = make(map[string]*Slot)
	}
	c.slots[slot.TypeName()][slot.Instance()] = slot
}

// Info returns the container's metadata. One SlotInfo per slot TYPE
// (not per instance) — the methods/schemas are the type contract,
// shared across instances. Instances() enumerates the per-type
// bindings for introspection callers that need the actual instance
// list.
func (c *Endpoint) Info() ContainerInfo {
	c.mu.RLock()
	defer c.mu.RUnlock()

	slotInfos := make([]SlotInfo, 0, len(c.slots))
	for _, instances := range c.slots {
		// Prefer DefaultInstance when present so the SlotInfo's tags
		// reflect the singleton case; otherwise pick any instance —
		// the schemas are the same across instances by contract.
		var pick *Slot
		if s, ok := instances[DefaultInstance]; ok {
			pick = s
		} else {
			for _, s := range instances {
				pick = s
				break
			}
		}
		if pick == nil {
			continue
		}
		slotInfos = append(slotInfos, pick.Info())
	}
	sort.Slice(slotInfos, func(i, j int) bool {
		return slotInfos[i].Name < slotInfos[j].Name
	})

	return ContainerInfo{
		Name:            c.name,
		Description:     c.description,
		Version:         c.version,
		ProtocolVersion: KTSPProtocolVersion, // so `ktsp/describe` advertises it for negotiation
		Slots:           slotInfos,
	}
}

// Slot returns the slot bound at (name, instance), or nil if not
// found. Callers that just need the type contract pass
// `DefaultInstance` for singleton lookup.
func (c *Endpoint) Slot(name, instance string) *Slot {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.slots[name][instance]
}

// Handle dispatches a request to (slot, instance, method). Missing
// instance returns ErrSlotNotFound with the instance form
// `slot(instance)` in the message — distinguishable from a missing
// slot type which keeps the bare slot name. Empty `instance` is
// rejected with the same explicit-required guard as AddSlot.
func (c *Endpoint) Handle(ctx context.Context, slot, instance, method string, params json.RawMessage) (json.RawMessage, error) {
	if instance == "" {
		return nil, ErrInvalidRequest("ktsp.Endpoint.Handle: instance is required (use ktsp.DefaultInstance for single-instance services)")
	}
	c.mu.RLock()
	s, ok := c.slots[slot][instance]
	c.mu.RUnlock()
	if !ok {
		if instance == DefaultInstance {
			return nil, ErrSlotNotFound(slot)
		}
		return nil, ErrSlotNotFound(slot + "(" + instance + ")")
	}
	return s.Handle(ctx, method, params)
}

// Instances returns the registered instance names for the slot,
// alphabetically sorted. Empty result when the slot is unknown.
// Used by introspection (manifest enumeration, GUI debug views) to
// list what's actually bound under a slot type at runtime.
func (c *Endpoint) Instances(slot string) []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	m := c.slots[slot]
	if len(m) == 0 {
		return nil
	}
	out := make([]string, 0, len(m))
	for name := range m {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}
