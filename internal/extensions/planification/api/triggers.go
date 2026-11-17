// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"

	"github.com/katyoti/katyoti-fwk/internal/core/meta"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// TRIGGERS AND RULES — the sources that file requests by themselves, beside
// people, agents and modules: a change on an object (a goal changed → review
// its features), a rule an object fails (an accepted goal no feature serves →
// decompose it), a move a gate refused. A want never creates work by itself;
// the trigger or the rule is the explicit filer. Each files once per source and
// object: firing again appends to its open request.

// What fires a trigger.
const (
	OnChange = "change" // the object's content changed (planning's own writes are no change)
	OnRule   = "rule"   // the object fails the trigger's check
)

// Trigger is one declared filer: on a change to, or a rule failed by, the
// objects of its types (in its states), it raises a request.
type Trigger struct {
	Name        string   `json:"name"                  yaml:"name"`
	Description string   `json:"description,omitempty" yaml:"description,omitempty"`
	On          string   `json:"on"                    yaml:"on"`
	Types       []string `json:"types"                 yaml:"types"`
	States      []string `json:"states,omitempty"      yaml:"states,omitempty"`
	// Check is, for a rule, what holds when the object is fine; for a change
	// trigger, when the change matters (it fires only when the check holds).
	Check    string      `json:"check,omitempty"       yaml:"check,omitempty"`
	File     TriggerFile `json:"file"                  yaml:"file"`
	OnObject bool        `json:"on_object,omitempty"   yaml:"on_object,omitempty"`
	Holds    bool        `json:"holds,omitempty"       yaml:"holds,omitempty"`
	// Grace is how long a rule leaves its object to its author first (a Go
	// duration, "1m"): it is checked then, on the object as it is by then -
	// what the author does meanwhile (prepares it, files its work) counts.
	Grace string `json:"grace,omitempty" yaml:"grace,omitempty"`
}

// TriggerFile is the request a trigger raises.
type TriggerFile struct {
	Kind    string `json:"kind"              yaml:"kind"`
	Title   string `json:"title"             yaml:"title"`
	Details string `json:"details,omitempty" yaml:"details,omitempty"`
	// Priority is the request's; the default when unset.
	Priority string `json:"priority,omitempty" yaml:"priority,omitempty"`
}

// ListTriggersInput lists the triggers.
type ListTriggersInput struct{}

// TriggersOutput is the declared triggers.
type TriggersOutput struct {
	Triggers []Trigger `json:"triggers"`
}

// FireOutput is what a change fired.
type FireOutput struct {
	Fired []RaiseOutput `json:"fired"`
}

// RaiseInput files a request from a source — or appends to the one it filed.
type RaiseInput struct {
	Source  RequestSource `json:"source" jsonschema:"description=What files it: kind (trigger or rule or gate) and name and object and the change,required"`
	Kind    string        `json:"kind" jsonschema:"description=The kind of work,required"`
	Title   string        `json:"title" jsonschema:"description=The ask in one line,required"`
	Details string        `json:"details,omitempty" jsonschema:"description=What the line leaves out"`
	// On files the request on the object itself (it carries it, and its type
	// and state choose the work template) instead of about it.
	On bool `json:"on,omitempty" jsonschema:"description=File it on the object (its type and state choose the template) rather than about it"`
	// Holds puts the review first: filed one priority above the open to-dos
	// about the object and the objects linked to it, it holds those not started
	// until it ends and marks the running ones may be stale.
	Holds bool `json:"holds,omitempty" jsonschema:"description=Review first: outrank and hold the to-dos about the object and what is linked to it"`
	// Priority is the request's when it is not a review first; the default
	// when unset.
	Priority string `json:"priority,omitempty" jsonschema:"description=Its priority when it holds nothing; P2 when unset,enum=P0|P1|P2|P3"`
}

// RaiseOutput says what a source did.
type RaiseOutput struct {
	Request string `json:"request,omitempty"`
	Filed   bool   `json:"filed" jsonschema:"description=A request was filed (false: the open one was appended to or left)"`
	Held    int    `json:"held,omitempty" jsonschema:"description=To-dos held until it ends"`
	Stale   int    `json:"stale,omitempty" jsonschema:"description=Running to-dos marked may be stale"`
	Note    string `json:"note,omitempty"`
}

// TriggerService files requests from triggers, rules and gates.
type TriggerService interface {
	Raise(ctx context.Context, in RaiseInput) (RaiseOutput, error)
	Fire(ctx context.Context, in cwmapi.HookCall) (FireOutput, error)
	List(ctx context.Context, in ListTriggersInput) (TriggersOutput, error)
}

// TriggerServiceDef describes the service for KTSP/CLI auto-generation.
var TriggerServiceDef = meta.ServiceDef{
	Name:    "triggers",
	Short:   "Triggers and rules: the sources that file requests by themselves - a change on an object, a rule it fails, a move a gate refused - once per source and object",
	Version: "1.0.0",
	Methods: map[string]meta.MethodDef{
		"Fire":  meta.Method("The hook point action: an object created or changed or moved - the triggers and rules that apply to it raise their requests (planning's own writes and lifecycle moves are no change)"),
		"List":  {Short: "The declared triggers and rules: what fires each and the request it raises", ReadOnly: true},
		"Raise": meta.Method("File a request from a source once per source and object - firing again appends to the open one, left alone while its own work runs; review first: it outranks and holds the to-dos about the object and what is linked to it"),
	},
}
