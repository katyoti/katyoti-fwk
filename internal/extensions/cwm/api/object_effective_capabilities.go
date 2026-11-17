// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"
	"fmt"
)

// ObjectEffectiveCapabilitiesInput asks which capabilities are in force on an object.
type ObjectEffectiveCapabilitiesInput struct {
	ID string `json:"id" jsonschema:"Object id; required"`
}

// ObjectEffectiveCapabilitiesOutput returns them, each tagged with where it came
// from — the object itself, or a policy contributed by selector.
type ObjectEffectiveCapabilitiesOutput struct {
	Capabilities []CapabilityRef `json:"capabilities"`
}

// HasCapability reports whether capability is in force on the object id — its
// type's bundle, its aspects', its own groups or one attached to it. This is
// how a reader asks "is it a module?": by what the object carries, never by
// its type's name (a software-module IS a module because it carries `module`).
func HasCapability(ctx context.Context, cwm CWMService, id, capability string) (bool, error) {
	out, err := cwm.ObjectEffectiveCapabilities(ctx, ObjectEffectiveCapabilitiesInput{ID: id})
	if err != nil {
		return false, fmt.Errorf("capabilities of %s: %w", id, err)
	}
	for _, c := range out.Capabilities {
		if c.ID == capability {
			return true, nil
		}
	}
	return false, nil
}
