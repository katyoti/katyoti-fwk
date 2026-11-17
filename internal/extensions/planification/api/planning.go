// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"

	"github.com/katyoti/katyoti-fwk/internal/core/meta"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// PLANNING — the requests of a release turned into workload. The release IS
// the plan: its network is the requests targeting it and its steps, ordered by
// `depends-on`; a step is a sync point that joins parallel branches into
// something deliverable. Steps are the release's children, in its planning
// space.

// Declarations of the planification pack, for planning.
const (
	StepType = "step"
	// CapStep is the step field-group.
	CapStep = "step"
	// CapNetworkNode marks what a release's network is made of: requests and
	// steps. `depends-on` joins network nodes.
	CapNetworkNode = "network-node"

	// EdgeShips: a step ships a release (N:1).
	EdgeShips = "ships"
)

// The states of the `step` workflow.
const (
	StepOpen    = "open"
	StepReached = "reached"
	StepDropped = "dropped"
)

// StepFields is the `step` field-group (cap-step).
type StepFields struct {
	Produces string `json:"produces,omitempty" jsonschema:"description=What reaching the step produces (a release candidate or a merged branch of work) - not a request's verified deliverables"`
}

// Plan is a release's plan as the service reads it: its steps and the
// requests targeting it.
type Plan struct {
	Release  string   `json:"release"`
	Steps    []Step   `json:"steps,omitempty"`
	Requests []string `json:"requests,omitempty" jsonschema:"The requests targeting it"`
}

// Step is a step as the service reads it.
type Step struct {
	ID      string   `json:"id"`
	Release string   `json:"release"`
	Name    string   `json:"name"`
	State   string   `json:"state"`
	After   []string `json:"after,omitempty" jsonschema:"What it waits on (its depends-on)"`
	Ships   string   `json:"ships,omitempty" jsonschema:"The release it ships"`
	StepFields
}

// PlanningServiceDef describes the service for KTSP/CLI auto-generation.
var PlanningServiceDef = meta.ServiceDef{
	Name:    "planning",
	Short:   "The plan of a release: its steps, its network of work, what can start",
	Version: "1.0.0",
	Methods: map[string]meta.MethodDef{
		"GetPlan":         meta.Method("Read a release's plan: its steps and the requests targeting it"),
		"AddStep":         meta.Method("Add a step (a sync point) to a release's plan"),
		"Advance":         meta.Method("Move a step through its workflow"),
		"CheckTransition": meta.Method("The on_transition hook of steps: a step is reached when all it waits on is done"),
		"GuiTested":       {Short: "The gui-tested requirement of ready-to-test: a feature that changes a screen passes only with a criterion running a live GUI test by element ids on a scenario case, its last verdict passed", ReadOnly: true},
		"ListRecipes":     meta.Method("The recipes: for each kind of request, the steps its work takes"),
		"Network":         meta.Method("A release's PERT network over its remaining work: what can start now, the critical path and its length, each node's slack, what each step waits on"),
		"Check":           {Short: "What holds each open piece of work of an increment, a release or the whole workspace (--census), sorted into a kind - the person's answer, the person's act, agent work ready, agent work held, outside, incident - with who acts and since when; writes nothing", ReadOnly: true},
		"Reconcile":       {Short: "One sweep of the reconciler: what it would repair safely (a done lifecycle with an open plan, a ready plan with no to-do, an answer not applied, a wait on something ended, stale labels) and the incidents it cannot repair; with apply, the repairs switched on in Planification.yaml are applied - each only if what it observed is unchanged, never undoing an act, stopping on an element it repairs too often", TimeoutS: 600},
	},
}

// NetNode is one node of a release's network, with its schedule over the
// REMAINING work: an ended node weighs 0, so the numbers say what is left.
type NetNode struct {
	ID   string `json:"id"`
	Type string `json:"type"  jsonschema:"The object's type: a step, or what carries the request (request, goal, feature...)"`
	// Request marks work asked for — an object carrying a request —
	// rather than a step.
	Request bool   `json:"request,omitempty"`
	Title   string `json:"title"`
	State   string `json:"state"`
	Ended   bool   `json:"ended" jsonschema:"Done or reached or dropped"`
	// Weight is the remaining work: a request's estimate (1 when missing), 0
	// for a step or an ended node.
	Weight float64 `json:"weight"`
	// After is what it waits on inside the release; Outside what it waits on
	// elsewhere that is not done yet.
	After   []string `json:"after,omitempty"`
	Outside []string `json:"outside,omitempty"`
	// Earliest / latest start, and slack: how much it may slip without
	// lengthening the release.
	EarliestStart float64 `json:"earliest_start"`
	LatestStart   float64 `json:"latest_start"`
	Slack         float64 `json:"slack"`
	Critical      bool    `json:"critical"`
	// Runnable: an open request whose predecessors are all done — it can start now.
	Runnable bool `json:"runnable,omitempty"`
}

// StepWait says what a step still waits on.
type StepWait struct {
	ID        string   `json:"id"`
	WaitsOn   []string `json:"waits_on"  jsonschema:"Its predecessors not done yet"`
	Reachable bool     `json:"reachable" jsonschema:"Every predecessor done: it can be reached now"`
}

// NetworkOutput is a release's network.
type NetworkOutput struct {
	Release      string     `json:"release"`
	Nodes        []NetNode  `json:"nodes"`
	Runnable     []string   `json:"runnable"      jsonschema:"What can start now"`
	CriticalPath []string   `json:"critical_path" jsonschema:"The longest chain of remaining work"`
	Duration     float64    `json:"duration"      jsonschema:"Its length: the remaining work's critical path in estimate units"`
	Steps        []StepWait `json:"steps,omitempty"`
}

// PlanningService manages the plan of each release: its steps and its
// network.
type PlanningService interface {
	GetPlan(ctx context.Context, in GetInput) (PlanOutput, error)
	AddStep(ctx context.Context, in AddStepInput) (StepOutput, error)
	Advance(ctx context.Context, in AdvanceInput) (AdvanceOutput, error)
	CheckTransition(ctx context.Context, in HookInput) (CheckReadyOutput, error)
	GuiTested(ctx context.Context, in HookInput) (CheckReadyOutput, error)
	Network(ctx context.Context, in GetInput) (NetworkOutput, error)
	Check(ctx context.Context, in PlanningCheckInput) (PlanningCheckOutput, error)
	Reconcile(ctx context.Context, in ReconcileInput) (ReconcileOutput, error)
	ListRecipes(ctx context.Context, in ListAllInput) (ListRecipesOutput, error)
}

// ListAllInput is empty: the list is the project's.
type ListAllInput struct{}

// PlanOutput returns a release's plan.
type PlanOutput struct {
	Plan Plan `json:"plan"`
}

// AddStepInput adds a step.
type AddStepInput struct {
	Release  string   `json:"release"               jsonschema:"The release whose plan it joins; required"`
	Name     string   `json:"name"                  jsonschema:"The step's name; required"`
	Produces string   `json:"produces,omitempty" jsonschema:"What reaching it produces"`
	After    []string `json:"after,omitempty"       jsonschema:"What it waits on: requests or steps of the release"`
	Ships    string   `json:"ships,omitempty"       jsonschema:"The release reaching it ships (its own, or a module release shipping in it)"`
}

// StepOutput returns a step.
type StepOutput struct {
	Step Step   `json:"step"`
	Path string `json:"path,omitempty"`
}

// AdvanceInput moves a step.
type AdvanceInput struct {
	ID   string       `json:"id"             jsonschema:"The step; required"`
	To   string       `json:"to"             jsonschema:"The state it moves to; required"`
	By   cwmapi.Actor `json:"by"             jsonschema:"Who moves it; required"`
	Note string       `json:"note,omitempty" jsonschema:"Why"`
}

// AdvanceOutput says where it moved.
type AdvanceOutput struct {
	ID    string `json:"id"`
	From  string `json:"from"`
	State string `json:"state"`
}
