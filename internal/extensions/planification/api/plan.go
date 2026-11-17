// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"

	"github.com/katyoti/katyoti-fwk/internal/core/meta"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// PLAN — where the work on an object stands: its state in the plan workflow
// and the trail of its moves, its priority, a note on why it is blocked or
// idle, and one step record per step its to-dos did. The steps themselves are
// the scheduler's to-dos, planned from the object's work template (Work.Plan);
// what a step runs, in which phase, is on its to-do, never on the plan.

// CapPlan is the capability any planned object carries.
const CapPlan = "plan"

// The plan workflow's states. Whether a step runs now is not one of them: it
// is read from the to-dos (one claimed).
const (
	PlanIdle    = "idle"    // nothing planned: what next is someone's call
	PlanReady   = "ready"   // its to-dos are planned and may be worked
	PlanBlocked = "blocked" // its step failed or was stopped: a person looks (note)
	PlanSplit   = "split"   // replaced by its parts; ends when they all have
	PlanDone    = "done"
	PlanDropped = "dropped"
)

// Why a plan is blocked (PlanFields.Block): what its actions are chosen by,
// never its note's words.
const (
	// BlockEndRefused: its to-dos are done but its end was refused - a
	// deliverable not verified, its module not up to date, what it follows
	// not done.
	BlockEndRefused = "end-refused"
	// BlockMoveRefused: its to-dos are done but its node cannot move to the
	// state its recipe reaches; a person moving it settles the end.
	BlockMoveRefused = "move-refused"
	// BlockHandTestFailed: a hand test of it failed.
	BlockHandTestFailed = "hand-test-failed"
	// BlockParked: an agent reviewer sent its work back past the cap.
	BlockParked = "parked"
	// BlockManual: blocked by hand (the block verb), for a reason of its own.
	BlockManual = "manual"
)

// PlanStates are the plan workflow's states, in the order a plan meets them.
var PlanStates = []string{PlanIdle, PlanReady, PlanBlocked, PlanSplit, PlanDone, PlanDropped}

// The phases a step belongs to, in their order. Every shipped template step
// declares its own; readiness reads it — an intake never waits, a Preparation
// step waits only for its dependencies' own preparation, any other for their
// end. The plans list shows them by phase; the to-do queue orders by
// priority, age and position.
const (
	PhaseRequests    = "requests"    // receive, route, split
	PhaseInputs      = "inputs"      // what the work needs is known and there: what exists, an answer awaited
	PhasePreparation = "preparation" // analyse, specify, decide
	PhaseBuild       = "build"
	PhaseVerify      = "verify"
	PhaseRelease     = "release"
)

// Phases lists the phases in order.
var Phases = []string{PhaseRequests, PhaseInputs, PhasePreparation, PhaseBuild, PhaseVerify, PhaseRelease}

// PhaseIndex is a phase's rank in Phases; -1 when unknown.
func PhaseIndex(phase string) int {
	for i, p := range Phases {
		if p == phase {
			return i
		}
	}
	return -1
}

// The module edges the planner learns from the steps it submits.
const (
	EdgeReadsModule  = "reads-module"
	EdgeWritesModule = "writes-module"
)

// CapModuleUses is the capability a module object carries for the module edges
// the planner learned: who showed each, and when it was last seen.
const CapModuleUses = "module-uses"

// ModuleUsesFields is the `module-uses` capability's field group.
type ModuleUsesFields struct {
	Seen []ModuleUse `json:"seen,omitempty" jsonschema:"description=Each module edge the steps showed: reads-module or writes-module to a module"`
}

// ModuleUse is one module edge as the steps showed it.
type ModuleUse struct {
	Edge       string `json:"edge" jsonschema:"description=reads-module or writes-module,enum=reads-module|writes-module"`
	To         string `json:"to" jsonschema:"description=The module read or written"`
	ObservedBy string `json:"observed_by" jsonschema:"description=The planned object whose step showed it"`
	Task       string `json:"task,omitempty" jsonschema:"description=The task of that step"`
	LastSeen   string `json:"last_seen" jsonschema:"description=When a step last showed it (RFC 3339); an edge not seen for long is stale"`
}

// PlanGroup is the task group a step runs in when it names none.
const PlanGroup = "planning"

// PlanFields is the `plan` capability's field group.
type PlanFields struct {
	State    string                  `json:"state,omitempty" jsonschema:"description=Where its planning stands (the plan workflow); idle when unset; moved only by a transition"`
	Trail    []cwmapi.LifecycleEntry `json:"trail,omitempty" jsonschema:"description=The plan's transitions; written by the transition"`
	Priority string                  `json:"priority,omitempty" jsonschema:"description=P0 (now) or P1 (next) or P2 (later; the default) or P3 (someday); orders its to-dos in the queue"`
	Note     string                  `json:"note,omitempty" jsonschema:"description=Why it is blocked or idle; the outcome of its last step"`
	Block    string                  `json:"block,omitempty" jsonschema:"description=Why it is blocked - end-refused or move-refused or hand-test-failed or parked or manual; read only while it is blocked"`
	Steps    []StepRecord            `json:"steps,omitempty" jsonschema:"description=Each step its to-dos did: who did it and what it came to; its run and that run's session (its transcript) or the inbox request a person's step was asked in; oldest first"`
	Stale    []StaleStep             `json:"stale,omitempty" jsonschema:"description=The steps that were running when the request changed (rescoped or superseded or merged): their record says so"`
}

// StaleStep is a step that was running when its request changed: its worker
// is never interrupted, its record says what changed under it.
type StaleStep struct {
	Todo string `json:"todo" jsonschema:"description=The to-do that was running"`
	Why  string `json:"why"  jsonschema:"description=What changed under it"`
}

// StepRecord is one step the planner started: what ran, where its transcript
// is, or — for a person's step — the inbox request it was asked in.
type StepRecord struct {
	Task      string `json:"task,omitempty" jsonschema:"description=The task or persona that ran it; empty for a person's step"`
	Doer      string `json:"doer,omitempty" jsonschema:"description=task or persona or person or session (taken by hand)"`
	Step      string `json:"step,omitempty" jsonschema:"description=The recipe step it was (recipe/step)"`
	Phase     string `json:"phase,omitempty" jsonschema:"description=The phase it ran in"`
	Run       string `json:"run,omitempty" jsonschema:"description=Its run"`
	Session   string `json:"session,omitempty" jsonschema:"description=Its run's session: where its journal (the agent's transcript) is"`
	Ask       string `json:"ask,omitempty" jsonschema:"description=The inbox request a person's step was asked in"`
	StartedAt string `json:"started_at" jsonschema:"description=When it started (RFC 3339)"`
	// What it came to — written when its to-do's result is collected: the
	// lasting trace of the step, on the request.
	By      *cwmapi.Actor `json:"by,omitempty" jsonschema:"description=Who did it"`
	EndedAt string        `json:"ended_at,omitempty" jsonschema:"description=When its result was handed back (RFC 3339)"`
	Outcome string        `json:"outcome,omitempty" jsonschema:"description=success or fail or blocked"`
	Result  string        `json:"result,omitempty" jsonschema:"description=What was done - or what failed or blocks it: one line,content=markdown"`
	Details string        `json:"details,omitempty" jsonschema:"description=Its detailed result when one line is not enough,content=markdown"`
	// Where a step added beyond its request's recipe comes from
	// , copied from its to-do: none for a recipe step.
	AddedBy    *cwmapi.Actor `json:"added_by,omitempty" jsonschema:"description=Who added the step beyond the recipe; none for a recipe step"`
	AddedWhy   string        `json:"added_why,omitempty" jsonschema:"description=Why it was added: the line of the report that added it"`
	Generation int           `json:"generation,omitempty" jsonschema:"description=How far from the recipe it is: the generation of the step that added it plus one; 0 for a recipe step"`
}

// Who does a step: a task, a persona, or a person.
const (
	DoerTask    = "task"    // a task the planner runs (the default)
	DoerPersona = "persona" // a catalog persona the planner spawns
	DoerPerson  = "person"  // a person, asked in the inbox; the plan waits for the answer
	// DoerSession records a step a session (or a person) took by hand (Take):
	// the planner neither submits nor settles it; the taker ends it.
	DoerSession = "session"
)

// PlanStep is one step to run on a planned object.
type PlanStep struct {
	Task string `json:"task,omitempty" jsonschema:"description=The task to run on the object (its name or URI) or the persona to spawn; required unless a person does the step"`
	Doer string `json:"doer,omitempty" jsonschema:"description=Who does it: a task (the default) or a persona or a person (asked in the inbox),enum=task|persona|person"`
	// Interaction is what a person's step asks.
	Interaction string `json:"interaction,omitempty" jsonschema:"description=What a person's step asks: validate or challenge or decide or provide,enum=validate|challenge|decide|provide"`
	// Step is the recipe step it is, <recipe>/<step>: as it ends, the planner
	// plans the recipe's next one unless the task planned another.
	Step   string         `json:"step,omitempty" jsonschema:"description=The recipe step it is (recipe/step); the recipe's next one follows it unless another is planned"`
	Params map[string]any `json:"params,omitempty" jsonschema:"description=The task's input beside the object; opaque to the planner"`
	// Gate is the gate the step works toward on the object; its doer is told.
	Gate   string   `json:"gate,omitempty" jsonschema:"description=The gate the step works toward on the object (its requirements say what done is); the doer is told it"`
	Phase  string   `json:"phase" jsonschema:"description=The phase the step belongs to,enum=requests|inputs|preparation|build|verify|release"`
	Group  string   `json:"group,omitempty" jsonschema:"description=The task group it runs in; planning when unset"`
	Reads  []string `json:"reads,omitempty" jsonschema:"description=The modules the step uses (their API or docs); many readers run at once"`
	Writes []string `json:"writes,omitempty" jsonschema:"description=The modules the step changes; a writer runs alone on a module"`
}

// PlannedObject is a planned object as the plan service reads it.
type PlannedObject struct {
	ID        string   `json:"id"`
	Type      string   `json:"type"`
	Module    string   `json:"module"`
	Title     string   `json:"title"`
	DependsOn []string `json:"depends_on,omitempty"`
	SplitFrom string   `json:"split_from,omitempty"`
	FollowsUp string   `json:"follows_up,omitempty"`
	PlanFields
}

// PlanNextInput makes an object's plan ready: an object that carries no plan
// gets one. Its steps are its to-dos, planned from its work template.
type PlanNextInput struct {
	ID       string       `json:"id" jsonschema:"The object; required"`
	Priority string       `json:"priority,omitempty" jsonschema:"P0 to P3; unchanged when empty (P2 for a new plan)"`
	By       cwmapi.Actor `json:"by" jsonschema:"Who plans it; required"`
	Note     string       `json:"note,omitempty" jsonschema:"Why; recorded in the trail"`
}

// FinishInput ends a plan: its object's work is done, or will not be.
type FinishInput struct {
	ID      string       `json:"id" jsonschema:"The object; required"`
	Outcome string       `json:"outcome,omitempty" jsonschema:"done (default) or dropped,enum=done|dropped"`
	By      cwmapi.Actor `json:"by" jsonschema:"Who finishes it; required"`
	Note    string       `json:"note,omitempty" jsonschema:"Why; recorded in the trail"`
}

// BlockInput blocks a plan: its work cannot go on until someone looks.
type BlockInput struct {
	ID   string       `json:"id" jsonschema:"The object; required"`
	Note string       `json:"note" jsonschema:"What blocks it; required"`
	By   cwmapi.Actor `json:"by" jsonschema:"Who blocks it; required"`
}

// UnwaitInput removes a waiting to-do's wait, by a person: the wait no rule
// ends (the Blockers tab's « Retirer l'attente »).
type UnwaitInput struct {
	Todo string       `json:"todo" jsonschema:"description=The waiting to-do,required"`
	By   cwmapi.Actor `json:"by" jsonschema:"description=The person who removes its wait,required"`
}

// GetPlanInput reads one plan.
type GetPlanInput struct {
	ID string `json:"id" jsonschema:"The object; required"`
}

// ListPlansInput lists the plans, filtered.
type ListPlansInput struct {
	Module string `json:"module,omitempty" jsonschema:"Only the plans of this module"`
	State  string `json:"state,omitempty" jsonschema:"Only the plans in this state"`
}

// PlannedOutput is a plan after a verb.
type PlannedOutput struct {
	Plan PlannedObject `json:"plan"`
}

// ListPlansOutput is the plans listed, by priority then id.
type ListPlansOutput struct {
	Plans []PlannedObject `json:"plans"`
}

// PlanNote is a plan and a word on it.
type PlanNote struct {
	Plan string `json:"plan"`
	Note string `json:"note"`
}

// PlanRun is a step the planner submitted.
type PlanRun struct {
	Plan  string `json:"plan"`
	Task  string `json:"task"`
	Run   string `json:"run"`
	Group string `json:"group"`
}

// PlanService plans objects: the one way an object becomes ready, and the ways
// its plan ends.
type PlanService interface {
	Plan(ctx context.Context, in PlanNextInput) (PlannedOutput, error)
	Finish(ctx context.Context, in FinishInput) (PlannedOutput, error)
	Block(ctx context.Context, in BlockInput) (PlannedOutput, error)
	Unwait(ctx context.Context, in UnwaitInput) (PlannedOutput, error)
	Get(ctx context.Context, in GetPlanInput) (PlannedOutput, error)
	List(ctx context.Context, in ListPlansInput) (ListPlansOutput, error)
	Pause(ctx context.Context, in PlanningSwitchInput) (PlanningSwitchOutput, error)
	Resume(ctx context.Context, in PlanningSwitchInput) (PlanningSwitchOutput, error)
}

// PlanningSwitchInput pauses or resumes planning, saying who.
type PlanningSwitchInput struct {
	By cwmapi.Actor `json:"by" jsonschema:"description=Who pauses or resumes it,required"`
}

// PlanningSwitchOutput is the planning group after the switch.
type PlanningSwitchOutput struct {
	Group string `json:"group"`
	On    bool   `json:"on"`
}

// PlanServiceDef describes the service for KTSP/CLI auto-generation.
var PlanServiceDef = meta.ServiceDef{
	Name:    "plans",
	Short:   "Plan objects: where the work on each stands, its priority and the record of its steps",
	Version: "1.0.0",
	Methods: map[string]meta.MethodDef{
		"Plan":   meta.Method("Make an object's plan ready, with its priority: one with no plan gets one; its steps are its to-dos"),
		"Finish": meta.Method("End an object's plan: done (default) or dropped"),
		"Block":  meta.Method("Block an object's plan with what blocks it, until someone plans it again"),
		"Unwait": meta.Method("Remove a waiting to-do's wait, by a person: a blocked report's hold on an object is cleared, a wait on a request it depends on drops that depends-on edge; the step goes back to the queue"),
		"Get":    {Short: "Read one plan", ReadOnly: true},
		"List":   {Short: "List the plans, by priority then id; filter by module, state", ReadOnly: true},
		"Pause":  meta.Method("Pause planning: the task group the planner and its steps run in stops starting runs (they queue)"),
		"Resume": meta.Method("Resume planning: the planning task group starts runs again"),
	},
}
