// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"

	"github.com/katyoti/katyoti-fwk/internal/core/meta"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	decisionsapi "github.com/katyoti/katyoti-fwk/internal/extensions/decisions/api"
	inboxapi "github.com/katyoti/katyoti-fwk/internal/extensions/inbox/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// THE WORK — planification's side of a to-do. Planning a request puts one
// to-do per step of its work template in the scheduler's queue (the module's
// `.ktw/todo/`), each carrying its work packet; planification decides when one
// is ready, collects each result, records what lasts on the request, and
// decides what follows. A to-do is a transient ticket: nothing lasting lives in
// it, nothing links to it.

// Owner is planification's name on the to-dos it puts.
const Owner = "planification"

// VerifiesUnchecked is WorkPacket.Verifies for a step whose approval verifies
// every deliverable of its request that no check verifies, as the request has
// them when it is approved.
const VerifiesUnchecked = "*"

// WorkPacket is what a to-do carries for planification: the step and what to
// do — copies of its template's, transient.
// Verifies is what a person approving the step verifies — a deliverable's
// title, or VerifiesUnchecked — set when the step is planned
// (RecipeStep.Verifies).
type WorkPacket struct {
	Template     string         `json:"template,omitempty" yaml:"template,omitempty"`
	Step         string         `json:"step" yaml:"step"`
	Doer         string         `json:"doer" yaml:"doer" jsonschema:"description=task or persona or person"`
	Task         string         `json:"task,omitempty" yaml:"task,omitempty"`
	Interaction  string         `json:"interaction,omitempty" yaml:"interaction,omitempty"`
	Instructions string         `json:"instructions,omitempty" yaml:"instructions,omitempty"`
	Params       map[string]any `json:"params,omitempty" yaml:"params,omitempty"`
	Gate         string         `json:"gate,omitempty" yaml:"gate,omitempty"`
	Confirms     string         `json:"confirms,omitempty" yaml:"confirms,omitempty"`
	Verifies     string         `json:"verifies,omitempty" yaml:"verifies,omitempty"`
	Phase        string         `json:"phase" yaml:"phase"`
	Starts       string         `json:"starts,omitempty" yaml:"starts,omitempty"`
	Moves        string         `json:"moves,omitempty" yaml:"moves,omitempty"`
	// EndsOn is, for a person's step, the state of its target that ends it:
	// the target reaching it (a decision resolved on its own panel) reports
	// the step done by whoever moved it, and closes its question in the inbox.
	EndsOn string `json:"ends_on,omitempty" yaml:"ends_on,omitempty"`
	// On is the object whose move to EndsOn ends the step, when it is not
	// the step's target: the decision a choice reported by an earlier step
	// opened (Choice) — the request's own node still moves when it ends.
	On      string   `json:"on,omitempty" yaml:"on,omitempty"`
	Refines bool     `json:"refines,omitempty" yaml:"refines,omitempty"`
	HeldBy  []string `json:"held_by,omitempty" yaml:"held_by,omitempty"`
	Stale   string   `json:"stale,omitempty" yaml:"stale,omitempty"`
	AskedIn string   `json:"asked_in,omitempty" yaml:"asked_in,omitempty"`
	// Ask is what a person's step asks as a human task (
	// with IN-039's switch on): its kind, its question, what it is about and
	// what its answer settles - the step itself. The to-do is its one record,
	// its report the answer; no inbox entry is filed.
	Ask *inboxapi.HumanTask `json:"ask,omitempty" yaml:"ask,omitempty"`
	// Label is the ask's kind's short label.
	Label string `json:"label,omitempty" yaml:"label,omitempty"`
	// AddedBy, AddedWhy and Generation say where a step added beyond its
	// request's recipe comes from: who added it - the
	// actor whose report, rejection or verb added it, or planification when
	// it adds one on its own -, why - the report's line -, and its
	// generation: the adding to-do's plus one. A recipe step has none
	// (generation 0).
	AddedBy    *cwmapi.Actor `json:"added_by,omitempty" yaml:"added_by,omitempty"`
	AddedWhy   string        `json:"added_why,omitempty" yaml:"added_why,omitempty"`
	Generation int           `json:"generation,omitempty" yaml:"generation,omitempty"`
	// AddedKey names a step a report added: the report it came from and its
	// rank among what that report added. A collect run again on the same
	// report finds the step it already added by it and adds no second one
	//
	AddedKey string `json:"added_key,omitempty" yaml:"added_key,omitempty"`
	// UnnamedRetries is how many times the reconciler sent the step back to
	// the queue after a block that named no cause:
	// once retried, its next unnamed block is the person's at once.
	UnnamedRetries int `json:"unnamed_retries,omitempty" yaml:"unnamed_retries,omitempty"`
}

// Added stamps p as a step added beyond its request's recipe by by, for why,
// from the to-do that added it: its generation is that one's plus one.
func (p *WorkPacket) Added(from WorkPacket, by cwmapi.Actor, why string) {
	a := by
	p.AddedBy, p.AddedWhy, p.Generation = &a, why, from.Generation+1
}

// WorkTodo is a to-do as planification reads it: the scheduler's ticket and
// its work packet.
type WorkTodo struct {
	schedapi.Todo
	Packet WorkPacket `json:"packet"`
}

// PlanWorkInput expands a request into its to-dos.
type PlanWorkInput struct {
	Request  string       `json:"request" jsonschema:"description=The request (or the object carrying it),required"`
	Again    bool         `json:"again,omitempty" jsonschema:"description=Plan it again: its to-dos not started are archived and the template expanded anew"`
	NoIntake bool         `json:"no_intake,omitempty" jsonschema:"description=Leave out the intake to-do (the request was already assessed)"`
	By       cwmapi.Actor `json:"by" jsonschema:"description=Who plans it,required"`
}

// WorkOutput is a list of to-dos.
type WorkOutput struct {
	Todos []WorkTodo `json:"todos"`
	Note  string     `json:"note,omitempty"`
}

// WorkTodoOutput is one to-do.
type WorkTodoOutput struct {
	Todo WorkTodo `json:"todo"`
	Note string   `json:"note,omitempty"`
}

// A person's verdict on a validation or a challenge.
const (
	VerdictApproved = "approved"
	VerdictRejected = "rejected"
)

// NewStep is a step a result adds to the plan.
type NewStep struct {
	Step         string         `json:"step" jsonschema:"description=Its name,required"`
	Doer         string         `json:"doer,omitempty" jsonschema:"description=task (default) or persona or person"`
	Task         string         `json:"task,omitempty" jsonschema:"description=The task or persona that does it; required unless a person does it"`
	Interaction  string         `json:"interaction,omitempty" jsonschema:"description=What a person's step asks: validate or challenge or decide or provide"`
	Skill        string         `json:"skill,omitempty" jsonschema:"description=The skill to follow (a skill URI)"`
	Instructions string         `json:"instructions" jsonschema:"description=What to do,required"`
	Phase        string         `json:"phase,omitempty" jsonschema:"description=Its phase; the finished to-do's when unset"`
	Params       map[string]any `json:"params,omitempty" jsonschema:"description=Its parameters"`
}

// Choice is a result that ends on a choice for a person (its more.choice):
// an analysis with options, a review that needs an arbitration. Planification
// opens a decision with it in the request's module (ktw-methods decisions:
// open, prepare, challenge), linked to the request, and plans a person's
// decide step on it — the validation that followed becomes that step; the
// option chosen passes to the steps after it.
type Choice struct {
	Question   string                      `json:"question" jsonschema:"description=The question in one line,required"`
	Context    string                      `json:"context,omitempty" jsonschema:"description=Why it is asked; the step's summary and details when unset"`
	Options    []decisionsapi.OptionDetail `json:"options" jsonschema:"description=The options: a short label each with what it means and its pros and cons and cost and consequence; at least two,required"`
	Proposal   decisionsapi.Proposal       `json:"proposal" jsonschema:"description=The option proposed (one of the labels) and why,required"`
	Challenges []string                    `json:"challenges,omitempty" jsonschema:"description=Points already raised against the proposal; one line each"`
}

// ChoiceKey is the result's more key a choice is reported under.
const ChoiceKey = "choice"

// FollowUp is a request a result files.
type FollowUp struct {
	Title    string `json:"title" jsonschema:"description=The ask in one line,required"`
	Details  string `json:"details,omitempty" jsonschema:"description=What the line leaves out"`
	Module   string `json:"module,omitempty" jsonschema:"description=The module it is filed in; the to-do's when unset"`
	Kind     string `json:"kind,omitempty" jsonschema:"description=build or fix or analyse or review or decide or maintain or question or discover"`
	Priority string `json:"priority,omitempty" jsonschema:"description=P0 to P3"`
	// Own says it is the request's own work, done in a request of its own:
	// linked to the request, it holds it open until it ends. Unset, it is
	// other work found on the way - filed, naming where it was found, and the
	// request ends without it.
	Own bool `json:"own,omitempty" jsonschema:"description=The request's own work: it holds the request open until it ends; unset for other work found on the way"`
}

// ListWorkInput lists a request's or a module's to-dos with their packets.
type ListWorkInput struct {
	Module  string `json:"module,omitempty" jsonschema:"description=The module"`
	Request string `json:"request,omitempty" jsonschema:"description=The request"`
	State   string `json:"state,omitempty" jsonschema:"description=waiting or ready or claimed or reported"`
}

// EditWorkInput changes a to-do not started: only what is given changes.
type EditWorkInput struct {
	ID           string         `json:"id" jsonschema:"description=The to-do,required"`
	Task         string         `json:"task,omitempty" jsonschema:"description=The task or persona that does it"`
	Skill        string         `json:"skill,omitempty" jsonschema:"description=The skill to follow (a skill URI)"`
	Instructions string         `json:"instructions,omitempty" jsonschema:"description=What to do"`
	Params       map[string]any `json:"params,omitempty" jsonschema:"description=Its parameters (replace the old ones)"`
	Priority     string         `json:"priority,omitempty" jsonschema:"description=P0 to P3"`
	Start        string         `json:"start,omitempty" jsonschema:"description=auto (the internal scheduler may run it) or manual"`
	By           cwmapi.Actor   `json:"by" jsonschema:"description=Who edits it,required"`
}

// AddWorkInput adds a step to a planned request.
type AddWorkInput struct {
	Request      string         `json:"request" jsonschema:"description=The request it is a step of,required"`
	Step         string         `json:"step" jsonschema:"description=Its name,required"`
	After        []string       `json:"after,omitempty" jsonschema:"description=The to-dos it comes after"`
	Before       []string       `json:"before,omitempty" jsonschema:"description=The to-dos that then come after it (not started)"`
	Doer         string         `json:"doer,omitempty" jsonschema:"description=task (default) or persona or person"`
	Task         string         `json:"task,omitempty" jsonschema:"description=The task or persona that does it; required unless a person does it"`
	Interaction  string         `json:"interaction,omitempty" jsonschema:"description=What a person's step asks: validate or challenge or decide or provide"`
	Skill        string         `json:"skill,omitempty" jsonschema:"description=The skill to follow (a skill URI)"`
	Instructions string         `json:"instructions" jsonschema:"description=What to do,required"`
	Phase        string         `json:"phase,omitempty" jsonschema:"description=Its phase; unset: preparation when it goes before a preparation step or reviews (a person's validate or challenge - a reviewer's challenge) after preparation steps only - build otherwise"`
	Params       map[string]any `json:"params,omitempty" jsonschema:"description=Its parameters"`
	Priority     string         `json:"priority,omitempty" jsonschema:"description=P0 to P3; the request's when unset"`
	Why          string         `json:"why,omitempty" jsonschema:"description=Why it is added: shown with the step in the request's steps"`
	By           cwmapi.Actor   `json:"by" jsonschema:"description=Who adds it,required"`
}

// RemoveWorkInput takes a to-do not started out of the plan.
type RemoveWorkInput struct {
	ID   string       `json:"id" jsonschema:"description=The to-do,required"`
	Note string       `json:"note" jsonschema:"description=Why it goes,required"`
	By   cwmapi.Actor `json:"by" jsonschema:"description=Who removes it,required"`
}

// MoveWorkInput sets what a to-do not started comes after.
type MoveWorkInput struct {
	ID    string       `json:"id" jsonschema:"description=The to-do,required"`
	After []string     `json:"after" jsonschema:"description=The to-dos it comes after now (empty: nothing)"`
	By    cwmapi.Actor `json:"by" jsonschema:"description=Who moves it,required"`
}

// RetryWorkInput sends a failed or blocked to-do back to the queue.
type RetryWorkInput struct {
	ID   string       `json:"id" jsonschema:"description=The to-do,required"`
	Note string       `json:"note,omitempty"`
	By   cwmapi.Actor `json:"by" jsonschema:"description=Who retries it,required"`
}

// Where a request stands, rolled up from its to-dos.
const (
	RollupNew        = "new"         // not planned: no to-dos
	RollupPlanned    = "planned"     // planned, nothing started
	RollupInProgress = "in-progress" // a to-do is pulled or has ended
	RollupWaiting    = "waiting"     // a person's to-do is theirs to do
	RollupBlocked    = "blocked"     // a to-do failed or is blocked, or its end was refused
	RollupDone       = "done"
	RollupDropped    = "dropped"
)

// RollupStates are where a request can stand, in the order it goes through them.
var RollupStates = []string{RollupNew, RollupPlanned, RollupInProgress, RollupWaiting, RollupBlocked, RollupDone, RollupDropped}

// RollupInput names the request.
type RollupInput struct {
	Request string `json:"request" jsonschema:"description=The request,required"`
}

// RollupOutput is where a request stands, from its to-dos.
type RollupOutput struct {
	Request string `json:"request"`
	State   string `json:"state" jsonschema:"description=new or planned or in-progress or waiting or blocked or done or dropped"`
	Done    int    `json:"done" jsonschema:"description=Its to-dos ended (archived)"`
	Total   int    `json:"total" jsonschema:"description=Its to-dos"`
	Note    string `json:"note,omitempty" jsonschema:"description=Why: what blocks it or whom it waits on"`
}

// WorkService is planification's work: a request planned into to-dos, edited
// before they run, its results collected.
type WorkService interface {
	Plan(ctx context.Context, in PlanWorkInput) (WorkOutput, error)
	List(ctx context.Context, in ListWorkInput) (WorkOutput, error)
	Edit(ctx context.Context, in EditWorkInput) (WorkTodoOutput, error)
	Add(ctx context.Context, in AddWorkInput) (WorkTodoOutput, error)
	Remove(ctx context.Context, in RemoveWorkInput) (WorkTodoOutput, error)
	Move(ctx context.Context, in MoveWorkInput) (WorkTodoOutput, error)
	Retry(ctx context.Context, in RetryWorkInput) (WorkTodoOutput, error)
	Rollup(ctx context.Context, in RollupInput) (RollupOutput, error)
}

// WorkServiceDef describes the service for KTSP/CLI auto-generation.
var WorkServiceDef = meta.ServiceDef{
	Name:    "work",
	Short:   "A request's work: planned into to-dos in its module's queue (the scheduler's), edited before they run, its results collected - what follows decided here",
	Version: "1.0.0",
	Methods: map[string]meta.MethodDef{
		"Plan":   meta.Method("Expand a request into its to-dos from its work template - the intake first unless no_intake - each with its work packet, in its module's queue; ready when nothing comes before it; refused when already planned unless again"),
		"List":   {Short: "A request's or a module's to-dos with their work packets, by request then position", ReadOnly: true},
		"Edit":   meta.Method("Change a to-do not started: its task, skill, instructions, parameters, priority, start - only what is given"),
		"Add":    meta.Method("Add a step to a planned request: after some to-dos, before others not started"),
		"Remove": meta.Method("Take a to-do not started out of the plan, saying why: what came after it waits on what it came after; the step that moves the node stays"),
		"Move":   meta.Method("Set what a to-do not started comes after; a cycle is refused"),
		"Retry":  meta.Method("Send a failed or blocked to-do back to the queue"),
		"Rollup": {Short: "Where a request stands from its to-dos: new, planned, in progress, waiting on a person, blocked, done or dropped", ReadOnly: true},
	},
}
