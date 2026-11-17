// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"fmt"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// Read-modify-write helpers for the Object substrate.
//
// NodeUpdate used to PATCH (omit-to-leave on type_specific). ObjectUpdate
// REPLACES the object wholesale, which is the honest primitive — but it means a
// caller that wants to change one field must carry the rest forward. Doing that
// by hand at every call site is how fields get silently dropped, so it lives
// here once.
//
// codeCapability is where a software module's code lives: its
// kind, packages, tiers and facets - what used to be
// node.TypeSpecific["tier_current"] is object.Capabilities["code"]["tier_current"].
const codeCapability = "code"

// patchCapability merges fields into one capability group of an object and
// writes it back, preserving every other capability.
func patchCapability(ctx stdctx.Context, cwm cwmapi.CWMService, id, capability string, fields map[string]any) (cwmapi.Object, error) {
	module, err := cwmapi.ModuleOfObjectID(id)
	if err != nil {
		return cwmapi.Object{}, err
	}
	getOut, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: module, ID: id})
	if err != nil {
		return cwmapi.Object{}, fmt.Errorf("patch %s: get %s: %w", capability, id, err)
	}
	obj := getOut.Object
	if obj.Capabilities == nil {
		obj.Capabilities = map[string]map[string]any{}
	}
	group := obj.Capabilities[capability]
	if group == nil {
		group = map[string]any{}
	}
	for k, v := range fields {
		group[k] = v
	}
	obj.Capabilities[capability] = group

	updOut, err := cwm.ObjectUpdate(ctx, cwmapi.ObjectUpdateInput{Module: module, Object: obj, Revision: getOut.Revision})
	if err != nil {
		return cwmapi.Object{}, fmt.Errorf("patch %s: update %s: %w", capability, id, err)
	}
	return updOut.Object, nil
}

// moduleAttr reads one scalar from a module object's capability group, coping
// with YAML/JSON giving a number as either int or float64.
func moduleAttrInt(o cwmapi.Object, key string) int {
	group, ok := o.Capability(codeCapability)
	if !ok {
		return 0
	}
	switch v := group[key].(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	}
	return 0
}
