// SPDX-License-Identifier: Apache-2.0

// Package api is the scheduler's contract: the work tickets of each module, as
// plain files out of CWM, and the few verbs that hand them out — the same for
// the internal scheduler and for an outside worker (a Claude Code session).
package api

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/meta"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// THE SCHEDULER — work scheduling, nothing else. A module's to-dos are records
// of the scheduler's store (a row of `SchedulerDir()/scheduler.db`, or a file on
// the file store); a worker pulls the next ready one, works it, reports its
// result; the to-do's owner (planification) collects the result, records what
// lasts on the CWM objects and archives the to-do (done, pruned after a while).
// A to-do is local runtime state, never committed. A to-do is a transient ticket: it holds
// what scheduling needs and pointers — never reference information; nothing
// links to it.

// A to-do's states.
const (
	StateWaiting  = "waiting"  // its owner has not released it
	StateReady    = "ready"    // it can be pulled
	StateClaimed  = "claimed"  // a worker holds it, for a lease
	StateReported = "reported" // its result is back, for its owner to collect
)

// How a to-do starts.
const (
	StartAuto   = "auto"   // the internal scheduler may run it (in an auto module)
	StartManual = "manual" // only a worker who pulls it runs it
)

// Who does a to-do.
const (
	ForAgent  = "agent"
	ForPerson = "person"
)

// A module's scheduling modes.
const (
	ModeManual = "manual" // its to-dos are pulled by people and sessions (the default)
	ModeAuto   = "auto"   // the internal scheduler runs its auto to-dos too
	ModeOff    = "off"    // nothing of it is handed out
)

// A result's statuses.
const (
	StatusSuccess = "success"
	StatusFail    = "fail"
	StatusBlocked = "blocked"
)

// DefaultLeaseSeconds is how long a pull holds a to-do unless renewed.
const DefaultLeaseSeconds = 900

// DefaultDoneDays is how long a done to-do is kept before it is pruned.
const DefaultDoneDays = 30

// RequestsModule is the one system module that holds to-dos: where a request
// no module owns yet, or a demand from outside (an import, an external
// system), is evaluated — its intake routes it on. Every other system module
// holds the platform's operational state, never to-dos.
const RequestsModule = cwmapi.SystemModulePrefix + "planification"

// THE INTERNAL SCHEDULER — one more worker on the same contract: for the
// modules in mode auto it pulls their auto to-dos and runs the work-todo task
// on each, in the to-do's task group; a run that ends without a report is
// reported failed for it; the done to-dos past their keeping time are pruned.

// WorkTodoTask is the task the internal scheduler runs on a to-do: it works it
// the way any worker does.
const WorkTodoTask = "work-todo"

// SchedulerTask is the internal scheduler's long-lived run.
const SchedulerTask = "scheduler"

// DefaultGroup is the task group a to-do without one runs in.
const DefaultGroup = "planning"

// RunLeaseSeconds is the lease an internal run starts with; the run renews it.
const RunLeaseSeconds = 3600

// Worker is who the internal scheduler's runs hold their to-dos as.
var Worker = cwmapi.Actor{ID: WorkTodoTask, Kind: "agent"}

// Todo is one work ticket.
type Todo struct {
	ID     string `json:"id" yaml:"id"`
	Module string `json:"module" yaml:"module"`
	// Repository is the store's: the uuid of the project its module belongs
	// to, the key its rows and its module locks carry beside the module; ""
	// in a store that serves one project, or for a to-do of no project.
	Repository string `json:"repository,omitempty" yaml:"repository,omitempty"`
	// Target is the CWM object the work is on: a worker gets its context, its
	// tools and its actions from it.
	Target  string `json:"target" yaml:"target"`
	Request string `json:"request,omitempty" yaml:"request,omitempty"`
	Title   string `json:"title" yaml:"title"`
	For     string `json:"for" yaml:"for"`
	Skill   string `json:"skill,omitempty" yaml:"skill,omitempty"`
	// Scheduling.
	Group    string   `json:"group,omitempty" yaml:"group,omitempty"`
	Start    string   `json:"start" yaml:"start"`
	Priority string   `json:"priority" yaml:"priority"`
	State    string   `json:"state" yaml:"state"`
	Since    string   `json:"since,omitempty" yaml:"since,omitempty"`
	Position int      `json:"position,omitempty" yaml:"position,omitempty"`
	After    []string `json:"after,omitempty" yaml:"after,omitempty"`
	Reads    []string `json:"reads,omitempty" yaml:"reads,omitempty"`
	Writes   []string `json:"writes,omitempty" yaml:"writes,omitempty"`
	Waits    string   `json:"waits,omitempty" yaml:"waits,omitempty"`
	// Rank is its owner's order within a priority: lower first, 0 (unset)
	// after every set rank. The scheduler gives it no other meaning.
	Rank int `json:"rank,omitempty" yaml:"rank,omitempty"`
	// Labels are its owner's, opaque `key:value` strings a worker filters on
	// (release:2026.10): the scheduler matches them and gives them no meaning.
	// A Put replaces them with the to-do.
	Labels []string `json:"labels,omitempty" yaml:"labels,omitempty"`
	// Key is its owner's: a Put with a key an open to-do of the module
	// carries returns that to-do — a replayed handover is put once.
	Key string `json:"key,omitempty" yaml:"key,omitempty"`
	// Retries: MaxAttempts and BackoffSeconds are its owner's; Attempts is
	// counted by the scheduler — a failure with attempts left puts it back
	// ready, not pulled before NotBefore.
	MaxAttempts    int    `json:"max_attempts,omitempty" yaml:"max_attempts,omitempty"`
	BackoffSeconds int    `json:"backoff_seconds,omitempty" yaml:"backoff_seconds,omitempty"`
	Attempts       int    `json:"attempts,omitempty" yaml:"attempts,omitempty"`
	NotBefore      string `json:"not_before,omitempty" yaml:"not_before,omitempty"`
	// Work is its owner's: what to do and what follows — the scheduler stores
	// it and never reads it.
	Work map[string]any `json:"work,omitempty" yaml:"work,omitempty"`
	// Assignee, when set, is the one worker who may pull it: a member of its
	// holding list it was given to (Assign); it goes back to the list when that
	// worker releases it.
	Assignee *cwmapi.Actor `json:"assignee,omitempty" yaml:"assignee,omitempty"`
	// While it is worked, and once worked.
	Claim  *Claim  `json:"claim,omitempty" yaml:"claim,omitempty"`
	Result *Result `json:"result,omitempty" yaml:"result,omitempty"`
	Owner  string  `json:"owner" yaml:"owner"`
	Made   string  `json:"made" yaml:"made"`
	// Archived is when its owner moved it to done; it is pruned after.
	Archived string `json:"archived,omitempty" yaml:"archived,omitempty"`
	Note     string `json:"note,omitempty" yaml:"note,omitempty"`
	// Held is the structured wait a blocked report gave: what holds it, written
	// in the report's own write. Its owner reads it; the scheduler gives it no
	// other meaning and drops it when the to-do is pulled again.
	Held *HeldOn `json:"held,omitempty" yaml:"held,omitempty"`
	// Revision is the store's: the version of the to-do a reader got. A write
	// carries the revision it read, and the store refuses it once another
	// write came between (ErrStaleRevision); 0 is a to-do no write of the
	// store's port has counted yet.
	Revision int64 `json:"revision,omitempty" yaml:"revision,omitempty"`
}

// The store's refusals: a write at a revision another write passed, and a
// holder's write whose claim another claim superseded. Both are conflicts.
var (
	ErrStaleRevision    = fmt.Errorf("%w: the to-do changed since it was read (stale revision)", cwmapi.ErrConflict)
	ErrSupersededTicket = fmt.Errorf("%w: the claim was superseded by another (stale ticket)", cwmapi.ErrConflict)
)

// ErrPlanningFileAfterMigration refuses a to-do or wait file found in a
// module's planning space once the planning files were moved into the
// scheduler's store.
var ErrPlanningFileAfterMigration = errors.New("a planning file was written after the migration")

// The kinds of thing a blocked to-do waits on. A blocked report naming none is
// unclassified - kept, with its reason, its age and who reported it.
const (
	HeldObject   = "object"   // a CWM object: a request, a feature, a decision
	HeldPerson   = "person"   // a person's act
	HeldResource = "resource" // a runner, a sandbox, a freeze, the hub
	HeldExternal = "external" // an outside party
)

// HeldOn is a blocked to-do's structured wait.
type HeldOn struct {
	Kind   string       `json:"kind,omitempty" yaml:"kind,omitempty" jsonschema:"description=object or person or resource or external; empty when the reporter named none (unclassified)"`
	Target string       `json:"target,omitempty" yaml:"target,omitempty" jsonschema:"description=What it waits on: an object id or a person or gui-runner or sandbox or freeze or hub or an outside party"`
	Reason string       `json:"reason" yaml:"reason" jsonschema:"description=One line for the person"`
	Since  string       `json:"since" yaml:"since"`
	By     cwmapi.Actor `json:"by" yaml:"by"`
}

// SchedulerConfig is Scheduler.yaml: whether the internal scheduler runs, and
// how often it looks when nothing moves.
type SchedulerConfig struct {
	On              *bool `yaml:"on,omitempty" json:"on,omitempty"`
	IntervalSeconds int   `yaml:"interval_seconds,omitempty" json:"interval_seconds,omitempty"`
}

// Enabled: on unless switched off.
func (c SchedulerConfig) Enabled() bool { return c.On == nil || *c.On }

// Interval is how often the internal scheduler looks when nothing moves: 30 s
// unless set.
func (c SchedulerConfig) Interval() time.Duration {
	if c.IntervalSeconds <= 0 {
		return 30 * time.Second
	}
	return time.Duration(c.IntervalSeconds) * time.Second
}

// Claim is who holds a to-do.
type Claim struct {
	By      cwmapi.Actor `json:"by" yaml:"by"`
	Until   string       `json:"until" yaml:"until"`
	At      string       `json:"at" yaml:"at"`
	Run     string       `json:"run,omitempty" yaml:"run,omitempty"`
	Session string       `json:"session,omitempty" yaml:"session,omitempty"`
	// Ticket is the claim's fencing token, the store's: the to-do's revision
	// the claim was taken at. A later claim has a higher one; a write naming a
	// ticket that is not the held claim's is refused (ErrSupersededTicket).
	Ticket int64 `json:"ticket,omitempty" yaml:"ticket,omitempty"`
}

// Result is what a worker handed back.
type Result struct {
	Status  string         `json:"status" yaml:"status" jsonschema:"description=success or fail or blocked"`
	Summary string         `json:"summary" yaml:"summary" jsonschema:"description=One line: what was done and how it was proven - or what failed or blocks it; more goes in details"`
	Details string         `json:"details,omitempty" yaml:"details,omitempty" jsonschema:"description=The detailed result when one line is not enough (markdown); shown on demand under the summary"`
	Verdict string         `json:"verdict,omitempty" yaml:"verdict,omitempty" jsonschema:"description=A person's validation or challenge: approved or rejected"`
	Answer  string         `json:"answer,omitempty" yaml:"answer,omitempty" jsonschema:"description=A person's decision or what they provided"`
	More    map[string]any `json:"more,omitempty" yaml:"more,omitempty" jsonschema:"description=For its owner: steps to add (add) and follow-ups (follow_ups) and a choice for a person (choice)"`
	By      cwmapi.Actor   `json:"by" yaml:"by"`
	At      string         `json:"at" yaml:"at"`
	// Relayed is set when an agent's session relayed the person's report.
	Relayed *Relayed `json:"relayed,omitempty" yaml:"relayed,omitempty" jsonschema:"description=Set when an agent's session relayed the person's report: which session and how the person confirmed it"`
}

// Filter narrows the to-dos a worker asks for.
type Filter struct {
	// Module is a module of the universe's projects: bare, the one the CWM
	// resolves; qualified (<key>:<module>), that project's.
	Module string `json:"module,omitempty" jsonschema:"description=The module: bare or qualified by its project (<key>:<module>)"`
	// Project keeps the to-dos of one project only: two
	// projects' modules of one name never share a queue. Its key (the project
	// object's id) or its uuid; read as the uuid the to-dos carry
	// (Todo.Repository).
	Project  string `json:"project,omitempty" jsonschema:"description=Only the to-dos of this project: its key (the project object's id) or its uuid"`
	Request  string `json:"request,omitempty" jsonschema:"description=The request its to-dos are steps of"`
	Priority string `json:"priority,omitempty" jsonschema:"description=Only this priority or a more urgent one (P0 to P3)"`
	Group    string `json:"group,omitempty" jsonschema:"description=The task group"`
	Skill    string `json:"skill,omitempty" jsonschema:"description=Only the to-dos that follow this skill"`
	For      string `json:"for,omitempty" jsonschema:"description=agent (default) or person"`
	// Labels: a to-do kept carries every one.
	Labels []string `json:"labels,omitempty" jsonschema:"description=Only the to-dos carrying every one of these labels (opaque key:value - e.g. release:2026.10)"`
}

// PullInput takes the next ready to-do for the filter.
type PullInput struct {
	Filter
	ID           string       `json:"id,omitempty" jsonschema:"description=A given to-do instead of the next"`
	LeaseSeconds int          `json:"lease_seconds,omitempty" jsonschema:"description=How long it is held unless renewed; 900 when unset"`
	By           cwmapi.Actor `json:"by" jsonschema:"description=The worker,required"`
	// Prefer breaks a tie: among the to-dos of one priority and one rank, the
	// module's come first. It never passes a lower rank or a higher priority.
	Prefer string `json:"prefer,omitempty" jsonschema:"description=A module whose to-dos come first among those of the same priority and rank (the one the worker last worked)"`
	// Auto is the internal scheduler's pull: only the auto to-dos of the auto
	// modules.
	Auto bool `json:"auto,omitempty" jsonschema:"description=Only the auto to-dos of the auto modules (the internal scheduler)"`
	// Wait keeps the pull open while the filter's scope still holds work an
	// agent can move: it answers the to-do once one can be pulled, finished
	// when none is left for an agent, or timeout.
	Wait string `json:"wait,omitempty" jsonschema:"description=How long to wait for a to-do of the filter (a duration: 30m - at most 1h); ends early with finished once the scope holds no agent to-do ready or held or reported"`
}

// MaxWait bounds a pull's wait: the command line waits for its answer a
// little longer (the Pull method's timeout).
const MaxWait = time.Hour

// What ended a pull that waited and took nothing (PullOutput.Ended).
const (
	EndedFinished = "finished" // the scope holds no agent to-do ready, held or reported: only a person's step or nothing is left
	EndedTimeout  = "timeout"  // the wait passed while work of the scope was still held or waiting
)

// PullOutput is the to-do pulled, or none and why the others wait.
type PullOutput struct {
	Todo    *Todo    `json:"todo,omitempty"`
	Nothing string   `json:"nothing,omitempty" jsonschema:"description=Nothing ready: why"`
	Waiting []string `json:"waiting,omitempty" jsonschema:"description=Each other ready to-do and why it waits"`
	Ended   string   `json:"ended,omitempty" jsonschema:"description=A pull that waited and took nothing: finished (no agent to-do of the scope is ready or held or reported) or timeout"`

	// Waits is each waiting line's structured wait, in the order of Waiting:
	// its cause and what holds it (: no owner's word).
	Waits []QueueWait `json:"waits,omitempty" jsonschema:"description=Each waiting line's cause and holder - in the order of waiting"`
}

// RenewInput is a worker's heartbeat.
type RenewInput struct {
	ID           string       `json:"id" jsonschema:"description=The to-do,required"`
	LeaseSeconds int          `json:"lease_seconds,omitempty"`
	By           cwmapi.Actor `json:"by" jsonschema:"description=The worker holding it,required"`
	Run          string       `json:"run,omitempty" jsonschema:"description=The run working it (the internal scheduler)"`
	Session      string       `json:"session,omitempty" jsonschema:"description=That run's session"`
}

// ReleaseInput gives a pulled to-do back.
type ReleaseInput struct {
	ID   string       `json:"id" jsonschema:"description=The to-do,required"`
	Note string       `json:"note,omitempty"`
	By   cwmapi.Actor `json:"by" jsonschema:"description=The worker holding it,required"`
}

// ReportInput hands a to-do's result back.
type ReportInput struct {
	ID string `json:"id" jsonschema:"description=The to-do,required"`
	Result
	// Wait is what holds a blocked report: kind (object or person or resource
	// or external) and target and reason. Kept on the to-do in the report's own
	// write; a blocked report without it is unclassified, its summary its reason.
	Wait *HeldOn `json:"wait,omitempty" jsonschema:"description=A blocked report's structured wait: kind and target and reason"`
}

// AssignInput gives a to-do to one worker: a member of its holding list.
type AssignInput struct {
	ID   string       `json:"id" jsonschema:"description=The to-do,required"`
	To   cwmapi.Actor `json:"to" jsonschema:"description=The worker it is given to (a member of its holding list),required"`
	By   cwmapi.Actor `json:"by" jsonschema:"description=Who gives it: a person - a member of its list - or the worker holding it,required"`
	Note string       `json:"note,omitempty"`
}

// TodoOutput is one to-do.
type TodoOutput struct {
	Todo Todo   `json:"todo"`
	Note string `json:"note,omitempty"`
}

// ListInput lists to-dos.
type ListInput struct {
	Module  string `json:"module,omitempty" jsonschema:"description=The module; empty: every module"`
	Request string `json:"request,omitempty"`
	State   string `json:"state,omitempty" jsonschema:"description=waiting or ready or claimed or reported"`
	Done    bool   `json:"done,omitempty" jsonschema:"description=The archived ones instead"`
	// Step is its owner's step name (work.step); a step done again
	// ("implement (again)") counts as its step.
	Step string `json:"step,omitempty" jsonschema:"description=Only the to-dos of this step (work.step - implement or review or ...); a step done again counts as its step"`
	// Labels: a to-do kept carries every one.
	Labels []string `json:"labels,omitempty" jsonschema:"description=Only the to-dos carrying every one of these labels (opaque key:value - e.g. increment:IN-1)"`
	// Since keeps the to-dos whose last moment - archived, else reported,
	// else claimed, else made - is at or after it.
	Since string `json:"since,omitempty" jsonschema:"description=Only the to-dos whose last moment (archived or reported or claimed or made) is at or after this time: RFC 3339 or a date YYYY-MM-DD"`
}

// SearchInput searches the to-dos of every module, scheduled and done: a to-do
// kept matches every filter given, and at least one is given.
type SearchInput struct {
	Text    string `json:"text,omitempty" jsonschema:"description=Words found in its id or title or note or result summary or details - any case"`
	Request string `json:"request,omitempty" jsonschema:"description=The request its to-do is a step of"`
	Target  string `json:"target,omitempty" jsonschema:"description=The object it works on"`
	Actor   string `json:"actor,omitempty" jsonschema:"description=An actor id: who holds or held it or who reported it or who it was given to"`
	Module  string `json:"module,omitempty" jsonschema:"description=The module; empty: every module"`
}

// HistoryInput names the to-do whose history is read.
type HistoryInput struct {
	ID string `json:"id" jsonschema:"description=The to-do,required"`
}

// Where a revision of a to-do was read.
const (
	FromStore = "store" // the scheduler's store
	FromGit   = "git"   // a commit of the to-do's file in its repository
)

// Revision is one version of a to-do.
type Revision struct {
	From     string `json:"from" jsonschema:"description=store or git"`
	Revision int64  `json:"revision,omitempty" jsonschema:"description=The store's revision of the to-do"`
	Commit   string `json:"commit,omitempty" jsonschema:"description=The commit that wrote it (from git)"`
	At       string `json:"at,omitempty" jsonschema:"description=When the commit was made (from git)"`
	Author   string `json:"author,omitempty" jsonschema:"description=Who made the commit (from git)"`
	Path     string `json:"path,omitempty" jsonschema:"description=The file the commit wrote - relative to the module's planning space (from git)"`
	Todo     Todo   `json:"todo"`
}

// HistoryOutput is a to-do's history: the revisions its store keeps, oldest
// first, then the commits of its file in the git archive, oldest first.
type HistoryOutput struct {
	ID        string     `json:"id"`
	Revisions []Revision `json:"revisions"`
}

// TodosOutput is a list of to-dos.
type TodosOutput struct {
	Todos []Todo `json:"todos"`
}

// GetInput names a to-do.
type GetInput struct {
	ID string `json:"id" jsonschema:"description=The to-do,required"`
}

// QueueEntry is a ready to-do in the queue: takeable, or why it waits.
type QueueEntry struct {
	Todo     Todo   `json:"todo"`
	Takeable bool   `json:"takeable"`
	Waits    string `json:"waits,omitempty"`
	// Wait is why it waits, structured: its cause and what holds it; nil when
	// it is takeable.
	Wait *QueueWait `json:"wait,omitempty"`
	// Only is who alone may take it, whoever looks: the worker it is given
	// to (cause assigned, Holder) or its holding list's members (cause
	// members, Group) - the rule a pull applies, never who is on the list;
	// nil when any worker may.
	Only *QueueWait `json:"only,omitempty" jsonschema:"description=Who alone may take it whoever looks: the worker it is given to (cause assigned) or its holding list's members (cause members); absent when any worker may"`
	// Takers are the recorded pulls that would take it (takeable only): open
	// now, ended lately, or whose session holds a to-do.
	Takers []Taker `json:"takers,omitempty" jsonschema:"description=The recorded pulls whose filter would take it (takeable to-dos only): open now or ended within the default lease or whose session holds a to-do; named on an entry this queue holds takeable"`
}

// Taker is a recorded pull that would take a to-do: its worker, its session,
// and whether it waits now, when it ended, or what its session holds.
type Taker struct {
	By      cwmapi.Actor `json:"by" jsonschema:"description=The worker that pulled"`
	Session string       `json:"session,omitempty" jsonschema:"description=The agent session it pulled from"`
	Waiting bool         `json:"waiting,omitempty" jsonschema:"description=The pull is open now: it waits for a to-do"`
	At      string       `json:"at,omitempty" jsonschema:"description=When the pull ended (RFC 3339)"`
	Holds   string       `json:"holds,omitempty" jsonschema:"description=The to-do its session holds (a pull ended earlier than the default lease)"`
}

// The causes of a queue wait (QueueWait.Cause): the scheduler's own words,
// which an owner's wait namer maps to its own (planification's kinds).
const (
	WaitLock     = "lock"     // a module it reads or writes is held by a running to-do
	WaitReserved = "reserved" // a module it writes is reserved by a ready to-do ahead of it in the queue
	WaitBackoff  = "backoff"  // retried after a back-off that has not passed
	WaitAssigned = "assigned" // given to another worker
	WaitMembers  = "members"  // held for the members of its holding list
	WaitPerson   = "person"   // a person's to-do, asked for by an agent
)

// QueueWait is why one ready to-do waits: its cause and what holds it. The
// scheduler gives it no meaning beyond its cause: an owner's wait namer
// (WaitNamer) names it in the owner's words.
type QueueWait struct {
	Todo  string `json:"todo" jsonschema:"description=The to-do that waits"`
	Cause string `json:"cause" jsonschema:"description=lock or reserved or backoff or assigned or members or person"`
	// Module is the module held or reserved (lock or reserved).
	Module string `json:"module,omitempty" jsonschema:"description=The module held or reserved"`
	// HolderTodo is the to-do holding or reserving it, Holder the worker that
	// holds that to-do (lock), HolderRequest its request.
	HolderTodo    string        `json:"holder_todo,omitempty" jsonschema:"description=The to-do holding or reserving the module"`
	Holder        *cwmapi.Actor `json:"holder,omitempty" jsonschema:"description=The worker holding the module or the one the to-do is given to"`
	HolderRequest string        `json:"holder_request,omitempty" jsonschema:"description=The request of the to-do holding or reserving the module"`
	// Group is the holding list it is held for (members).
	Group string `json:"group,omitempty" jsonschema:"description=The holding list it is held for"`
	// Until is the end of its back-off (backoff).
	Until string `json:"until,omitempty" jsonschema:"description=The end of its retry back-off"`
}

// QueueOutput is the ready to-dos in the order they are pulled.
type QueueOutput struct {
	Queue []QueueEntry `json:"queue"`
	// TakersSince is when the pull record started: the record is kept in
	// memory, a restart begins it again.
	TakersSince string `json:"takers_since,omitempty" jsonschema:"description=When the pull record the takers come from started (RFC 3339): kept in memory and begun again at a restart"`
}

// PutInput writes a to-do (its owner's verb): new when it has no id.
type PutInput struct {
	Todo Todo `json:"todo" jsonschema:"required"`
}

// ArchiveInput moves a to-do to done once its owner collected it.
type ArchiveInput struct {
	ID   string `json:"id" jsonschema:"description=The to-do,required"`
	Note string `json:"note,omitempty"`
}

// ModeInput sets a module's scheduling mode.
type ModeInput struct {
	Module string       `json:"module" jsonschema:"description=The module,required"`
	Mode   string       `json:"mode" jsonschema:"description=manual or auto or off,required"`
	By     cwmapi.Actor `json:"by" jsonschema:"description=Who sets it,required"`
}

// ModeOutput is a module's scheduling mode.
type ModeOutput struct {
	Module string `json:"module"`
	Mode   string `json:"mode"`
}

// ModesInput lists the modes.
type ModesInput struct{}

// ModesOutput is every module's scheduling mode, and whether the scheduler is on.
type ModesOutput struct {
	Scheduler bool         `json:"scheduler" jsonschema:"description=The internal scheduler is on (Scheduler.yaml)"`
	Modules   []ModeOutput `json:"modules"`
}

// PruneInput removes the done to-dos past their module's keeping time.
type PruneInput struct {
	Module string `json:"module,omitempty" jsonschema:"description=The module; empty: every module"`
}

// PruneOutput says what was removed - and what was kept past its time because
// its repository's export failed (the SQLite store prunes only what an export
// holds).
type PruneOutput struct {
	Removed int `json:"removed"`
	Kept    int `json:"kept,omitempty"`
}

// ExportInput writes the scheduler's store to the export refs: every
// repository's rows - or one repository's.
type ExportInput struct {
	Repository string `json:"repository,omitempty" jsonschema:"description=The project uuid whose rows are exported; empty: every one"`
}

// ExportedRef is one repository's export: the ref it went to and what it holds.
type ExportedRef struct {
	Repository string `json:"repository" jsonschema:"description=The project uuid"`
	Dir        string `json:"dir" jsonschema:"description=The git repository the ref is in"`
	Ref        string `json:"ref" jsonschema:"description=The export ref (refs/katyoti/scheduler/<project uuid>)"`
	Commit     string `json:"commit" jsonschema:"description=The commit the ref points at"`
	Changed    bool   `json:"changed" jsonschema:"description=A new commit was written; false: the ref held the same rows"`
	Todos      int    `json:"todos" jsonschema:"description=To-dos exported - open and done"`
	Revisions  int    `json:"revisions" jsonschema:"description=Revisions of the to-dos exported"`
	Waits      int    `json:"waits" jsonschema:"description=Waits exported"`
	Remote     string `json:"remote,omitempty" jsonschema:"description=The private remote the ref is pushed to; empty: the repository has none"`
	Pushed     bool   `json:"pushed,omitempty" jsonschema:"description=The remote holds the ref's commit"`
	PushError  string `json:"push_error,omitempty" jsonschema:"description=Why the ref was not pushed: a public remote refused - a remote whose visibility could not be told - a push rejected or failed"`
}

// SkippedRepository is rows no ref can keep, and why.
type SkippedRepository struct {
	Repository string `json:"repository,omitempty" jsonschema:"description=The project uuid; empty: rows of no project"`
	Todos      int    `json:"todos"`
	Waits      int    `json:"waits"`
	Reason     string `json:"reason"`
}

// ExportOutput is the refs written and the rows no ref keeps.
type ExportOutput struct {
	Refs    []ExportedRef       `json:"refs"`
	Skipped []SkippedRepository `json:"skipped,omitempty"`
}

// SnapshotInput asks for the newest hourly snapshot of the store.
type SnapshotInput struct{}

// SnapshotOutput is the newest hourly snapshot of the store.
type SnapshotOutput struct {
	Path string `json:"path,omitempty" jsonschema:"description=The newest hourly snapshot's file; empty: the store is not the SQLite store"`
}

// ImportInput reads the export refs back into the scheduler's store: every
// known project's - or one's.
type ImportInput struct {
	Repository string `json:"repository,omitempty" jsonschema:"description=The project uuid whose export ref is read; empty: every project of the universe in a git repository"`
}

// ImportedRef is what one export ref brought back.
type ImportedRef struct {
	Repository   string `json:"repository"`
	Dir          string `json:"dir"`
	Ref          string `json:"ref"`
	Commit       string `json:"commit"`
	Todos        int    `json:"todos" jsonschema:"description=To-dos brought in"`
	TodosPresent int    `json:"todos_present" jsonschema:"description=To-dos the store already held - left as they are"`
	Revisions    int    `json:"revisions" jsonschema:"description=Revisions brought in"`
	Waits        int    `json:"waits"`
	WaitsPresent int    `json:"waits_present"`
}

// ImportOutput is what each export ref read brought.
type ImportOutput struct {
	Refs []ImportedRef `json:"refs"`
}

// DoctorInput runs the full integrity check of the scheduler's store
// (todos doctor): it takes nothing.
type DoctorInput struct{}

// DoctorFault is one fault the full integrity check found, and its repair.
type DoctorFault struct {
	Fault  string `json:"fault" jsonschema:"description=The fault as PRAGMA integrity_check names it"`
	Repair string `json:"repair" jsonschema:"description=What brings the store back"`
}

// DoctorOutput is the store checked and each fault found: none on a sound
// store; a fault makes the command exit non-zero.
type DoctorOutput struct {
	Store  string        `json:"store" jsonschema:"description=The store file checked"`
	Found  bool          `json:"found" jsonschema:"description=The file exists; false: this universe has no SQLite store yet"`
	OK     bool          `json:"ok" jsonschema:"description=No fault found"`
	Failed int           `json:"failed" jsonschema:"description=The faults found"`
	Faults []DoctorFault `json:"faults" ui-cli:"mode=table,columns=fault;repair,failure-count-field=failed"`
}

// MigrateInput moves the planning files - every module's .ktw/todo, .ktw/done
// and .ktw/waiting, and the universe's own requests space - into the
// scheduler's SQLite store.
type MigrateInput struct {
	DryRun bool `json:"dry_run,omitempty" jsonschema:"description=Read the files and say what the move would do; write nothing"`
	Remove bool `json:"remove,omitempty" jsonschema:"description=Once every file has its row: remove the files (git history is their archive) and record the migration's end - from then on a to-do file written there again is refused. The real run is a person's go: nothing else may write to-dos meanwhile"`
}

// MigratedCounts counts the to-dos and waits on one side of the move.
type MigratedCounts struct {
	Open  int `json:"open"`
	Done  int `json:"done"`
	Waits int `json:"waits"`
}

// MigratedSpace is one planning space read: its module, its folder and what
// its files hold.
type MigratedSpace struct {
	Module string         `json:"module"`
	Dir    string         `json:"dir"`
	Files  MigratedCounts `json:"files"`
}

// MigrateRepair is a planning file the move could not take as it is, and what
// it did with it.
type MigrateRepair struct {
	File   string `json:"file"`
	Repair string `json:"repair"`
}

// MigrateOutput is the files before and their rows after: equal, or what
// differs, named.
type MigrateOutput struct {
	DryRun bool `json:"dry_run,omitempty"`
	// Spaces are the planning spaces read, each with what its files hold.
	Spaces []MigratedSpace `json:"spaces"`
	Files  MigratedCounts  `json:"files" jsonschema:"description=The to-dos and waits the files hold (a to-do in both todo and done counted once - done)"`
	Rows   MigratedCounts  `json:"rows" jsonschema:"description=The rows the store holds of those files after the move"`
	// Added, Updated and Unchanged count the to-do and wait rows the move wrote
	// new, brought up to their file, or found equal.
	Added     int `json:"added"`
	Updated   int `json:"updated"`
	Unchanged int `json:"unchanged"`
	// Kept are the rows further on than their file: left as they are.
	Kept []string `json:"kept,omitempty"`
	// Missing are the files with no row of their revision and folder after
	// the move: the files are not removed while any is missing.
	Missing []string `json:"missing,omitempty"`
	// Unread are planning folders holding files beside the spaces read that no
	// module listed owns (a module the list missed): the files are not removed
	// while any is unread.
	Unread  []string        `json:"unread,omitempty"`
	Repairs []MigrateRepair `json:"repairs,omitempty" jsonschema:"description=Planning files the move repaired: a to-do in both todo and done (done is kept); a to-do naming no module (its folder's)"`
	// Removed is the files removed; Migrated when the migration ended (RFC 3339).
	Removed  int    `json:"removed,omitempty"`
	Migrated string `json:"migrated,omitempty"`
	Store    string `json:"store" jsonschema:"description=The store file the rows are in"`
}

// RunInput runs a ready to-do with the internal worker now, whatever its
// module's mode.
type RunInput struct {
	ID string       `json:"id" jsonschema:"description=The to-do,required"`
	By cwmapi.Actor `json:"by" jsonschema:"description=Who asks for it,required"`
}

// TickInput runs one step of the internal scheduler.
type TickInput struct{}

// TickReport is what one step of the internal scheduler did.
type TickReport struct {
	Started   []string `json:"started,omitempty" jsonschema:"description=The to-dos started and their runs"`
	GivenBack []string `json:"given_back,omitempty" jsonschema:"description=The to-dos whose run ended without a report: reported failed"`
	Collected []string `json:"collected,omitempty" jsonschema:"description=The to-dos left reported by a collect that was cut: handed to their owner again"`
	Pruned    int      `json:"pruned,omitempty" jsonschema:"description=The done to-dos removed"`
	Reminded  []string `json:"reminded,omitempty" jsonschema:"description=The waits flagged past their deadline"`
	Waiting   []string `json:"waiting,omitempty" jsonschema:"description=Why nothing more started"`
	Followed  []string `json:"followed,omitempty" jsonschema:"description=The rows re-pointed or ended after a move or split or archive the store missed"`
	Flagged   []string `json:"flagged,omitempty" jsonschema:"description=The object names rows carry that no longer resolve and the journal has no record of: kept"`
	Errors    []string `json:"errors,omitempty"`
}

// Settings is a module's `.ktw/settings.yaml`.
type Settings struct {
	Scheduling string `json:"scheduling,omitempty" yaml:"scheduling,omitempty"`
	DoneDays   int    `json:"done_days,omitempty" yaml:"done_days,omitempty"`
	WaitDays   int    `json:"wait_days,omitempty" yaml:"wait_days,omitempty"`
}

// THE WAITS — what a module awaits from another: a record of the scheduler's
// store (a file per wait on the file store), out of CWM, with a deadline; it is
// removed once the answer came. A module asking another (a request filed in
// it) awaits it; past the deadline the internal scheduler flags it once, and
// the module chases it or gives up. Like a to-do, a wait is transient: nothing
// links to it.

// DefaultWaitDays is how long a module waits for an answer unless it says.
const DefaultWaitDays = 7

// Wait is what a module awaits.
type Wait struct {
	ID       string       `json:"id" yaml:"id"`
	Module   string       `json:"module" yaml:"module" jsonschema:"description=The module that waits"`
	On       string       `json:"on" yaml:"on" jsonschema:"description=What it waits on: the request it filed in the other module"`
	From     string       `json:"from" yaml:"from" jsonschema:"description=The module asked"`
	What     string       `json:"what" yaml:"what" jsonschema:"description=What is awaited in one line"`
	Since    string       `json:"since" yaml:"since"`
	Deadline string       `json:"deadline" yaml:"deadline"`
	Reminded string       `json:"reminded,omitempty" yaml:"reminded,omitempty" jsonschema:"description=When it was flagged past its deadline"`
	By       cwmapi.Actor `json:"by" yaml:"by"`
	Overdue  bool         `json:"overdue,omitempty" yaml:"-" jsonschema:"description=Its deadline passed"`
}

// AddWaitInput records what a module awaits.
type AddWaitInput struct {
	Module string       `json:"module" jsonschema:"description=The module that waits,required"`
	On     string       `json:"on" jsonschema:"description=What it waits on: the request it filed in the other module,required"`
	From   string       `json:"from" jsonschema:"description=The module asked,required"`
	What   string       `json:"what" jsonschema:"description=What is awaited in one line,required"`
	Days   int          `json:"days,omitempty" jsonschema:"description=How many days it waits; the module's wait_days (7) when unset"`
	By     cwmapi.Actor `json:"by" jsonschema:"description=Who records it,required"`
}

// ListWaitsInput lists waits.
type ListWaitsInput struct {
	Module  string `json:"module,omitempty" jsonschema:"description=The module that waits; empty: every module"`
	On      string `json:"on,omitempty" jsonschema:"description=Only the waits on this"`
	Overdue bool   `json:"overdue,omitempty" jsonschema:"description=Only the ones past their deadline"`
}

// WaitsOutput is a list of waits, oldest first.
type WaitsOutput struct {
	Waits []Wait `json:"waits"`
}

// ClearWaitInput ends a wait: the answer came, or the module gives up.
type ClearWaitInput struct {
	ID string `json:"id,omitempty" jsonschema:"description=The wait"`
	On string `json:"on,omitempty" jsonschema:"description=Or every wait on this (the request answered)"`
}

// ClearWaitOutput says how many waits ended.
type ClearWaitOutput struct {
	Cleared int `json:"cleared"`
}

// WaitService is the waits.
type WaitService interface {
	Add(ctx context.Context, in AddWaitInput) (Wait, error)
	List(ctx context.Context, in ListWaitsInput) (WaitsOutput, error)
	Clear(ctx context.Context, in ClearWaitInput) (ClearWaitOutput, error)
}

// WaitServiceDef describes the service for KTSP/CLI auto-generation.
var WaitServiceDef = meta.ServiceDef{
	Name:    "waits",
	Short:   "What each module awaits from another (a wait in the scheduler's store, with a deadline): added when a module asks another, removed when the answer comes, flagged past its deadline",
	Version: "1.0.0",
	Methods: map[string]meta.MethodDef{
		"Add":   meta.Method("Record that a module awaits something from another, until a deadline (the module's wait_days, 7 by default)"),
		"List":  {Short: "The waits of a module, or of every module - each flagged overdue past its deadline", ReadOnly: true},
		"Clear": meta.Method("End a wait - or every wait on what was answered: the answer came, or the module gives up"),
	},
}

// Collector is what an owner contributes to collect its reported to-dos: it
// reads the result, records what lasts on the CWM objects, and archives it.
type Collector struct {
	Owner   string
	Collect func(ctx context.Context, t Todo) error
}

// Runner is what an owner contributes to run its to-dos itself, without the
// work-todo task: given the to-do the internal worker pulled, it does the work
// and reports it through the contract. It answers the run it started —
// watched like a work-todo run — or none once it has reported.
type Runner struct {
	Owner string
	Run   func(ctx context.Context, t Todo) (run, session string, err error)
}

// WaitNamer is what an owner contributes to name, in its own words, why one of
// its to-dos waits: the pull prefixes the waiting line of a to-do of Owner
// with the label Name answers ("" adds none). Name runs outside the store's
// hold and must not pull, report or change to-dos.
type WaitNamer struct {
	Owner string
	Name  func(ctx context.Context, t Todo, w QueueWait) string
}

// HOLDING LISTS — a to-do's group can be a holding list: a
// group whose members, persons or agents, are its workers. Only a member pulls
// its to-dos; the claim says who took it; the worker hands it back (Release)
// or it is given to another member (Assign). The to-do stays the one record
// and its claim the one claim: there is no second queue. A group no source
// lists is open, as before: any worker pulls its to-dos.

// GroupMembers is what an owner of holding lists contributes (inbox's holding
// lists): the members of a group, and whether it is a list at all - a group
// none lists is open.
type GroupMembers struct {
	Name    string
	Members func(ctx context.Context, group string) (members []cwmapi.Actor, listed bool, err error)
}

// TodoService is the scheduler.
type TodoService interface {
	Pull(ctx context.Context, in PullInput) (PullOutput, error)
	Renew(ctx context.Context, in RenewInput) (TodoOutput, error)
	Release(ctx context.Context, in ReleaseInput) (TodoOutput, error)
	Assign(ctx context.Context, in AssignInput) (TodoOutput, error)
	Report(ctx context.Context, in ReportInput) (TodoOutput, error)
	Get(ctx context.Context, in GetInput) (TodoOutput, error)
	List(ctx context.Context, in ListInput) (TodosOutput, error)
	Search(ctx context.Context, in SearchInput) (TodosOutput, error)
	History(ctx context.Context, in HistoryInput) (HistoryOutput, error)
	Queue(ctx context.Context, in Filter) (QueueOutput, error)
	Put(ctx context.Context, in PutInput) (TodoOutput, error)
	Archive(ctx context.Context, in ArchiveInput) (TodoOutput, error)
	Mode(ctx context.Context, in ModeInput) (ModeOutput, error)
	Modes(ctx context.Context, in ModesInput) (ModesOutput, error)
	Prune(ctx context.Context, in PruneInput) (PruneOutput, error)
	Export(ctx context.Context, in ExportInput) (ExportOutput, error)
	Import(ctx context.Context, in ImportInput) (ImportOutput, error)
	Doctor(ctx context.Context, in DoctorInput) (DoctorOutput, error)
	Snapshot(ctx context.Context, in SnapshotInput) (SnapshotOutput, error)
	Migrate(ctx context.Context, in MigrateInput) (MigrateOutput, error)
	Run(ctx context.Context, in RunInput) (TodoOutput, error)
	Tick(ctx context.Context, in TickInput) (TickReport, error)
}

// TodoServiceDef describes the service for KTSP/CLI auto-generation.
var TodoServiceDef = meta.ServiceDef{
	Name:    "todos",
	Short:   "The scheduler: each module's to-dos (work tickets in the scheduler's store) handed out to workers inside and outside - pull, renew, report",
	Version: "1.0.0",
	Methods: map[string]meta.MethodDef{
		"Pull":     {Short: "Take the next ready to-do for the filter (module, request, priority, group, skill, for agent or person, labels - every one carried) - or a given one: held by the worker for a lease; a holding list's to-do only by one of its members, a to-do given to a worker only by that worker; prefer names a module that wins a tie of priority and rank; nothing ready says why; wait keeps it open until a to-do can be pulled, or answers finished once the scope holds nothing an agent can move, or timeout. Labels and rank are the to-do owner's, opaque: the scheduler matches and orders them and gives them no meaning", TimeoutS: int(MaxWait/time.Second) + 60},
		"Renew":    meta.Method("A worker's heartbeat on the to-do it holds"),
		"Release":  meta.Method("Give a pulled to-do back: ready again - for its whole holding list when it was given to the worker releasing it"),
		"Assign":   meta.Method("Give a to-do to one worker, a member of its holding list: only that worker pulls it then - a claim another holds ends; by a person, a member of the list or the worker holding it"),
		"Report":   meta.Method("Hand a to-do's result back (success or fail or blocked, what was done; a person's verdict or answer): its owner collects it and decides what follows"),
		"Get":      {Short: "One to-do", ReadOnly: true},
		"List":     {Short: "The to-dos of a module or a request, in a state, of a step, carrying labels, since a time - or the done ones", ReadOnly: true},
		"Search":   {Short: "Find to-dos of every module, scheduled and done, by words (id, title, note, result), request, target or actor (who holds, held, reported it or was given it); every filter given matches", ReadOnly: true},
		"History":  {Short: "One to-do's history: the revisions its store keeps, then the commits of its file in the git archive, each oldest first", ReadOnly: true},
		"Queue":    {Short: "The ready to-dos in the order they are pulled - priority, then rank (lower first, unset last), then age, then position - each takeable or why it waits (a module a running to-do writes); labels filter them (every one carried). Labels and rank are the to-do owner's, opaque to the scheduler", ReadOnly: true},
		"Put":      meta.Method("Write a to-do (its owner's verb: planification)"),
		"Archive":  meta.Method("Move a to-do to done once its owner collected it (its owner's verb)"),
		"Mode":     meta.Method("Set a module's scheduling mode: manual (pulled by people and sessions - the default) or auto (the internal scheduler runs its auto to-dos too) or off (nothing handed out)"),
		"Modes":    {Short: "Each module's scheduling mode, and whether the internal scheduler is on", ReadOnly: true},
		"Prune":    meta.Method("Remove the done to-dos kept past their module's done_days (30 by default) - on the SQLite store only once an export holds them"),
		"Export":   meta.Method("Write the scheduler's SQLite store - open and done to-dos with every revision and claim ticket, waits, the ticket sequence - to a ref of each project's git repository (refs/katyoti/scheduler/<project uuid>): git objects only, a commit over the last export when something changed"),
		"Import":   meta.Method("Read the export refs back into the scheduler's SQLite store: the same rows with the same revisions; a row whose id the store holds is left as it is, so an import run again adds nothing"),
		"Doctor":   {Short: "Run the full integrity check of the scheduler's SQLite store (PRAGMA integrity_check - an open runs only the quick one) and name each fault found with its repair: move the file aside, then todos import; a fault exits non-zero", ReadOnly: true},
		"Snapshot": meta.Method("The newest hourly snapshot of the scheduler's SQLite store - taken first (VACUUM INTO, kept only once its quick_check passes) when the current hour has none - the generations pruned to the 24 newest hourly and one a day for 14 days; empty on the file store"),
		"Migrate":  meta.Method("Move the planning files - every module's .ktw/todo, .ktw/done and .ktw/waiting - into the scheduler's SQLite store: files before, rows after; a run again changes nothing; remove then deletes the files (git history their archive) and records the end, after which a to-do file written there is refused"),
		"Run":      meta.Method("Run a ready agent to-do with the internal worker now (the work-todo task, in the to-do's group), whatever its module's mode: pulled for the run, which reports it"),
		"Tick":     meta.Method("One step of the internal scheduler: report failed the to-dos whose run ended without a report, collect again those a stopped process left reported, prune, flag the waits past their deadline, start the auto to-dos of the auto modules within their groups' windows"),
	},
}

// Relayed records a person's report an agent's session relayed: which session,
// how the person confirmed it (prompt or totp or entra) and when.
type Relayed struct {
	By        string `json:"by" yaml:"by" jsonschema:"description=The agent session that relayed it"`
	Confirmed string `json:"confirmed" yaml:"confirmed" jsonschema:"description=How the person confirmed it: prompt or totp or entra"`
	At        string `json:"at,omitempty" yaml:"at,omitempty" jsonschema:"description=When the person confirmed it (RFC 3339)"`
}
