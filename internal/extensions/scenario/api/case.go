// SPDX-License-Identifier: Apache-2.0

// Package api is the scenario test case contract: a FOLDER holding a project's
// initial content, the user a simulated person plays, and `test-case.yaml` —
// how to set the case up (a universe, its sidecars, the project) and the
// STAGES that carry it from intent to a tested result. Each stage is steps
// (calls through the universe's hub, as the GUI makes them; a simulated user
// answering the Inbox; commands in the project) and checks; a stage that
// passes is checkpointed, so a long scenario restarts just before the stage
// that failed instead of from the beginning.
package api

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/marshal"
)

// CaseFile is the spec's name inside a case folder.
const CaseFile = "test-case.yaml"

// Case is one scenario test case, as `test-case.yaml` declares it.
type Case struct {
	Name        string `yaml:"name" json:"name"`
	Title       string `yaml:"title,omitempty" json:"title,omitempty"`
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
	// Universe is the universe the case runs in (one per case, named scn-<name>).
	Universe CaseUniverse `yaml:"universe" json:"universe"`
	// OtherUniverses are the other universes of the solution the case is
	// published as, beside its own (the main one): each set up at install
	// beside it (scn-<name>-<its name>), never started, and snapshotted with it.
	OtherUniverses []CaseOtherUniverse `yaml:"other_universes,omitempty" json:"otherUniverses,omitempty"`
	// Project is the project the universe serves: created from a folder.
	Project CaseProject `yaml:"project" json:"project"`
	// Repositories are the other repositories the universe serves beside its
	// project, each a member of its own: created
	// at install under repositories/<name>, recorded as created on this PC
	// and attached after the project.
	Repositories []CaseRepository `yaml:"repositories,omitempty" json:"repositories,omitempty"`
	// User is the file (case-relative) describing the person a simulated user
	// plays: who they are, what they want, their preferences and facts, how
	// they answer and challenge.
	User string `yaml:"user,omitempty" json:"user,omitempty"`
	// Answers are the person's scripted answers, read from the front matter of
	// the User file (`answers:`): when present, the simulated user answers
	// from them — no agent runs, no LLM. Set by Load.
	Answers []UserAnswer `yaml:"-" json:"-"`
	// Stages carry the case from its start to its end, in order.
	Stages []Stage `yaml:"stages" json:"stages"`
	// Machine is the case on a clean machine: the installer a channel ships
	// double-clicked, then the machine stages run through the installed
	// programs, each timed (`ktw-compose scenario machine`).
	Machine *CaseMachine `yaml:"machine,omitempty" json:"machine,omitempty"`

	// Dir is the case folder (set by Load).
	Dir string `yaml:"-" json:"-"`
}

// UserAnswer is one scripted answer of the person: given to the first pending
// question whose title or description contains When (case-insensitive; empty
// matches any), once. Decision is what the inbox records (approved, rejected,
// or a word of the person's); Note is the answer in their words — required
// with rejected, as the inbox requires it.
type UserAnswer struct {
	When     string `yaml:"when,omitempty" json:"when,omitempty"`
	Decision string `yaml:"decision" json:"decision"`
	Note     string `yaml:"note,omitempty" json:"note,omitempty"`
}

// userFile is the front matter of a case's User file.
type userFile struct {
	Answers []UserAnswer `yaml:"answers,omitempty"`
}

// CaseUniverse sets up the case's universe.
type CaseUniverse struct {
	// Sidecars are the members, by name; their binaries are copied from the
	// bin folder into the case's runtime folder.
	Sidecars []string `yaml:"sidecars" json:"sidecars"`
	// Copy puts files into the universe at install, in order: config files
	// (TaskGroups.yaml, LLMProfiles.yaml, …), a .env with the keys, tasks.
	Copy  []Copy `yaml:"copy,omitempty" json:"copy,omitempty"`
	Color string `yaml:"color,omitempty" json:"color,omitempty"`
	// DisplayName and Icon are the universe's when the case is published as a
	// solution (default: the case's name); the case's own sandbox universe is
	// « Scenario <case> ».
	DisplayName string `yaml:"display_name,omitempty" json:"displayName,omitempty"`
	Icon        string `yaml:"icon,omitempty" json:"icon,omitempty"`
	// GUI is what the universe's GUI hides and lists first when the case is
	// published as a solution (Univers.yaml `gui:`, written at its install);
	// absent, the solution keeps what the case's universe declares.
	GUI *cenv.UniverseGUI `yaml:"gui,omitempty" json:"gui,omitempty"`
	// Tags are the universe's tags when the case is published as a solution
	// (Univers.yaml `tags:`); absent, the solution keeps the case universe's.
	Tags []string `yaml:"tags,omitempty" json:"tags,omitempty"`
	// FirstGuide is the guide the universe starts with when the case is
	// published as a solution (Univers.yaml `first_guide:`): a guide-recipe
	// URI of one of its members; absent, the solution keeps the case
	// universe's.
	FirstGuide string `yaml:"first_guide,omitempty" json:"firstGuide,omitempty"`
	// Solution is the solution the case's universe stands for, written into
	// its Univers.yaml (`solution:`) as a solution's install writes it: a
	// project whose type needs that solution (a dossier d'architecture needs
	// archifactory) opens in the case's universe as it does in an
	// installed one. Absent, the universe has none.
	Solution string `yaml:"solution,omitempty" json:"solution,omitempty"`
	// NoWelcome creates the case's universe with its welcome screens off
	// (Univers.yaml no_welcome): its GUI opens neither its first guide nor
	// « Quoi de neuf » by itself, so the case's tests and captures start clean.
	NoWelcome bool `yaml:"no_welcome,omitempty" json:"noWelcome,omitempty"`
}

// CaseOtherUniverse is another universe of the case's solution: its name in
// the solution, its members and copies as the case's universe has them, its
// project folder (published as its seed) and its display.
type CaseOtherUniverse struct {
	Name     string   `yaml:"name" json:"name"`
	Sidecars []string `yaml:"sidecars" json:"sidecars"`
	Copy     []Copy   `yaml:"copy,omitempty" json:"copy,omitempty"`
	// Project is its project: `from`, a folder (case-relative), published as
	// the universe's seed with its own tasks. Nothing runs in it, so the seed
	// is that folder as it is.
	Project     CaseProject       `yaml:"project,omitempty" json:"project,omitempty"`
	Color       string            `yaml:"color,omitempty" json:"color,omitempty"`
	DisplayName string            `yaml:"display_name,omitempty" json:"displayName,omitempty"`
	Icon        string            `yaml:"icon,omitempty" json:"icon,omitempty"`
	GUI         *cenv.UniverseGUI `yaml:"gui,omitempty" json:"gui,omitempty"`
	Tags        []string          `yaml:"tags,omitempty" json:"tags,omitempty"`
}

// Copy is one file or one folder copied into the universe at install.
type Copy struct {
	// From is the source: relative to the case folder, or absolute, or under
	// the home folder (`~/…`) — e.g. a .env the case must not carry.
	From string `yaml:"from" json:"from"`
	// To is the destination, relative to the universe's home (its config root:
	// `config/`, `.env`, `tasks/`); empty is that home. A file copied to a path
	// ending in `/` keeps its name.
	To string `yaml:"to,omitempty" json:"to,omitempty"`
	// Select keeps, from a folder, the files matching one of these globs (on
	// the path relative to From, or on the file name for a pattern without a
	// `/`); none keeps all. Exclude then drops the files matching one of its.
	Select  []string `yaml:"select,omitempty" json:"select,omitempty"`
	Exclude []string `yaml:"exclude,omitempty" json:"exclude,omitempty"`
	// Optional skips a source that does not exist (a .env present on one
	// machine only) instead of failing the install.
	Optional bool `yaml:"optional,omitempty" json:"optional,omitempty"`
}

// CaseProject sets up the project.
type CaseProject struct {
	// From is the folder (case-relative) copied as the project's initial content.
	From string `yaml:"from,omitempty" json:"from,omitempty"`
	// Live is a live project's folder (absolute, `~/`, or case-relative)
	// whose DATA the case's project takes: its project.cwm.yaml and its modules
	// folder, never its code,.katyoti/ or.git/. The case's own
	// hub serves the copy; the live project is only read. `refresh-project`
	// takes it again, before a migration is tried.
	Live string `yaml:"live,omitempty" json:"live,omitempty"`
	// Type and Title are the project object's when the project carries none:
	// written as the project's seed (cwmapi.ProjectSeed, in its store) at the
	// install, so CWM mints the project object of that type, with its own
	// identity, at the first opening - and published with the
	// case's seed, so a solution's install gets the same.
	// Empty: the default project type.
	Type  string `yaml:"type,omitempty" json:"type,omitempty"`
	Title string `yaml:"title,omitempty" json:"title,omitempty"`
	// Name is the project object's id when the project carries none, written
	// in its seed beside Type and Title: the key the universe's members
	// address it by (cwmapi.ProjectKeyOf). Empty: the folder's name.
	Name string `yaml:"name,omitempty" json:"name,omitempty"`
	// Demo: the project is demo data - its seed is published marked so, a
	// « Démo » badge in the Marketplace.
	Demo bool `yaml:"demo,omitempty" json:"demo,omitempty"`
	// Git makes the project a git repository, committed after each stage —
	// what changed between two stages is a diff.
	Git bool `yaml:"git,omitempty" json:"git,omitempty"`
}

// CaseRepository is another repository the case's universe serves beside its
// project: its folder repositories/<Name> in the case's runtime, its content
// copied From a case-relative folder, its project object seeded with Name as
// its id (its key among the members), Type and Title when it carries none,
// a git repository committed after each stage when Git says so.
type CaseRepository struct {
	Name  string `yaml:"name" json:"name"`
	From  string `yaml:"from,omitempty" json:"from,omitempty"`
	Type  string `yaml:"type,omitempty" json:"type,omitempty"`
	Title string `yaml:"title,omitempty" json:"title,omitempty"`
	Git   bool   `yaml:"git,omitempty" json:"git,omitempty"`
}

// Stage is one step of the scenario: what to do, then what must hold.
type Stage struct {
	ID          string `yaml:"id" json:"id"`
	Title       string `yaml:"title,omitempty" json:"title,omitempty"`
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
	// TimeoutMin bounds the stage (default 30).
	TimeoutMin int `yaml:"timeout_min,omitempty" json:"timeout_min,omitempty"`
	// MaxAgentRuns bounds the agent runs the stage may start (the simulated
	// user's included); 0 = no bound. The spend guard of a live stage.
	MaxAgentRuns int     `yaml:"max_agent_runs,omitempty" json:"max_agent_runs,omitempty"`
	Steps        []Step  `yaml:"steps,omitempty" json:"steps,omitempty"`
	Expect       []Check `yaml:"expect,omitempty" json:"expect,omitempty"`
}

// Step is one thing a stage does. Exactly one of its kinds is set.
type Step struct {
	Note string `yaml:"note,omitempty" json:"note,omitempty"`
	// Call calls a method of a sidecar through the hub: `<provider>:<slot>/<Method>`.
	Call *CallStep `yaml:"call,omitempty" json:"call,omitempty"`
	// Action runs an action URI through the ActionService, as the GUI does.
	Action *CallStep `yaml:"action,omitempty" json:"action,omitempty"`
	// AnswerInbox lets the simulated user answer the Inbox until its check holds.
	AnswerInbox *AnswerInboxStep `yaml:"answer_inbox,omitempty" json:"answer_inbox,omitempty"`
	// StartTask starts a task attended, as the GUI's chat does
	// (Terminal.StartTask, context _attended and _target): its ask-user-data
	// forms are answered by answer_form steps.
	StartTask *StartTaskStep `yaml:"start_task,omitempty" json:"start_task,omitempty"`
	// AnswerForm submits the form a started task shows, as the GUI does
	// (Terminal.Cmd ui_event submit).
	AnswerForm *AnswerFormStep `yaml:"answer_form,omitempty" json:"answer_form,omitempty"`
	// Wait waits until a check holds.
	Wait *WaitStep `yaml:"wait,omitempty" json:"wait,omitempty"`
	// Run runs a command in the project.
	Run *RunStep `yaml:"run,omitempty" json:"run,omitempty"`
	// Find saves the first element of a listing that matches, for later steps.
	Find *FindStep `yaml:"find,omitempty" json:"find,omitempty"`
	// Each runs its steps once per element of a listing (`${item.<path>}`).
	Each *EachStep `yaml:"each,omitempty" json:"each,omitempty"`
	// Open names a GUI element by its address (`object//links`
	// the case's universe when it names none): checked at load like a guide
	// target; a headless run records it, a GUI-driven one opens it.
	Open string `yaml:"open,omitempty" json:"open,omitempty"`
}

// FindStep calls a method, takes the list at Items in its answer (a dot path),
// and saves the first element whose fields contain Where's values
// (case-insensitive) under Save. None is a failure.
type FindStep struct {
	URI   string            `yaml:"uri" json:"uri"`
	Input any               `yaml:"input,omitempty" json:"input,omitempty"`
	Items string            `yaml:"items" json:"items"`
	Where map[string]string `yaml:"where,omitempty" json:"where,omitempty"`
	Save  string            `yaml:"save" json:"save"`
}

// EachStep calls a method, takes the list at Items in its answer, keeps the
// elements matching Where, and runs Steps once per element; their inputs read
// the element as `${item.<path>}`. An empty list runs nothing.
type EachStep struct {
	URI   string            `yaml:"uri" json:"uri"`
	Input any               `yaml:"input,omitempty" json:"input,omitempty"`
	Items string            `yaml:"items" json:"items"`
	Where map[string]string `yaml:"where,omitempty" json:"where,omitempty"`
	Steps []Step            `yaml:"steps" json:"steps"`
}

// CallStep calls a method or runs an action. Strings of Input may hold
// `${saved.<name>.<path>}` (a value an earlier step saved) and `${case.<field>}`
// (name, universe, project).
type CallStep struct {
	URI   string `yaml:"uri" json:"uri"`
	Input any    `yaml:"input,omitempty" json:"input,omitempty"`
	// Save keeps the answer under this name, for later steps (and stages:
	// saved values are part of the checkpoint).
	Save string `yaml:"save,omitempty" json:"save,omitempty"`
	// MayFail lets the call be refused without failing the stage — a move a
	// gate is expected to refuse first (its refusal asks a person; a later step
	// waits for the outcome). The refusal is reported.
	MayFail bool `yaml:"may_fail,omitempty" json:"may_fail,omitempty"`
}

// AnswerInboxStep: the simulated user answers each pending Inbox request
// until Until holds, the stage times out, or Max answers are given.
type AnswerInboxStep struct {
	Until      *Check `yaml:"until,omitempty" json:"until,omitempty"`
	TimeoutMin int    `yaml:"timeout_min,omitempty" json:"timeout_min,omitempty"`
	Max        int    `yaml:"max,omitempty" json:"max,omitempty"`
}

// StartTaskStep starts Task attended on Target (an object id, optional) with
// Input, as the GUI's chat does; Save keeps its answer ({runID, sessionID})
// for the answer_form steps after it (default "task").
type StartTaskStep struct {
	Task   string `yaml:"task" json:"task"`
	Target string `yaml:"target,omitempty" json:"target,omitempty"`
	Input  any    `yaml:"input,omitempty" json:"input,omitempty"`
	Save   string `yaml:"save,omitempty" json:"save,omitempty"`
}

// AnswerFormStep waits until the task a start_task step started (Task, its
// save name; default "task") shows a form not yet answered - one whose
// question or title holds Question, when given - and submits Value to it, as
// the GUI does; then waits until the run took the answer.
type AnswerFormStep struct {
	Task       string `yaml:"task,omitempty" json:"task,omitempty"`
	Question   string `yaml:"question,omitempty" json:"question,omitempty"`
	Value      any    `yaml:"value" json:"value"`
	TimeoutMin int    `yaml:"timeout_min,omitempty" json:"timeout_min,omitempty"`
}

// WaitStep waits until a check holds.
type WaitStep struct {
	Until      Check `yaml:"until" json:"until"`
	TimeoutMin int   `yaml:"timeout_min,omitempty" json:"timeout_min,omitempty"`
}

// RunStep runs a command in the project (Dir project-relative).
type RunStep struct {
	Cmd  string `yaml:"cmd" json:"cmd"`
	Dir  string `yaml:"dir,omitempty" json:"dir,omitempty"`
	Exit *int   `yaml:"exit,omitempty" json:"exit,omitempty"`
}

// The kinds of clean machine a case runs on.
const (
	// MachineWindowsSandbox is a fresh Windows Sandbox: nothing installed,
	// the channel's marketplace mapped read-only.
	MachineWindowsSandbox = "windows-sandbox"
	// MachineLocal is this machine, closed in a world of its own (its home,
	// its installation folder): the same stages, a dry run where the clean
	// machine installs programs for real.
	MachineLocal = "local"
)

// The preparations a Windows Sandbox can be given, as a desktop has them:
// winget (App Installer), which Windows 10/11 ships; the Microsoft Visual C++
// 2015-2022 runtime, which a desktop usually has (other software installs
// it) and only an administrator installs - it needs winget, prepared first.
const (
	MachinePrepareWinget   = "winget"
	MachinePrepareVCRedist = "vcredist"
	// MachinePrepareBash is Git for Windows (its bash), through winget: the
	// case stages' command checks run in bash, as on any host.
	MachinePrepareBash = "bash"
)

// CaseMachine runs the case on a clean machine: from the installer's
// double-click to its last stage, each stage timed.
type CaseMachine struct {
	// Kind is windows-sandbox or local (default windows-sandbox).
	Kind string `yaml:"kind,omitempty" json:"kind,omitempty"`
	// Channel is the infra channel folder the installer is taken from
	// (<marketplace>/infra): case-relative or absolute; `--channel` overrides
	// it. Its marketplace (the folder above) is what the machine is given.
	Channel string `yaml:"channel,omitempty" json:"channel,omitempty"`
	// TimeoutMin bounds the whole run (default 120).
	TimeoutMin int `yaml:"timeout_min,omitempty" json:"timeout_min,omitempty"`
	// PassEnv are the variables of the machine running the tool passed to the
	// stages (a test vault's password): in a Windows Sandbox they are written
	// into its plan, so only a test secret belongs here.
	PassEnv []string `yaml:"pass_env,omitempty" json:"pass_env,omitempty"`
	// Prepare is what a Windows Sandbox is given before the stages, as a
	// person's Windows 10/11 desktop has it: winget, then vcredist (the VC++
	// runtime). Absent, a Sandbox is prepared with both; `prepare: []` gives
	// it nothing. The local kind is this machine: it is never prepared.
	Prepare *[]string `yaml:"prepare,omitempty" json:"prepare,omitempty"`
	// Stages run in order; the first that fails stops the run.
	Stages []MachineStage `yaml:"stages" json:"stages"`
}

// MachineStage is one timed stage on the clean machine: the installer
// double-clicked (Install), or one command of the installed programs.
// Command arguments, Contains and Excludes read ${install} (the installation
// folder), ${katyoti} (its katyoti), ${bin:<name>} (an installed sidecar),
// ${universe} (its default universe), ${solution} (the solution it was
// installed from), ${channel}, ${home}, ${documents}, ${results} (the run's
// results folder, where a stage saves what it shows) and ${env:<VAR>}.
type MachineStage struct {
	ID    string `yaml:"id" json:"id"`
	Title string `yaml:"title,omitempty" json:"title,omitempty"`
	// TimeoutMin bounds the stage (default 30).
	TimeoutMin int `yaml:"timeout_min,omitempty" json:"timeout_min,omitempty"`
	// Install double-clicks the channel's installer (install-on-windows.cmd,
	// install.sh elsewhere), headless: setup does not open the GUI.
	Install bool `yaml:"install,omitempty" json:"install,omitempty"`
	// Cmd is the program and its arguments (no shell).
	Cmd []string `yaml:"cmd,omitempty" json:"cmd,omitempty"`
	// GUI drives the GUI a previous stage started, by its element ids.
	GUI *GUIStage `yaml:"gui,omitempty" json:"gui,omitempty"`
	// Case runs stages of another case - its steps and checks, the case
	// grammar - on the installed universe, as a scenario universe runs them
	// : one definition for both.
	Case *MachineCase `yaml:"case,omitempty" json:"case,omitempty"`
	// LocalCmd runs instead of Cmd on the local kind: a dry run where the
	// clean machine installs for real.
	LocalCmd []string `yaml:"local_cmd,omitempty" json:"local_cmd,omitempty"`
	// Env is added to the stage's environment.
	Env map[string]string `yaml:"env,omitempty" json:"env,omitempty"`
	// Exit is the exit code expected (default 0).
	Exit *int `yaml:"exit,omitempty" json:"exit,omitempty"`
	// Contains must all be in the output; Excludes none of it.
	Contains []string `yaml:"contains,omitempty" json:"contains,omitempty"`
	Excludes []string `yaml:"excludes,omitempty" json:"excludes,omitempty"`
	// Images are the PNG files the stage saves (a screenshot): each must
	// decode and show something - not one flat colour - whatever the output
	// says.
	Images []string `yaml:"images,omitempty" json:"images,omitempty"`
}

// MachineCase is a machine stage that runs another case's stages on the
// installed universe: its hub reached through the installation's data, its
// project the one the universe serves, its Inbox answered by that case's
// simulated user.
type MachineCase struct {
	// Name is the case's folder, beside this case's.
	Name string `yaml:"name" json:"name"`
	// Stages are the ids run, in that case's order; none: all of them.
	Stages []string `yaml:"stages,omitempty" json:"stages,omitempty"`
	// Universe is the installed universe they run on; empty: the
	// installation's default universe.
	Universe string `yaml:"universe,omitempty" json:"universe,omitempty"`
	// Resolved are the stages themselves, and User that case's user file:
	// read on the machine running the tool when the plan is made, carried in
	// the plan (a clean machine sees only its tools).
	Resolved []Stage `yaml:"-" json:"resolved,omitempty"`
	User     string  `yaml:"-" json:"user,omitempty"`
}

// GUIDebugPort is the GUI's remote debugging port a gui stage attaches to
// when it names none.
const GUIDebugPort = 9222

// GUIStage drives the built GUI as a person does, by the element ids its
// screens carry (data-element), over the Chrome DevTools Protocol of its
// WebView2. The stage that starts the GUI opens the port, in the proof's
// environment only: KTW_GUI_DEBUG_PORT=<port>, which ktw-gui gives WebView2.
type GUIStage struct {
	// Port is the GUI's remote debugging port (default GUIDebugPort).
	Port  int       `yaml:"port,omitempty" json:"port,omitempty"`
	Steps []GUIStep `yaml:"steps" json:"steps"`
}

// GUIStep is one thing a gui stage does or waits for. Exactly one of Title,
// Click, Type, Wait, Shot is set.
type GUIStep struct {
	// Title waits for the page's title to contain it.
	Title string `yaml:"title,omitempty" json:"title,omitempty"`
	// Click clicks the element of that id.
	Click string `yaml:"click,omitempty" json:"click,omitempty"`
	// Type types Text into the element of that id - after the text it holds,
	// or in its place with Clear (a field the screen fills in ahead).
	Type  string `yaml:"type,omitempty" json:"type,omitempty"`
	Text  string `yaml:"text,omitempty" json:"text,omitempty"`
	Clear bool   `yaml:"clear,omitempty" json:"clear,omitempty"`
	// Wait waits for the element of that id - with State, for its
	// data-state; with Contains, for its text.
	Wait     string `yaml:"wait,omitempty" json:"wait,omitempty"`
	State    string `yaml:"state,omitempty" json:"state,omitempty"`
	Contains string `yaml:"contains,omitempty" json:"contains,omitempty"`
	// Shot saves the page as a PNG at that path; it is checked like the
	// stage's images.
	Shot string `yaml:"shot,omitempty" json:"shot,omitempty"`
	// TimeoutSec bounds the step (default 60).
	TimeoutSec int `yaml:"timeout_sec,omitempty" json:"timeout_sec,omitempty"`
}

// Kind is the step's one action, or an error naming what is wrong.
func (s GUIStep) Kind() (string, error) {
	var kinds []string
	for k, v := range map[string]string{"title": s.Title, "click": s.Click, "type": s.Type, "wait": s.Wait, "shot": s.Shot} {
		if v != "" {
			kinds = append(kinds, k)
		}
	}
	switch {
	case len(kinds) != 1:
		return "", fmt.Errorf("exactly one of title, click, type, wait, shot")
	case kinds[0] != "wait" && (s.State != "" || s.Contains != ""):
		return "", fmt.Errorf("state and contains go with wait")
	case (kinds[0] == "type") != (s.Text != ""):
		return "", fmt.Errorf("text goes with type, and type needs it")
	}
	return kinds[0], nil
}

// Check is one thing that must hold. Exactly one of its kinds is set.
type Check struct {
	Name string `yaml:"name,omitempty" json:"name,omitempty"`
	// Objects counts the CWM objects matching (type, module, a name substring,
	// a lifecycle state).
	Objects *ObjectsCheck `yaml:"objects,omitempty" json:"objects,omitempty"`
	// Plans counts the plans in a state (planification).
	Plans *PlansCheck `yaml:"plans,omitempty" json:"plans,omitempty"`
	// File is a project-relative path that must exist.
	File string `yaml:"file,omitempty" json:"file,omitempty"`
	// Cmd runs a command in the project; it must exit with Exit (default 0).
	Cmd *RunStep `yaml:"cmd,omitempty" json:"cmd,omitempty"`
	// Call calls a method; its answer must contain Contains.
	Call *CallCheck `yaml:"call,omitempty" json:"call,omitempty"`
}

// ObjectsCheck counts matching objects: at least Min (default 1), at most Max.
type ObjectsCheck struct {
	Type   string `yaml:"type,omitempty" json:"type,omitempty"`
	Module string `yaml:"module,omitempty" json:"module,omitempty"`
	Name   string `yaml:"name,omitempty" json:"name,omitempty"`
	State  string `yaml:"state,omitempty" json:"state,omitempty"`
	Min    *int   `yaml:"min,omitempty" json:"min,omitempty"`
	Max    *int   `yaml:"max,omitempty" json:"max,omitempty"`
}

// PlansCheck counts plans in a state: at least Min (default 1), at most Max.
type PlansCheck struct {
	Module string `yaml:"module,omitempty" json:"module,omitempty"`
	State  string `yaml:"state,omitempty" json:"state,omitempty"`
	Min    *int   `yaml:"min,omitempty" json:"min,omitempty"`
	Max    *int   `yaml:"max,omitempty" json:"max,omitempty"`
}

// CallCheck calls a method; the answer must contain Contains and, when
// Excludes is set, must not contain it (a member that must be absent). With
// Refused the call must be refused instead, and its refusal is what Contains
// and Excludes read (an ambiguous bare id refused naming its candidates).
type CallCheck struct {
	URI      string `yaml:"uri" json:"uri"`
	Input    any    `yaml:"input,omitempty" json:"input,omitempty"`
	Contains string `yaml:"contains" json:"contains"`
	Excludes string `yaml:"excludes,omitempty" json:"excludes,omitempty"`
	Refused  bool   `yaml:"refused,omitempty" json:"refused,omitempty"`
}

// Holds says whether a call's answer satisfies the check.
func (c CallCheck) Holds(answer string) bool {
	return strings.Contains(answer, c.Contains) && (c.Excludes == "" || !strings.Contains(answer, c.Excludes))
}

// Describe is a check in words, for reports.
func (c Check) Describe() string {
	if c.Name != "" {
		return c.Name
	}
	switch {
	case c.Objects != nil:
		return fmt.Sprintf("objects type=%s module=%s name~%q state=%s", c.Objects.Type, c.Objects.Module, c.Objects.Name, c.Objects.State)
	case c.Plans != nil:
		return fmt.Sprintf("plans module=%s state=%s", c.Plans.Module, c.Plans.State)
	case c.File != "":
		return "file " + c.File
	case c.Cmd != nil:
		return "command " + c.Cmd.Cmd
	case c.Call != nil:
		what := "call " + c.Call.URI
		if c.Call.Refused {
			what += " refused"
		}
		if c.Call.Excludes != "" {
			return what + " contains " + c.Call.Contains + " excludes " + c.Call.Excludes
		}
		return what + " contains " + c.Call.Contains
	}
	return "(empty check)"
}

// Load reads a case folder's test-case.yaml and checks it.
func Load(dir string) (*Case, error) {
	raw, err := os.ReadFile(filepath.Join(dir, CaseFile))
	if err != nil {
		return nil, err
	}
	var c Case
	if err := config.DecodeBytes(raw, marshal.YAML, &c); err != nil {
		return nil, fmt.Errorf("%s: %w", CaseFile, err)
	}
	c.Dir = dir
	if err := c.loadUser(); err != nil {
		return nil, fmt.Errorf("%s: %w", filepath.Join(dir, c.User), err)
	}
	if err := c.check(); err != nil {
		return nil, fmt.Errorf("%s: %w", filepath.Join(dir, CaseFile), err)
	}
	return &c, nil
}

// loadUser reads the scripted answers from the User file's front matter; a
// case without a User file, or one without answers, has none.
func (c *Case) loadUser() error {
	if c.User == "" {
		return nil
	}
	raw, err := os.ReadFile(filepath.Join(c.Dir, c.User))
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var u userFile
	if err := config.DecodeBytes(raw, marshal.Frontmatter, &u); err != nil {
		return err
	}
	for i, a := range u.Answers {
		if strings.TrimSpace(a.Decision) == "" {
			return fmt.Errorf("answers %d: a decision (approved, rejected, or the person's word)", i+1)
		}
		if strings.TrimSpace(a.Decision) == "rejected" && strings.TrimSpace(a.Note) == "" {
			return fmt.Errorf("answers %d: a rejection says what to change (a note): the inbox refuses one without", i+1)
		}
	}
	c.Answers = u.Answers
	return nil
}

func (c *Case) check() error {
	if c.Name == "" || strings.ContainsAny(c.Name, ` /\:`) {
		return fmt.Errorf("name %q: a case needs a name without spaces or slashes", c.Name)
	}
	if err := c.Machine.check(); err != nil {
		return err
	}
	// A case that runs only on a clean machine has no universe of its own.
	if len(c.Universe.Sidecars) == 0 && (c.Machine == nil || len(c.Stages) > 0) {
		return fmt.Errorf("universe.sidecars: at least one member (ktw-agents)")
	}
	if err := checkCopies(c.Universe.Copy, "universe.copy"); err != nil {
		return err
	}
	others := map[string]bool{}
	for i, o := range c.OtherUniverses {
		at := fmt.Sprintf("other_universes %d", i+1)
		switch {
		case o.Name == "" || strings.ContainsAny(o.Name, ` /\:`) || o.Name == "." || o.Name == "..":
			return fmt.Errorf("%s: a name without spaces or slashes", at)
		case others[strings.ToLower(o.Name)]:
			return fmt.Errorf("%s: %q is named twice", at, o.Name)
		case len(o.Sidecars) == 0:
			return fmt.Errorf("%s (%s): sidecars: at least one member", at, o.Name)
		case o.Project.Git:
			return fmt.Errorf("%s (%s): project.git: nothing runs in another universe, its project is not a repository", at, o.Name)
		}
		others[strings.ToLower(o.Name)] = true
		if err := checkCopies(o.Copy, at+".copy"); err != nil {
			return err
		}
	}
	if err := c.checkRepositories(); err != nil {
		return err
	}
	seen := map[string]bool{}
	for i, s := range c.Stages {
		if s.ID == "" || seen[s.ID] {
			return fmt.Errorf("stage %d: an id, unique in the case", i+1)
		}
		seen[s.ID] = true
		if err := checkSteps(s.Steps, "stage "+s.ID); err != nil {
			return err
		}
	}
	return nil
}

// checkRepositories refuses a repository whose name is no project key -
// lowercase letters, digits and dashes, as a project's name - one named
// twice, or one named as the project's own key.
func (c *Case) checkRepositories() error {
	names := map[string]bool{}
	if n := strings.TrimSpace(c.Project.Name); n != "" {
		names[n] = true
	}
	for i, r := range c.Repositories {
		at := fmt.Sprintf("repositories %d", i+1)
		switch {
		case !isKey(r.Name):
			return fmt.Errorf("%s: name %q: lowercase letters, digits and dashes - its folder and its key among the universe's members", at, r.Name)
		case names[r.Name]:
			return fmt.Errorf("%s: %q is the key of another project of the case: two members of one key would be one project served twice", at, r.Name)
		}
		names[r.Name] = true
	}
	return nil
}

// isKey says whether s is a project key a case may name: lowercase letters,
// digits and dashes, starting with a letter or a digit.
func isKey(s string) bool {
	if s == "" || s[0] == '-' {
		return false
	}
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-') {
			return false
		}
	}
	return true
}

// check refuses a machine of an unknown kind, a stage with no id or one
// named twice, a stage that is neither the installer nor a command.
func (m *CaseMachine) check() error {
	if m == nil {
		return nil
	}
	if m.Kind != "" && m.Kind != MachineWindowsSandbox && m.Kind != MachineLocal {
		return fmt.Errorf("machine.kind %q: %s or %s", m.Kind, MachineWindowsSandbox, MachineLocal)
	}
	if len(m.Stages) == 0 {
		return fmt.Errorf("machine.stages: at least one (the installer)")
	}
	if m.Prepare != nil {
		winget := false
		for _, p := range *m.Prepare {
			switch p {
			case MachinePrepareWinget:
				winget = true
			case MachinePrepareVCRedist, MachinePrepareBash:
				if !winget {
					return fmt.Errorf("machine.prepare: %s is installed with winget - list %s before it", p, MachinePrepareWinget)
				}
			default:
				return fmt.Errorf("machine.prepare %q: %s, %s or %s", p, MachinePrepareWinget, MachinePrepareVCRedist, MachinePrepareBash)
			}
		}
	}
	seen := map[string]bool{}
	for i, s := range m.Stages {
		at := fmt.Sprintf("machine.stages %d", i+1)
		switch {
		case s.ID == "" || seen[s.ID]:
			return fmt.Errorf("%s: an id, unique in the machine's stages", at)
		case btoi(s.Install)+btoi(len(s.Cmd) > 0)+btoi(s.GUI != nil)+btoi(s.Case != nil) != 1:
			return fmt.Errorf("%s (%s): exactly one of install, cmd, gui, case", at, s.ID)
		case s.Case != nil && strings.TrimSpace(s.Case.Name) == "":
			return fmt.Errorf("%s (%s): case.name: the case whose stages it runs", at, s.ID)
		case len(s.LocalCmd) > 0 && len(s.Cmd) == 0:
			return fmt.Errorf("%s (%s): local_cmd replaces a cmd, the stage has none", at, s.ID)
		case s.GUI != nil && len(s.GUI.Steps) == 0:
			return fmt.Errorf("%s (%s): gui.steps: at least one", at, s.ID)
		}
		if s.GUI != nil {
			for j, st := range s.GUI.Steps {
				if _, err := st.Kind(); err != nil {
					return fmt.Errorf("%s (%s): gui.steps %d: %w", at, s.ID, j+1, err)
				}
			}
		}
		seen[s.ID] = true
	}
	return nil
}

// checkCopies refuses a copy with no source, one whose destination leaves the
// universe, a bad glob.
func checkCopies(copies []Copy, where string) error {
	for i, cp := range copies {
		to := filepath.ToSlash(cp.To)
		switch {
		case cp.From == "":
			return fmt.Errorf("%s %d: a from", where, i+1)
		case filepath.IsAbs(cp.To) || strings.HasPrefix(to, "/") || slices.Contains(strings.Split(to, "/"), ".."):
			return fmt.Errorf("%s %d: to %q must stay inside the universe (relative, no ..)", where, i+1, cp.To)
		}
		for _, g := range append(append([]string{}, cp.Select...), cp.Exclude...) {
			if _, err := path.Match(g, ""); err != nil {
				return fmt.Errorf("%s %d: glob %q: %w", where, i+1, g, err)
			}
		}
	}
	return nil
}

// checkSteps refuses a step with no kind or two, a find or each that names no
// list, a find that saves nothing — and checks an each's steps the same way.
func checkSteps(steps []Step, where string) error {
	for j, st := range steps {
		n := 0
		for _, set := range []bool{st.Call != nil, st.Action != nil, st.AnswerInbox != nil, st.StartTask != nil, st.AnswerForm != nil, st.Wait != nil, st.Run != nil, st.Find != nil, st.Each != nil, st.Open != ""} {
			if set {
				n++
			}
		}
		at := fmt.Sprintf("%s, step %d", where, j+1)
		switch {
		case n != 1:
			return fmt.Errorf("%s: exactly one of call, action, answer_inbox, start_task, answer_form, wait, run, find, each, open", at)
		case st.StartTask != nil && st.StartTask.Task == "":
			return fmt.Errorf("%s: start_task needs task", at)
		case st.AnswerForm != nil && st.AnswerForm.Value == nil:
			return fmt.Errorf("%s: answer_form needs value", at)
		case st.Find != nil && (st.Find.URI == "" || st.Find.Items == "" || st.Find.Save == ""):
			return fmt.Errorf("%s: find needs uri, items and save", at)
		case st.Each != nil && (st.Each.URI == "" || st.Each.Items == "" || len(st.Each.Steps) == 0):
			return fmt.Errorf("%s: each needs uri, items and steps", at)
		}
		if st.Open != "" {
			// The universe is the case's when the address names none.
			if _, err := ParseAddress(WithUniverse(st.Open, "case")); err != nil {
				return fmt.Errorf("%s: open: %w", at, err)
			}
		}
		if st.Each != nil {
			if err := checkSteps(st.Each.Steps, at+" (each)"); err != nil {
				return err
			}
		}
	}
	return nil
}

// UniverseName is the case's universe.
func (c *Case) UniverseName() string { return "scn-" + c.Name }

// OtherUniverseName is the registered name of one of the case's other
// universes.
func (c *Case) OtherUniverseName(name string) string { return c.UniverseName() + "-" + name }

// StageIndex is a stage's position; -1 when unknown.
func (c *Case) StageIndex(id string) int {
	for i, s := range c.Stages {
		if s.ID == id {
			return i
		}
	}
	return -1
}

func btoi(b bool) int {
	if b {
		return 1
	}
	return 0
}
