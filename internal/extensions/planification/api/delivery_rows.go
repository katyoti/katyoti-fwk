// SPDX-License-Identifier: Apache-2.0

package api

import (
	schemaapi "github.com/katyoti/katyoti-fwk/internal/core/schema/api"
)

// THE DELIVERY ROWS - the data the Delivery view and
// the stream, release and increment panes show, as data types that declare
// their display once: a stream's lane on the timeline, a release card, the
// release level's increment rows, an increment's detail. A source answers
// rows of them; the display components draw them (to F-036)
// never a builder of its own. Each field's renderer is its content type
// (status, object-id, priority, stage, progress-segments, gate-dots,
// stepper); each type's layout is its ui-* extensions (DeliveryDisplay).

// GateDot is one gate of a release: passed, failed, or not judged yet (absent:
// it needs a candidate).
type GateDot struct {
	Name   string `json:"name" jsonschema:"description=The gate: increments or dependencies or no-open-p0-fix or e2e or alpha-sign-off or change-log"`
	Passed *bool  `json:"passed,omitempty" jsonschema:"description=Passed - or not - absent while it needs a candidate"`
	Why    string `json:"why,omitempty" jsonschema:"description=Why it does not pass"`
}

// StepChip is one stage of a stepper: a release's stage or a request's step.
type StepChip struct {
	Name  string `json:"name" jsonschema:"description=The stage or the step"`
	State string `json:"state" jsonschema:"description=done or current or todo (a release's stages) - done or running or on-you (a person's step) or failed or blocked or todo (a request's steps)"`
}

// BacklogItem is one increment waiting in a stream's backlog.
type BacklogItem struct {
	ID       string `json:"id" jsonschema:"description=The increment,content=object-id"`
	Title    string `json:"title" jsonschema:"description=Its short title"`
	Priority string `json:"priority" jsonschema:"description=P0 to P3,content=priority"`
	Held     bool   `json:"held,omitempty" jsonschema:"description=It is on hold"`
}

// ReleaseCard is one release as a lane and the release level show it.
type ReleaseCard struct {
	ID        string    `json:"id" jsonschema:"description=The release,content=object-id"`
	Label     string    `json:"label" jsonschema:"description=How every view names it (ReleaseLabel)"`
	Version   string    `json:"version,omitempty" jsonschema:"description=Its version"`
	Date      string    `json:"date,omitempty" jsonschema:"description=Its planned date (YYYY-MM-DD)"`
	DaysLeft  *int      `json:"days_left,omitempty" jsonschema:"description=Days left to its date - negative once late"`
	Stage     string    `json:"stage" jsonschema:"description=planned or frozen or candidate or released or dropped,content=stage"`
	Candidate string    `json:"candidate,omitempty" jsonschema:"description=Its last candidate (rc2)"`
	Build     string    `json:"build,omitempty" jsonschema:"description=Its last candidate's build"`
	Done      int       `json:"done" jsonschema:"description=Goals and features it ships that are done"`
	Total     int       `json:"total" jsonschema:"description=Goals and features it ships"`
	Gates     []GateDot `json:"gates,omitempty" jsonschema:"description=A dot per gate - its reason on hover,content=gate-dots"`
	Planned   int       `json:"planned,omitempty" jsonschema:"description=Increments planned for it"`
	Next      bool      `json:"next,omitempty" jsonschema:"description=It is its stream's next release"`
	Stream    string    `json:"stream,omitempty" jsonschema:"description=Its stream,content=object-id"`
	// Summary, PlannedText and CandidateText are its card's lines, written
	// once here.
	Summary       string `json:"summary" jsonschema:"description=Its date and what it ships done of the total in one line (2026-10-31 · 5/8 done)"`
	PlannedText   string `json:"planned_text,omitempty" jsonschema:"description=What is planned for it (+6 planned) - drawn in the planned tone"`
	CandidateText string `json:"candidate_text,omitempty" jsonschema:"description=Its last candidate and its build in one line (rc1 · build 1.006-rc1)"`
}

// StreamLane is one stream on the timeline: its mode and next cut, its
// releases by date, its backlog at the lane's start.
type StreamLane struct {
	ID         string        `json:"id" jsonschema:"description=The stream,content=object-id"`
	Name       string        `json:"name" jsonschema:"description=Its name"`
	Color      string        `json:"color,omitempty" jsonschema:"description=Its colour in the drawings"`
	Scheme     string        `json:"scheme,omitempty" jsonschema:"description=Its version scheme: semver or calendar or date"`
	Mode       string        `json:"mode" jsonschema:"description=Its release mode: manual or on-change or periodic"`
	ModeText   string        `json:"mode_text" jsonschema:"description=Its mode and next cut in one line"`
	SchemeMode string        `json:"scheme_mode" jsonschema:"description=Its scheme and mode as a chip (semver · periodic · 14d)"`
	NextText   string        `json:"next_text" jsonschema:"description=Its next cut in a few words (next: cut 2026-10-31 - next: by hand - next: when an increment is delivered)"`
	NextCut    string        `json:"next_cut,omitempty" jsonschema:"description=The day of its next cut (YYYY-MM-DD) - empty when none is due by date"`
	Releases   []ReleaseCard `json:"releases" jsonschema:"description=Its releases by date: its last released one then those still to ship"`
	Backlog    []BacklogItem `json:"backlog" jsonschema:"description=Its open increments in no release - held ones among them"`
}

// IncrementRow is one increment of the release level: its priority, title and
// id, where it stands, what it holds and its requests as one bar, what it
// waits on, and whether it needs the person.
type IncrementRow struct {
	ID       string `json:"id" jsonschema:"description=The increment,content=object-id"`
	Title    string `json:"title" jsonschema:"description=Its short title"`
	Priority string `json:"priority" jsonschema:"description=P0 to P3,content=priority"`
	Standing string `json:"standing" jsonschema:"description=Where it stands: defined or running or delivering or on-hold or done or shipped or dropped,content=status"`
	Progress []int  `json:"progress" jsonschema:"description=Its requests: done - running - awaiting a person - to do - blocked,content=progress-segments"`
	Waits    string `json:"waits,omitempty" jsonschema:"description=The first thing it waits on"`
	NeedsYou int    `json:"needs_you,omitempty" jsonschema:"description=Its steps awaiting the person"`
	Held     string `json:"held,omitempty" jsonschema:"description=Why it is on hold"`
	// The row's inline chips, written once here.
	NeedsYouText string `json:"needs_you_text,omitempty" jsonschema:"description=needs you - while a step of it awaits the person,content=alert"`
	HeldText     string `json:"held_text,omitempty" jsonschema:"description=on-hold - while it is on hold,content=status"`
	PlannedChip  string `json:"planned_chip,omitempty" jsonschema:"description=planned - while it is planned for a later release,content=status"`
	ShipsIn      string `json:"ships_in,omitempty" jsonschema:"description=The release it ships in,content=object-id"`
	PlannedFor   string `json:"planned_for,omitempty" jsonschema:"description=The later release it is planned for,content=object-id"`
	Release      string `json:"release,omitempty" jsonschema:"description=The version of the release it ships in or is planned for (1.006)"`
	Place        string `json:"place,omitempty" jsonschema:"description=Its id and the version of its release in one line (billing.IN-001 · 1.006) - the overview's second line"`
	Description  string `json:"description,omitempty" jsonschema:"description=What it delivers - shown on hover over its title"`
	// Options are the releases Change release offers on its row.
	Options []RowOption `json:"options,omitempty" jsonschema:"description=The releases it can move to: the next ships it in - a later one plans it"`
}

// RowOption is one choice of a row's menu: its value and how it reads.
type RowOption struct {
	Value string `json:"value" jsonschema:"description=The release"`
	Label string `json:"label" jsonschema:"description=The release and what it would be (1.007 — planned for)"`
}

// ReleaseRows is the release level: its card, its stage stepper and next
// action, the increments that ship in it and those planned for it.
type ReleaseRows struct {
	Release ReleaseCard `json:"release"`
	// Gates are its card's gates, named in line in its header.
	Gates []GateDot `json:"gates,omitempty" jsonschema:"description=Each gate named in line with its dot (Increments done (3 open)),content=gate-dots(labels)"`
	// Eyebrow, Title and Due are its header's lines, written once here.
	Eyebrow string `json:"eyebrow" jsonschema:"description=Its stream and id - and whether it is its stream's next release (Acme · RL-1 · the stream's next release)"`
	Title   string `json:"title" jsonschema:"description=Its stream and how every view names it (KaTyoti 1.006 — A software factory driven from the PC)"`
	Due     string `json:"due,omitempty" jsonschema:"description=Its date and the days left (due 2026-10-31 · 13 days left)"`
	// NextStep and NextLabel are what a person does next on it, by its stage:
	// the header's primary action.
	NextStep  string `json:"next_step,omitempty" jsonschema:"description=Its next step: freeze or deliver or letter or signoff or promote - empty once released or dropped"`
	NextLabel string `json:"next_label,omitempty" jsonschema:"description=Its next step in words (Finish 3 increments then Freeze - Deliver a candidate - Promote rc1)"`
	// What its stage allows a person to change: the header's other actions.
	CanReschedule bool           `json:"can_reschedule,omitempty" jsonschema:"description=Its date can be moved: it is still to ship"`
	CanUnfreeze   bool           `json:"can_unfreeze,omitempty" jsonschema:"description=It can be unfrozen: frozen with no candidate"`
	CanDrop       bool           `json:"can_drop,omitempty" jsonschema:"description=It can be dropped: it is still to ship"`
	Stepper       []StepChip     `json:"stepper" jsonschema:"description=Its stages - planned frozen candidate released - the current one marked,content=stepper"`
	NextAction    string         `json:"next_action" jsonschema:"description=What a person does next on it"`
	Ships         []IncrementRow `json:"ships" jsonschema:"description=The increments that ship in it - by priority"`
	Planned       []IncrementRow `json:"planned" jsonschema:"description=The increments planned for it - they ship in it once it is its stream's next release"`
	ShipPlanned   bool           `json:"ship_planned,omitempty" jsonschema:"description=Ship the planned increments is offered: it is its stream's next release and some are planned for it"`
}

// ReleaseOption is a release an increment can be moved to - Change release.
type ReleaseOption struct {
	ID    string `json:"id,omitempty" jsonschema:"description=The release - empty for its backlog"`
	Label string `json:"label" jsonschema:"description=The release as every view names it"`
	Mode  string `json:"mode" jsonschema:"description=ships-in (its stream's next release) or planned-for (a later one) or backlog,enum=ships-in|planned-for|backlog"`
}

// RequestLine is one request serving what an increment holds: where it is in
// its work.
type RequestLine struct {
	ID    string     `json:"id" jsonschema:"description=The request,content=object-id"`
	Title string     `json:"title" jsonschema:"description=Its title"`
	State string     `json:"state" jsonschema:"description=Its plan state,content=status"`
	Steps []StepChip `json:"steps,omitempty" jsonschema:"description=Its steps - done - running - to do,content=stepper"`
	Where string     `json:"where" jsonschema:"description=Its current step and who holds it - or what it awaits"`
}

// HeldLine is a goal or a feature an increment's proposal made of a delivery
// goal, and the requests serving it.
type HeldLine struct {
	ID       string        `json:"id" jsonschema:"description=The goal or the feature,content=object-id"`
	Title    string        `json:"title" jsonschema:"description=Its title"`
	State    string        `json:"state" jsonschema:"description=Its lifecycle state,content=status"`
	Requests []RequestLine `json:"requests,omitempty" jsonschema:"description=The requests serving it"`
}

// GoalLine is one delivery goal and what it became.
type GoalLine struct {
	Chip   string     `json:"chip" jsonschema:"description=Its name by its increment and rank (IN-1·G2)"`
	Goal   string     `json:"goal" jsonschema:"description=The delivery goal in the user's words"`
	State  string     `json:"state" jsonschema:"description=done (all it became is done) or running or todo (nothing named for it yet),content=status"`
	Became []HeldLine `json:"became,omitempty" jsonschema:"description=The goals and features its proposal made of it"`
}

// IncrementDetail is an increment's detail - the drawer, and its own pane:
// its row, where it is assigned with the releases it can move to, its
// proposal and each delivery goal down to its requests.
type IncrementDetail struct {
	Row      IncrementRow    `json:"row"`
	Eyebrow  string          `json:"eyebrow" jsonschema:"description=Increment · its id"`
	Assigned string          `json:"assigned" jsonschema:"description=Ships in - planned for - or not assigned - in one line"`
	Options  []ReleaseOption `json:"options,omitempty" jsonschema:"description=The releases it can move to (Change release) - with its backlog"`
	Proposal string          `json:"proposal" jsonschema:"description=Where its proposal stands"`
	Goals    []GoalLine      `json:"goals,omitempty" jsonschema:"description=Each delivery goal and what it became"`
}

// LanesInput asks the streams' lanes: every stream, or one.
type LanesInput struct {
	ID string `json:"id,omitempty" jsonschema:"description=A stream: its lane alone"`
	// Language is the person's: the cards' and the lane's words
	//
	Language string `json:"language,omitempty" jsonschema:"description=The person's language (en or fr); empty = English"`
}

// LanesOutput is the streams' lanes.
type LanesOutput struct {
	Lanes []StreamLane `json:"lanes"`
}

// THE DISPLAY - each type's layout, declared once beside it: what the data
// type documents carry as their ui-* extensions (the contract of
// with to F-036). A view of layout slots
// places fields in its slots (eyebrow, title and titleId, badges, meta joined
// by « · », footer, actions); an action carries its verb, its args reading
// the page's templates (${item.id}, ${person}, ${value}, ${selection.<key>}),
// a choice is a row field of [{value, label}] or a literal list, and a
// prompt asks the person for the text ${value} carries.

// incrementRowMenu is an increment row's menu: Change release as its button,
// the rest under its « … ».
func incrementRowMenu() []any {
	return []any{
		act("change-release", "Change release", displayPlanForURI, map[string]any{"id": "${item.id}", "in": "${value}", "by": "${person}"}, "choice", "options", "button", true),
		act("take-out", "Take out", displayUnshipURI, map[string]any{"id": "${item.id}", "by": "${person}"}, "confirm", "Take it out of its release?"),
		act("hold", "Put on hold", displayHoldURI, map[string]any{"id": "${item.id}", "reason": "${value}", "by": "${person}"}, "prompt", "Why does it wait?"),
		act("resume", "Resume", displayResumeURI, map[string]any{"id": "${item.id}", "by": "${person}"}),
		act("priority", "Priority", displayAssessURI, map[string]any{"id": "${item.id}", "priority": "${value}", "by": "${person}"}, "choice", "P0|P1|P2|P3"),
	}
}

// ext is one ui-* extension.
func ext(kind string, payload map[string]any) schemaapi.Extension {
	return schemaapi.Extension{Kind: kind, Payload: payload}
}

// act is an action of a view or a menu.
func act(id, label, uri string, args map[string]any, extra ...any) map[string]any {
	a := map[string]any{"id": id, "label": label, "uri": uri, "args": args}
	for i := 0; i+1 < len(extra); i += 2 {
		if k, ok := extra[i].(string); ok {
			a[k] = extra[i+1]
		}
	}
	return a
}

// stageAct is a release's stage action in its header: shown only when it is
// its next step, primary, worded by its row.
func stageAct(step, uri string, args map[string]any, extra ...any) map[string]any {
	a := act(step, "${item.next_label}", uri, args, extra...)
	a["when"] = "next_step=" + step
	a["primary"] = true
	return a
}

// The verbs the delivery views run, as the person.
const (
	displayPlanForURI  = "ktw-agents:increments/plan-for"
	displayUnshipURI   = "ktw-agents:increments/unship"
	displayHoldURI     = "ktw-agents:increments/hold"
	displayResumeURI   = "ktw-agents:increments/resume"
	displayAssessURI   = "ktw-agents:requests/assess"
	displayCutURI      = "ktw-agents:streams/cut"
	displaySetURI      = "ktw-agents:streams/set"
	displayReschedURI  = "ktw-agents:releases/reschedule"
	displayUnfreezeURI = "ktw-agents:releases/unfreeze"
	displayDropURI     = "ktw-agents:releases/drop"
	displayFreezeURI   = "ktw-agents:releases/freeze"
	displayDeliverURI  = "ktw-agents:releases/deliver"
	displayLetterURI   = "ktw-agents:releases/letter"
	displaySignOffURI  = "ktw-agents:releases/sign-off"
	displayPromoteURI  = "ktw-agents:releases/promote"
)

// DeliveryDisplay is each delivery type's ui-* extensions, by its type name.
var DeliveryDisplay = map[string][]schemaapi.Extension{
	"StreamLane": {
		ext("ui-timeline-lanes", map[string]any{"name": "default", "laneHeader": "lane-header", "items": "releases", "itemDate": "date",
			"itemView": "card", "backlog": "backlog", "today": true, "select": "release", "axisLabel": "Stream · release mode"}),
		ext("ui-view", map[string]any{"name": "lane-header", "layout": "slots", "slots": map[string]any{
			"title": "name", "titleId": "id", "badges": []any{"scheme_mode"}, "meta": []any{"next_text"},
			"actions": []any{
				act("mode", "Release mode", displaySetURI, map[string]any{"id": "${item.id}", "release_mode": "${value}"}, "choice", "manual|on-change|periodic", "value", "mode"),
				act("cut", "Cut now", displayCutURI, map[string]any{"id": "${item.id}", "by": "${person}"}, "confirm", "Make its next release now from its done increments in no release?"),
				// No « New increment »: an increment comes from a study's
				// hand-over (k-study --decide), never made here
				// (the person).
			}}}),
	},
	"ReleaseCard": {
		ext("ui-view", map[string]any{"name": "card", "layout": "slots", "slots": map[string]any{
			"eyebrow": "id", "title": "label", "badges": []any{"stage"},
			"meta": []any{"summary", map[string]any{"field": "planned_text", "tone": "planned"}, "candidate_text"}, "footer": []any{"gates"}}}),
	},
	"BacklogItem": {
		ext("ui-view", map[string]any{"name": "chip", "layout": "slots", "slots": map[string]any{"title": "id", "badges": []any{"priority"}}}),
	},
	"ReleaseRows": {
		// The release level's header: its next action - the one its stage
		// shows - first and primary, Reschedule, then Unfreeze and Drop under
		// its menu; at the header's top right.
		ext("ui-view", map[string]any{"name": "header", "layout": "slots", "slots": map[string]any{
			"eyebrow": "eyebrow", "title": "title", "meta": []any{"due"}, "footer": []any{"stepper", "gates"}, "actionsAt": "top",
			"actions": []any{
				stageAct("freeze", displayFreezeURI, map[string]any{"id": "${item.release.id}", "by": "${person}"}),
				stageAct("deliver", displayDeliverURI, map[string]any{"id": "${item.release.id}", "build": "${value}", "by": "${person}"}, "prompt", "Its build (optional)"),
				stageAct("letter", displayLetterURI, map[string]any{"id": "${item.release.id}", "candidate": "${item.release.candidate}", "by": "${person}"}),
				stageAct("signoff", displaySignOffURI, map[string]any{"id": "${item.release.id}", "candidate": "${item.release.candidate}", "by": "${person}"}),
				stageAct("promote", displayPromoteURI, map[string]any{"id": "${item.release.id}", "candidate": "${item.release.candidate}", "by": "${person}"}),
				act("reschedule", "Reschedule", displayReschedURI, map[string]any{"id": "${item.release.id}", "date": "${value}", "by": "${person}"}, "prompt", "Its new date (YYYY-MM-DD)", "when", "can_reschedule"),
				act("unfreeze", "Unfreeze", displayUnfreezeURI, map[string]any{"id": "${item.release.id}", "by": "${person}"}, "confirm", "Unfreeze it to change what it ships?", "menu", true, "when", "can_unfreeze"),
				act("drop", "Drop", displayDropURI, map[string]any{"id": "${item.release.id}", "by": "${person}"}, "confirm", "Drop it? Its increments return to the backlog.", "menu", true, "when", "can_drop"),
			}}}),
		ext("ui-view", map[string]any{"name": "default", "layout": "detail-pane", "groups": []any{
			map[string]any{"id": "release", "fields": []any{"release", "stepper", "next_action"}},
			map[string]any{"id": "ships", "title": "Ships in this release", "fields": []any{"ships"}},
			map[string]any{"id": "planned", "title": "Planned for this release", "fields": []any{"planned"}},
		}}),
	},
	"GoalLine": {
		ext("ui-view", map[string]any{"name": "card", "layout": "slots", "slots": map[string]any{"eyebrow": "chip", "title": "goal", "badges": []any{"state"}}}),
	},
	"HeldLine": {
		ext("ui-view", map[string]any{"name": "line", "layout": "slots", "slots": map[string]any{"title": "title", "titleId": "id", "badges": []any{"state"}}}),
	},
	"RequestLine": {
		// Three short lines - a drawer's width: its title and id, its step
		// chips, where it is in its work.
		ext("ui-view", map[string]any{"name": "line", "layout": "slots", "slots": map[string]any{
			"title": "title", "titleId": "id", "meta": []any{"where"}, "footer": []any{"steps"}}}),
		// The request an object carries itself, under that object's line:
		// where it is and its steps, its title not repeated.
		ext("ui-view", map[string]any{"name": "steps", "layout": "slots", "slots": map[string]any{
			"meta": []any{"where"}, "footer": []any{"steps"}}}),
	},
	"IncrementRow": {
		ext("ui-table", map[string]any{"name": "default", "density": "compact", "header": false, "select": "increment",
			"columns": []any{
				map[string]any{"id": "priority", "width": 52},
				map[string]any{"id": "title", "append": []any{"id", "held_text", "planned_chip"}, "hover": "description"},
				map[string]any{"id": "progress", "width": 160},
				map[string]any{"id": "waits", "prepend": []any{"needs_you_text"}},
			},
			"menu": incrementRowMenu()}),
		// Team priorities' lines: the same row with the version of the release
		// it ships in after its id.
		ext("ui-table", map[string]any{"name": "team", "density": "compact", "header": false,
			"columns": []any{
				map[string]any{"id": "priority", "width": 52},
				map[string]any{"id": "title", "append": []any{"id", "release", "held_text", "planned_chip"}, "hover": "description"},
				map[string]any{"id": "progress", "width": 160},
				map[string]any{"id": "waits", "prepend": []any{"needs_you_text"}},
			},
			"menu": incrementRowMenu()}),
		// The planning deck's Overview: a compact line on two lines - its
		// priority, its title with its id and release under it, its progress.
		ext("ui-table", map[string]any{"name": "overview", "density": "compact", "header": false,
			"columns": []any{
				map[string]any{"id": "priority", "width": 52},
				map[string]any{"id": "title", "below": []any{"place"}, "hover": "description"},
				map[string]any{"id": "progress", "width": 120},
			}}),
	},
	"IncrementDetail": {
		// The drawer's head: its id, title and chips.
		ext("ui-view", map[string]any{"name": "head", "layout": "slots", "slots": map[string]any{
			"eyebrow": "eyebrow", "title": "row.title", "badges": []any{"row.priority", "row.standing", "row.held_text"}}}),
		ext("ui-view", map[string]any{"name": "default", "layout": "detail-pane", "groups": []any{
			map[string]any{"id": "assignment", "fields": []any{"row", "assigned", "options"}},
			map[string]any{"id": "proposal", "title": "Its proposal", "fields": []any{"proposal"}},
			map[string]any{"id": "goals", "title": "Delivery goals, and what each became", "fields": []any{"goals"}},
		}}),
	},
}
