// SPDX-License-Identifier: Apache-2.0

// Package api is the planification extension's request contract. Work asked
// for is a REQUEST: the `request` capability — the ask: where it comes from, its
// details, what it must deliver — carried by any object. A request filed ON a
// goal, a feature or a use case is carried by that object, which keeps its own
// lifecycle beside it; a request filed on its own (a fix, a decision to take) is
// a `request` object, which carries the capability and nothing more. Every
// verb is implemented over the CWM Object API — the service keeps no storage
// and no index of its own.
//
// A request has ONE state: its plan's (plan.go). Filing gives it a plan whose
// first step is intake; the planner runs it; the steps plan what comes next;
// it is done when its plan is (Finish refuses done while a deliverable is
// unverified), or — split into parts, one per module — when its parts are. A
// request not yet routed to a module, and a parent concerning several, lives
// in the `sys-planification` system module, which Ensure creates (the
// `bootstrap` hook calls it when a project opens). Ended requests stay, as
// history.
package api

import (
	"context"

	"github.com/katyoti/katyoti-fwk/internal/core/meta"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// Declarations of the planification pack.
const (
	// RequestType is the object type of work asked for on its own.
	RequestType = "request"
	// CapRequest is the request capability: the ask's field-group.
	CapRequest = "request"
	// EdgeDependsOn orders network nodes (requests, steps): the source waits on the target.
	EdgeDependsOn = "depends-on"
	// EdgeResolves points a request at the object whose gate requirement it
	// meets (gates.go); a request filed on the object itself has none.
	EdgeResolves = "resolves"
	// EdgeSplitFrom points a part at the request it was split from.
	EdgeSplitFrom = "split-from"
	// EdgeFollowsUp points a follow-up — a request a step's result filed — at
	// the request that filed it; that request ends once its follow-ups have.
	EdgeFollowsUp = "follows-up"
	// EdgeJoined: a request filed straight into a running increment.
	EdgeJoined = "joined"
	// EdgeAskedIn points what waits on a person's answer at the inbox
	// approval-request carrying the question: a request whose requester was
	// asked, an object whose gate needs a person's confirmation.
	EdgeAskedIn = "asked-in"
	// SystemModule holds the requests not yet routed to a module and the
	// demands from outside: the scheduler's requests module, the one system
	// module whose intake to-dos are queued.
	SystemModule = schedapi.RequestsModule
)

// The ended states a request's plan reaches (the plan's own words).
const (
	StateDone    = PlanDone
	StateDropped = PlanDropped
)

// The kinds of request: what the work is for — and so the recipe that
// plans it.
const (
	KindBuild    = "build"    // makes something exist or change
	KindFix      = "fix"      // closes a gap or a defect
	KindAnalyse  = "analyse"  // finds out; produces requirements, not code
	KindReview   = "review"   // judges quality
	KindDecide   = "decide"   // settles a question; a person decides
	KindMaintain = "maintain" // keeps an artefact true
	KindQuestion = "question" // asks; done when answered (Answer)
	KindDiscover = "discover" // finds what existing software does and why: proposed wants
)

// Priorities, most urgent first; unset counts as DefaultPriority.
var Priorities = []string{"P0", "P1", "P2", "P3"}

// DefaultPriority is a request's priority when none is set.
const DefaultPriority = "P2"

// Who reviews a build's work: an agent by default, a
// person where the request, its release or its stream asks it.
const (
	ReviewAgent  = "agent"
	ReviewPerson = "person"
)

// Kinds lists the kinds of request, in their documented order.
var Kinds = []string{KindBuild, KindFix, KindAnalyse, KindReview, KindDecide, KindMaintain, KindQuestion, KindDiscover, KindDemand}

// The sources a request may come from, beyond a person or an agent (who file
// and move): each files, none moves anything.
const (
	// RequesterModule — a module asks another: its id is the asking module.
	RequesterModule = "module"
	// RequesterFeature — a feature asks for what it needs: its id is the feature.
	RequesterFeature = "feature"
	// RequesterGoal — a goal asks for work that serves it: its id is the goal.
	RequesterGoal = "goal"
	// RequesterExternal — a system outside the workspace (a tracker, a mail
	// gateway, a customer portal): its id names the system and its reference.
	RequesterExternal = "external"
)

// RequesterKinds lists the kinds of source a request may come from.
var RequesterKinds = []string{"person", "agent", RequesterModule, RequesterFeature, RequesterGoal, RequesterExternal}

// EdgeAbout points a request at the object it concerns (CWM's core edge;
// the object's pane lists it).
const EdgeAbout = "about"

// EdgeServes points a request at the want it serves — a goal, a feature, a use
// case (the method's alignment edge). A request filed ON a want serves it
// without one.
const EdgeServes = "serves"

// RequestFields is the `request` field-group — the Go type its
// data-type (cap-request) is rendered from: the ask. The one-line ask is the
// object's title; what it targets is its module; its state is its plan's.
type RequestFields struct {
	Requester    cwmapi.Actor  `json:"requester" jsonschema:"description=Where it comes from: a person or an agent; a module or a feature or a goal (its id); an external system"`
	Details      string        `json:"details,omitempty" jsonschema:"description=What the one-line ask leaves out,content=markdown"`
	Deliverables []Deliverable `json:"deliverables,omitempty" jsonschema:"description=What the work must produce; done waits until every one is verified"`
	BudgetUSD    float64       `json:"budget_usd,omitempty" jsonschema:"description=Spend ceiling for the work in US dollars; 0 means none stated"`
	Estimate     float64       `json:"estimate,omitempty" jsonschema:"description=Its weight in its release's network (agent-hours or cost); missing counts as 1"`
	FiledAt      string        `json:"filed_at,omitempty" jsonschema:"description=When it was filed (RFC 3339)"`
	Kind         string        `json:"kind,omitempty" jsonschema:"description=What the work is for: build or fix or analyse or review or decide or maintain or question or discover or demand; it gives the default first step after intake (build when unset)"`
	// Delta is a demand's: what its reconcile proposed, checked; applied once
	// the person approved it.
	Delta []DeltaOp `json:"delta,omitempty" jsonschema:"description=A demand's delta: the operations its reconcile proposed and planification checked"`
	// Applied is what each operation of the delta changed or created.
	Applied  []Applied `json:"applied,omitempty" jsonschema:"description=A demand's applied delta: the object each operation changed or created"`
	Priority string    `json:"priority,omitempty" jsonschema:"description=P0 (now) or P1 (next) or P2 (later; the default) or P3 (someday)"`
	Depth    string    `json:"depth,omitempty" jsonschema:"description=How deep its work goes: full (the default) or quick - the depth of the work template its plan is expanded from"`
	// Review is who reviews its work: an agent (the default) or a person.
	Review          string         `json:"review,omitempty" jsonschema:"description=Who reviews its work: agent (the default - unless its release or stream asks a person) or person,enum=agent|person"`
	Source          *RequestSource `json:"source,omitempty" jsonschema:"description=What filed it when no one did: a trigger or a rule or a gate - and the change that fired it"`
	Answer          *Answer        `json:"answer,omitempty" jsonschema:"description=The answer to a question; written by Answer which then finishes the request"`
	GateRequirement string         `json:"gate_requirement,omitempty" jsonschema:"description=The gate requirement this request meets as <gate>/<requirement>; set by the planner when a gate refused a move (the object is the resolves edge or the object carrying the request)"`
	ModuleUpdate    *ModuleUpdate  `json:"module_update,omitempty" jsonschema:"description=What its module-update step recorded: the change-log entry (or why no user sees the change) and the acceptance checks it changed; written by ModuleUpdate"`
	Waiver          *Waiver        `json:"waiver,omitempty" jsonschema:"description=A person's waiver of the module-updated gate: who and when and why; written by Waive"`
}

// ModuleUpdate is what a request's module-update step recorded: the change
// log entry it wrote — or that no user sees the change, and why — and the
// acceptance checks it added or changed to prove it.
type ModuleUpdate struct {
	Changelog   string   `json:"changelog,omitempty" jsonschema:"description=The change-log entry that names this request: its page and number (e.g. docs/confluence-tew/changelog/2026.10.md#345)"`
	UserVisible *bool    `json:"user_visible,omitempty" jsonschema:"description=false when no user sees the change: no change-log entry then and the reason says why"`
	Reason      string   `json:"reason,omitempty" jsonschema:"description=Why the change is not in the change log (with user_visible false)"`
	Checks      []string `json:"checks,omitempty" jsonschema:"description=The ids of the acceptance checks added or changed to prove the change"`
	NoCheck     string   `json:"no_check,omitempty" jsonschema:"description=Direct work only: why no acceptance check proves it - nothing a test can see changed (a label or a message or a doc line)"`
	At          string   `json:"at,omitempty" jsonschema:"description=When it was recorded (RFC 3339)"`
}

// Recorded reports a change log answered: an entry, or user_visible false
// with its reason.
func (m ModuleUpdate) Recorded() bool {
	return m.Changelog != "" || (m.UserVisible != nil && !*m.UserVisible && m.Reason != "")
}

// Waiver is a person's waiver of the module-updated gate on one request.
type Waiver struct {
	By   cwmapi.Actor `json:"by" jsonschema:"description=The person who waived it"`
	At   string       `json:"at" jsonschema:"description=When (RFC 3339)"`
	Note string       `json:"note" jsonschema:"description=Why the module needs no update for this request"`
	// Relayed is set when an agent's session relayed the person's waiver.
	Relayed *Relayed `json:"relayed,omitempty" jsonschema:"description=Set when an agent's session relayed the person's waiver"`
}

// Relayed records a person's act an agent's session relayed: which session,
// how the person confirmed it (prompt or totp or entra) and when.
type Relayed struct {
	By        string `json:"by"           jsonschema:"description=The agent session that relayed it"`
	Confirmed string `json:"confirmed"    jsonschema:"description=How the person confirmed it: prompt or totp or entra"`
	At        string `json:"at,omitempty" jsonschema:"description=When the person confirmed it (RFC 3339)"`
}

// Deliverable is one thing the work must produce, and whether it is verified.
type Deliverable struct {
	Title string `json:"title" jsonschema:"description=What is delivered"`
	Check string `json:"check,omitempty" jsonschema:"description=How it is verified: a command or an action URI"`
	// Timeout is how long its command check may run when that is longer than
	// the default (10 minutes): a slow check declares its own bound.
	Timeout string `json:"timeout,omitempty" jsonschema:"description=How long its command check may run before it is stopped: a duration (30m) at most one hour; empty is the default of 10 minutes"`
	// ByHand is, for a deliverable only a person can test, how: the exact
	// steps, and why no agent can — what « À faire » shows its hand test with.
	ByHand     string `json:"by_hand,omitempty" jsonschema:"description=For a deliverable a person tests by hand: the exact steps and why no agent can"`
	Verified   bool   `json:"verified,omitempty" jsonschema:"description=The check passed"`
	VerifiedAt string `json:"verified_at,omitempty" jsonschema:"description=RFC 3339"`
	// VerifiedBy is the person who verified it by hand (Verify); empty when
	// its check passed.
	VerifiedBy *cwmapi.Actor `json:"verified_by,omitempty" jsonschema:"description=The person who verified it by hand; empty when its check passed"`
	Evidence   string        `json:"evidence,omitempty" jsonschema:"description=What the person saw that verifies it (by hand only)"`
	// Relayed is set when an agent's session relayed the person's
	// verification by hand.
	Relayed *Relayed `json:"relayed,omitempty" jsonschema:"description=Set when an agent's session relayed the person's verification by hand"`
}

// Answer is the answer to a question: the text, who gave it, when.
type Answer struct {
	Text string       `json:"text" jsonschema:"description=The answer"`
	By   cwmapi.Actor `json:"by" jsonschema:"description=Who answered"`
	At   string       `json:"at,omitempty" jsonschema:"description=When (RFC 3339)"`
}

// ChannelInput names the module.
type ChannelInput struct {
	Module string `json:"module" jsonschema:"description=The module,required"`
}

// ChannelItem is one request in a module's channel.
type ChannelItem struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Kind    string `json:"kind,omitempty"`
	State   string `json:"state"`
	Module  string `json:"module" jsonschema:"description=The module it is filed in"`
	From    string `json:"from" jsonschema:"description=Who or what filed it: its requester (id and kind) or its source (trigger or rule or gate and its name)"`
	Answer  string `json:"answer,omitempty" jsonschema:"description=A question's answer"`
	FiledAt string `json:"filed_at,omitempty"`
}

// ChannelOutput is a module's channel, the latest first in each list.
type ChannelOutput struct {
	Module  string        `json:"module"`
	In      []ChannelItem `json:"in" jsonschema:"description=The open requests filed in the module"`
	Out     []ChannelItem `json:"out" jsonschema:"description=The requests it filed in other modules"`
	Answers []ChannelItem `json:"answers" jsonschema:"description=Its questions answered"`
}

// Request is a request as the service reads it, with the object carrying it.
type Request struct {
	ID        string      `json:"id"`
	Type      string      `json:"type" jsonschema:"The object's type: request, or the goal / feature / use case the request is on"`
	Module    string      `json:"module"`
	Title     string      `json:"title" jsonschema:"The one-line ask"`
	State     string      `json:"state" jsonschema:"The request's state: its plan's"`
	Plan      *PlanFields `json:"plan,omitempty" jsonschema:"Its plan: where it stands and what each step came to"`
	DependsOn []string    `json:"depends_on,omitempty" jsonschema:"What it waits on"`
	// The relations, read from its edges.
	SplitFrom string   `json:"split_from,omitempty" jsonschema:"The request it was split from"`
	FollowsUp string   `json:"follows_up,omitempty" jsonschema:"The request whose step filed it (a follow-up)"`
	Resolves  string   `json:"resolves,omitempty"   jsonschema:"The object whose gate requirement it meets"`
	AskedIn   []string `json:"asked_in,omitempty"   jsonschema:"The inbox requests carrying its questions, oldest first"`
	Serves    []string `json:"serves,omitempty"     jsonschema:"The wants it serves (its serves edges)"`
	RequestFields
}

// RequestServiceDef describes the service for KTSP/CLI auto-generation.
var RequestServiceDef = meta.ServiceDef{
	Name:    "requests",
	Short:   "Work asked for: file a request from any source, split it per module, order it, answer it",
	Version: "1.0.0",
	Methods: map[string]meta.MethodDef{
		"FixCriterion":       meta.Method("File the fix of a failed acceptance criterion in one action: on the feature it proves, into the open increment holding that feature, naming the criterion and its failure"),
		"File":               meta.Method("File a request from any source (a person, an agent, a module, a feature, a goal, an external system): on an object (a goal, a feature, a use case), which then carries it beside its own lifecycle, or on its own in the module it targets (sys-planification when none); its plan's first step is intake"),
		"Get":                meta.Method("Read one request"),
		"List":               meta.Method("List requests, by module, state (its plan's) and requester"),
		"Split":              meta.Method("Split any demand (a request, a feature, a goal) into parts — a request per module it affects, one filed on an unmet goal, or a sub-feature under it — each keeping its requester and dependencies and planned on its own; the demand stays, split, and ends when its parts have"),
		"Demands":            {Short: "The demands — requests and every planned feature or goal — as trees, each part under the demand it was split from (in any module): its state, next step and why it waits; open, mine or history", ReadOnly: true},
		"Assess":             meta.Method("Record intake's assessment of a request that has not ended: its kind, priority, estimate — only what is given changes"),
		"Ask":                meta.Method("Ask a request's requester one question, in the inbox; the request waits (its plan idle) and the answer plans intake again, the question and answer added to its details"),
		"Channel":            {Short: "A module's channel: In - the open requests filed in it and from whom (a person or an agent or a module or an external system or a trigger or a rule or a gate) - Out - the requests it filed in other modules and where they stand - and the answers it received to its questions", ReadOnly: true},
		"Answer":             meta.Method("Answer a request of kind question: the answer is recorded on it and it is done"),
		"Depend":             meta.Method("Order a request or a step after another (a depends-on edge); refused when it would close a cycle"),
		"Ensure":             meta.Method("Give the project its sys-planification module, where unrouted requests wait (idempotent; the bootstrap hook)"),
		"PlanModules":        meta.Method("The bootstrap of a project: every module with planning work - a request, a goal or a feature of its own, or code packages - carries the planned capability (idempotent); a module with none, a study or a dossier, is left as it is"),
		"PlanModule":         meta.Method("The on_create hook of a module: a module created plain - with no capability of its own beyond its type's - is given the planned capability (its work is planned here); one created with a domain capability of its own (a study or a dossier) is left to its domain"),
		"Revise":             meta.Method("Correct an open request's deliverables (a check, a title): the full new list, why it changes recorded in its details; a deliverable whose check changed must be verified again"),
		"Check":              {Short: "Run an open request's unverified deliverable checks now: each one that passes is recorded verified; each result is answered", TimeoutS: 3600}, // a check may run a clean-machine proof (a Windows Sandbox, about 15 min)
		"AutoTest":           meta.Method("Ask an agent to test the deliverables of an open request no check verifies (all, or those named): a step that writes a falsified check for each and runs it, or marks one only a person can judge by_hand with why; a blocked request goes on"),
		"Test":               meta.Method("Record a hand test of one deliverable of an open request, by a person: it passes (verified, the note as what was seen) or fails with a note (the validation waiting on it sent back with that note, else the request blocked saying why)"),
		"AmendDelta":         meta.Method("Amend one operation of a demand's delta before it is applied, by a person: the fields given replace its own (title, details, under, module, object, increment, for), the delta checked again whole; an operation already applied is refused"),
		"Rescope":            meta.Method("Reword an open request (its title or details) saying why: the old wording kept in its details; its steps not started take the new title and a running one is marked stale"),
		"Supersede":          meta.Method("End an open request for another open request that carries its work (superseded-by): it ends dropped; what depended on it depends on its successor; a running step is marked stale; its requester is told"),
		"Merge":              meta.Method("Merge an open request into another whose work has not started: its details and deliverables and its serves and targets and depends-on links carried over; then it ends as superseded"),
		"Hold":               meta.Method("Put one item of the person's queue (« À faire ») on hold, by a person, with an optional reason: it leaves its action and the count, listed last under « En attente »; nothing else changes - what it is about keeps its state and what waits on it stays waiting"),
		"Unhold":             meta.Method("Take an item on hold back to its action in the person's queue (« Reprendre »), by a person"),
		"Verify":             meta.Method("Record one deliverable of an open request as verified by hand, by a person, with what they saw - for a check no command can run"),
		"ModuleUpdate":       meta.Method("Record what a request's module-update step did: the change-log entry naming the request (or user_visible false and why) and the acceptance checks it added or changed"),
		"Waive":              meta.Method("Waive the module-updated gate on one open request, by a person only, saying why: it ends without its module brought up to date"),
		"CheckModuleUpdated": {Short: "Whether a request's module is up to date (the module-updated gate's requirement): the want it serves set, a changed acceptance check, its change log recorded - or why it is exempt", ReadOnly: true},
	},
}

// RequestService manages requests.
type RequestService interface {
	File(ctx context.Context, in FileInput) (FileOutput, error)
	FixCriterion(ctx context.Context, in FixCriterionInput) (FileOutput, error)
	Get(ctx context.Context, in GetInput) (GetOutput, error)
	List(ctx context.Context, in ListInput) (ListOutput, error)
	Split(ctx context.Context, in SplitInput) (SplitOutput, error)
	Assess(ctx context.Context, in AssessInput) (RequestOutput, error)
	Ask(ctx context.Context, in AskInput) (RequestOutput, error)
	Answer(ctx context.Context, in AnswerInput) (RequestOutput, error)
	Depend(ctx context.Context, in DependInput) (RequestOutput, error)
	Ensure(ctx context.Context, in EnsureInput) (EnsureOutput, error)
	PlanModule(ctx context.Context, in cwmapi.HookCall) (PlanModuleOutput, error)
	PlanModules(ctx context.Context, in EnsureInput) (PlanModulesOutput, error)
	Demands(ctx context.Context, in DemandsInput) (DemandsOutput, error)
	Revise(ctx context.Context, in ReviseInput) (RequestOutput, error)
	Check(ctx context.Context, in CheckInput) (CheckOutput, error)
	Verify(ctx context.Context, in VerifyInput) (RequestOutput, error)
	Test(ctx context.Context, in TestInput) (RequestOutput, error)
	AutoTest(ctx context.Context, in AutoTestInput) (RequestOutput, error)
	Rescope(ctx context.Context, in RescopeInput) (RequestOutput, error)
	AmendDelta(ctx context.Context, in AmendDeltaInput) (RequestOutput, error)
	Supersede(ctx context.Context, in SupersedeInput) (RequestOutput, error)
	Merge(ctx context.Context, in MergeInput) (RequestOutput, error)
	Hold(ctx context.Context, in HoldInput) (HoldOutput, error)
	Unhold(ctx context.Context, in UnholdInput) (HoldOutput, error)
	Channel(ctx context.Context, in ChannelInput) (ChannelOutput, error)
	ModuleUpdate(ctx context.Context, in ModuleUpdateInput) (RequestOutput, error)
	Waive(ctx context.Context, in WaiveInput) (RequestOutput, error)
	CheckModuleUpdated(ctx context.Context, in ModuleUpdatedInput) (CheckReadyOutput, error)
}

// ModuleUpdateInput records what a request's module-update step did.
type ModuleUpdateInput struct {
	ID        string       `json:"id" jsonschema:"description=The request,required"`
	Changelog string       `json:"changelog,omitempty" jsonschema:"description=The change-log entry that names the request: its page and number (e.g. docs/confluence-tew/changelog/2026.10.md#345)"`
	Internal  bool         `json:"internal,omitempty" jsonschema:"description=No user sees the change (a refactor or a test or an internal API): no change-log entry - say why in reason (recorded user_visible false)"`
	Reason    string       `json:"reason,omitempty" jsonschema:"description=Why no change-log entry: what makes the change invisible to every user"`
	Checks    []string     `json:"checks,omitempty" jsonschema:"description=The ids of the acceptance checks added or changed to prove the change"`
	NoCheck   string       `json:"no_check,omitempty" jsonschema:"description=Direct work only (a fix or a quick build serving no want): why no acceptance check proves it - nothing a test can see changed"`
	By        cwmapi.Actor `json:"by" jsonschema:"description=Who records it,required"`
}

// WaiveInput waives the module-updated gate on one request.
type WaiveInput struct {
	ID   string       `json:"id" jsonschema:"description=The request,required"`
	Note string       `json:"note" jsonschema:"description=Why its module needs no update,required"`
	By   cwmapi.Actor `json:"by" jsonschema:"description=The person who waives it (a person only),required"`
}

// ModuleUpdatedInput names the request whose module update is checked: the
// gate's call ({point, object}) or an id.
type ModuleUpdatedInput struct {
	ID     string         `json:"id,omitempty"     jsonschema:"description=The request (a direct call)"`
	Point  string         `json:"point,omitempty"  jsonschema:"description=The hook point (the gate's call)"`
	Object *cwmapi.Object `json:"object,omitempty" jsonschema:"description=The request (the gate's call)"`
}

// PlanificationConfig is planification's configuration (Planification.yaml).
type PlanificationConfig struct {
	ModuleUpdate ModuleUpdateConfig `json:"module_update,omitempty" yaml:"module_update,omitempty"`
	Reconciler   ReconcilerConfig   `json:"reconciler,omitempty" yaml:"reconciler,omitempty"`
}

// ModuleUpdateConfig turns the module-updated gate on: from Since on, a
// request of a template that changes a module (build, fix, maintain,
// feature-build) ends only with its module up to date. No Since: the gate is
// off.
type ModuleUpdateConfig struct {
	Since string `json:"since,omitempty" yaml:"since,omitempty" jsonschema:"description=The date (YYYY-MM-DD or RFC 3339) from which a request filed or filed again is held to the module-updated gate; empty: the gate is off"`
}

// ReviseInput corrects an open request's deliverables: the full new list.
type ReviseInput struct {
	ID           string        `json:"id" jsonschema:"description=The request,required"`
	Deliverables []Deliverable `json:"deliverables,omitempty" jsonschema:"description=Its deliverables as they should be (title and check each); a deliverable whose check is unchanged keeps its verification; absent: they are kept and only the note is recorded; an empty list removes them all"`
	Note         string        `json:"note" jsonschema:"description=Why they change (recorded in its details),required"`
	By           cwmapi.Actor  `json:"by" jsonschema:"description=Who revises it,required"`
}

// CheckInput runs an open request's deliverable checks now.
type CheckInput struct {
	ID string       `json:"id" jsonschema:"description=The request,required"`
	By cwmapi.Actor `json:"by" jsonschema:"description=Who asks for the checks,required"`
}

// CheckResult is one deliverable's check, run now.
type CheckResult struct {
	Title  string `json:"title"`
	Check  string `json:"check,omitempty"`
	Passed bool   `json:"passed"`
	Error  string `json:"error,omitempty" jsonschema:"description=Why it failed, with the end of its output; or why it was not run"`
}

// CheckOutput is the request after its checks, and each check's result.
type CheckOutput struct {
	Request Request       `json:"request"`
	Results []CheckResult `json:"results"`
}

// VerifyInput records one deliverable as verified by hand.
type VerifyInput struct {
	ID          string       `json:"id" jsonschema:"description=The request,required"`
	Deliverable string       `json:"deliverable" jsonschema:"description=The deliverable's title,required"`
	Evidence    string       `json:"evidence" jsonschema:"description=What was seen that verifies it,required"`
	By          cwmapi.Actor `json:"by" jsonschema:"description=The person who verified it (a person only),required"`
}

// The views of the demands.
const (
	DemandsOpen    = "open"    // the demands not ended (the default)
	DemandsMine    = "mine"    // the open demands a person asked
	DemandsHistory = "history" // the demands ended — done or dropped — the latest first
)

// DemandsInput asks for the demands of a module (all modules when empty).
type DemandsInput struct {
	Module string `json:"module,omitempty" jsonschema:"The module whose demands; empty: every module"`
	View   string `json:"view,omitempty"   jsonschema:"description=open (default) or mine or history,enum=open|mine|history"`
	Person string `json:"person,omitempty" jsonschema:"The person whose demands the mine view shows; required for mine"`
	// Split separates what people asked from the modules' work.
	Split string `json:"split,omitempty" jsonschema:"description=demands (what people asked: kind demand) or work (the modules' work: every other kind); empty: both,enum=demands|work"`
}

// Demand is one demand and its parts: a request, or any planned object (a
// feature, a goal), with its plan.
type Demand struct {
	Request
	// Why is why it is not moving: a waiting or blocked plan's note, or — for a
	// ready one — why the planner holds it back now (empty: it starts next).
	Why string `json:"why,omitempty" jsonschema:"description=Why it is not moving: the note of a waiting or blocked plan  or why the planner holds a ready one back"`
	// EndedAt is when its plan ended (done or dropped).
	EndedAt string   `json:"ended_at,omitempty" jsonschema:"description=When its plan ended (RFC 3339)"`
	Parts   []Demand `json:"parts,omitempty" jsonschema:"description=The demands split from it  in any module"`
	// Timeline is its activity, oldest first: each step started — with its
	// session — and each move of its plan.
	Timeline []TimelineEntry `json:"timeline,omitempty" jsonschema:"description=Its activity oldest first: the steps started (each with its session) and the moves of its plan"`
	// Step is the step an open demand is on, read from its to-dos: what it is,
	// who does it, who holds it now — and whether a person must act.
	Step *DemandStep `json:"step,omitempty" jsonschema:"description=The step it is on: read from its to-dos"`
	// WaitsOn is what it still waits on: its dependencies not ended.
	WaitsOn []string `json:"waits_on,omitempty" jsonschema:"description=Its dependencies not ended yet"`
}

// DemandStep is the step a demand is on.
type DemandStep struct {
	Step  string `json:"step" jsonschema:"description=The template step"`
	Phase string `json:"phase,omitempty"`
	// Who does it: a task or persona, or `person` and what is asked.
	Who         string `json:"who,omitempty"`
	Interaction string `json:"interaction,omitempty" jsonschema:"description=What a person's step asks: validate or challenge or decide or provide"`
	// HeldBy is the worker that pulled it, until its lease passes.
	HeldBy *cwmapi.Actor `json:"held_by,omitempty"`
	Until  string        `json:"until,omitempty" jsonschema:"description=When its lease passes (RFC 3339)"`
	// NeedsYou: a person's step is up — answer it in the Inbox (Ask).
	NeedsYou bool   `json:"needs_you,omitempty"`
	Ask      string `json:"ask,omitempty" jsonschema:"description=The inbox request a person's step was asked in"`
}

// The kinds of a timeline entry.
const (
	TimelineStep = "step" // a step started: a run (its session) or a person asked (the inbox request)
	TimelineMove = "move" // its plan moved: planned, ended, waits, blocked, split, done, dropped
)

// TimelineEntry is one line of a demand's activity.
type TimelineEntry struct {
	At      string `json:"at" jsonschema:"description=When (RFC 3339)"`
	Kind    string `json:"kind" jsonschema:"description=step or move,enum=step|move"`
	Title   string `json:"title"`
	Note    string `json:"note,omitempty"`
	State   string `json:"state,omitempty" jsonschema:"description=The plan state a move led to"`
	By      string `json:"by,omitempty" jsonschema:"description=Who made the move"`
	Run     string `json:"run,omitempty"`
	Session string `json:"session,omitempty" jsonschema:"description=The step's session: open it to read the transcript"`
	Ask     string `json:"ask,omitempty" jsonschema:"description=The inbox request a person's step was asked in"`
}

// DemandsOutput is the demands, as trees.
type DemandsOutput struct {
	Demands []Demand `json:"demands"`
}

// FileInput files a request: on an object, or on its own.
type FileInput struct {
	On           string        `json:"on,omitempty"           jsonschema:"description=Optional: the id of an existing object the work is asked on (a goal or a feature or a use case) — leave empty for a request of its own"`
	Title        string        `json:"title,omitempty"        jsonschema:"description=The ask in one line — required for a request of its own (one filed on an object takes its title)"`
	Details      string        `json:"details,omitempty"      jsonschema:"description=What the one line leaves out,content=markdown"`
	Module       string        `json:"module,omitempty"       jsonschema:"description=The module it targets — empty: sys-planification where intake routes it (unused with on)"`
	About        string        `json:"about,omitempty"        jsonschema:"description=Optional: the id of an existing object it concerns (a page or a type or a use case — e.g. billing.UC-001) — not free text"`
	Deliverables []Deliverable `json:"deliverables,omitempty" jsonschema:"description=What the work must produce"`
	BudgetUSD    float64       `json:"budget_usd,omitempty"   jsonschema:"description=Optional: a spend ceiling in US dollars"`
	DependsOn    []string      `json:"depends_on,omitempty"   jsonschema:"description=Requests it waits on"`
	Estimate     float64       `json:"estimate,omitempty"     jsonschema:"description=Optional: its weight in agent-hours — intake estimates it otherwise"`
	Kind         string        `json:"kind,omitempty"         jsonschema:"description=The kind of work — leave empty and intake sets it,enum=build|fix|analyse|review|decide|maintain|question|discover|demand"`
	Increment    string        `json:"increment,omitempty"    jsonschema:"description=Optional: a running increment the work joins - aimed at it at once (labelled increment:<IN>) and counted in its progress"`
	Priority     string        `json:"priority,omitempty"     jsonschema:"description=P0 now · P1 next · P2 later (the default) · P3 someday,enum=P0|P1|P2|P3"`
	Review       string        `json:"review,omitempty"       jsonschema:"description=Optional: who reviews its work - agent (the default - unless its release or stream asks a person) or person,enum=agent|person"`
	By           cwmapi.Actor  `json:"by"                     jsonschema:"description=Where it comes from: its id and kind (person or agent or module or feature or goal or external),required"`
	// Resolves and GateRequirement are set by the planner only: the request
	// meets that requirement of a gate on that object (gates.go).
	Resolves        string `json:"-"`
	GateRequirement string `json:"-"`
	// Source is set by what files by itself (a trigger, a rule, a gate).
	Source *RequestSource `json:"-"`
}

// The sources that file requests by themselves.
const (
	SourceTrigger = "trigger" // a change on an object
	SourceRule    = "rule"    // a rule an object fails
	SourceGate    = "gate"    // a move a gate refused
)

// RequestSource says what filed a request by itself, and why: one open
// request per source and object — firing again appends to it.
type RequestSource struct {
	Kind    string `json:"kind" jsonschema:"description=trigger or rule or gate"`
	Name    string `json:"name" jsonschema:"description=The trigger's or the rule's name; a gate's: gate/requirement"`
	Object  string `json:"object" jsonschema:"description=The object it fired on"`
	Change  string `json:"change,omitempty" jsonschema:"description=What fired it the last time"`
	FiredAt string `json:"fired_at,omitempty" jsonschema:"description=When it last fired (RFC 3339)"`
	Times   int    `json:"times,omitempty" jsonschema:"description=How many times it fired while the request was open"`
}

// FileOutput returns the filed request.
type FileOutput struct {
	Request Request `json:"request"`
	Path    string  `json:"path,omitempty" jsonschema:"Repo-relative path of the request's file (a request of its own)"`
	// Proposed is the proposal filed when the work was filed on a goal or a
	// feature a running increment does not hold yet: it joins once a person
	// approves it.
	Proposed string `json:"proposed,omitempty" jsonschema:"The proposal filed instead of joining: the goal or feature is added to the running increment once a person approves it"`
}

// GetInput names one request.
type GetInput struct {
	ID string `json:"id" jsonschema:"Request id; required"`
}

// GetOutput returns it.
type GetOutput struct {
	Request Request `json:"request"`
}

// ListInput filters requests; every filter is optional.
type ListInput struct {
	Module    string `json:"module,omitempty" jsonschema:"Only the requests of this module"`
	State     string `json:"state,omitempty"     jsonschema:"Only the requests in this state (their plan's)"`
	Requester string `json:"requester,omitempty" jsonschema:"Only the requests this requester asked (a person, an agent or a module id): a module's outbox"`
}

// ListOutput lists them, sorted by id.
type ListOutput struct {
	Requests []Request `json:"requests"`
}

// MoveInput moves one object (a release), saying who and why.
type MoveInput struct {
	ID   string       `json:"id"             jsonschema:"The object; required"`
	By   cwmapi.Actor `json:"by"             jsonschema:"Who moves it: its id and kind (person or agent); required"`
	Note string       `json:"note,omitempty" jsonschema:"Why; recorded in the lifecycle history"`
}

// SplitInput replaces a request by its parts.
type SplitInput struct {
	ID    string       `json:"id"             jsonschema:"The demand to split — a request or a feature or a goal or any object to plan; required"`
	Parts []SplitPart  `json:"parts"          jsonschema:"Its parts: at least one"`
	By    cwmapi.Actor `json:"by"             jsonschema:"Who splits it; required"`
	Note  string       `json:"note,omitempty" jsonschema:"Why; recorded in the request's plan"`
}

// SplitPart is one part of a split demand: a request of its own in a module,
// a request filed on an object, or a new object of the demand's own type under
// it (a sub-feature). What it leaves empty — title, details, kind, priority —
// it takes from the demand; its requester and dependencies are the demand's.
type SplitPart struct {
	Module       string        `json:"module,omitempty"       jsonschema:"A request of its own in this module (one of module, on, sub)"`
	On           string        `json:"on,omitempty"           jsonschema:"The object it is filed on — an unmet goal or a feature (one of module, on, sub)"`
	Sub          bool          `json:"sub,omitempty"          jsonschema:"A new object of the demand's type under it — a sub-feature — named by title (one of module, on, sub)"`
	Title        string        `json:"title,omitempty"        jsonschema:"Its ask in one line; the demand's when empty (unused with on; required with sub)"`
	Details      string        `json:"details,omitempty"      jsonschema:"What its title leaves out; the split request's when empty"`
	Deliverables []Deliverable `json:"deliverables,omitempty" jsonschema:"What this part must produce"`
	Estimate     float64       `json:"estimate,omitempty"     jsonschema:"Its weight in the network"`
	Kind         string        `json:"kind,omitempty"         jsonschema:"Its kind; the split request's when empty"`
	Priority     string        `json:"priority,omitempty"     jsonschema:"Its priority; the split request's when empty"`
}

// SplitOutput is the split request (it stays, split) and its parts, in order.
type SplitOutput struct {
	Split Request   `json:"split"`
	Parts []Request `json:"parts"`
}

// AssessInput records intake's assessment; what it leaves empty is kept.
type AssessInput struct {
	ID       string       `json:"id"                 jsonschema:"The request; required"`
	Kind     string       `json:"kind,omitempty"     jsonschema:"build or fix or analyse or review or decide or maintain"`
	Priority string       `json:"priority,omitempty" jsonschema:"P0 or P1 or P2 or P3"`
	Estimate float64      `json:"estimate,omitempty" jsonschema:"Its weight in the network (agent-hours or cost)"`
	Depth    string       `json:"depth,omitempty"    jsonschema:"full or quick: how deep its work goes (the template's depth)"`
	Review   string       `json:"review,omitempty"   jsonschema:"agent or person: who reviews its work (agent by default - unless its release or stream asks a person)"`
	Serves   string       `json:"serves,omitempty"   jsonschema:"The want it serves: the id of a goal or a feature or a use case (a serves edge); refused for an object that wants nothing"`
	By       cwmapi.Actor `json:"by"                 jsonschema:"Who assesses; required"`
}

// AnswerInput answers a request of kind question; it is then done.
type AnswerInput struct {
	ID   string       `json:"id"   jsonschema:"description=The question (a request of kind question),required"`
	Text string       `json:"text" jsonschema:"description=The answer,content=markdown,required"`
	By   cwmapi.Actor `json:"by"   jsonschema:"description=Who answers: a person or an agent,required"`
}

// AskInput asks the requester of a request one question.
type AskInput struct {
	ID       string       `json:"id"                jsonschema:"The request; required"`
	Question string       `json:"question"          jsonschema:"One well-framed question; its answer may take hours; required"`
	Context  string       `json:"context,omitempty" jsonschema:"What the requester needs to answer it"`
	By       cwmapi.Actor `json:"by"                jsonschema:"Who asks; required"`
}

// DependInput orders a network node (a request, a step) after another.
type DependInput struct {
	ID string `json:"id" jsonschema:"The request that waits; required"`
	On string `json:"on" jsonschema:"The request it waits on; required"`
}

// RequestOutput returns the request after the change.
type RequestOutput struct {
	Request Request `json:"request"`
	// Proposal is what an assessment proposes when the request changes a
	// module and serves no want: nothing of it is written.
	Proposal *ServesProposal `json:"proposal,omitempty" jsonschema:"description=Assess only: the wants it could serve when it serves none - nothing is written"`
}

// ServesProposal is the wants a request could serve, found by search; the
// intake chooses one (Assess serves), files the feature it needs, or asks.
type ServesProposal struct {
	Note       string           `json:"note"`
	Candidates []ServeCandidate `json:"candidates,omitempty"`
}

// ServeCandidate is one want a request could serve.
type ServeCandidate struct {
	ID    string `json:"id"`
	Type  string `json:"type"`
	Title string `json:"title"`
}

// CheckReadyInput is what an on_transition hook sends — the object as it is
// about to be and as it was — or, called directly, the id of an object to check.
type CheckReadyInput struct {
	Object *cwmapi.Object `json:"object,omitempty" jsonschema:"The object moving, as it is about to be (the hook payload)"`
	Prior  *cwmapi.Object `json:"prior,omitempty"  jsonschema:"The object as it was (the hook payload)"`
	ID     string         `json:"id,omitempty"     jsonschema:"A request to check (a direct call)"`
}

// CheckReadyOutput is a before-hook verdict.
type CheckReadyOutput struct {
	Accept bool   `json:"accept"`
	Reason string `json:"reason,omitempty"`
}

// EnsureInput carries nothing the service reads; a bootstrap hook sends its
// {point, object} payload, which is ignored.
type EnsureInput struct{}

// EnsureOutput names the module and says whether this call created it.
type EnsureOutput struct {
	Module  string `json:"module"`
	Created bool   `json:"created,omitempty"`
}

// CapPlanned is the capability of a module whose work is planned here: its
// requests, its plan and its queue - planification's panels and the method's
// show only on a module carrying it.
const CapPlanned = "planned"

// PlanModuleOutput says whether a module just created was given CapPlanned.
type PlanModuleOutput struct {
	Planned bool `json:"planned"`
}

// PlanModulesOutput is the modules the project's bootstrap gave CapPlanned.
type PlanModulesOutput struct {
	Planned []string `json:"planned"`
}

// FixCriterionInput names a failed acceptance criterion whose fix joins the
// open increment holding the feature it proves.
type FixCriterionInput struct {
	Criterion string       `json:"criterion" jsonschema:"description=The acceptance criterion whose last verdict failed; required"`
	By        cwmapi.Actor `json:"by" jsonschema:"description=Who files the fix; required"`
}
