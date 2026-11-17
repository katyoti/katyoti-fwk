// SPDX-License-Identifier: Apache-2.0

package api

// RECIPES — how each kind of work goes: the steps it takes, in order, each
// done by a role (a task or a persona) or a person, in a phase. A recipe is the
// DEFAULT next step: intake plans its first step, and as each step ends the
// planner plans the next one — unless the step's task planned another.

// The classes of work: refine work reveals what the work is and its
// dependencies, so it comes before deliver work.
const (
	ClassRefine  = "refine"
	ClassDeliver = "deliver"
)

// Who does a step, and what a person's step asks.
const (
	ActorAgent  = "agent"
	ActorPerson = "person"

	InteractionValidate  = "validate"  // accept or reject a result
	InteractionChallenge = "challenge" // question it; the agent answers or reworks
	InteractionDecide    = "decide"    // choose among options
	InteractionProvide   = "provide"   // give missing input
)

// The isolations a step may ask for; inline — the one working tree — is the
// only one there is.
const (
	IsolationInline = "inline"
)

// Isolations lists the isolations a step may carry.
var Isolations = []string{IsolationInline}

// Intake is the first step of every request: the intake agent evaluates it —
// routes and splits it, sets its kind, priority and estimate, finds what it
// depends on, asks when it is unclear — and plans its next step.
const (
	StepIntake = "intake"
	RoleIntake = "intake"
)

// Recipe says how a request is worked: the transition the work achieves
// and its steps, in order. It is chosen by the request's type and state
// (the stage to reach) first, then by its kind.
type Recipe struct {
	Name        string   `json:"name"                  yaml:"name"`
	Description string   `json:"description,omitempty" yaml:"description,omitempty"`
	Types       []string `json:"types,omitempty"       yaml:"types,omitempty"`
	Kind        string   `json:"kind,omitempty"        yaml:"kind,omitempty"`
	States      []string `json:"states,omitempty"      yaml:"states,omitempty"`
	// Resolves names the types of object a request's work meets (its resolves
	// edge — a gate's work): the template for that work, before its kind's.
	Resolves   []string `json:"resolves,omitempty" yaml:"resolves,omitempty"`
	Transition string   `json:"transition"            yaml:"transition"`
	// Depth is how deep the work goes: full (the default) or quick; a request
	// of depth quick takes the quick template of its kind when there is one.
	Depth string `json:"depth,omitempty" yaml:"depth,omitempty"`
	// Start is the state its object enters as the recipe's first step starts
	// (a feature being built is active); empty: it stays where it is.
	Start string       `json:"start,omitempty" yaml:"start,omitempty"`
	Steps []RecipeStep `json:"steps"                 yaml:"steps"`
}

// RecipeStep is one step of a recipe: the role that does it, in its class.
type RecipeStep struct {
	Name  string `json:"name"                  yaml:"name"`
	Class string `json:"class,omitempty"       yaml:"class,omitempty"`
	Actor string `json:"actor,omitempty"       yaml:"actor,omitempty"`
	Role  string `json:"role,omitempty"        yaml:"role,omitempty"`
	// Interaction is what a person's step asks; on an agent's step, the
	// verdict its review answers (validate or challenge): rejected, the work
	// before it is done again.
	Interaction string `json:"interaction,omitempty" yaml:"interaction,omitempty"`
	Isolation   string `json:"isolation,omitempty"   yaml:"isolation,omitempty"`
	// Phase is the phase the step runs in; unset: preparation for refine
	// work, verify for a person's step, build otherwise.
	Phase string `json:"phase,omitempty" yaml:"phase,omitempty"`
	// Confirms names a person requirement of a gate, <gate>/<requirement>: a
	// person approving this step confirms it on the object the request is on.
	Confirms string `json:"confirms,omitempty"    yaml:"confirms,omitempty"`
	// Verifies marks a person's step whose approval verifies the request's
	// deliverable: planned on a request with one deliverable no check
	// verifies, that deliverable is the step's (WorkPacket.Verifies).
	Verifies bool `json:"verifies,omitempty"    yaml:"verifies,omitempty"`
	// PersonWhenAsked makes a review an agent's by default: its role does it,
	// answering its interaction's verdict, unless person review is asked - on
	// the request, its release or its stream (ReviewPerson) - then a person
	// does.
	PersonWhenAsked bool `json:"person_when_asked,omitempty" yaml:"person_when_asked,omitempty"`
	// Gate names the gate an agent's step works toward on the object (its
	// requirements say what done is): the doer is told it, not left to its
	// own default.
	Gate  string   `json:"gate,omitempty"        yaml:"gate,omitempty"`
	After []string `json:"after,omitempty"       yaml:"after,omitempty"`
	// Skill is the skill a worker follows to do the step (a skill URI).
	Skill string `json:"skill,omitempty" yaml:"skill,omitempty"`
	// EndsOn is the state of the target that ends this person's step: it is
	// done when the target reaches it, however it got there (WorkPacket.EndsOn).
	EndsOn string `json:"ends_on,omitempty" yaml:"ends_on,omitempty"`
	// Instructions say what the step does, for the worker.
	Instructions string `json:"instructions,omitempty" yaml:"instructions,omitempty"`
	// Refines marks a planning step: its result refines the request's plan
	// (the to-dos after it), rather than only moving the work on.
	Refines bool `json:"refines,omitempty" yaml:"refines,omitempty"`
	// Share is its part of the request's estimate; the parts left unstated
	// share what remains equally.
	Share float64 `json:"share,omitempty" yaml:"share,omitempty"`
}

// The depths of a work template.
const (
	DepthFull  = "full"
	DepthQuick = "quick"
)

// Depths are the depths a template or a request may have.
var Depths = []string{DepthFull, DepthQuick}

// ListRecipesOutput lists the recipes.
type ListRecipesOutput struct {
	Recipes []Recipe `json:"recipes"`
}
