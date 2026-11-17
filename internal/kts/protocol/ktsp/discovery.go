// SPDX-License-Identifier: Apache-2.0

package ktsp

import (
	"context"
	"encoding/json"
)

// registerKTSPSlot creates the built-in ktsp slot with standard discovery methods.
// Every container gets this slot automatically — it provides protocol introspection.
func (c *Endpoint) registerKTSPSlot() {
	ktsp := NewSlot("ktsp", DefaultInstance, "KTSP protocol introspection. Lists slots and their APIs.", SlotTags{
		TagSlotInternal: "true",
		TagSlotCLI:      "false",
		TagSlotMCP:      "false",
	})

	// slots — list all slots on this container
	ktsp.Add(RawMethod("slots", MethodOpts{Desc: "List all slots"}, func(_ context.Context, _ json.RawMessage) (json.RawMessage, error) {
		info := c.Info()
		return json.Marshal(info.Slots)
	}))

	// describe — describe a specific slot or the container itself
	ktsp.Add(RawMethod("describe", MethodOpts{Desc: "Describe a slot or the container"}, func(_ context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p struct {
			Slot string `json:"slot"`
		}
		if params != nil {
			if err := json.Unmarshal(params, &p); err != nil {
				return nil, ErrInvalidParams(err.Error())
			}
		}
		if p.Slot == "" {
			// No slot specified — describe the container itself
			info := c.Info()
			return json.Marshal(info)
		}
		// `describe` reports the slot TYPE's contract, not a per-
		// instance binding — pick DefaultInstance when present and
		// otherwise any instance (the methods are the type contract,
		// shared by definition).
		c.mu.RLock()
		instances := c.slots[p.Slot]
		s, ok := instances[DefaultInstance]
		if !ok {
			for _, anyInstance := range instances {
				s = anyInstance
				ok = true
				break
			}
		}
		c.mu.RUnlock()
		if !ok {
			return nil, ErrSlotNotFound(p.Slot)
		}
		return json.Marshal(s.Info())
	}))

	// ping — health check
	ktsp.Add(RawMethod("ping", MethodOpts{Desc: "Health check"}, func(_ context.Context, _ json.RawMessage) (json.RawMessage, error) {
		return json.Marshal(map[string]string{"status": "ok"})
	}))

	// The ktsp introspection slot is always bound under DefaultInstance
	// only — it has no concept of multiple instances. Writing through
	// AddSlot would panic because AddSlot rejects the reserved name;
	// we touch the map directly.
	c.mu.Lock()
	if c.slots["ktsp"] == nil {
		c.slots["ktsp"] = make(map[string]*Slot)
	}
	c.slots["ktsp"][DefaultInstance] = ktsp
	c.mu.Unlock()
}
