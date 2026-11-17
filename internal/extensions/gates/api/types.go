// SPDX-License-Identifier: Apache-2.0

package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// Gate is a named definition of done.
type Gate struct {
	Name         string            `json:"name"                   yaml:"name"                   jsonschema:"The gate id; what gate('...') names"`
	Label        string            `json:"label,omitempty"        yaml:"label,omitempty"        jsonschema:"Human name"`
	Description  string            `json:"description,omitempty"  yaml:"description,omitempty"  jsonschema:"What passing it means"`
	Requirements []GateRequirement `json:"requirements,omitempty" yaml:"requirements,omitempty" jsonschema:"The requirements the gate itself declares"`
	Provider     string            `json:"-"                      yaml:"-"`
}

// GateRequirement is one item of a gate's definition of done. Exactly one of
// Check, URI or Person says how it is checked.
type GateRequirement struct {
	Name        string `json:"name"                  yaml:"name"                  jsonschema:"The requirement id within its gate"`
	Label       string `json:"label,omitempty"       yaml:"label,omitempty"       jsonschema:"What the requirement says"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
	// Gate names the gate a contributed requirement adds to; empty inside a
	// gate's own list.
	Gate string `json:"gate,omitempty" yaml:"gate,omitempty" jsonschema:"The gate this requirement adds to (a gate-requirement contribution)"`
	// Applies restricts the requirement to the objects it selects; empty =
	// every object that goes through the gate. By shape only (types,
	// capabilities — cwmapi.ShapeFilter): a requirement that stopped applying
	// when the object's content changed would be a check passing by
	// vanishing; a condition on content belongs in the check.
	Applies cwmapi.ShapeFilter `json:"applies,omitempty" yaml:"applies,omitempty" jsonschema:"The objects it applies to by types and capabilities (never by content); empty = every object through the gate"`
	// Check is an el predicate over the object, evaluated by CWM as a guard
	// would be (by / from / to, the capabilities' functions but gate()).
	Check string `json:"check,omitempty" yaml:"check,omitempty" jsonschema:"el predicate over the object (plus by from to and the capabilities' functions)"`
	// URI is an action answering {accept, reason} over the object.
	URI string `json:"uri,omitempty" yaml:"uri,omitempty" jsonschema:"Action URI answering accept and reason"`
	// Person says a person confirms it (Confirm); the confirmation is
	// recorded on the object.
	Person bool `json:"person,omitempty" yaml:"person,omitempty" jsonschema:"A person confirms it; recorded on the object"`
	// Resolve is how the requirement is met, read by the planner: when unmet,
	// the request it files (title, kind, on) or the question a person is asked
	// (interaction); and the step it adds to the request that plans the phase
	// its gate closes (step — RequirementStep, read through Step).
	Resolve  map[string]any `json:"resolve,omitempty" yaml:"resolve,omitempty" jsonschema:"How it is met (read by the planner): title kind on interaction step"`
	Provider string         `json:"-"                 yaml:"-"`
}

// AppliesTo reports whether the requirement applies to the subject (an
// object's type and effective capabilities), and when not, why.
func (r GateRequirement) AppliesTo(s cwmapi.FilterSubject) (bool, string) {
	return r.Applies.Matches(s)
}

// gateRequirementFields is GateRequirement stripped of its codec.
type gateRequirementFields GateRequirement

// A requirement refuses a key no field takes, naming it, through CWM's strict
// declaration codec: a removed key (the old `types:`) or a misspelt one would
// otherwise decode into nothing and apply the requirement to every object.
func (r *GateRequirement) UnmarshalJSON(b []byte) error {
	f, err := cwmapi.DecodeDeclarationJSON(GateRequirementKind, b, func(f *gateRequirementFields) string { return f.Name })
	if err == nil {
		*r = GateRequirement(f)
	}
	return err
}

func (r *GateRequirement) UnmarshalYAML(n *yaml.Node) error {
	f, err := cwmapi.DecodeDeclarationYAML(GateRequirementKind, n, func(f *gateRequirementFields) string { return f.Name })
	if err == nil {
		*r = GateRequirement(f)
	}
	return err
}

// Validate refuses a requirement that says no way, or two ways, to check it,
// or that declares a step it cannot add (Step): a gate holding it cannot pass
// — its item reads unproven — rather than the step being dropped.
func (r GateRequirement) Validate() error {
	n := 0
	for _, set := range []bool{r.Check != "", r.URI != "", r.Person} {
		if set {
			n++
		}
	}
	if n != 1 {
		return fmt.Errorf("%w: gate requirement %q: exactly one of check, uri, person", cwmapi.ErrInvalidInput, r.Name)
	}
	if _, _, err := r.Step(); err != nil {
		return err
	}
	return nil
}

// ResolveStep is the key of Resolve holding the step a requirement adds.
const ResolveStep = "step"

// The questions a person's step asks — the values planification accepts.
const (
	InteractionValidate  = "validate"  // accept or reject a result
	InteractionChallenge = "challenge" // question it; the agent answers or reworks
	InteractionDecide    = "decide"    // choose among options
	InteractionProvide   = "provide"   // give missing input
)

// RequirementStep is the step a requirement adds to the request that plans
// the phase its gate closes (`resolve.step`): how a discipline plugs into a
// phase. Exactly one of Role (an agent's step) or Interaction (a person's).
type RequirementStep struct {
	Name         string `json:"name"                   jsonschema:"The step's name; required"`
	Role         string `json:"role,omitempty"         jsonschema:"The role (a persona) an agent's step is given to"`
	Interaction  string `json:"interaction,omitempty"  jsonschema:"What a person's step asks: validate or challenge or decide or provide"`
	Skill        string `json:"skill,omitempty"        jsonschema:"The skill the step uses"`
	Instructions string `json:"instructions,omitempty" jsonschema:"What the step does"`
	// Phase is the phase the step joins; empty = the phase the gate closes.
	Phase string `json:"phase,omitempty" jsonschema:"The phase the step joins; empty = the phase the gate closes"`
	// Priority orders the steps of a gate: the lower first.
	Priority int `json:"priority,omitempty" jsonschema:"Orders the added steps: the lower first"`
}

// Step is the step the requirement adds (`resolve.step`), decoded strictly:
// a key no field takes is refused, named; so is a step without a name, or
// with both or neither of role and interaction, or an interaction planning
// does not ask. ok is false when the requirement declares no step.
func (r GateRequirement) Step() (step RequirementStep, ok bool, err error) {
	raw, ok := r.Resolve[ResolveStep]
	if !ok {
		return RequirementStep{}, false, nil
	}
	fail := func(format string, a ...any) (RequirementStep, bool, error) {
		return RequirementStep{}, true, fmt.Errorf("%w: gate requirement %q: resolve.step: %s", cwmapi.ErrInvalidInput, r.Name, fmt.Sprintf(format, a...))
	}
	b, err := json.Marshal(raw)
	if err != nil {
		return fail("%v", err)
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&step); err != nil {
		return fail("%v", strings.TrimPrefix(err.Error(), "json: "))
	}
	switch {
	case step.Name == "":
		return fail("a step has a name")
	case (step.Role == "") == (step.Interaction == ""):
		return fail("exactly one of role, interaction")
	}
	switch step.Interaction {
	case "", InteractionValidate, InteractionChallenge, InteractionDecide, InteractionProvide:
	default:
		return fail("interaction %q is none of validate, challenge, decide, provide", step.Interaction)
	}
	return step, true, nil
}

// Requirement statuses in a checklist.
const (
	Met      = "met"
	Unmet    = "unmet"
	Unproven = "unproven" // the check could not be evaluated
)

// GateItem is one requirement as a checklist shows it.
type GateItem struct {
	Name    string         `json:"name"`
	Label   string         `json:"label,omitempty"`
	Status  string         `json:"status" jsonschema:"met | unmet | unproven"`
	Reason  string         `json:"reason,omitempty"`
	Kind    string         `json:"kind" jsonschema:"check | uri | person"`
	Resolve map[string]any `json:"resolve,omitempty"`
	// ConfirmedBy is the person who confirmed a person requirement.
	ConfirmedBy *GateConfirmation `json:"confirmed_by,omitempty"`
}

// GateChecklist is a gate evaluated on one object — what gate('...') answers.
type GateChecklist struct {
	Gate  string     `json:"gate"`
	Label string     `json:"label,omitempty"`
	Items []GateItem `json:"items"`
	// Passes is true when every item is met.
	Passes bool `json:"passes"`
}

// Unmet is the items not met (unmet or unproven).
func (c GateChecklist) Unmet() []GateItem {
	var out []GateItem
	for _, it := range c.Items {
		if it.Status != Met {
			out = append(out, it)
		}
	}
	return out
}

// CapGates is the field-group where an object keeps the confirmations of its
// person requirements, keyed `<gate>/<requirement>` (GateKey), and the trace
// of the confirmations withdrawn (RevokedKey).
const CapGates = "gates"

// The contribution kinds the gates extension declares and reads; a pack
// carries them in `gates/` and `gate-requirements/` (contrib.Kinds).
const (
	GateKind            = "gate"
	GateRequirementKind = "gate-requirement"
)

// RevokedKey is the entry of the `gates` group listing the withdrawn
// confirmations, oldest first. It holds no `/`, so it never reads as a
// confirmation key.
const RevokedKey = "revoked"

// GateRevocation is a person's withdrawal of a confirmation: what was
// confirmed, by whom and when, and who withdrew it, when and why.
type GateRevocation struct {
	Gate        string           `json:"gate"`
	Requirement string           `json:"requirement"`
	Confirmed   GateConfirmation `json:"confirmed"`
	By          cwmapi.Actor     `json:"by"`
	At          string           `json:"at"`
	Note        string           `json:"note,omitempty"`
}

// GateConfirmation is a person's confirmation of one requirement.
type GateConfirmation struct {
	By   cwmapi.Actor `json:"by"`
	At   string       `json:"at"`
	Note string       `json:"note,omitempty"`
}

// GateKey is how a confirmation is keyed in the `gates` group.
func GateKey(gate, requirement string) string { return gate + "/" + requirement }
