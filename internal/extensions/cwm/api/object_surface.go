// SPDX-License-Identifier: Apache-2.0

package api

// The effective SURFACE of an object: everything it offers, resolved from its
// type, from every capability that type composes, and from the policies that
// attach themselves to it.
//
// This is what makes the capability side real. Phase 1 gave capabilities
// somewhere to declare actions, badges, checks, hooks and an agent surface;
// without a resolver those slots are declared and read by nobody — the exact
// shape this refactor exists to close.

// ObjectEffectiveSurfaceInput names the object to resolve.
type ObjectEffectiveSurfaceInput struct {
	ID string `json:"id" jsonschema:"Object id; required"`
}

// SurfaceAction is one action with WHERE it came from. The source is carried
// because a reader must be able to tell a type's own curated binding from one a
// capability contributed — the same reason an aspect records object vs policy.
type SurfaceAction struct {
	Ref    ActionRef `json:"ref"`
	Source string    `json:"source" jsonschema:"type | capability:<name>"`
}

// ObjectEffectiveSurfaceOutput is the composed surface.
type ObjectEffectiveSurfaceOutput struct {
	ObjectID string `json:"object_id"`
	Type     string `json:"type"`

	// Capabilities in force — composed by the type, plus attached policies.
	Capabilities []string `json:"capabilities,omitempty"`
	// Policies names the subset that arrived by selector rather than by
	// declaration, so a bar another module imposed stays distinguishable.
	Policies []string `json:"policies,omitempty"`
	// UndeclaredCapabilities are ids a type names that nothing declares.
	// REPORTED, never invented: a capability that contributes nothing because
	// nobody declared it must not look identical to one that declares nothing.
	UndeclaredCapabilities []string `json:"undeclared_capabilities,omitempty"`

	Actions []SurfaceAction `json:"actions,omitempty"`
	Badges  []BadgeDef      `json:"badges,omitempty"`
}
