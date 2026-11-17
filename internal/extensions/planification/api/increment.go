// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"

	"github.com/katyoti/katyoti-fwk/internal/core/meta"
	"github.com/katyoti/katyoti-fwk/internal/core/render"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// INCREMENTS — one sentence of user value, holding the goals and features that
// make it: done when every one of them is. An increment is an `increment`
// object carrying the release capability — the same targets, scope, grade and
// network, never a copy: work TARGETS it; it SHIPS IN at most one release, which
// then ships its targets (through it, as a module release's); it may exist
// before a release takes it and move to another when it is not ready. One
// level: an increment neither targets nor ships in an increment.

// The `increment` object type and its workflow's states.
const (
	IncrementType    = "increment"
	IncrementOpen    = "open"
	IncrementDone    = "done"
	IncrementDropped = "dropped"
)

// Increment is an increment as the service reads it.
type Increment struct {
	ID     string `json:"id"`
	Module string `json:"module"`
	Title  string `json:"title" jsonschema:"description=Its short title: the name lists and boards and its pane show"`
	// Description is its intent: the value sentence - what the user gets and why.
	Description string `json:"description,omitempty" jsonschema:"description=Its intent: what the user gets and why"`
	State       string `json:"state" jsonschema:"description=open or done or dropped"`
	ShipsIn     string `json:"ships_in,omitempty" jsonschema:"description=The release it ships in"`
	// PlannedFor is the later release of its stream it is aimed at: an
	// intention - it ships in it once that release is the next.
	PlannedFor string `json:"planned_for,omitempty" jsonschema:"description=The later release of its stream it is planned for: it ships in it once that release is its stream's next"`
	// Returned says it waits in the backlog because the release it was
	// planned for was dropped.
	Returned string `json:"returned,omitempty" jsonschema:"description=Set when the release it was planned for was dropped: it waits in the backlog - plan it again"`
	// Moved says why it left the release it shipped in.
	Moved  string `json:"moved,omitempty" jsonschema:"description=Why it left the release it shipped in: not done by its date - and where it went"`
	Stream string `json:"stream,omitempty"   jsonschema:"description=The stream it aims at"`
	// Delivery is what it promises, in the user's words.
	Delivery []string `json:"delivery,omitempty" jsonschema:"description=Its delivery goals: what it promises in the user's words"`
	// DependsOn is what it waits on: the increments its depends-on edges name.
	DependsOn []string `json:"depends_on,omitempty" jsonschema:"description=The increments it depends on: its work waits while one is neither done nor dropped"`
	// Held is set while it is on hold: who, when and why.
	Held *Hold `json:"held,omitempty" jsonschema:"description=Set while it is on hold: who and when and why - its work is not pulled and no release waits for it"`
	// Running is set once a person ran it: its proposal is filed (define now,
	// plan when run - rule).
	Running bool `json:"running,omitempty" jsonschema:"description=A person ran it: its analysis and proposal are filed; before - only defined - nothing is planned for it"`
	// Priority is its plan's: the order agents deliver increments in.
	Priority string `json:"priority,omitempty" jsonschema:"description=P0 or P1 or P2 or P3: the order agents deliver increments in (P2 when unset)"`
	// DeliveryReview is its last delivery review.
	DeliveryReview *DeliveryReview `json:"delivery_review,omitempty" jsonschema:"description=Its last delivery review: the verdict and the demo path and the findings"`
	// TestDelivery is set on a test delivery: the increment that turns its
	// switch on.
	TestDelivery *TestDelivery `json:"test_delivery,omitempty" jsonschema:"description=Set on a test delivery: the increment that turns its display switch on"`
}

// IncrementSummaryOutput is where an increment stands, as its pane counts it:
// what it holds done of the total, its requests by where they stand, its state
// and what it waits on.
type IncrementSummaryOutput struct {
	ID           string   `json:"id"`
	Title        string   `json:"title"`
	State        string   `json:"state" jsonschema:"description=open or done or dropped"`
	Standing     string   `json:"standing" jsonschema:"description=Where it stands (IncrementStanding): defined or running or delivering or on-hold or done or shipped or dropped"`
	Held         bool     `json:"held,omitempty" jsonschema:"description=It is on hold"`
	Priority     string   `json:"priority,omitempty"`
	Done         int      `json:"done" jsonschema:"description=Goals and features it holds that are done"`
	Total        int      `json:"total" jsonschema:"description=Goals and features it holds"`
	Awaiting     int      `json:"awaiting" jsonschema:"description=Requests waiting on a person"`
	Running      int      `json:"running" jsonschema:"description=Requests whose step runs now"`
	ToDo         int      `json:"to_do" jsonschema:"description=Requests with a step ready for an agent to take now"`
	WaitingOn    int      `json:"waiting_on" jsonschema:"description=Requests whose next step waits on another request or on their previous step"`
	Idle         int      `json:"idle" jsonschema:"description=Open requests with no step open and nothing awaited: they should end"`
	Blocked      int      `json:"blocked" jsonschema:"description=Requests blocked"`
	RequestsDone int      `json:"requests_done" jsonschema:"description=Requests done"`
	Requests     int      `json:"requests" jsonschema:"description=Requests it counts - those serving or carried by what it holds (never the increment itself); the pane's header counts requests_done of these"`
	Waits        []string `json:"waits" jsonschema:"description=What it waits on - one line each"`
}

// IncrementWaitsOutput is what an increment waits on: one line each.
// Readiness of an increment: computed, never set.
const (
	// ReadinessReady: nothing it waits on is open - an agent can take it.
	ReadinessReady = "ready"
	// ReadinessWaiting: something it waits on is open (WaitsFor says what).
	ReadinessWaiting = "waiting"
	// ReadinessRunning: someone holds one of its work's to-dos (HeldBy).
	ReadinessRunning = "running"
)

// IncrementReadiness is whether an increment can take an agent now: running
// when an agent or a person holds one of its work's to-dos (who), else
// waiting when anything it waits on is open (what), else ready. Done or
// dropped increments are neither: State is empty.
type IncrementReadiness struct {
	State    string   `json:"state" jsonschema:"description=ready or waiting or running; empty when it is done or dropped"`
	WaitsFor []string `json:"waits_for,omitempty" jsonschema:"description=What it waits on while waiting"`
	HeldBy   []string `json:"held_by,omitempty" jsonschema:"description=Who holds its work while running: an agent or a person id"`
}

type IncrementWaitsOutput struct {
	Waits []string `json:"waits" jsonschema:"description=What it waits on - its hold - an increment it depends on still open - a request blocked or awaiting a person - a goal or feature no work serves; empty when it is done or its work runs"`
}

// IncrementRunInput runs an increment: the person's go.
type IncrementRunInput struct {
	ID string       `json:"id" jsonschema:"description=The increment,required"`
	By cwmapi.Actor `json:"by" jsonschema:"description=The person who runs it,required"`
}

// IncrementHoldInput puts an increment on hold, or resumes it.
type IncrementHoldInput struct {
	ID     string       `json:"id"               jsonschema:"description=The increment,required"`
	Reason string       `json:"reason,omitempty" jsonschema:"description=Why it waits (required to hold it)"`
	By     cwmapi.Actor `json:"by"               jsonschema:"description=Who holds or resumes it,required"`
}

// CreateIncrementInput creates an increment, open.
type CreateIncrementInput struct {
	Module string `json:"module,omitempty"   jsonschema:"description=Its module; empty: sys-planification (an increment across modules)"`
	Title  string `json:"title"              jsonschema:"description=Its short title: the name lists and boards show,required"`
	// Description is its intent: what the user gets and why.
	Description string `json:"description,omitempty" jsonschema:"description=Its intent: what the user gets and why"`
	ShipsIn     string `json:"ships_in,omitempty" jsonschema:"description=The release it ships in"`
	// Stream is the stream it aims at: created in its module when none is named.
	Stream string `json:"stream,omitempty" jsonschema:"description=The stream it aims at: its module when none is named"`
	// Breaking marks a change that breaks a user's habit or an API.
	Breaking bool `json:"breaking,omitempty" jsonschema:"description=It breaks a user's habit or an API: its release is a major"`
	// Goals are its delivery goals: kept on it; nothing is planned until a
	// person runs it.
	Goals []string     `json:"goals,omitempty" jsonschema:"description=Its delivery goals in the user's words: kept on it - nothing is planned until a person runs it (increments run)"`
	By    cwmapi.Actor `json:"by,omitempty"    jsonschema:"description=Who creates it: the proposal's requester"`
}

// RenameIncrementInput changes an increment's title or its description; an
// empty field keeps its value.
type RenameIncrementInput struct {
	ID          string       `json:"id"                    jsonschema:"description=The increment,required"`
	Title       string       `json:"title,omitempty"       jsonschema:"description=Its short title; empty: kept"`
	Description string       `json:"description,omitempty" jsonschema:"description=Its intent; empty: kept"`
	By          cwmapi.Actor `json:"by"                    jsonschema:"description=Who renames it,required"`
}

// SetGoalsInput gives an increment its delivery goals.
type SetGoalsInput struct {
	ID    string       `json:"id"    jsonschema:"description=The increment,required"`
	Goals []string     `json:"goals"          jsonschema:"description=Its delivery goals in the user's words (the full list),required"`
	Drop  []string     `json:"drop,omitempty" jsonschema:"description=The goals of a run increment this call removes - each word for word; a goal missing from goals and not named here is refused"`
	By    cwmapi.Actor `json:"by"             jsonschema:"description=Who sets them: the proposal's requester,required"`
}

// IncrementOutput answers one increment.
type IncrementOutput struct {
	Increment Increment `json:"increment"`
	Path      string    `json:"path,omitempty"`
	// Dropped names the delivery goals a set-goals removed.
	Dropped []string `json:"dropped,omitempty"`
}

// ListIncrementsInput lists increments of a module, in a state — both optional.
type ListIncrementsInput struct {
	Module string `json:"module,omitempty"`
	State  string `json:"state,omitempty"`
}

// ListIncrementsOutput lists increments.
type ListIncrementsOutput struct {
	Increments []Increment `json:"increments"`
}

// IncrementTargetInput aims a goal or a feature at an increment.
type IncrementTargetInput struct {
	ID        string `json:"id"        jsonschema:"description=The goal or feature,required"`
	Increment string `json:"increment" jsonschema:"description=The increment,required"`
	// By is who adds it: on a running increment the addition is proposed in
	// their name.
	By cwmapi.Actor `json:"by,omitempty" jsonschema:"description=Who adds it - the proposal's requester when the increment is running"`
	// Demand is the approved demand whose delta holds this very aim: applying
	// it aims at once, the person having approved it already.
	Demand string `json:"demand,omitempty" jsonschema:"description=The approved demand whose delta aims it (applying an increment-target operation): aimed at once"`
}

// IncrementMoveInput moves an increment: done, or dropped.
type IncrementMoveInput struct {
	ID   string       `json:"id"   jsonschema:"description=The increment,required"`
	By   cwmapi.Actor `json:"by"   jsonschema:"description=Who moves it,required"`
	Note string       `json:"note,omitempty"`
	// Force passes the usable-delivered gate on done: a person's, with a
	// note - recorded as forced.
	Force bool `json:"force,omitempty" jsonschema:"description=Done only: pass the usable-delivered gate - a person with a note - recorded as forced"`
}

// SetTestDeliveryInput makes an increment a test delivery: it ships behind
// its display switch on purpose, and the increment named turns it on.
type SetTestDeliveryInput struct {
	ID         string       `json:"id"           jsonschema:"description=The increment,required"`
	TurnedOnBy string       `json:"turned_on_by" jsonschema:"description=The increment that turns its display switch on for the users - planned in the same release; empty clears it"`
	By         cwmapi.Actor `json:"by"           jsonschema:"description=Who sets it,required"`
}

// UsableDeliveredInput asks the usable-delivered gate's requirement of an
// increment: a direct call names it, the gate's call passes it.
type UsableDeliveredInput struct {
	ID     string         `json:"id,omitempty"     jsonschema:"description=The increment (a direct call)"`
	Point  string         `json:"point,omitempty"  jsonschema:"description=The hook point (the gate's call)"`
	Object *cwmapi.Object `json:"object,omitempty" jsonschema:"description=The increment (the gate's call)"`
}

// AcceptWantsInput accepts the goals and features an increment holds that are
// put forward (proposed or in review): every one, or the one named.
type AcceptWantsInput struct {
	ID   string       `json:"id"             jsonschema:"description=The increment,required"`
	Want string       `json:"want,omitempty" jsonschema:"description=The one goal or feature to accept; empty: every one put forward"`
	By   cwmapi.Actor `json:"by"             jsonschema:"description=The person accepting,required"`
}

// AcceptWantsOutput says what was accepted and what was refused, with why.
type AcceptWantsOutput struct {
	Accepted []string          `json:"accepted"`
	Refused  map[string]string `json:"refused,omitempty" jsonschema:"description=Each want not accepted and why"`
}

// IncrementPlanWorkInput plans one increment's work from the gap.
type IncrementPlanWorkInput struct {
	ID string       `json:"id" jsonschema:"description=The increment,required"`
	By cwmapi.Actor `json:"by" jsonschema:"description=Who plans it,required"`
}

// IncrementPaneInput asks one tab of an increment's pane.
type IncrementPaneInput struct {
	ID  string `json:"id" jsonschema:"description=The increment,required"`
	Tab string `json:"tab,omitempty" jsonschema:"description=open-requests (the default: its page on its open requests) or open-request-list (that tab's section) or goals or page (its Increment tab) or blockers or blocker-list (the blockers tab's section)"`
	// Language is the person's (en or fr): the Blockers and Open requests tabs' words.
	Language string `json:"language,omitempty" jsonschema:"description=The person's language (en or fr); empty = English"`
	// Round is how many times the Blockers tab's « Recompute » was pressed:
	// each press reads its section again.
	Round int `json:"round,omitempty" jsonschema:"description=blocker-list: how many times Recompute was pressed - each press reads it again"`
}

// IncrementDetailInput asks an increment's detail, its words in the person's
// language.
type IncrementDetailInput struct {
	ID       string `json:"id" jsonschema:"description=The increment,required"`
	Language string `json:"language,omitempty" jsonschema:"description=The person's language (en or fr); empty = English"`
}

// IncrementPanelInput asks the short panel of the increment picked in
// Planned delivery.
type IncrementPanelInput struct {
	ID string `json:"id" jsonschema:"description=The increment,required"`
	// Language is the person's (en or fr): the panel's words.
	Language string `json:"language,omitempty" jsonschema:"description=The person's language (en or fr); empty = English"`
}

// IncrementCountOutput is a count drawn as a tab's badge: how many, and its
// tone.
type IncrementCountOutput struct {
	Count int    `json:"count"`
	Color string `json:"color,omitempty"`
}

// IncrementServiceDef describes the service for KTSP/CLI auto-generation.
var IncrementServiceDef = meta.ServiceDef{
	Name:    "increments",
	Short:   "Increments of user value: one sentence holding the goals and features that make it - created and aimed at a release (ships-in) - done once each of them is",
	Version: "1.0.0",
	Methods: map[string]meta.MethodDef{
		"Create":               meta.Method("Create an increment - open - in a module or in sys-planification: its short title and its intent (description); optionally the release it ships in and its delivery goals. It is only defined: nothing is planned until a person runs it (increments run)"),
		"Rename":               meta.Method("Change an increment's short title or its intent (description): an empty one is kept"),
		"Get":                  {Short: "Read one increment", ReadOnly: true},
		"List":                 {Short: "The increments of a module or of every module - in a state", ReadOnly: true},
		"Target":               meta.Method("Aim a goal or a feature at an open increment (one increment per object: it replaces the one it targeted); an increment is never aimed at another"),
		"Untarget":             meta.Method("Take a goal or a feature out of its increment"),
		"PlanFor":              meta.Method("Plan an increment for a planned release of its stream: the stream's next release ships it in (ships-in); a later one records the intention (planned-for) - it leaves the release it shipped in, and ships in that release in one action once it is next (releases ship-planned)"),
		"Unplan":               meta.Method("Take an increment's planned-for release off it: it waits in the backlog"),
		"Unship":               meta.Method("Take an increment out of the release it ships in: it waits for another - its stream's next cut or a person's ship-in"),
		"ShipIn":               meta.Method("Ship an increment in an open release (at most one: it replaces the one it shipped in) - with none named, in its stream's next planned release; a release then ships the increment's goals and features"),
		"Backlog":              {Short: "Every open increment that ships in no release, across the streams, by priority - each with its stream and the release ship-in would put it in (its stream's next planned one)", ReadOnly: true},
		"Scope":                {Short: "What an increment holds: its goals and features with their state - done and total", ReadOnly: true},
		"Grade":                {Short: "An increment's grade: computed from the acceptance criteria of what it holds", ReadOnly: true},
		"Done":                 meta.Method("Mark an increment done, as a person: refused while one of its goals or features is not done, or while usable-delivered refuses it (its delivery review has not passed) - a person forces that gate with --force and a note"),
		"SetTestDelivery":      meta.Method("Make an increment a test delivery: it ships behind its display switch on purpose and the increment named - planned in the same release - turns it on for the users; empty clears it"),
		"CheckUsableDelivered": {Short: "Whether an increment is usable-delivered (the gate on its done): its delivery review passed and still holds - every target done - every screen feature's GUI test passed - every display switch retired or turned on by an increment of the same release - a demo path - each follow-up in an increment of the same release", ReadOnly: true},
		"Drop":                 meta.Method("Drop an increment"),
		"Hold":                 meta.Method("Put an open increment on hold saying why: its work is not pulled - each to-do waits saying so - and it leaves the release it ships in (the release no longer waits for it); its hold shows wherever it appears"),
		"PanePage":             {Short: "An increment's pane as a page - the tab its panel names - open-requests (the default: its head and its open requests) or open-request-list (that tab's section) or goals (each delivery goal in detail) or page (its Increment tab: what a person manages - its intent - its proposal - what it depends on) or blockers (everything that locks it - grouped by kind with who acts) or blocker-list (that tab's section)", ReadOnly: true}, "Detail": {Short: "An increment's detail - its drawer and its pane: its row, where it is assigned with the releases it can move to, its proposal, each delivery goal down to the requests serving what it became with their steps - rows of the increment-detail data type", ReadOnly: true},
		"Summary":            {Short: "Where an increment stands, as its pane counts it: what it holds done of the total - its requests awaiting a person, running, blocked and done - its state and what it waits on", ReadOnly: true},
		"Readiness":          {Short: "Whether an increment can take an agent now - running (who holds one of its work's to-dos) or waiting (what it waits on) or ready - computed", ReadOnly: true},
		"OpenRequestCount":   {Short: "How many open requests an increment holds - running + on you + to do: the count of its page's « Open requests (n) » - with its tone (orange): the Open requests tab's badge", ReadOnly: true},
		"BlockerCount":       {Short: "How many roots hold an increment - the total of increments blockers - in red: the Blockers tab's badge", ReadOnly: true},
		"Blockers":           {Short: "Everything that locks an increment through its dependencies - not one hop: each root that holds its work listed once - grouped by kind K1 to K6 (the person's answer - the person's act - agent work ready - agent work held - outside - incidents) with who acts and since when - read-only", ReadOnly: true},
		"Waits":              {Short: "What an increment waits on, one line each: its hold - an increment it depends on still open - a request blocked or awaiting a person - a goal or feature no work serves", ReadOnly: true},
		"PanelPage":          {Short: "The short panel of the increment picked in Delivery's Planned delivery - its header, its release, priority and hold, its requests bar, one bar per delivery goal, Open the increment (its proposal and requests stay on its own page)", ReadOnly: true},
		"OpenPage":           {Short: "Delivery's Open increments table: one line per open increment in no release - id - title - held tag - one-line description - stream - its priority - Hold or Resume and the release it ships in at the end of the line - for a stream (all - an id - none) and held or not held - by priority then rank - the first shown lines (25 by default)", ReadOnly: true},
		"DeliveredPage":      {Short: "Delivery's Delivered tab: the increments done grouped by the release they shipped in - newest release first - No release last - each group a fold (the newest open) - each line its id - title - what it delivered in one line - its number of requests and the day its delivery was approved", ReadOnly: true},
		"BoardMove":          meta.Method("Move an increment on Delivery's Open increments board: to a priority (resumed there when on hold) or to held saying why; a row of another stream is refused; answers {ok, message, undo}"),
		"BoardShip":          meta.Method("Ship an increment into a release of its stream from its card or its panel - the next ships it in, a later one plans it - or take it out (in: out); answers {ok, message, undo}"),
		"SetStream":          meta.Method("Put an increment in a stream (its in-stream edge); answers {ok, message, undo}"),
		"TeamPriorities":     {Short: "Team priorities' data: the increments of the streams shown - delivered in the current release (unless hidden) then the current release then the next then later then none - by priority then rank - each with its progress and what blocks it and the releases it can move to", ReadOnly: true},
		"TeamPrioritiesPage": {Short: "Team priorities - the weekly team meeting's view: a toggle per stream - the delivered toggle - a separator per group with its releases' due date and days left and how much is done - each line's priority (board-move) and release (board-ship) changed from a drop-down", ReadOnly: true},
		"Resume":             meta.Method("Resume an increment on hold: its work is pulled again; it waits for a release - its stream's next cut or a person's ship-in"),
		"Accept":             meta.Method("Accept, as a person, the goals and features an increment holds that are put forward (proposed or in review): every one, or the one named - each reviewed then accepted"),
		"PlanWork":           meta.Method("Plan an open increment's work from the gap - released or not (an increment of a periodic or on-change stream has no release until it is delivered): each goal not decomposed gets its decomposition - each feature (its own and its goals') not passing its acceptance criteria gets its specification or its build - open work reused - two requests filed in one module ordered"),
		"SetGoals":           meta.Method("Give an increment its delivery goals in the user's words: kept on it - nothing planned while it has not run; once run, the changed goals are proposed again"),
		"Run":                meta.Method("Run an increment, as a person (define now - plan when run): its first step analyses its delivery goals against the day's state - what exists, is superseded or is missing - then its proposal (the module goals and features and direct work that deliver them) is approved once and applied, and its work planned. Refused with no goals, ended, or already run"),
	},
}

// IncrementService manages increments.
type IncrementService interface {
	Create(ctx context.Context, in CreateIncrementInput) (IncrementOutput, error)
	Get(ctx context.Context, in GetInput) (IncrementOutput, error)
	List(ctx context.Context, in ListIncrementsInput) (ListIncrementsOutput, error)
	Target(ctx context.Context, in IncrementTargetInput) (TargetOutput, error)
	Untarget(ctx context.Context, in UntargetInput) (TargetOutput, error)
	ShipIn(ctx context.Context, in ShipInInput) (IncrementOutput, error)
	Unship(ctx context.Context, in GetInput) (IncrementOutput, error)
	PlanFor(ctx context.Context, in PlanForInput) (IncrementOutput, error)
	Unplan(ctx context.Context, in GetInput) (IncrementOutput, error)
	Scope(ctx context.Context, in GetInput) (ScopeOutput, error)
	Grade(ctx context.Context, in GetInput) (GradeOutput, error)
	Done(ctx context.Context, in IncrementMoveInput) (IncrementOutput, error)
	Drop(ctx context.Context, in IncrementMoveInput) (IncrementOutput, error)
	// SetTestDelivery names the increment that turns its switch on
	//
	SetTestDelivery(ctx context.Context, in SetTestDeliveryInput) (IncrementOutput, error)
	// CheckUsableDelivered is the usable-delivered gate's requirement.
	CheckUsableDelivered(ctx context.Context, in UsableDeliveredInput) (CheckReadyOutput, error)
	Accept(ctx context.Context, in AcceptWantsInput) (AcceptWantsOutput, error)
	Hold(ctx context.Context, in IncrementHoldInput) (IncrementOutput, error)
	Resume(ctx context.Context, in IncrementHoldInput) (IncrementOutput, error)
	Waits(ctx context.Context, in GetInput) (IncrementWaitsOutput, error)
	// Blockers is everything that locks it, grouped by kind.
	Blockers(ctx context.Context, in GetInput) (IncrementBlockersOutput, error)
	// OpenRequestCount is the Open requests tab's badge: its page's « Open
	// requests (n) » count, orange.
	OpenRequestCount(ctx context.Context, in IncrementPaneInput) (IncrementCountOutput, error)
	// BlockerCount is the Blockers tab's badge: how many roots hold it, red
	//
	BlockerCount(ctx context.Context, in IncrementPaneInput) (IncrementCountOutput, error)
	// Readiness is whether it can take an agent now.
	Readiness(ctx context.Context, in GetInput) (IncrementReadiness, error)
	Summary(ctx context.Context, in GetInput) (IncrementSummaryOutput, error)
	Detail(ctx context.Context, in IncrementDetailInput) (IncrementDetail, error)
	PanePage(ctx context.Context, in IncrementPaneInput) (render.Def, error)
	Rename(ctx context.Context, in RenameIncrementInput) (IncrementOutput, error)
	SetGoals(ctx context.Context, in SetGoalsInput) (IncrementOutput, error)
	PlanWork(ctx context.Context, in IncrementPlanWorkInput) (ReleasePlanWorkOutput, error)
	Run(ctx context.Context, in IncrementRunInput) (IncrementOutput, error)
	Backlog(ctx context.Context, in IncrementBacklogInput) (IncrementBacklogOutput, error)
	PanelPage(ctx context.Context, in IncrementPanelInput) (render.Def, error)
	// OpenPage is Delivery's Open increments table.
	OpenPage(ctx context.Context, in OpenIncrementsInput) (render.Def, error)
	// DeliveredPage is Delivery's Delivered tab.
	DeliveredPage(ctx context.Context, in DeliveredIncrementsInput) (render.Def, error)
	BoardMove(ctx context.Context, in BoardMoveInput) (render.Result, error)
	BoardShip(ctx context.Context, in BoardShipInput) (render.Result, error)
	SetStream(ctx context.Context, in IncrementStreamInput) (render.Result, error)
	TeamPriorities(ctx context.Context, in TeamPrioritiesInput) (TeamPrioritiesOutput, error)
	TeamPrioritiesPage(ctx context.Context, in TeamPrioritiesInput) (render.Def, error)
}

// OpenIncrementsInput asks Delivery's Open increments table
// : the filters, the search and how many lines the page's selection
// holds.
type OpenIncrementsInput struct {
	Stream   string `json:"stream,omitempty"   jsonschema:"description=all (the default) or a stream's id or none (the increments in no stream)"`
	Held     string `json:"held,omitempty"     jsonschema:"description=not-held (the default) or held"`
	Search   string `json:"search,omitempty"   jsonschema:"description=Part of an increment's id or title: the lines whose id or title hold it - ignoring case; empty = every line"`
	Shown    int    `json:"shown,omitempty"    jsonschema:"description=How many lines it shows: 25 by default - Afficher plus adds 25"`
	Language string `json:"language,omitempty" jsonschema:"description=The person's language (en or fr); empty = English"`
	// Selection is the page's selection, the Delivery tab's way to pass what
	// is typed in its search box (open_search) without its section waiting
	// on an empty search.
	Selection map[string]any `json:"selection,omitempty" jsonschema:"description=The Delivery page's selection: its open_search is the search when search is empty"`
}

// DeliveredIncrementsInput asks Delivery's Delivered tab.
type DeliveredIncrementsInput struct {
	Language string `json:"language,omitempty" jsonschema:"description=The person's language (en or fr); empty = English"`
}

// BoardMoveInput moves an increment on the Open increments board
// : to a priority - resumed there when on hold - or to
// On hold saying why.
type BoardMoveInput struct {
	ID     string       `json:"id"               jsonschema:"description=The increment,required"`
	Row    string       `json:"row,omitempty"    jsonschema:"description=The board row it is dropped in: its stream (none for no stream) - another stream's is refused"`
	Column string       `json:"column"           jsonschema:"description=P0 to P3 or held,required"`
	Reason string       `json:"reason,omitempty" jsonschema:"description=Why it waits - required for held"`
	By     cwmapi.Actor `json:"by"               jsonschema:"description=Who moves it,required"`
	// Language is the person's (en or fr): the answer's words.
	Language string `json:"language,omitempty" jsonschema:"description=The person's language (en or fr) for the answer's words; empty = English"`
}

// BoardShipInput ships an increment into a release of its stream from its
// card or its panel - or takes it out (in: out).
type BoardShipInput struct {
	ID string       `json:"id"           jsonschema:"description=The increment,required"`
	In string       `json:"in,omitempty" jsonschema:"description=A planned release of its stream - out or empty takes it out of its release - a stream puts an increment in no stream in it"`
	By cwmapi.Actor `json:"by"           jsonschema:"description=Who ships it,required"`
	// Language is the person's (en or fr): the answer's words.
	Language string `json:"language,omitempty" jsonschema:"description=The person's language (en or fr) for the answer's words; empty = English"`
}

// IncrementStreamInput puts an increment in a stream.
type IncrementStreamInput struct {
	ID     string       `json:"id"     jsonschema:"description=The increment,required"`
	Stream string       `json:"stream" jsonschema:"description=The stream,required"`
	By     cwmapi.Actor `json:"by"     jsonschema:"description=Who sets it,required"`
	// Language is the person's (en or fr): the answer's words.
	Language string `json:"language,omitempty" jsonschema:"description=The person's language (en or fr) for the answer's words; empty = English"`
}

// PlanForInput plans an increment for a release of its stream.
type PlanForInput struct {
	ID string       `json:"id" jsonschema:"description=The increment,required"`
	In string       `json:"in" jsonschema:"description=A planned release of its stream: the next one ships it in - a later one is planned for,required"`
	By cwmapi.Actor `json:"by,omitempty" jsonschema:"description=Who plans it"`
}

// IncrementBacklogInput asks the backlog of increments in no release: every stream.
type IncrementBacklogInput struct {
	// Bare leaves out each affected increment's summary - for a view that
	// does not show it (: the Delivery lanes).
	Bare bool `json:"bare,omitempty" jsonschema:"description=Without each affected increment's summary - for a view that does not show it"`
}

// IncrementBacklogOutput is every open increment that ships in no release, by
// priority (Items) - and the open ones already in a release (Affected), each
// with where it stands.
type IncrementBacklogOutput struct {
	Items    []IncrementBacklogItem `json:"items"`
	Affected []IncrementBacklogItem `json:"affected"`
}

// IncrementBacklogItem is one increment of the backlog: its stream and the release
// « ship in the next release » would put it in.
type IncrementBacklogItem struct {
	Increment  Increment `json:"increment"`
	StreamName string    `json:"stream_name,omitempty" jsonschema:"description=Its stream's name"`
	Next       string    `json:"next,omitempty"        jsonschema:"description=Its stream's next planned release: where ship-in puts it; empty when the stream has none"`
	NextName   string    `json:"next_name,omitempty"   jsonschema:"description=That release's version and name"`
	// ReleaseName names the release an affected increment ships in.
	ReleaseName string `json:"release_name,omitempty" jsonschema:"description=The release it ships in: its version and name"`
	// Summary is where an affected increment stands.
	Summary *IncrementSummaryOutput `json:"summary,omitempty" jsonschema:"description=Where it stands: what it holds done and its requests and waits"`
}
