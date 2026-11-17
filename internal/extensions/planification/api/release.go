// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"

	"github.com/katyoti/katyoti-fwk/internal/core/meta"
	"github.com/katyoti/katyoti-fwk/internal/core/render"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// RELEASES — what ships, and when. A release is a `release` object: a module's
// in `modules/<m>/.ktw/releases/`, the project's in sys-planification. Work
// TARGETS a release by an edge (N:1 — an object ships once); a module release
// SHIPS IN a project release. Freezing records the frozen scope; a person
// unfreezes to change it; releasing waits until every target is done.

// Declarations of the planification pack, for releases.
const (
	// ReleaseType is the object type of a release.
	ReleaseType = "release"
	// CapRelease is the release field-group.
	CapRelease = "release"
	// EdgeTargets points a request at the increment or release it ships in:
	// a request's own link. A module object - a goal, a feature, a use case,
	// a decision, a rule - carries none (rule): the
	// increment or release lists it through EdgeTakes.
	EdgeTargets = "targets"
	// EdgeTakes points an increment or a release at the module work it takes
	// : M:N, at most one open taker per object, kept by
	// the planification service; a done or dropped increment keeps its takes.
	EdgeTakes = "takes"
	// EdgeShipsIn points a module release at the project release it ships in.
	EdgeShipsIn = "ships-in"
	// EdgePlannedFor aims an increment at a later release of its stream: an
	// intention, shipped in once that release is the next.
	EdgePlannedFor = "planned-for"
	// EdgeInStream places a release in its stream (exactly one), or names the
	// stream an increment aims at.
	EdgeInStream = "in-stream"
	// EdgeJudgedBy names, among what targets a release, what judges it: its
	// grade is theirs when it names any.
	EdgeJudgedBy = "judged-by"
	// TargetDone is the lifecycle state that makes a target done — the name the
	// `approval`, `manual` and `request` workflows share.
	TargetDone = "done"
)

// The states of the `release` workflow.
const (
	ReleasePlanned  = "planned"
	ReleaseFrozen   = "frozen"
	ReleaseReleased = "released"
	ReleaseDropped  = "dropped"
)

// ReleaseFields is the `release` field-group — the Go type its data-type
// (cap-release) is rendered from. The release's name is the object's name.
type ReleaseFields struct {
	Version     string   `json:"version,omitempty" jsonschema:"description=The version it ships as (1.4.0); unique among the releases of its module; none for an increment"`
	Date        string   `json:"date,omitempty" jsonschema:"description=When it is planned to ship (YYYY-MM-DD)"`
	FrozenScope []string `json:"frozen_scope,omitempty" jsonschema:"description=The targets it was frozen with; set by Freeze and cleared by Unfreeze"`
	BudgetUSD   float64  `json:"budget_usd,omitempty" jsonschema:"description=Spend ceiling for its work in US dollars; 0 means none stated"`
	// Level is what a release's content is: patch (direct work only), minor
	// (increments) or major (a breaking increment among them).
	Level string `json:"level,omitempty" jsonschema:"description=major or minor or patch: what its content is (patch: direct work only - minor: increments - major: a breaking increment among them),enum=major|minor|patch"`
	// Breaking marks an increment that breaks a user's habit or an API.
	Breaking bool `json:"breaking,omitempty" jsonschema:"description=An increment only: it breaks a user's habit or an API (its release is a major)"`
	// Candidates are the deliveries of one release, in order (rc1, rc2...).
	Candidates []Candidate `json:"candidates,omitempty" jsonschema:"description=Its deliveries before it is released (rc1 - rc2 - ...) in order"`
	// Delivery is an increment's delivery goals: what it promises, in the
	// user's words - its proposal turns them into module goals and features.
	Delivery []string `json:"delivery,omitempty" jsonschema:"description=An increment only: its delivery goals - what it promises in the user's words"`
	// Moved says why an increment left the release it shipped in: not done
	// by that release's date.
	Moved string `json:"moved,omitempty" jsonschema:"description=An increment only: why it left the release it shipped in - not done by its date - and where it went"`
	// Review asks who reviews the work it ships: a person, or an agent.
	Review string `json:"review,omitempty" jsonschema:"description=Who reviews the work it ships: agent (the default - unless its stream asks a person) or person,enum=agent|person"`
	// ChangeLogHead and ChangeLogTail are its change-log page around the
	// entries: the info box and En un coup d'œil above, Réserves below. The
	// entries are its `change` objects (ChangeService.Render).
	ChangeLogHead string `json:"changelog_head,omitempty" jsonschema:"description=Its change-log page above the entries (markdown): the info box and En un coup d'œil"`
	ChangeLogTail string `json:"changelog_tail,omitempty" jsonschema:"description=Its change-log page below the entries (markdown): Prochaine release and Réserves"`
	// DeliveryReview is an increment's last delivery review: what the
	// usable-delivered gate reads before its done.
	DeliveryReview *DeliveryReview `json:"delivery_review,omitempty" jsonschema:"description=An increment only: its last delivery review - the verdict and the demo path and the findings - which the usable-delivered gate reads"`
	// TestDelivery marks an increment delivered behind its display switch on
	// purpose: the increment that turns it on for the users.
	TestDelivery *TestDelivery `json:"test_delivery,omitempty" jsonschema:"description=An increment only: a test delivery - it ships behind its display switch on purpose and the increment named turns it on for the users"`
}

// DeliveryReview is an increment's delivery review: an
// agent's - a person's where asked - check that the user can use what it
// delivers, recorded before the person's approve-delivery.
type DeliveryReview struct {
	Verdict string `json:"verdict" jsonschema:"description=approved or rejected,enum=approved|rejected"`
	// Demo is the short path a user follows to see what it delivers.
	Demo string `json:"demo,omitempty" jsonschema:"description=The demo path: how a user sees what it delivers - a few steps"`
	// Findings are what the review found missing - each with the request
	// that follows it up when one is planned.
	Findings []DeliveryFinding `json:"findings,omitempty" jsonschema:"description=What the review found missing - each with its follow-up request"`
	// SwitchesRetired are the display-switch features the review found
	// retired: the screens they hid are shown to every user.
	SwitchesRetired []string     `json:"switches_retired,omitempty" jsonschema:"description=The display-switch features found retired: their screens are shown to every user"`
	By              cwmapi.Actor `json:"by"`
	At              string       `json:"at" jsonschema:"description=When it was recorded (RFC 3339)"`
}

// DeliveryFinding is one thing a delivery review found missing.
type DeliveryFinding struct {
	What string `json:"what" jsonschema:"description=What is missing"`
	// FollowUp is the request planned to deliver it: in an increment of the
	// same release.
	FollowUp string `json:"follow_up,omitempty" jsonschema:"description=The request that follows it up - in an increment of the same release"`
}

// TestDelivery is an increment delivered behind its display switch on
// purpose: the increment that turns the switch on.
type TestDelivery struct {
	TurnedOnBy string       `json:"turned_on_by" jsonschema:"description=The increment that turns its display switch on for the users - planned in the same release"`
	By         cwmapi.Actor `json:"by"`
	At         string       `json:"at" jsonschema:"description=When it was set (RFC 3339)"`
}

// The levels of a release.
const (
	LevelMajor = "major"
	LevelMinor = "minor"
	LevelPatch = "patch"
)

// Candidate is one delivery of a release: a release candidate, not a release
// of its own.
type Candidate struct {
	Name string       `json:"name" jsonschema:"description=Its name (rc1)"`
	At   string       `json:"at"   jsonschema:"description=When it was delivered (RFC 3339)"`
	By   cwmapi.Actor `json:"by"`
	Note string       `json:"note,omitempty"`
	// Build is the build it was delivered as: promotion ships it unchanged.
	Build string `json:"build,omitempty" jsonschema:"description=The build it was delivered as (a build id): promoting it ships that build - nothing rebuilt"`
	// Gates are the results recorded on its build (the end-to-end test).
	Gates []GateResult `json:"gates,omitempty" jsonschema:"description=The gates' results recorded on its build - the end-to-end test on a clean machine"`
	// SignedOff is the alpha sign-off: a person's approval naming it.
	SignedOff *SignOff `json:"signed_off,omitempty" jsonschema:"description=The alpha sign-off - a person's approval naming this candidate"`
	// Promoted marks the candidate that became the official version.
	Promoted bool `json:"promoted,omitempty" jsonschema:"description=It was promoted: the official version is this build"`
	// Letter is its « Quoi de neuf »: the main points, generated from the
	// release's note, approved with the alpha sign-off.
	Letter string `json:"letter,omitempty" jsonschema:"description=Its letter - the main new functions only - generated from the release's note,content=markdown"`
}

// NoteOutput is a release's note: the change entries of the increments it
// carries, and of its fixes - generated, never written by hand.
type NoteOutput struct {
	Release    string          `json:"release"`
	Increments []IncrementNote `json:"increments"`
	Fixes      []Change        `json:"fixes" jsonschema:"description=The entries of its direct work and of its own fixes"`
}

// IncrementNote is one increment's part of a release's note.
type IncrementNote struct {
	ID      string   `json:"id"`
	Title   string   `json:"title"`
	Changes []Change `json:"changes"`
}

// GateResult is one gate's result on a candidate's build.
type GateResult struct {
	Name   string       `json:"name"   jsonschema:"description=The gate (e2e)"`
	Passed bool         `json:"passed"`
	Why    string       `json:"why,omitempty" jsonschema:"description=What the run said - its evidence or why it failed"`
	At     string       `json:"at"     jsonschema:"description=When it was recorded (RFC 3339)"`
	By     cwmapi.Actor `json:"by"`
}

// SignOff is a person's approval of a candidate.
type SignOff struct {
	By cwmapi.Actor `json:"by"`
	At string       `json:"at"`
}

// DeliverInput records a delivery of a frozen release.
type DeliverInput struct {
	ID    string       `json:"id"   jsonschema:"description=The release,required"`
	Name  string       `json:"name,omitempty" jsonschema:"description=The candidate's name (rc1) - the next rcN when empty"`
	Build string       `json:"build,omitempty" jsonschema:"description=The build it is delivered as (a build id)"`
	Note  string       `json:"note,omitempty"`
	By    cwmapi.Actor `json:"by"   jsonschema:"description=Who delivers it,required"`
}

// The gates a candidate passes before it is promoted (decision
const (
	GateIncrements   = "increments"
	GateDependencies = "dependencies"
	GateNoOpenP0Fix  = "no-open-p0-fix"
	GateEndToEnd     = "e2e"
	GateSignOff      = "alpha-sign-off"
	// GateChangeLog is the release's own ready-to-release gate (its change
	// log listed and confirmed by a person), shown beside the candidate's:
	// the release's guard holds it, so Promote leaves it to that guard - the
	// refusal files the change-log work, then asks the confirmation.
	GateChangeLog = "change-log"
)

// ReleaseRowsPageInput asks a release's level as a page.
type ReleaseRowsPageInput struct {
	ID string `json:"id" jsonschema:"description=The release,required"`
	// Language is the person's (en or fr): the level's words.
	Language string `json:"language,omitempty" jsonschema:"description=The person's language (en or fr); empty = English"`
}

// GatesInput names a release and, optionally, its candidate (the last one when empty).
type GatesInput struct {
	ID        string `json:"id" jsonschema:"description=The release,required"`
	Candidate string `json:"candidate,omitempty" jsonschema:"description=Its candidate (rc2) - the last one when empty"`
}

// GatesOutput is where a release stands toward its promotion.
type GatesOutput struct {
	Release   string     `json:"release"`
	Stage     string     `json:"stage" jsonschema:"description=planned or frozen or candidate or released or dropped"`
	Candidate string     `json:"candidate,omitempty" jsonschema:"description=The candidate the gates are read on"`
	Build     string     `json:"build,omitempty"`
	Gates     []GateLine `json:"gates"`
	// Filed is the requests filed on what it carries since the candidate was delivered: its testing's findings.
	Filed []string `json:"filed,omitempty" jsonschema:"description=The requests filed on the release or what it carries since the candidate was delivered"`
	Ready bool     `json:"ready" jsonschema:"description=Every gate passes: it can be promoted"`
}

// GateLine is one gate: passed or not, and why.
type GateLine struct {
	Name   string `json:"name"`
	Passed bool   `json:"passed"`
	Why    string `json:"why,omitempty"`
}

// RecordGateInput records a gate's result on a candidate's build.
type RecordGateInput struct {
	ID        string       `json:"id"        jsonschema:"description=The release,required"`
	Candidate string       `json:"candidate" jsonschema:"description=Its candidate (rc2),required"`
	Gate      string       `json:"gate"      jsonschema:"description=The gate recorded on a build: e2e (the end-to-end test on a clean machine),enum=e2e,required"`
	Passed    bool         `json:"passed"`
	Why       string       `json:"why,omitempty" jsonschema:"description=Its evidence - or why it failed"`
	By        cwmapi.Actor `json:"by" jsonschema:"description=Who ran it,required"`
}

// ExportLetterInput writes a candidate's letter to a file.
type ExportLetterInput struct {
	ID        string `json:"id"        jsonschema:"description=The release,required"`
	Candidate string `json:"candidate" jsonschema:"description=Its candidate (rc2),required"`
	To        string `json:"to"        jsonschema:"description=The markdown file written (absolute),required"`
}

// ExportLetterOutput says what was written.
type ExportLetterOutput struct {
	Path      string `json:"path"`
	SignedOff bool   `json:"signed_off" jsonschema:"description=Whether the letter is the one the alpha sign-off approved"`
}

// CandidateInput names a release's candidate and who acts on it.
type CandidateInput struct {
	ID        string       `json:"id"        jsonschema:"description=The release,required"`
	Candidate string       `json:"candidate" jsonschema:"description=Its candidate (rc2),required"`
	Note      string       `json:"note,omitempty"`
	By        cwmapi.Actor `json:"by" jsonschema:"description=Who acts - a person for the sign-off,required"`
}

// ReleasePageSectionInput names one section of a release's pane, loaded on
// its own.
type ReleasePageSectionInput struct {
	ID      string `json:"id"      jsonschema:"description=The release,required"`
	Section string `json:"section" jsonschema:"description=The section: work,enum=work,required"`
}

// ReleasePageWork is the section of a release's pane holding its work: its
// gauges and everything they count.
const ReleasePageWork = "work"

// ReleasePlanWorkInput plans a release's work from the gap.
type ReleasePlanWorkInput struct {
	ID string       `json:"id" jsonschema:"description=The release,required"`
	By cwmapi.Actor `json:"by" jsonschema:"description=Who plans it,required"`
}

// ReleaseRunInput runs a release: the person's go for the increments it holds.
type ReleaseRunInput struct {
	ID string       `json:"id" jsonschema:"description=The release,required"`
	By cwmapi.Actor `json:"by" jsonschema:"description=The person who runs it,required"`
}

// ReleaseRunOutput says which increments of the release were run and which
// not, with why.
type ReleaseRunOutput struct {
	Ran     []string          `json:"ran"`
	Skipped map[string]string `json:"skipped,omitempty" jsonschema:"description=Each increment not run and why (already running - no delivery goals - on hold)"`
}

// GapItem is one goal or feature a release's plan-work met, and what it did.
type GapItem struct {
	Object  string `json:"object"`
	Request string `json:"request,omitempty" jsonschema:"description=The request filed or reused"`
	Why     string `json:"why"`
}

// ReleasePlanWorkOutput is what plan-work did: the requests it filed, those it
// found open, what passes already and what waits.
type ReleasePlanWorkOutput struct {
	Filed   []GapItem `json:"filed"`
	Reused  []GapItem `json:"reused"`
	Passing []GapItem `json:"passing"`
	Waiting []GapItem `json:"waiting"`
}

// DeliverablesOutput is what a release ships: the modules its content touched
// and its change-log page.
type DeliverablesOutput struct {
	Release   string   `json:"release"`
	Modules   []string `json:"modules"    jsonschema:"description=The modules its direct work and its increments' work touched"`
	ChangeLog string   `json:"change_log" jsonschema:"description=Its change-log page"`
}

// NextPlanned is a stream's next release among its releases: the earliest
// planned one by date — one with no date after those with one, then by id —
// never a frozen, released or dropped one: the only one a person assigns
// increments to (decision). Every view that names a
// stream's next release reads it.
func NextPlanned(releases []Release) (Release, bool) {
	var next Release
	found := false
	for _, r := range releases {
		if r.State != ReleasePlanned {
			continue
		}
		if !found || plannedBefore(r, next) {
			next, found = r, true
		}
	}
	return next, found
}

func plannedBefore(a, b Release) bool {
	switch {
	case a.Date == b.Date:
		return a.ID < b.ID
	case a.Date == "":
		return false
	case b.Date == "":
		return true
	}
	return a.Date < b.Date
}

// Release is a release as the service reads it.
type Release struct {
	ID      string `json:"id"`
	Module  string `json:"module"`
	Name    string `json:"name"`
	State   string `json:"state"`
	Project bool   `json:"project,omitempty" jsonschema:"The project's release (in sys-planification) rather than a module's"`
	ShipsIn string `json:"ships_in,omitempty" jsonschema:"The project release a module release ships in"`
	Stream  string `json:"stream,omitempty"   jsonschema:"The stream it is in"`
	ReleaseFields
}

// Target is one piece of work in a release's scope.
type Target struct {
	ID    string `json:"id"`
	Type  string `json:"type"`
	Title string `json:"title,omitempty"`
	State string `json:"state,omitempty" jsonschema:"Its lifecycle state; empty when it has none"`
	Done  bool   `json:"done"`
	// Via names the module release a project release reaches it through.
	Via string `json:"via,omitempty"`
}

// ReleaseServiceDef describes the service for KTSP/CLI auto-generation.
var ReleaseServiceDef = meta.ServiceDef{
	Name:    "releases",
	Short:   "What ships: plan a release, target work at it, freeze it, release it; its scope and the backlog",
	Version: "1.0.0",
	Methods: map[string]meta.MethodDef{
		"Plan":             meta.Method("Plan a release of a module (or of the project), planned"),
		"Get":              meta.Method("Read one release"),
		"List":             meta.Method("List releases, by module and state"),
		"Target":           meta.Method("Aim a piece of work at a release (replaces the release it targeted); refused when the release is frozen"),
		"Untarget":         meta.Method("Take a piece of work out of its release"),
		"ShipIn":           meta.Method("Ship a module release in a project release"),
		"Freeze":           meta.Method("Freeze a release: its scope is recorded and no target is added any more"),
		"Unfreeze":         meta.Method("Unfreeze a release to change its scope — a person only"),
		"Release":          meta.Method("Release it; refused while a target is not done or was added after the freeze"),
		"Drop":             meta.Method("Drop a release: it will not ship - the increments shipping in it or planned for it wait in the backlog"),
		"ShipPlanned":      meta.Method("Ship in a release every open increment planned for it, once it is its stream's next planned release: each ships in it and its planned-for is cleared; a held one stays planned"),
		"Scope":            meta.Method("What a release ships: its targets (and, for the project, its module releases' targets), done or not"),
		"Backlog":          meta.Method("The work of a module that targets no release and is not finished"),
		"CheckTransition":  meta.Method("The on_transition hook of a release: a freeze matches its targets; a release has every target done"),
		"Lint":             meta.Method("The lint hook of a release: a target added after the freeze; what judges it outside what targets it; a version off its stream's scheme; an open increment in no stream"),
		"Reschedule":       meta.Method("Move a release's planned date, its budget or its level — only what is given changes; refused once it is released or dropped"),
		"Deliver":          meta.Method("Record a delivery of a frozen or released release - a candidate (rc1 - rc2 - ...) in order, with its build; the next rcN when unnamed; refused for a name already used"),
		"Gates":            {Short: "Where a release stands toward its promotion: its stage and, on a candidate (the last one by default), each gate - its increments and their cross-stream dependencies done, no open P0 fix, the end-to-end test green on its build, the alpha sign-off - and the requests its testing filed", ReadOnly: true},
		"RecordGate":       meta.Method("Record a gate's result on a candidate's build: the end-to-end test on a clean machine (e2e), passed or not, with its evidence"),
		"SignOff":          meta.Method("The alpha sign-off: a person approves a candidate - the last gate before its promotion"),
		"Note":             {Short: "A release's note: the change entries of the increments it carries and of its fixes, read from the entries' requests - generated, never written by hand", ReadOnly: true},
		"ExportLetter":     {Short: "Write a candidate's letter - its « Quoi de neuf » - to a markdown file: what a delivery's build publishes as its solution's « Quoi de neuf »", CLIName: "releases letter-export", ReadOnly: true},
		"Letter":           meta.Method("Write a candidate's letter - its « Quoi de neuf »: per increment its title and its major and notable changes, then its corrections in one line, in French - from the release's note; kept on the candidate, approved with the alpha sign-off"),
		"Promote":          meta.Method("Promote a candidate to the official version: refused naming each failing gate; the candidate's own build becomes the release, nothing rebuilt, and the release is released"),
		"Deliverables":     {Short: "What a release ships: the modules its direct work and its increments' work touched - and its change-log page", ReadOnly: true},
		"PlanWork":         meta.Method("Plan a release's work from the gap: for each increment shipping in it - each goal not decomposed gets its decomposition - each feature (its own and its goals') not passing its acceptance criteria gets its specification or its build - open work reused - two requests filed in one module ordered; nothing outside the release"),
		"Run":              meta.Method("Run a release, as a person (define now - plan when run): each open increment shipping in it that has delivery goals and has not run is run - its analysis first, then its proposal; one already running, on hold or with no goals is skipped, saying why"),
		"RowsPage":         {Short: "A release's level as a page - the region under the Delivery timeline and the release's own pane: its header (stage stepper, gates, due date) with its next action, the legend, the increments that ship in it and those planned for it as dense rows", ReadOnly: true},
		"Rows":             {Short: "A release's level: its card, its stage stepper and next action, its increments as rows (priority, standing, a bar of their requests, what each waits on) and those planned for it - rows of the release-rows data type", ReadOnly: true},
		"Page":             {Short: "A release's pane, its overview: can it ship (its stage, verdict and next action, each gate), what is left (its increments by priority with their open requests and what they wait on, the other streams' increments it needs), what needs a person, what runs now; released, what it delivered", ReadOnly: true},
		"PageSection":      {Short: "One section of a release's pane, loaded on its own: work - its gauges, its gates, what awaits a person, its increments, the other streams' increments it needs, what runs and what is blocked, read in one computation", ReadOnly: true},
		"DependenciesPage": {Short: "A release's Dependencies panel: its increments as frames holding their goals, features and requests, every depends-on an arrow, the work outside the release they wait on in dotted frames saying where it is scheduled and how it stands, each increment ready to run, waiting (on what) or running (who holds it)", ReadOnly: true},
		"Grade":            {Short: "A release's grade: the verification of what judges it (judged-by) - else of what targets it - rolled up: green only when every member passed; progress and the frontier", ReadOnly: true},
	},
}

// ReleaseService manages releases.
type ReleaseService interface {
	Plan(ctx context.Context, in PlanInput) (ReleaseOutput, error)
	Get(ctx context.Context, in GetInput) (ReleaseOutput, error)
	List(ctx context.Context, in ListInput) (ListReleasesOutput, error)
	Target(ctx context.Context, in TargetInput) (TargetOutput, error)
	Untarget(ctx context.Context, in UntargetInput) (TargetOutput, error)
	ShipIn(ctx context.Context, in ShipInInput) (ReleaseOutput, error)
	Freeze(ctx context.Context, in MoveInput) (ReleaseOutput, error)
	Unfreeze(ctx context.Context, in MoveInput) (ReleaseOutput, error)
	Release(ctx context.Context, in MoveInput) (ReleaseOutput, error)
	Drop(ctx context.Context, in MoveInput) (ReleaseOutput, error)
	ShipPlanned(ctx context.Context, in MoveInput) (ShipPlannedOutput, error)
	Scope(ctx context.Context, in GetInput) (ScopeOutput, error)
	Backlog(ctx context.Context, in BacklogInput) (BacklogOutput, error)
	CheckTransition(ctx context.Context, in HookInput) (CheckReadyOutput, error)
	Lint(ctx context.Context, in HookInput) (LintOutput, error)
	Reschedule(ctx context.Context, in RescheduleInput) (ReleaseOutput, error)
	Deliver(ctx context.Context, in DeliverInput) (ReleaseOutput, error)
	Gates(ctx context.Context, in GatesInput) (GatesOutput, error)
	RecordGate(ctx context.Context, in RecordGateInput) (ReleaseOutput, error)
	SignOff(ctx context.Context, in CandidateInput) (ReleaseOutput, error)
	Promote(ctx context.Context, in CandidateInput) (ReleaseOutput, error)
	Note(ctx context.Context, in GetInput) (NoteOutput, error)
	Letter(ctx context.Context, in CandidateInput) (ReleaseOutput, error)
	Rows(ctx context.Context, in GetInput) (ReleaseRows, error)
	RowsPage(ctx context.Context, in ReleaseRowsPageInput) (render.Def, error)
	ExportLetter(ctx context.Context, in ExportLetterInput) (ExportLetterOutput, error)
	Deliverables(ctx context.Context, in GetInput) (DeliverablesOutput, error)
	PlanWork(ctx context.Context, in ReleasePlanWorkInput) (ReleasePlanWorkOutput, error)
	Grade(ctx context.Context, in GetInput) (GradeOutput, error)
	// Page is a release's pane: its overview - can it ship, what is left,
	// what needs a person, what runs now.
	Page(ctx context.Context, in GetInput) (render.Def, error)
	// PageSection is one section of a release's pane loaded on its own
	// : its work.
	PageSection(ctx context.Context, in ReleasePageSectionInput) (render.Def, error)
	// DependenciesPage is a release's Dependencies panel:
	// its increments and their work as a network, what waits for what, the
	// work outside framed apart, each increment ready to run, waiting or running.
	DependenciesPage(ctx context.Context, in GetInput) (render.Def, error)
	Run(ctx context.Context, in ReleaseRunInput) (ReleaseRunOutput, error)
}

// Member statuses in a grade. Only pass is a pass.
const (
	MemberPass     = "pass"
	MemberFail     = "fail"
	MemberUnproven = "unproven" // its verification ran nothing, or proved nothing either way
	MemberMissing  = "missing"  // the edge names no object
	MemberError    = "error"    // its verification could not run
)

// MemberGrade is one member's verification, counted.
type MemberGrade struct {
	ID       string `json:"id"`
	Status   string `json:"status" jsonschema:"description=pass or fail or unproven or missing or error"`
	Runs     int    `json:"runs"`
	Passed   int    `json:"passed"`
	Failed   int    `json:"failed"`
	Unproven int    `json:"unproven"`
	Reason   string `json:"reason,omitempty"`
	// Extended names each check that ran on an extended bound — one its
	// checkable declared longer than the default: "<id> (30m)".
	Extended []string `json:"extended,omitempty" jsonschema:"description=The checks that ran on an extended bound their checkable declared: <id> (30m)"`
}

// GradeOutput is a release's grade: the verification of what judges it, else
// of what targets it, rolled up — green only when every member passed.
type GradeOutput struct {
	Release string `json:"release"`
	// Over is what was graded: judged-by when the release names any, else
	// targets.
	Over     string        `json:"over" jsonschema:"description=judged-by or targets"`
	Members  []MemberGrade `json:"members"`
	Status   string        `json:"status" jsonschema:"description=green or unmet or ungraded"`
	Total    int           `json:"total"`
	Passed   int           `json:"passed"`
	Progress float64       `json:"progress"`
	Frontier []string      `json:"frontier" jsonschema:"description=The members that did not pass"`
}

// RescheduleInput moves a release's planned date or its budget; only what is
// given changes.
type RescheduleInput struct {
	ID        string       `json:"id" jsonschema:"description=The release,required"`
	Date      string       `json:"date,omitempty" jsonschema:"description=Its new planned date (YYYY-MM-DD); unchanged when empty"`
	BudgetUSD *float64     `json:"budget_usd,omitempty" jsonschema:"description=Its new spend ceiling in US dollars (0 = none stated); unchanged when absent"`
	Level     string       `json:"level,omitempty" jsonschema:"description=Its new level: major or minor or patch; unchanged when empty"`
	By        cwmapi.Actor `json:"by" jsonschema:"description=Who reschedules it,required"`
}

// PlanInput plans a release.
type PlanInput struct {
	Module  string `json:"module,omitempty"   jsonschema:"The module it releases; empty = the project"`
	Version string `json:"version"            jsonschema:"The version (1.4.0); required"`
	Name    string `json:"name,omitempty"     jsonschema:"A name; default the version"`
	Date    string `json:"date,omitempty"     jsonschema:"Planned ship date (YYYY-MM-DD)"`
	Level   string `json:"level,omitempty"    jsonschema:"major or minor or patch,enum=major|minor|patch"`
	ShipsIn string `json:"ships_in,omitempty" jsonschema:"The project release a module release ships in"`
	Stream  string `json:"stream,omitempty"   jsonschema:"The stream it is in: its version follows the stream's scheme"`
	// The limit of its plan. How many agents work at once is the task
	// groups' (TaskGroups.yaml), not a release's.
	BudgetUSD float64 `json:"budget_usd,omitempty" jsonschema:"Spend ceiling for its work in US dollars"`
}

// ReleaseOutput returns a release.
type ReleaseOutput struct {
	Release Release `json:"release"`
	Path    string  `json:"path,omitempty" jsonschema:"Repo-relative path of its file (Plan)"`
}

// ShipPlannedOutput is what shipped in a release from what was planned for
// it, and what stays planned and why.
type ShipPlannedOutput struct {
	Release string   `json:"release"`
	Shipped []string `json:"shipped" jsonschema:"description=The increments it now ships"`
	Stayed  []string `json:"stayed" jsonschema:"description=The increments still planned for it - one line each saying why"`
}

// ListReleasesOutput lists releases, sorted by id.
type ListReleasesOutput struct {
	Releases []Release `json:"releases"`
}

// TargetInput aims a piece of work at a release.
type TargetInput struct {
	ID      string `json:"id"      jsonschema:"The work (any object); required"`
	Release string `json:"release" jsonschema:"The release; required"`
}

// UntargetInput takes a piece of work out of its release.
type UntargetInput struct {
	ID string `json:"id" jsonschema:"The work; required"`
}

// TargetOutput says what the work targets now.
type TargetOutput struct {
	ID       string `json:"id"`
	Release  string `json:"release,omitempty" jsonschema:"The release it targets now; empty when none"`
	Replaced string `json:"replaced,omitempty" jsonschema:"The release it targeted before, when that changed"`
	// Proposed is the proposal filed instead of aiming it: a goal or a
	// feature added to a running increment waits on a person's approval.
	Proposed string `json:"proposed,omitempty" jsonschema:"The proposal filed instead - a running increment takes a new goal or feature once a person approves it"`
}

// ShipInInput ships a module release in a project release.
type ShipInInput struct {
	ID string `json:"id" jsonschema:"The module release; required"`
	In string `json:"in" jsonschema:"The project release; for an increment empty means its stream's next planned release"`
}

// ScopeOutput is what a release ships.
type ScopeOutput struct {
	Release Release   `json:"release"`
	Targets []Target  `json:"targets"`
	Modules []Release `json:"modules,omitempty" jsonschema:"The module releases shipping in a project release"`
	// Increments are the increments shipping in it - each with its state and
	// whether it is done - apart from the module releases.
	Increments []Target `json:"increments,omitempty" jsonschema:"The increments shipping in it: each with its title and state and whether it is done"`
	Done       int      `json:"done"`
	Total      int      `json:"total"`
	OffScope   []string `json:"off_scope,omitempty" jsonschema:"Targets added after the freeze"`
}

// BacklogInput names the module.
type BacklogInput struct {
	Module string `json:"module" jsonschema:"The module; required"`
}

// BacklogOutput lists the untargeted, unfinished work.
type BacklogOutput struct {
	Items []Target `json:"items"`
}

// HookInput is a hook's default payload: the point and the object — for an
// on_transition hook, the object as it is about to be.
type HookInput struct {
	Point  string         `json:"point,omitempty"`
	Object *cwmapi.Object `json:"object,omitempty"`
}

// LintFinding is one finding a lint hook reports.
type LintFinding struct {
	Rule     string `json:"rule"`
	Message  string `json:"message"`
	Severity string `json:"severity" jsonschema:"error or warn or info"`
}

// LintOutput is a lint hook's answer.
type LintOutput struct {
	Status   string        `json:"status" jsonschema:"ok or findings"`
	Findings []LintFinding `json:"findings"`
}
