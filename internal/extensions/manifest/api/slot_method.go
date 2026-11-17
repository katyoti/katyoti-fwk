// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"
	"strings"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/meta"
	corereflect "github.com/katyoti/katyoti-fwk/internal/core/reflect"
)

// ManifestOf reads one provider's manifest - a Catalog's Get, a remote
// catalog's ManifestSlots (the callable lane alone).
type ManifestOf func(ctx context.Context, provider string) (*Manifest, error)

// FindSlotMethod is the method a tool URI names, read from its provider's
// manifest; nil when the provider, the slot type or the method is unknown
// there. Names compare in kebab form, as the wire does.
func FindSlotMethod(ctx context.Context, manifestOf ManifestOf, uri ToolURI) *SlotMethod {
	if manifestOf == nil {
		return nil
	}
	man, err := manifestOf(ctx, uri.Provider)
	if err != nil || man == nil {
		return nil
	}
	for i := range man.SlotTypes {
		st := &man.SlotTypes[i]
		if corereflect.ToKebab(st.Type) != uri.SlotType {
			continue
		}
		for j := range st.Methods {
			if corereflect.ToKebab(st.Methods[j].Name) == uri.Method {
				return &st.Methods[j]
			}
		}
	}
	return nil
}

// MethodTimeout is the request timeout a caller applies to a method: its
// declared TimeoutS (meta.MethodDef.TimeoutS), or 0 - the transport's
// default - when it declares none or the path names no method the catalog
// knows. Every caller dispatching a method over the bus applies it, so a
// method declared long is never cut at the transport's few seconds. The
// path is read the way the sender reads it: a PascalCase slot or method
// segment (`cwm/Migrate`) is kebab-normalised with meta.DefaultBusName, the
// conversion the bus path parser applies, so it finds the same method.
func MethodTimeout(ctx context.Context, manifestOf ManifestOf, path string) time.Duration {
	provider, slot, instance, method, err := splitToolURI(path)
	if err != nil {
		return 0
	}
	segs := strings.Split(method, "/")
	for i, seg := range segs {
		segs[i] = meta.DefaultBusName(seg)
	}
	uri, err := NewToolURI(provider, meta.DefaultBusName(slot), instance, strings.Join(segs, "/"))
	if err != nil {
		return 0
	}
	if sm := FindSlotMethod(ctx, manifestOf, uri); sm != nil && sm.TimeoutS > 0 {
		return time.Duration(sm.TimeoutS) * time.Second
	}
	return 0
}
