// SPDX-License-Identifier: Apache-2.0

package core

import (
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// A LIFECYCLE SLOT is where one state machine of an object lives: the object's
// own lifecycle (the `lifecycle` capability), or the state of a capability that
// declares a workflow — its own fields, named by the capability, which knows
// what they mean. A use case stays `accepted` (its own lifecycle) while its
// requirement is `scheduled` (the requirement capability's).

type lifecycleSlot struct {
	capability string // "" = the object's own lifecycle
	workflow   string
	state      string
	// history is the capability's own trail, when it names a field for one
	// (planification's plan keeps its `trail`): its domain reads it. The
	// object's own lifecycle keeps none — its moves are the journal's.
	history []api.LifecycleEntry
	decl    *api.CapabilityWorkflow // nil for the object's own lifecycle
}

// lifecycleSlotOf reads the slot `capability` names on obj. The object's own
// lifecycle follows its TYPE's workflow — an object never names its own, so a
// raw write cannot swap in a laxer one; a capability's state follows the
// workflow the capability declares.
func (s *Service) lifecycleSlotOf(obj api.Object, capability string) (lifecycleSlot, error) {
	if capability == "" || capability == api.CapLifecycle {
		life, _ := obj.Lifecycle()
		t, known := s.lookupObjectType(obj.Type)
		if !known {
			// The type is another pack's, and that pack is not loaded:
			// this process opened the workspace on its own types only,
			// with no universe hub to read the catalog from.
			return lifecycleSlot{}, fmt.Errorf("%w: %q is of type %s, which no loaded pack declares — the pack's types reach this process through the universe's hub: start the universe (or run the command while it runs) and try again", api.ErrInvalidInput, obj.ID, obj.Type)
		}
		if t.Workflow == "" {
			return lifecycleSlot{}, fmt.Errorf("%w: %q (type %s) has no workflow", api.ErrInvalidInput, obj.ID, obj.Type)
		}
		return lifecycleSlot{workflow: t.Workflow, state: life.State}, nil
	}
	decl, ok := s.capabilitiesByName()[capability]
	if !ok {
		return lifecycleSlot{}, fmt.Errorf("%w: capability %q is declared by no pack", api.ErrInvalidInput, capability)
	}
	if decl.Workflow == nil || decl.Workflow.Name == "" || decl.Workflow.State == "" {
		return lifecycleSlot{}, fmt.Errorf("%w: capability %q declares no workflow", api.ErrInvalidInput, capability)
	}
	group, carried := obj.Capabilities[capability]
	if !carried {
		return lifecycleSlot{}, fmt.Errorf("%w: %q does not carry capability %q", api.ErrInvalidInput, obj.ID, capability)
	}
	slot := lifecycleSlot{capability: capability, workflow: decl.Workflow.Name, decl: decl.Workflow}
	slot.state, _ = group[decl.Workflow.State].(string)
	if decl.Workflow.History != "" {
		if raw, ok := group[decl.Workflow.History]; ok && raw != nil {
			b, err := json.Marshal(raw)
			if err == nil {
				_ = json.Unmarshal(b, &slot.history)
			}
		}
	}
	return slot, nil
}

// record appends the move to the capability's own trail, when it keeps one.
func (slot *lifecycleSlot) record(e api.LifecycleEntry) {
	if slot.decl != nil && slot.decl.History != "" {
		slot.history = append(slices.Clone(slot.history), e)
	}
}

// now is a move's time as a trail entry writes it.
func now() string { return time.Now().UTC().Format(time.RFC3339) }

// write puts the slot's state back on obj — and a capability's trail when it
// keeps one. The object's own moves are the journal's (its `transitioned`
// records), never written on the object.
func (slot lifecycleSlot) write(obj *api.Object) error {
	if slot.decl == nil {
		life, _ := obj.Lifecycle()
		life.State = slot.state
		obj.SetLifecycle(life)
		return nil
	}
	group := map[string]any{}
	for k, v := range obj.Capabilities[slot.capability] {
		group[k] = v
	}
	group[slot.decl.State] = slot.state
	if slot.decl.History != "" {
		b, err := json.Marshal(slot.history)
		if err != nil {
			return err
		}
		var entries []any
		if err := json.Unmarshal(b, &entries); err != nil {
			return err
		}
		group[slot.decl.History] = entries
	}
	return obj.SetCapability(slot.capability, group)
}

// workflowStates lists every slot of obj that follows a workflow, with its
// state: the object's own lifecycle, then each carried capability's.
func (s *Service) workflowStates(obj api.Object) map[string]string {
	out := map[string]string{}
	if life, ok := obj.Lifecycle(); ok {
		out[""] = life.State
	}
	declared := s.capabilitiesByName()
	for id, group := range obj.Capabilities {
		c, ok := declared[id]
		if !ok || c.Workflow == nil || c.Workflow.State == "" {
			continue
		}
		st, _ := group[c.Workflow.State].(string)
		out[id] = st
	}
	return out
}
