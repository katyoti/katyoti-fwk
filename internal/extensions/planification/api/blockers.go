// SPDX-License-Identifier: Apache-2.0

package api

// WHAT HOLDS EACH PIECE OF WORK - one reader decides, for
// every open piece of work of a scope, what holds it and sorts it into one
// kind, with who acts and since when. `planning check` prints it and writes
// nothing: the diagnosis, the census before any repair, the measurement.

// The kinds a piece of work is sorted into (K1-K6 of the planification
// resilience study).
const (
	// BlockerAnswer (K1): a question waits for the person's answer, or an
	// answer is not yet applied.
	BlockerAnswer = "answer"
	// BlockerAct (K2): the person's act moves it - validate, decide, confirm,
	// plan, a hold to lift, a blocked plan to look at.
	BlockerAct = "act"
	// BlockerReady (K3): agent work any worker may take now.
	BlockerReady = "ready"
	// BlockerHeld (K4): agent work held - a worker on it, the module in use, a
	// previous step, a dependency, a review, a retry back-off.
	BlockerHeld = "held"
	// BlockerOutside (K5): it waits on what lies outside the scope - another
	// stream's or release's work, work in no release, a resource, an outside
	// party.
	BlockerOutside = "outside"
	// BlockerIncident (K6): it will not move by itself - a cycle, a wait on
	// nothing that moves, an unclassified wait, an inconsistency.
	BlockerIncident = "incident"
)

// BlockerKinds are the kinds in their order, K1 first.
var BlockerKinds = []string{BlockerAnswer, BlockerAct, BlockerReady, BlockerHeld, BlockerOutside, BlockerIncident}

// BlockerClass is a kind's K-number (K1-K6); "" for an unknown kind.
func BlockerClass(kind string) string {
	for i, k := range BlockerKinds {
		if k == kind {
			return "K" + string(rune('1'+i))
		}
	}
	return ""
}

// Who acts on a blocker, beside ActorPerson and ActorAgent.
const (
	ActorSystem  = "system"
	ActorOutside = "outside"
)

// What causes an incident (Blocker.Cause, K6): its two ways out are chosen by
// it, never by its reason's words. An incident with
// none is planned again or taken out.
const (
	CauseCycle          = "cycle"            // a cycle of waits
	CauseNobodyWorks    = "nobody-works"     // it waits on work nobody works on
	CauseNoWork         = "no-work"          // nothing open works on it
	CauseUnreadable     = "unreadable"       // what it waits on cannot be read
	CauseWaitNotCleared = "wait-not-cleared" // what it waits on has ended, the wait stays
	CauseStepClosed     = "step-closed"      // the step it waits after is no longer open
	CauseUnclassified   = "unclassified"     // a wait that names no kind
	CauseUnknownKind    = "unknown-kind"     // a structured wait of a kind no rule reads
	CauseTakenTwice     = "taken-twice"      // two increments or releases take it
)

// The person's act a K2 blocker calls for (Blocker.Act), beside the
// interactions of a person's step (validate, challenge, decide, confirm).
const (
	ActAccept  = "accept"  // an increment whose work is done is accepted
	ActPlan    = "plan"    // nothing is planned: plan its work
	ActUnblock = "unblock" // its plan is blocked: look at the block
)

// Blocker is what holds one open piece of work.
type Blocker struct {
	Class string `json:"class" jsonschema:"description=K1 to K6"`
	Kind  string `json:"kind" jsonschema:"description=answer or act or ready or held or outside or incident"`
	// Work is the piece of work: the object carrying the request.
	Work  string `json:"work"`
	Title string `json:"title,omitempty"`
	Todo  string `json:"todo,omitempty" jsonschema:"description=The to-do it stands on; empty for what is read on the object alone"`
	Step  string `json:"step,omitempty"`
	// Reason is one line saying what holds it.
	Reason string `json:"reason"`
	// Actor is who moves it: a person's or an agent's id, a module's queue,
	// the reconciler, an outside party; ActorKind is person, agent, system
	// or outside.
	Actor     string `json:"actor"`
	ActorKind string `json:"actor_kind"`
	// Since is when it started waiting so (RFC 3339), as stored.
	Since string `json:"since,omitempty"`
	// On is the work or the thing it waits on; Chain the pieces of work
	// it waits on, one after the other, to its root.
	On    string   `json:"on,omitempty"`
	Chain []string `json:"chain,omitempty"`
	// Cycle is, for work in a cycle of stored waits or held by one, the
	// cycle's root - its smallest member, where its one incident is raised
	//
	Cycle string `json:"cycle,omitempty" jsonschema:"description=The root of the cycle of stored waits it is in or held by"`
	// Due is, for a step blocked with no named cause and not retried yet,
	// when its deadline passes (RFC 3339): until then it stays with who
	// reported it, never the person's; past it the reconciler retries it once
	//
	Due string `json:"due,omitempty"`
	// Cause is an incident's cause (K6) and Act the person's act a K2
	// blocker calls for: its actions are chosen by them.
	Cause string `json:"cause,omitempty" jsonschema:"description=An incident's cause: cycle or nobody-works or no-work or unreadable or wait-not-cleared or step-closed or unclassified or unknown-kind or taken-twice"`
	Act   string `json:"act,omitempty" jsonschema:"description=The person's act a K2 blocker calls for: accept or plan or unblock or a step's interaction"`
}

// PlanningCheckInput names the scope the check reads: one of them.
type PlanningCheckInput struct {
	Increment string `json:"increment,omitempty" jsonschema:"description=An increment: the work it holds and what carries its label"`
	Release   string `json:"release,omitempty" jsonschema:"description=A release: the work of its increments and what carries its label"`
	Census    bool   `json:"census,omitempty" jsonschema:"description=Every open piece of work of the workspace"`
	Measure   bool   `json:"measure,omitempty" jsonschema:"description=Add the measures of the planification resilience study over the scope: stalls and incidents a day - time to unblock for system and person causes - unclassified waits and their age - agent spend against the cap"`
	Days      int    `json:"days,omitempty" jsonschema:"description=The measures' window in days back from now; 7 when unset"`
}

// PlanningCheckOutput is every open piece of work of the scope, each once,
// sorted K1 first; the counts by kind; with measure, the scope's measures.
type PlanningCheckOutput struct {
	Scope    string            `json:"scope"`
	Blockers []Blocker         `json:"blockers"`
	Counts   map[string]int    `json:"counts" jsonschema:"description=How many of each kind"`
	Measures *PlanningMeasures `json:"measures,omitempty"`
}

// PLANIFICATION'S RESILIENCE MEASURED - `planning check
// --measure`: the study's « Measure » over a scope and a window, computed
// from stored timestamps only - the incidents' list (since, closed), the
// to-dos done (made, result at, what they came after), the structured waits
// (since) - never reported by an agent. What no store records yet is listed
// in Unmeasured with why, never guessed.

// DefaultMeasureDays is the measures' window when none is given.
const DefaultMeasureDays = 7

// Durations sums up how long a set of waits lasted, in seconds.
type Durations struct {
	Count   int   `json:"count"`
	MedianS int64 `json:"median_s,omitempty" jsonschema:"description=The median wait in seconds"`
	MaxS    int64 `json:"max_s,omitempty" jsonschema:"description=The longest wait in seconds"`
}

// PlanningMeasures are the measures of a scope over a window.
type PlanningMeasures struct {
	From string `json:"from" jsonschema:"description=The window's start (RFC 3339)"`
	To   string `json:"to" jsonschema:"description=The window's end: now (RFC 3339)"`
	Days int    `json:"days"`
	// Stalls are the incidents raised on the scope's work in the window -
	// the stalls the system found - and StallsOpen those still open now.
	Stalls     int `json:"stalls"`
	StallsOpen int `json:"stalls_open"`
	// IncidentsPerDay counts the incidents raised each day (UTC date).
	IncidentsPerDay map[string]int `json:"incidents_per_day,omitempty"`
	// UnblockSystem is how long the incidents closed in the window stayed
	// open; UnblockPerson how long the person's steps reported in the window
	// waited from the moment they could be acted on; PersonWaiting the age of
	// what waits on the person now.
	UnblockSystem Durations `json:"unblock_system"`
	UnblockPerson Durations `json:"unblock_person"`
	PersonWaiting Durations `json:"person_waiting"`
	// Unclassified are the blocked waits that name no kind, with the oldest's
	// age and how many are older than a day.
	Unclassified      int   `json:"unclassified"`
	UnclassifiedMaxS  int64 `json:"unclassified_max_s,omitempty"`
	UnclassifiedOver1 int   `json:"unclassified_over_24h,omitempty"`
	// CapUSD is the agent spend's cap: the increment's own budget, else its
	// release's; 0 when none is stated. SpendUSD is the spend, when known.
	CapUSD   float64  `json:"cap_usd,omitempty"`
	SpendUSD *float64 `json:"spend_usd,omitempty"`
	// Unmeasured are the measures no store records yet, each with why.
	Unmeasured map[string]string `json:"unmeasured,omitempty"`
}

// WHAT BLOCKS AN INCREMENT - `increments blockers <id>`:
// the same reader, followed through the increment's dependencies down to what
// holds each piece of its work at the root - beyond the increment when the
// root lies in another one - each root listed once, under its kind, with the
// pieces of the increment's work it holds; the open incidents of the system
// list on that work are its K6.

// BlockerGroupTitles are what each kind's group is called, K1 first.
var BlockerGroupTitles = map[string]string{
	BlockerAnswer:   "the person's answer",
	BlockerAct:      "the person's act",
	BlockerReady:    "agent work ready",
	BlockerHeld:     "agent work held",
	BlockerOutside:  "outside the increment",
	BlockerIncident: "incidents",
}

// IncrementBlocker is one root that holds the increment's work: what holds it
// (its Blocker: who acts, since when), how long it has waited so, the pieces
// of the increment's work it holds, where it lies when it is outside the
// increment, and its record in the incidents' list when it has one.
type IncrementBlocker struct {
	Blocker
	Age     string   `json:"age,omitempty" jsonschema:"description=How long it has waited so - from its since"`
	Holds   []string `json:"holds,omitempty" jsonschema:"description=The pieces of the increment's work it holds through their waits - besides itself"`
	Outside string   `json:"outside,omitempty" jsonschema:"description=Where it lies when it is not the increment's own work - its increment and release"`
	// Incident is the key of its record in the incidents' list and Sweeps
	// how many sweeps saw it.
	Incident string `json:"incident,omitempty"`
	Sweeps   int    `json:"sweeps,omitempty"`
	// Unwait says a person may remove its to-do's wait: plans unwait
	// accepts it (the Blockers tab offers « Remove the wait » only then).
	Unwait bool `json:"unwait,omitempty" jsonschema:"description=A person may remove its to-do's wait - plans unwait accepts it"`
}

// IncrementBlockerGroup is one kind's roots, the oldest first.
type IncrementBlockerGroup struct {
	Class    string             `json:"class" jsonschema:"description=K1 to K6"`
	Kind     string             `json:"kind"`
	Title    string             `json:"title"`
	Blockers []IncrementBlocker `json:"blockers"`
}

// IncrementBlockersOutput is everything that locks an increment, grouped by
// kind K1 first: only the kinds that hold something.
type IncrementBlockersOutput struct {
	Increment string                  `json:"increment"`
	Total     int                     `json:"total" jsonschema:"description=How many roots hold it"`
	Counts    map[string]int          `json:"counts" jsonschema:"description=How many roots of each kind"`
	Groups    []IncrementBlockerGroup `json:"groups"`
	// Repaired are the repairs the reconciler applied to the increment's
	// work in the week its list keeps them, the newest first.
	Repaired []IncrementRepair `json:"repaired,omitempty" jsonschema:"description=The repairs the reconciler applied to its work - kept a week - the newest first"`
}

// IncrementRepair is one repair the reconciler applied to the increment's
// work: what was wrong, what it did, when, and the work's title.
type IncrementRepair struct {
	RepairDone
	Title string `json:"title,omitempty"`
}
