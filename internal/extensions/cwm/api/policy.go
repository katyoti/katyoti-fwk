// SPDX-License-Identifier: Apache-2.0

package api

// A POLICY is a rule put on objects: its name, what it says, the object types
// it applies to (by shape: types, capabilities), the hooks that measure it (a `verify` entry answers a verdict)
// and what an agent working on such an object is told. It holds no field of the object — a capability does — so
// it is never in an object's capability set: it applies to the object.
//
//	name: config-no-direct-io
//	label: Config through the service
//	description: Config access goes through the configuration primitives.
//	applies: {types: ["*"]}
//	hooks:
//	  verify:
//	    - uri: ktw-methods:acceptance/probe
//	      input: {id: "${id}", kind: command, probe: "bash probes/cfg1.sh"}
type Policy struct {
	Name        string      `json:"name"                  yaml:"name"                  jsonschema:"The policy id"`
	Label       string      `json:"label,omitempty"       yaml:"label,omitempty"       jsonschema:"A short human name"`
	Description string      `json:"description,omitempty" yaml:"description,omitempty" jsonschema:"What the rule says"`
	Icon        string      `json:"icon,omitempty"        yaml:"icon,omitempty"        jsonschema:"An icon name"`
	Applies     ShapeFilter `json:"applies"               yaml:"applies"               jsonschema:"The objects it applies to by types and capabilities (never by content); empty applies to none"`
	Hooks       Hooks       `json:"hooks,omitempty"       yaml:"hooks,omitempty"       jsonschema:"Its entries at the hook points, a layer of its own (policy:<name>): a verify entry measures it"`
	Agent       *AgentDecl  `json:"agent,omitempty"       yaml:"agent,omitempty"       jsonschema:"What a chat on an object it applies to is told"`
	// Provider is the binary that contributed it — set by the loader.
	Provider string `json:"provider,omitempty" yaml:"-"`
}

// AppliesTo reports whether the policy applies to an object of type typ
// carrying the capabilities caps (its effective set), and when not, why. A
// policy selects by shape — an object's rules must not depend on its content
// (ShapeFilter). An empty `applies` applies to no object: a rule put on every
// object says so (`types: ["*"]`).
func (p Policy) AppliesTo(typ string, caps []string) (bool, string) {
	if p.Applies.IsZero() {
		return false, "applies to no object (applies is empty)"
	}
	return p.Applies.Matches(FilterSubject{Type: typ, Capabilities: caps})
}

// DisplayName is the label, else the id.
func (p Policy) DisplayName() string {
	if p.Label != "" {
		return p.Label
	}
	return p.Name
}
