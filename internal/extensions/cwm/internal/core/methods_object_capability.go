// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"fmt"
	"slices"
	"sort"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// GIVING AND REMOVING A CAPABILITY. An object is a container of fields grouped
// by capability; a capability manages the fields it owns. Giving one gives its
// fields — the schema's defaults where the caller sets none, the required ones
// checked by the write's validation — requires the capabilities it `uses`, and,
// when it declares a workflow, starts that workflow at its initial state.

// ObjectAddCapability gives obj the capability, written through ObjectUpdate so
// the whole object is validated (its schema, its required fields) and the
// write hooks and events run.
func (s *Service) ObjectAddCapability(ctx stdctx.Context, in api.ObjectAddCapabilityInput) (api.ObjectAddCapabilityOutput, error) {
	return retryOnConflict(func() (api.ObjectAddCapabilityOutput, error) { return s.objectAddCapability(ctx, in) })
}

func (s *Service) objectAddCapability(ctx stdctx.Context, in api.ObjectAddCapabilityInput) (api.ObjectAddCapabilityOutput, error) {
	if in.ID == "" || in.Capability == "" {
		return api.ObjectAddCapabilityOutput{}, fmt.Errorf("%w: id and capability required", api.ErrInvalidInput)
	}
	obj, module, rev, err := s.fullObjectRev(ctx, in.ID)
	if err != nil {
		return api.ObjectAddCapabilityOutput{}, err
	}
	decl, ok := s.capabilitiesByName()[in.Capability]
	if !ok {
		return api.ObjectAddCapabilityOutput{}, fmt.Errorf("%w: capability %q is declared by no pack", api.ErrInvalidInput, in.Capability)
	}
	if _, carried := obj.Capabilities[in.Capability]; carried {
		return api.ObjectAddCapabilityOutput{}, fmt.Errorf("%w: %q already carries capability %q", api.ErrAlreadyExists, in.ID, in.Capability)
	}
	inForce := s.effectiveCapabilityIDs(obj)
	var missing []string
	for _, u := range decl.Uses {
		if !slices.Contains(inForce, u) {
			missing = append(missing, u)
		}
	}
	if len(missing) > 0 {
		return api.ObjectAddCapabilityOutput{}, fmt.Errorf("%w: capability %q uses %v, which %q does not carry — give them first", api.ErrInvalidInput, in.Capability, missing, in.ID)
	}

	group := map[string]any{}
	var defaulted []string
	if decl.Schema != "" {
		for k, v := range s.schemaDefaults(decl.Schema) {
			if _, given := in.Fields[k]; !given {
				group[k] = v
				defaulted = append(defaulted, k)
			}
		}
	}
	for k, v := range in.Fields {
		group[k] = v
	}
	if w := decl.Workflow; w != nil && w.State != "" {
		wf, ok := s.workflowNamed(w.Name)
		if !ok {
			return api.ObjectAddCapabilityOutput{}, fmt.Errorf("%w: capability %q follows workflow %q, which no pack declares", api.ErrInvalidInput, in.Capability, w.Name)
		}
		if st, given := in.Fields[w.State]; given && st != wf.Initial {
			return api.ObjectAddCapabilityOutput{}, fmt.Errorf("%w: capability %q starts in %s, the initial state of %s; %v is reached through ObjectTransition",
				api.ErrInvalidInput, in.Capability, wf.Initial, wf.Name, st)
		}
		group[w.State] = wf.Initial
		defaulted = append(defaulted, w.State)
	}
	sort.Strings(defaulted)
	if err := obj.SetCapability(in.Capability, group); err != nil {
		return api.ObjectAddCapabilityOutput{}, err
	}
	out, err := s.ObjectUpdate(ctx, api.ObjectUpdateInput{Module: module, Object: obj, Revision: rev})
	if err != nil {
		return api.ObjectAddCapabilityOutput{}, err
	}
	return api.ObjectAddCapabilityOutput{Object: out.Object, Defaulted: defaulted}, nil
}

// ObjectRemoveCapability removes a capability the object carries itself. One
// its type, an aspect or a policy brings is refused — the object would still
// have it — and so is one another capability in force uses.
func (s *Service) ObjectRemoveCapability(ctx stdctx.Context, in api.ObjectRemoveCapabilityInput) (api.ObjectRemoveCapabilityOutput, error) {
	return retryOnConflict(func() (api.ObjectRemoveCapabilityOutput, error) { return s.objectRemoveCapability(ctx, in) })
}

func (s *Service) objectRemoveCapability(ctx stdctx.Context, in api.ObjectRemoveCapabilityInput) (api.ObjectRemoveCapabilityOutput, error) {
	if in.ID == "" || in.Capability == "" {
		return api.ObjectRemoveCapabilityOutput{}, fmt.Errorf("%w: id and capability required", api.ErrInvalidInput)
	}
	obj, module, rev, err := s.fullObjectRev(ctx, in.ID)
	if err != nil {
		return api.ObjectRemoveCapabilityOutput{}, err
	}
	if slices.Contains(s.broughtCapabilities(obj), in.Capability) {
		return api.ObjectRemoveCapabilityOutput{}, fmt.Errorf("%w: capability %q comes with %q's type, an aspect or a policy — it cannot be removed from the object", api.ErrInvalidInput, in.Capability, in.ID)
	}
	if _, carried := obj.Capabilities[in.Capability]; !carried {
		return api.ObjectRemoveCapabilityOutput{}, fmt.Errorf("%w: %q does not carry capability %q", api.ErrNotFound, in.ID, in.Capability)
	}
	declared := s.capabilitiesByName()
	for _, id := range s.effectiveCapabilityIDs(obj) {
		if id == in.Capability {
			continue
		}
		if slices.Contains(declared[id].Uses, in.Capability) {
			return api.ObjectRemoveCapabilityOutput{}, fmt.Errorf("%w: capability %q is used by %q, which %q carries — remove that one first", api.ErrInvalidInput, in.Capability, id, in.ID)
		}
	}
	obj.Capabilities = cloneCaps(obj.Capabilities)
	delete(obj.Capabilities, in.Capability)
	// Removing a capability with a workflow removes its state: a sanctioned
	// change, not a state walked around its gates.
	out, err := s.ObjectUpdate(stdctx.WithValue(ctx, transitionKey{}, true), api.ObjectUpdateInput{Module: module, Object: obj, Revision: rev})
	if err != nil {
		return api.ObjectRemoveCapabilityOutput{}, err
	}
	return api.ObjectRemoveCapabilityOutput{Object: out.Object}, nil
}

// fullObject reads an object in full from its file (the index copy has no
// body), with its module.
func (s *Service) fullObject(id string) (api.Object, string, error) {
	obj, module, _, err := s.fullObjectRev(stdctx.Background(), id)
	return obj, module, err
}

// fullObjectRev is fullObject with the revision of the bytes read: a verb that
// writes the object back passes it, so the write is refused if someone wrote
// in between (and the verb redoes its change).
//
// A reaction with an open change set reads its own staged write (its revision
// the staged bytes'), so its read-modify-write verbs build on it.
func (s *Service) fullObjectRev(ctx stdctx.Context, id string) (api.Object, string, string, error) {
	if w, ok := s.stagedOf(ctx, id); ok {
		if w.Op == api.ChangeSetDelete {
			return api.Object{}, "", "", api.ObjectNotFound(id)
		}
		return *w.Object, w.Module, w.Revision, nil
	}
	ix := s.index()
	if err := ix.ensureLoaded(); err != nil {
		return api.Object{}, "", "", err
	}
	path, ok := ix.pathOf(id)
	if !ok {
		return api.Object{}, "", "", api.ObjectNotFound(id)
	}
	obj, rev, err := readObjectFileRev(path)
	if err != nil {
		return api.Object{}, "", "", err
	}
	module, _ := ix.moduleOf(id)
	return obj, module, rev, nil
}

// effectiveCapabilityIDs is every capability in force on obj: its type's, its
// aspects', its own groups and the policies attached to its type.
func (s *Service) effectiveCapabilityIDs(obj api.Object) []string {
	refs := s.effectiveRefs(obj)
	out := make([]string, 0, len(refs))
	for _, r := range refs {
		out = append(out, r.ID)
	}
	return out
}

// broughtCapabilities is what obj has without carrying it itself: its type's
// bundle, its aspects' lists and the policies attached to its type.
func (s *Service) broughtCapabilities(obj api.Object) []string {
	bare := obj
	bare.Capabilities = nil
	return s.effectiveCapabilityIDs(bare)
}
