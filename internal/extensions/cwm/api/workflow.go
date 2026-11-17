// SPDX-License-Identifier: Apache-2.0

package api

import (
	"encoding/json"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// LIFECYCLES AND WORKFLOWS. An intent object (a goal, a use-case, a decision, a
// plan…) carries a `lifecycle`: its state. How it may move is a WORKFLOW — a
// named state machine, contributed like any other declaration and named by the
// object's type (or, for a capability's own state, by the capability), so
// whoever builds a solution chooses the workflow: the standard one (a person
// sets the state) or something stricter (an approval). An object never names
// its own workflow: a laxer one would walk around the type's gates.
//
// The engine is a VALIDATOR, not a runner: on a transition it checks the
// transition exists from the current state, evaluates its gate and writes the
// state. Consequences are `on_transition` hooks. How the object got there is
// not stored on it: the journal's `transitioned` records are its history
// (journal Transitions).

// CapLifecycle is the field-group an object's state lives in.
const CapLifecycle = "lifecycle"

// Actor kinds: who made a transition. A gate may require a person. Outside
// is who edited files beside the CWM verbs (a git pull or merge, an editor):
// a catch-up names it - the commit's author, or the local git user for an
// edit not committed yet.
const (
	ActorPerson  = "person"
	ActorAgent   = "agent"
	ActorOutside = "outside"
)

// Workflow is a named state machine an object's lifecycle moves through.
type Workflow struct {
	Name        string               `json:"name"                  yaml:"name"                  jsonschema:"Workflow id; unique across the catalog"`
	Label       string               `json:"label,omitempty"       yaml:"label,omitempty"       jsonschema:"Human name"`
	Description string               `json:"description,omitempty" yaml:"description,omitempty" jsonschema:"What the workflow is for"`
	Initial     string               `json:"initial"               yaml:"initial"               jsonschema:"The state a new object starts in"`
	States      []WorkflowState      `json:"states"                yaml:"states"                jsonschema:"Every state the workflow has"`
	Transitions []WorkflowTransition `json:"transitions"           yaml:"transitions"           jsonschema:"The moves allowed between states"`
}

// WorkflowState is one state of a workflow.
type WorkflowState struct {
	Name        string `json:"name"                  yaml:"name"                  jsonschema:"State id"`
	Description string `json:"description,omitempty" yaml:"description,omitempty" jsonschema:"What being in this state means"`
	Terminal    bool   `json:"terminal,omitempty"    yaml:"terminal,omitempty"    jsonschema:"No transition leaves it"`
}

// WorkflowTransition is one allowed move.
type WorkflowTransition struct {
	Name string   `json:"name"           yaml:"name"           jsonschema:"Transition id"`
	From []string `json:"from"           yaml:"from"           jsonschema:"States it may start from; * means any"`
	To   string   `json:"to"             yaml:"to"             jsonschema:"The state it leads to"`
	// Gate is an el predicate over the object vocabulary plus `by` (the actor:
	// id and kind) and `from` / `to`. Empty = always allowed.
	Gate string `json:"gate,omitempty" yaml:"gate,omitempty" jsonschema:"el predicate over the object vocabulary plus by / from / to; empty means always"`
	// Force says who may pass a failing guard: `anyone` (authored `true`), or
	// `person` — a person, with a note. The forced pass is recorded.
	Force ForceRule `json:"force,omitempty" yaml:"force,omitempty" jsonschema:"description=Who may force past a failing guard (true = anyone; person = a person with a note); recorded as forced,enum=anyone|person"`
}

// ForceRule says who may force a transition past its failing guard.
type ForceRule string

// Force rules. Authored `force: true` reads as ForceAnyone.
const (
	ForceNone   ForceRule = ""
	ForceAnyone ForceRule = "anyone"
	ForcePerson ForceRule = "person"
)

// Allows reports whether `by` may force, with `note`.
func (f ForceRule) Allows(by Actor, note string) bool {
	switch f {
	case ForceAnyone:
		return true
	case ForcePerson:
		return by.Kind == ActorPerson && strings.TrimSpace(note) != ""
	}
	return false
}

func (f *ForceRule) set(v any) error {
	switch x := v.(type) {
	case bool:
		*f = ForceNone
		if x {
			*f = ForceAnyone
		}
	case string:
		switch r := ForceRule(x); r {
		case ForceNone, ForceAnyone, ForcePerson:
			*f = r
		case "true":
			*f = ForceAnyone
		case "false":
			*f = ForceNone
		default:
			return fmt.Errorf("force: %q is not true, false, anyone or person", x)
		}
	case nil:
		*f = ForceNone
	default:
		return fmt.Errorf("force: %v is not true, false, anyone or person", v)
	}
	return nil
}

// UnmarshalJSON reads true / false / "anyone" / "person".
func (f *ForceRule) UnmarshalJSON(b []byte) error {
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	return f.set(v)
}

// UnmarshalYAML reads true / false / anyone / person.
func (f *ForceRule) UnmarshalYAML(n *yaml.Node) error {
	var v any
	if err := n.Decode(&v); err != nil {
		return err
	}
	return f.set(v)
}

// Actor is who made a transition.
type Actor struct {
	ID   string `json:"id"   jsonschema:"The person or the agent; for outside the commit's author"`
	Kind string `json:"kind" jsonschema:"person or agent or outside"`
}

// LifecycleEntry is one transition in an object's history, as the journal
// derives it from its `transitioned` records (journal Transitions) — never
// stored on the object.
type LifecycleEntry struct {
	From       string `json:"from,omitempty"`
	To         string `json:"to"`
	Transition string `json:"transition,omitempty"`
	By         Actor  `json:"by"`
	At         string `json:"at" jsonschema:"RFC 3339"`
	Note       string `json:"note,omitempty"`
	Forced     bool   `json:"forced,omitempty"`
	// Calls is what the functions of the guard a forced move passed answered.
	Calls []FunctionAnswer `json:"calls,omitempty"`
}

// Lifecycle is the `lifecycle` field-group: the state, in the workflow the
// object's type names. Its history is the journal's (journal Transitions).
type Lifecycle struct {
	State string `json:"state,omitempty"`
}

// Lifecycle returns the object's lifecycle capability.
func (o Object) Lifecycle() (Lifecycle, bool) {
	v, present, err := DecodeCapability[Lifecycle](o, CapLifecycle)
	return v, present && err == nil
}

// SetLifecycle writes the lifecycle capability.
func (o *Object) SetLifecycle(l Lifecycle) { _ = o.SetCapability(CapLifecycle, l) }

// StateNamed returns a workflow's state by name.
func (w Workflow) StateNamed(name string) (WorkflowState, bool) {
	for _, s := range w.States {
		if s.Name == name {
			return s, true
		}
	}
	return WorkflowState{}, false
}

// ObjectTransitionInput moves an object to another state of its workflow.
type ObjectTransitionInput struct {
	ID string `json:"id" jsonschema:"The object; required"`
	To string `json:"to" jsonschema:"The target state; required"`
	// Transition names the transition when several lead to the same state.
	Transition string `json:"transition,omitempty" jsonschema:"The transition's name when several lead to the target state"`
	By         Actor  `json:"by"                   jsonschema:"Who makes the transition; required"`
	Note       string `json:"note,omitempty"       jsonschema:"Why; recorded in the journal"`
	// Force passes a failing guard when the transition allows it for `by`
	// (force: person takes a person and a note); recorded.
	Force bool `json:"force,omitempty" jsonschema:"Pass a failing guard when the transition allows it; recorded as forced"`
	// Capability names the capability whose own workflow moves (one that
	// declares `workflow:`); empty moves the object's own lifecycle.
	Capability string `json:"capability,omitempty" jsonschema:"The capability whose own workflow moves; empty = the object's lifecycle"`
	// DryRun answers "would this move pass for this actor, and why not"
	// without moving: the same checks, no hook asked, nothing written, no
	// event published.
	DryRun bool `json:"dry_run,omitempty" jsonschema:"Check the move without making it: the same checks and the same refusal, nothing written"`
	// NoCostly, with DryRun, is a preview of the move: a costly function the
	// guard calls is not run, and the refusal's call answers it AtMove —
	// the guard is decided at the move. A move itself runs every function.
	NoCostly bool `json:"no_costly,omitempty" jsonschema:"With dry_run: a preview - a costly function is not run and the guard is decided at the move"`
}

// ObjectTransitionOutput reports the move — or, for a dry run, the move that
// would be made.
type ObjectTransitionOutput struct {
	Object     Object `json:"object"`
	From       string `json:"from"`
	To         string `json:"to"`
	Transition string `json:"transition"`
	Forced     bool   `json:"forced,omitempty"`
	DryRun     bool   `json:"dry_run,omitempty"`
}

// TransitionRefusedError is a move its guard refused: the reason and what
// each function the guard called answered. It wraps ErrTransitionRefused.
type TransitionRefusedError struct {
	Object     string
	Transition string
	From       string
	To         string
	Guard      string
	Reason     string
	Calls      []FunctionAnswer
}

func (e *TransitionRefusedError) Error() string {
	return fmt.Sprintf("%v: %s: %s", ErrTransitionRefused, e.Transition, e.Reason)
}

func (e *TransitionRefusedError) Unwrap() error { return ErrTransitionRefused }
