// SPDX-License-Identifier: Apache-2.0

package api

import cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"

// VerificationCapability is the verification capability — the one that adds
// verify() and lint() to expressions — and the section a type writes its
// verification settings under. Type-level: no group on objects.
const VerificationCapability = "verification"

// VerificationSettings is a type's `verification:` section. The Go type is the
// one source of its schema (the `type-verification` data-type).
type VerificationSettings struct {
	// GateBlocksOn is the lowest lint severity that blocks: what lint() counts
	// as blocking for an object of the type. Empty = error.
	GateBlocksOn string `json:"gate_blocks_on,omitempty" yaml:"gate_blocks_on,omitempty" jsonschema:"description=Lowest lint severity that blocks: what lint() counts as blocking; error when empty,enum=error|warn|info|none"`
}

// VerificationOf is a type's `verification` section, zero when it writes none.
func VerificationOf(t cwmapi.ObjectType) (VerificationSettings, error) {
	s, _, err := cwmapi.TypeSection[VerificationSettings](t, VerificationCapability)
	return s, err
}
