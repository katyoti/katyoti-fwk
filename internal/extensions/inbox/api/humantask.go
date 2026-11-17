// SPDX-License-Identifier: Apache-2.0

package api

import (
	"time"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// THE HUMAN TASK - a person's piece of work is a task like any
// other: a run of the `human-task` machine strategy, which any asker starts -
// a step, a workflow, a gate, an agent, a tool, a connector. Its record and
// its claim are one scheduler to-do for a person, owned by the inbox: the
// person answers it the way every person's step is answered (todos report,
// with the proof the scheduler and iam require), whatever the channel; the
// run waits durably for it and its result is the answer.

// HumanTaskStrategy is the machine strategy a human task runs on.
const HumanTaskStrategy = "human-task"

// Owner is the scheduler owner of a human task's to-do: the inbox collects
// its answer.
const Owner = "inbox"

// WorkAsk is the key of a human task's to-do work holding what it asks.
const WorkAsk = "ask"

// WorkLabel is the key of a human task's to-do work holding its kind's short
// label: what every channel shows first.
const WorkLabel = "label"

// THE KINDS - every question says first, in one
// short label, what is expected of the person; a human task without a known
// kind is refused. Kinds of questions are contributions of the `ask-kind`
// kind (AskKind): the inbox pack ships the nine built-in ones
// (contrib/embed/ask-kinds), and any pack adds its own. Which kinds exist is
// read from the catalog, never a list in code; the constants below are the
// built-in names askers use.
const (
	KindApprove    = "approve"
	KindChoose     = "choose"
	KindDecide     = "decide"
	KindProvide    = "provide"
	KindForm       = "form"
	KindTestByHand = "test-by-hand"
	KindPermission = "permission"
	KindUnblock    = "unblock"
	KindAccept     = "accept"
)

// ContributionTypeAskKind is the contribution kind of a kind of question.
const ContributionTypeAskKind = "ask-kind"

// AskKind is an `ask-kind` entry: one kind of question - what is expected of
// the person, its short label, the shape of its answer, who may answer it by
// default, the permission code it requires by default and what its answer
// settles by default. A human task of that kind takes these defaults unless
// it says otherwise.
type AskKind struct {
	Name         string   `json:"name" yaml:"name" jsonschema:"description=The kind's name as a human task names it (kebab): approve or test-by-hand or a pack's own,required"`
	Label        string   `json:"label" yaml:"label" jsonschema:"description=The short label every card and list and channel shows first: Validation or Test by hand,required"`
	Description  string   `json:"description,omitempty" yaml:"description,omitempty" jsonschema:"description=What a person does to answer it"`
	Answer       string   `json:"answer,omitempty" yaml:"answer,omitempty" jsonschema:"description=The data-type of its answer (a URI) - its card variant draws the question"`
	AnswerableBy string   `json:"answerable_by,omitempty" yaml:"answerable_by,omitempty" jsonschema:"description=Who may answer by default: person or agent or any"`
	Requires     string   `json:"requires,omitempty" yaml:"requires,omitempty" jsonschema:"description=The permission code an answerer must hold by default - empty: <about type>:<kind>"`
	Satisfies    []string `json:"satisfies,omitempty" yaml:"satisfies,omitempty" jsonschema:"description=What its answer settles by default (gate requirements or verifications or steps)"`
	Order        int      `json:"order,omitempty" yaml:"order,omitempty" jsonschema:"description=Its place among the counts of a list (lower first)"`
}

// PermissionAnswer is the answer of a `permission` question - its kind's
// answer data-type (ktw-agents:/data-type/permission-answer): allow or deny,
// once or always, and the rule an « always » keeps. Its card reports the
// decision as the answer and the whole as the answer's data; the runtime
// reads both.
type PermissionAnswer struct {
	Decision string `json:"decision" yaml:"decision" jsonschema:"description=Allow or deny the act - once or always,enum=allow-once|allow-always|deny-once|deny-always,required"`
	Rule     string `json:"rule,omitempty" yaml:"rule,omitempty" jsonschema:"description=The rule an always keeps: one the question proposes or one of the person's own"`
}

// Permission decisions, as a PermissionAnswer names them.
const (
	PermissionAllowOnce   = "allow-once"
	PermissionAllowAlways = "allow-always"
	PermissionDenyOnce    = "deny-once"
	PermissionDenyAlways  = "deny-always"
)

// HumanTask is what a human task asks: who asks, of whom, about what, and what
// its answer settles.
type HumanTask struct {
	// Kind is what is expected of the person: approve, choose, decide,
	// provide, form, test-by-hand, permission, unblock, accept.
	Kind string `json:"kind,omitempty" jsonschema:"description=What is expected: approve or choose or decide or provide or form or test-by-hand or permission or unblock or accept"`
	// Title is the question in one line.
	Title string `json:"title" jsonschema:"description=The question in one line,required"`
	// Question is the whole question when one line is not enough.
	Question string `json:"question,omitempty" jsonschema:"description=The whole question (markdown) when one line is not enough"`
	// Options are the choices offered, kept as the asker gave them: a choose
	// question's options - its card offers them and its answer is one of
	// them -, or a permission's proposed rules - an « always » keeps one.
	Options []string `json:"options,omitempty" jsonschema:"description=The choices offered: a choose question's options or a permission's proposed rules"`
	// FormDef is a form question's FormDefinition (YAML): its card draws the
	// form and its answer is the form's data.
	FormDef string `json:"form_def,omitempty" jsonschema:"description=A form question's FormDefinition (YAML)"`
	// About is the object the question is about: « Ouvrir » goes there and
	// its answer is recorded on it. Its module holds the to-do.
	About string `json:"about,omitempty" jsonschema:"description=The object the question is about"`
	// Assignee is the one person (or agent) asked, when it is not a holding list.
	Assignee *cwmapi.Actor `json:"assignee,omitempty" jsonschema:"description=The one person or agent asked"`
	// Candidates is the holding list asked: its members may answer.
	Candidates string `json:"candidates,omitempty" jsonschema:"description=The holding list whose members may answer"`
	// Satisfies are what its answer settles: gate requirements, verifications, steps.
	Satisfies []string `json:"satisfies,omitempty" jsonschema:"description=What its answer settles: gate requirements and verifications and steps"`
	// Proof is the minimum proof of the person's act (prompt, totp, passkey, entra).
	Proof string `json:"proof,omitempty" jsonschema:"description=The minimum proof of the act: prompt or totp or passkey or entra"`
	// AnswerableBy says whether an agent member may answer: person, agent or any.
	AnswerableBy string `json:"answerable_by,omitempty" jsonschema:"description=Who may answer: person or agent or any"`
	// Requires is the permission code an answerer must hold.
	Requires string `json:"requires,omitempty" jsonschema:"description=The permission code an answerer must hold"`
	// AskedBy is the cause: the step, gate, workflow node, agent or tool that asks.
	AskedBy string `json:"asked_by,omitempty" jsonschema:"description=Who asks: a step or a gate or a workflow node or an agent or a tool"`
	// Chat is the run whose conversation asks it: its card is
	// shown inline in that chat - the same card as in « À faire », answered
	// once in either place.
	Chat string `json:"chat,omitempty" jsonschema:"description=The run whose conversation asks it: its card shows inline in that chat"`
	// Label is its kind's short label, set from the kind when it is asked.
	Label string `json:"label,omitempty" jsonschema:"description=The kind's short label (set from the kind when asked)"`
	// FollowUp is when it reminds and Due when it escalates:
	// RFC3339, or a duration from when it is asked ("48h").
	FollowUp string `json:"follow_up,omitempty" jsonschema:"description=When it reminds: RFC3339 or a duration from when it is asked (48h)"`
	Due      string `json:"due,omitempty" jsonschema:"description=By when it must be answered - past it it escalates: RFC3339 or a duration from when it is asked"`
	// Priority is P0 to P3; P2 when unset.
	Priority string `json:"priority,omitempty" jsonschema:"description=P0 to P3 (P2 when unset)"`
}

// HumanAnswer is a human task's result: the answer reported on its to-do.
type HumanAnswer struct {
	// Task is the human task's to-do.
	Task    string `json:"task"`
	Status  string `json:"status"`
	Verdict string `json:"verdict,omitempty"`
	Answer  string `json:"answer,omitempty"`
	Summary string `json:"summary,omitempty"`
	Details string `json:"details,omitempty"`
	// More is what a structured answer carries (a form's data, a choice).
	More map[string]any `json:"more,omitempty"`
	// By is who answered; Relayed when an agent's session relayed the person's answer.
	By      cwmapi.Actor      `json:"by"`
	At      string            `json:"at"`
	Relayed *schedapi.Relayed `json:"relayed,omitempty"`
	// List is the holding list it was answered from: an agent answers as one
	// of its members.
	List string `json:"list,omitempty"`
	// Run and Session are the agent's run and session that answered, when
	// its claim named them.
	Run     string `json:"run,omitempty"`
	Session string `json:"session,omitempty"`
}

// Who may answer a question: a person only (the default), an
// agent, or any member of its holding list.
const (
	AnswerablePerson = "person"
	AnswerableAgent  = "agent"
	AnswerableAny    = "any"
)

// OpenToAgents reports a question an agent member of its list may answer.
func OpenToAgents(answerableBy string) bool {
	return answerableBy == AnswerableAgent || answerableBy == AnswerableAny
}

// AnswerOf is a human task's to-do's answer, once it is reported.
func AnswerOf(t schedapi.Todo) (HumanAnswer, bool) {
	if t.Result == nil {
		return HumanAnswer{}, false
	}
	r := t.Result
	a := HumanAnswer{
		Task: t.ID, Status: r.Status, Verdict: r.Verdict, Answer: r.Answer, Summary: r.Summary,
		Details: r.Details, More: r.More, By: r.By, At: r.At, Relayed: r.Relayed, List: t.Group,
	}
	if t.Claim != nil && t.Claim.By.ID == r.By.ID && t.Claim.By.Kind == r.By.Kind {
		a.Run, a.Session = t.Claim.Run, t.Claim.Session
	}
	return a, true
}

// HOLDING LISTS - a question waits on a holding list: an object
// with an id and members, persons, roles or agents. It is the scheduler group
// of the human tasks it holds: only its members pull them
// a claim says who took one, and the first proven answer wins. A human task
// names its list (candidates); by default it waits on the project's list,
// ProjectList, whose members are everyone with access to the project. A list
// that does not exist is open: any worker of its kind pulls its to-dos.

// HoldingListType is the object type of a holding list, filed in sys-inbox.
const HoldingListType = "holding-list"

// ProjectList is the project's holding list: everyone with access to the
// project, the default list a human task waits on.
const ProjectList = "sys-inbox.project"

// HoldingListFields are a holding list's fields.
type HoldingListFields struct {
	Description string   `json:"description,omitempty" yaml:"description,omitempty" jsonschema:"description=Who the list is for and what it holds"`
	Members     []Member `json:"members,omitempty" yaml:"members,omitempty" jsonschema:"description=Who may answer its questions: persons and roles and agents"`
}

// Member is one member of a holding list: a person (their handle), an agent
// (a persona or a task, answering as a person would) or a role.
type Member struct {
	ID   string `json:"id" yaml:"id" jsonschema:"description=A person's handle or an agent's id or a role's name,required"`
	Kind string `json:"kind" yaml:"kind" jsonschema:"description=person or agent or role,required"`
}

// BY WHEN: the keys of a human task's to-do work holding its
// dates (RFC3339) and when it reminded and escalated.
const (
	WorkFollowUp  = "follow_up"
	WorkDue       = "due"
	WorkReminded  = "reminded"
	WorkEscalated = "escalated"
)

// Overdue is whether a human task's to-do is past its due date, unanswered.
func Overdue(t schedapi.Todo, now time.Time) bool {
	if t.Result != nil {
		return false
	}
	s, _ := t.Work[WorkDue].(string)
	due, err := time.Parse(time.RFC3339, s)
	return err == nil && !now.Before(due)
}

// ANSWERS KEPT: an answer is never lost - received, checked
// and stored on the object it is about before anything goes on; then its
// to-do is archived and the asker told. The stored answer is an `answer`
// record under that object (sys-inbox when it is about none or a system
// object), committed with it, so it outlives the scheduler's done to-dos and
// the journal's retention.

// AnswerType is the object type of a stored answer.
const AnswerType = "answer"

// AnswerRecord is a stored answer's fields: what was asked, what was
// answered, by whom, with what proof.
type AnswerRecord struct {
	Task      string         `json:"task" yaml:"task" jsonschema:"description=The human task's to-do it answered,required"`
	Kind      string         `json:"kind,omitempty" yaml:"kind,omitempty" jsonschema:"description=What was expected: the kind of question"`
	Question  string         `json:"question,omitempty" yaml:"question,omitempty" jsonschema:"description=The question in one line"`
	Satisfies []string       `json:"satisfies,omitempty" yaml:"satisfies,omitempty" jsonschema:"description=What the answer settled"`
	Status    string         `json:"status" yaml:"status" jsonschema:"description=success or fail or blocked,required"`
	Verdict   string         `json:"verdict,omitempty" yaml:"verdict,omitempty" jsonschema:"description=approved or rejected"`
	Answer    string         `json:"answer,omitempty" yaml:"answer,omitempty" jsonschema:"description=What was decided or provided"`
	Summary   string         `json:"summary,omitempty" yaml:"summary,omitempty" jsonschema:"description=The answer in one line"`
	Details   string         `json:"details,omitempty" yaml:"details,omitempty" jsonschema:"description=More of the answer (markdown)"`
	Data      map[string]any `json:"data,omitempty" yaml:"data,omitempty" jsonschema:"description=A structured answer's data (a form or a choice)"`
	ByID      string         `json:"by_id" yaml:"by_id" jsonschema:"description=Who answered: a person's handle or an agent's id,required"`
	ByKind    string         `json:"by_kind" yaml:"by_kind" jsonschema:"description=person or agent,required"`
	At        string         `json:"at" yaml:"at" jsonschema:"description=When it was answered (RFC 3339),required"`
	Proof     string         `json:"proof,omitempty" yaml:"proof,omitempty" jsonschema:"description=How a relayed person's answer was confirmed: prompt or totp or passkey or entra"`
	RelayedBy string         `json:"relayed_by,omitempty" yaml:"relayed_by,omitempty" jsonschema:"description=The agent session that relayed the person's answer"`
	List      string         `json:"list,omitempty" yaml:"list,omitempty" jsonschema:"description=The holding list it was answered from"`
	Run       string         `json:"run,omitempty" yaml:"run,omitempty" jsonschema:"description=The answering agent's run"`
}
