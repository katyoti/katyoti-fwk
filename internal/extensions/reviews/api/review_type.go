// SPDX-License-Identifier: Apache-2.0

package api

import cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"

// ReviewTypeKind is the contribution kind a review type is published as: one
// YAML per type in a CWM pack's `review-types/` folder, from any binary — the
// reviews pack ships the generic types, ktw-code its code reviews, a check
// owner its checks. A new review type is a file, never code.
const ReviewTypeKind = "review-type"

// Review type triggers: started by a person, or by the change a request made.
const (
	ModeManual    = "manual"
	ModeAutomatic = "automatic"
)

// ReviewTypeDef is one review type: what a person reads to choose it, the
// dimension it is grouped under, what it applies to, when it runs and what
// performs it. The unit a review supersedes: a module keeps its current
// review of each type and the one before it.
type ReviewTypeDef struct {
	Name        string `json:"name"                  yaml:"name"                  jsonschema:"description=The review type's id: kebab-case and unique in the universe"`
	Title       string `json:"title"                 yaml:"title"                 jsonschema:"description=What a person reads to choose it"`
	Description string `json:"description,omitempty" yaml:"description,omitempty" jsonschema:"description=What it looks at and what it answers,content=markdown"`
	// Dimension groups review types in the views: a run can take a whole
	// dimension, and the overview has a column per dimension.
	Dimension string `json:"dimension" yaml:"dimension" jsonschema:"description=The dimension it is grouped under: security or quality or architecture or usability or documentation or method or another"`
	// Applies selects what it reviews, as other contributions select objects:
	// types, capabilities, and a when predicate over the object.
	Applies cwmapi.ObjectFilter `json:"applies,omitempty" yaml:"applies,omitempty" jsonschema:"description=The objects it reviews: types and capabilities (any of) and a when predicate; empty = every reviewable object"`
	Trigger ReviewTrigger       `json:"trigger,omitempty" yaml:"trigger,omitempty" jsonschema:"description=When it runs: started by a person (manual) or by a request's change (automatic)"`
	Runner  ReviewRunner        `json:"runner"            yaml:"runner"            jsonschema:"description=What performs it: a skill an agent follows or a check action or both"`
	// Rating says how the run rates its result (good, needs-attention,
	// critical) and, when it scores, how the score is computed.
	Rating string `json:"rating,omitempty" yaml:"rating,omitempty" jsonschema:"description=How the run rates its result and computes a score when it gives one,content=markdown"`
	// Presets are the named sets it belongs to (full, code …): the New review
	// form offers each preset as one choice.
	Presets []string `json:"presets,omitempty" yaml:"presets,omitempty" jsonschema:"description=The named sets of review types it belongs to (full or code or another)"`
	// MaxCostUSD bounds what one run of it may spend; an automatic run of a
	// type over its ceiling is not started.
	MaxCostUSD float64 `json:"max_cost_usd,omitempty" yaml:"max_cost_usd,omitempty" jsonschema:"description=The most one run may spend in US dollars; 0 = no ceiling"`
	Provider   string  `json:"-"                      yaml:"-"`
}

// ReviewTrigger says when a review type runs.
type ReviewTrigger struct {
	Mode string `json:"mode,omitempty" yaml:"mode,omitempty" jsonschema:"description=manual (a person starts it; the default) or automatic (a request's change starts it),enum=manual|automatic"`
	// Kinds and Paths select the request whose change starts an automatic
	// review: its kind (build, fix …) and a path it changed (any of each;
	// empty = any).
	Kinds []string `json:"kinds,omitempty" yaml:"kinds,omitempty" jsonschema:"description=The kinds of request whose change starts it (build or fix or another); empty = any"`
	Paths []string `json:"paths,omitempty" yaml:"paths,omitempty" jsonschema:"description=Path globs of which the change must touch one; empty = any"`
	// Owners names the action that answers which objects own the paths a
	// change touched (ChangeOwnersInput to ChangeOwnersOutput): an automatic
	// run of a change reviews each owner on its own paths - and only them -
	// in place of the object it was started on. The owner of the paths knows
	// them (the Go packages' owners are gocode's); the reviews never guess.
	Owners string `json:"owners,omitempty" yaml:"owners,omitempty" jsonschema:"description=An action answering the objects that own the paths a change touched; an automatic run reviews each of them on its paths in place of the object it was started on (an action URI)"`
}

// ChangeOwnersInput is what a trigger's owners action is given: the paths a
// change touched.
type ChangeOwnersInput struct {
	Paths []string `json:"paths" jsonschema:"description=The paths the change touched (slash-separated - relative to the project)"`
}

// ChangeOwnersOutput is the objects owning them, each with its own paths; a
// path no object owns is left out.
type ChangeOwnersOutput struct {
	Owners []ChangeOwner `json:"owners"`
}

// ChangeOwner is one object owning some of a change's paths.
type ChangeOwner struct {
	Object string   `json:"object" jsonschema:"description=The owning object (a module)"`
	Paths  []string `json:"paths"  jsonschema:"description=The change's paths it owns"`
}

// Automatic says whether a request's change starts the type.
func (t ReviewTrigger) Automatic() bool { return t.Mode == ModeAutomatic }

// ReviewRunner is what performs a review type: a skill an agent follows (the
// code reviews follow ktw-code's code-review skill), a check action that
// answers findings, or both — with the input either is given.
type ReviewRunner struct {
	Skill string         `json:"skill,omitempty" yaml:"skill,omitempty" jsonschema:"description=The skill an agent follows to perform it (a skill URI)"`
	Check string         `json:"check,omitempty" yaml:"check,omitempty" jsonschema:"description=An action that performs it and answers its findings (an action URI)"`
	Input map[string]any `json:"input,omitempty" yaml:"input,omitempty" jsonschema:"description=What the skill or the check is given beside the object reviewed"`
}

// TypesInput asks the review types that apply to an object.
type TypesInput struct {
	ID       string `json:"id"                 jsonschema:"description=The object to review; required"`
	Mode     string `json:"mode,omitempty"     jsonschema:"description=Only the types of this trigger mode (manual or automatic); empty = both"`
	Language string `json:"language,omitempty" jsonschema:"description=The person's language (en or fr); empty = English"`
}

// TypesOutput is the review types that apply to an object, grouped by
// dimension (dimensions and their types in name order), and the presets they
// form.
type TypesOutput struct {
	Dimensions []DimensionTypes `json:"dimensions"`
	Presets    []Preset         `json:"presets,omitempty"`
}

// DimensionTypes is one dimension and its review types.
type DimensionTypes struct {
	Dimension string          `json:"dimension"`
	Types     []ReviewTypeDef `json:"types"`
}

// Preset is a named set of review types that apply to the object.
type Preset struct {
	Name  string   `json:"name"`
	Types []string `json:"types"`
}
