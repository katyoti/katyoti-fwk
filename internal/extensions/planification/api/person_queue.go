// SPDX-License-Identifier: Apache-2.0

package api

import (
	"github.com/katyoti/katyoti-fwk/internal/core/render"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	decisionsapi "github.com/katyoti/katyoti-fwk/internal/extensions/decisions/api"
	inboxapi "github.com/katyoti/katyoti-fwk/internal/extensions/inbox/api"
)

// THE PERSON'S QUEUE — « À faire »: everything waiting on the person, in one
// place, each item asking exactly ONE action from a closed set. Its sources
// are the person's steps of the plans, the open decisions with a proposal
// (planned or not), the hand tests a request waits on and the Inbox's entries
// nothing else carries; a step needing a hand test and a judgement is two
// items, the test first. Read, never stored.

// The actions a person is asked: the closed set, in the order the queue shows
// them. A notice asks nothing: it is shown apart and never counted.
const (
	ActionDecide   = "decide"   // choose an option of a decision
	ActionTest     = "test"     // test a deliverable by hand: it passes, or fails with a note
	ActionValidate = "validate" // accept a result, or send it back with what to change
	ActionAnswer   = "answer"   // answer a question
	ActionUnblock  = "unblock"  // a step failed or a request is blocked: try it again, resume it, or drop it
	ActionNotice   = "notice"   // for information only
	// ActionConversation is a chat running in the person's sessions (IN-039's
	// switch on): listed after the actions, never
	// counted; opening it opens the chat.
	ActionConversation = "conversation"
)

// PersonActions is the closed set, in the queue's order.
var PersonActions = []string{ActionDecide, ActionTest, ActionValidate, ActionAnswer, ActionUnblock}

// PersonItem is one thing waiting on the person: the one action it asks, what
// it is about, what is already proven and what its buttons need.
type PersonItem struct {
	Key     string `json:"key"               jsonschema:"description=Its stable key: the action and the object (and the deliverable a test is of)"`
	Action  string `json:"action"            jsonschema:"description=The one action it asks: decide or test or validate or answer or unblock; notice for information only"`
	Object  string `json:"object"            jsonschema:"description=What it is about: a request or a decision or an inbox entry"`
	Request string `json:"request,omitempty" jsonschema:"description=The request it moves on"`
	Module  string `json:"module,omitempty"`
	// Project is the key of the project holding what it is about when the
	// universe serves several projects - Object and Request are then
	// qualified <key>:<id>, Module stays bare; empty when one is served.
	Project string `json:"project,omitempty" jsonschema:"description=When several projects are served: the key of the project holding what it is about"`
	Title   string `json:"title"`
	// Increment and Release are the increment and the release the work it
	// moves on belongs to (the to-dos' labels' own reading); empty outside any.
	Increment      string `json:"increment,omitempty"       jsonschema:"description=The increment the work it moves on belongs to"`
	IncrementTitle string `json:"increment_title,omitempty" jsonschema:"description=That increment's title"`
	Release        string `json:"release,omitempty"         jsonschema:"description=The release that work ships in"`
	ReleaseTitle   string `json:"release_title,omitempty"   jsonschema:"description=That release's version and name"`
	// Asks is the one line of what is expected.
	Asks string `json:"asks" jsonschema:"description=What is expected in one line"`
	// ByHand is how a hand test is done — the exact steps — and why no agent
	// can (the deliverable's own words).
	ByHand string   `json:"by_hand,omitempty" jsonschema:"description=A hand test: the exact steps and why no agent can do it"`
	Proven []string `json:"proven,omitempty"  jsonschema:"description=What is already proven: checks passed and deliverables verified and what the agents' steps came to"`
	// Ask is the inbox entry answering it; Todo the to-do it reports or
	// tries again; Deliverable the deliverable a test is of.
	Ask         string `json:"ask,omitempty"`
	Todo        string `json:"todo,omitempty"`
	Deliverable string `json:"deliverable,omitempty"`
	// Expects is what an answer is: verdict or text.
	Expects string `json:"expects,omitempty"`
	// A decision's options and the one proposed.
	Options     []string `json:"options,omitempty"`
	Proposal    string   `json:"proposal,omitempty"`
	ProposalWhy string   `json:"proposal_why,omitempty"`
	// Reasoning and Prepared are a decision's content the choice is made on:
	// its question and context; each option with what it means, its pros and
	// cons, its cost and what follows if chosen (decisions' own reading).
	Reasoning string                        `json:"reasoning,omitempty" jsonschema:"description=A decision's question and context (markdown)"`
	Prepared  []decisionsapi.PreparedOption `json:"prepared,omitempty"  jsonschema:"description=A decision's options as prepared: what each means and its pros and cons and cost and what follows"`
	// Done is what the work before a validation did: its last agent step's
	// result and details, to judge it by.
	Done string `json:"done,omitempty" jsonschema:"description=What the work before it did: its last step's result and details (markdown)"`
	// Challenged is the points raised against a decision's proposal.
	Challenged []string `json:"challenged,omitempty" jsonschema:"description=The points raised against a decision's proposal"`
	// Refs are the objects it refers to, each to open: a decision's challenges.
	Refs []Ref `json:"refs,omitempty" jsonschema:"description=The objects it refers to: each opens"`
	// Wants are the goals and features an increment holds that wait on the
	// person's acceptance (validate): each accepted or dropped.
	Wants []Target `json:"wants,omitempty" jsonschema:"description=The goals and features put forward that an increment holds: each to accept or drop"`
	// Criteria are the acceptance criteria of a want waiting on the person's
	// confirmation, each with its last result - what it is judged on.
	Criteria []Criterion `json:"criteria,omitempty" jsonschema:"description=A want's acceptance criteria each with its last result"`
	// Confirm is how the person accepts or rejects a want recorded as done by
	// the work (a feature realized, a goal assumed): its two moves.
	Confirm *ConfirmMove `json:"confirm,omitempty" jsonschema:"description=The moves accepting or rejecting a want waiting on the person's confirmation"`
	// Checks: a deliverable's check can run again (unblock).
	Checks bool `json:"checks,omitempty"`
	// Resume: a request blocked for another cause can be planned again.
	Resume bool `json:"resume,omitempty"`
	// WaitsOn says why it cannot be done yet: a validation waits on the hand
	// tests before it.
	WaitsOn string `json:"waits_on,omitempty" jsonschema:"description=Why it cannot be done yet; empty: it can"`
	Since   string `json:"since,omitempty"    jsonschema:"description=Since when it waits (RFC 3339)"`
	// Held is set when the person put it on hold: out of its action and the
	// count, listed last.
	Held *Hold `json:"held,omitempty" jsonschema:"description=The person put it on hold: who and when and why"`
	// HoldIsBlock says the hold is the person's own block of its request's
	// plan: taken back, the plan resumes.
	HoldIsBlock bool `json:"hold_is_block,omitempty" jsonschema:"description=The hold is the person's own block of its request's plan: taking it back resumes the plan"`
	// The inbox's own questions - its human tasks - and the conversations,
	// in the same queue with IN-039's switch on: the
	// kind of question and its label, the title of what it is about, who
	// asks, the nodes answering it (the inbox's, drawn as they are) and a
	// conversation's chat panel.
	Kind       string        `json:"kind,omitempty"        jsonschema:"description=The kind of question (an ask-kind's name)"`
	KindLabel  string        `json:"kind_label,omitempty"  jsonschema:"description=The kind's short label"`
	AboutTitle string        `json:"about_title,omitempty" jsonschema:"description=The title of what it is about"`
	AskedBy    string        `json:"asked_by,omitempty"    jsonschema:"description=Who asks it"`
	Answer     []render.Node `json:"answer,omitempty"      jsonschema:"description=The nodes answering it in place (the inbox's)"`
	Chat       string        `json:"chat,omitempty"        jsonschema:"description=A conversation's chat panel (dp-...)"`
	// Answered says who answered it and what, once it is answered: a chat's
	// card shows it in place of its answer area.
	Answered string `json:"answered,omitempty" jsonschema:"description=Who answered it and what, once answered"`
	// What the inbox's page showed of a question: by
	// when, whether it is late, the proof its answer needs, who may answer,
	// who took it, what its answer settles, and who holds it - the person
	// (mine) or a holding list with its members.
	Due          string   `json:"due,omitempty"           jsonschema:"description=By when it must be answered (RFC 3339)"`
	Overdue      bool     `json:"overdue,omitempty"       jsonschema:"description=Past its due date and unanswered"`
	Proof        string   `json:"proof,omitempty"         jsonschema:"description=The proof its answer requires"`
	AnswerableBy string   `json:"answerable_by,omitempty" jsonschema:"description=Who may answer"`
	TakenBy      string   `json:"taken_by,omitempty"      jsonschema:"description=Who took it"`
	Satisfies    []string `json:"satisfies,omitempty"     jsonschema:"description=What its answer settles"`
	Holder       string   `json:"holder,omitempty"        jsonschema:"description=Who holds it: mine or list:<the holding list>"`
	HolderLabel  string   `json:"holder_label,omitempty"  jsonschema:"description=Its holder as the page names it"`
}

// Hold is the person's flag on one item of their queue: it leaves its action
// and the count, listed last under « En attente », until they take it back.
// It changes nothing else — what it is about keeps its state, what waits on
// it stays waiting, no agent takes it as answered.
type Hold struct {
	Key    string       `json:"key"              jsonschema:"description=The item's key"`
	Object string       `json:"object,omitempty" jsonschema:"description=What the item is about"`
	Reason string       `json:"reason,omitempty" jsonschema:"description=Why it waits"`
	By     cwmapi.Actor `json:"by"`
	At     string       `json:"at"               jsonschema:"description=Since when it is on hold (RFC 3339)"`
	// Relayed is set when an agent's session relayed the person's hold.
	Relayed *Relayed `json:"relayed,omitempty" jsonschema:"description=Set when an agent's session relayed the person's hold"`
}

// Ref is an object an item refers to, to open.
type Ref struct {
	ID    string `json:"id"`
	Label string `json:"label" jsonschema:"description=What it is to this item (Challenged by - for instance)"`
}

// Criterion is an acceptance criterion with its last result: its verdict's
// status (pass or fail; unproven when it never ran) and when it ran.
type Criterion struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Status string `json:"status" jsonschema:"description=pass or fail or another verdict status; unproven when it never ran"`
	RanAt  string `json:"ran_at,omitempty"`
}

// ConfirmMove is a want's two moves on the person's word: Accept (its
// transition and the state it reaches) and Reject (the same, with a reason).
type ConfirmMove struct {
	Accept   string `json:"accept"`
	AcceptTo string `json:"accept_to"`
	Reject   string `json:"reject"`
	RejectTo string `json:"reject_to"`
}

// HoldInput puts one item of the person's queue on hold, by a person.
type HoldInput struct {
	Key    string       `json:"key"              jsonschema:"description=The item's key (PersonQueue),required"`
	Reason string       `json:"reason,omitempty" jsonschema:"description=Why it waits"`
	By     cwmapi.Actor `json:"by"               jsonschema:"description=The person (a person only),required"`
}

// UnholdInput takes an item on hold back to its action, by a person.
type UnholdInput struct {
	Key string       `json:"key" jsonschema:"description=The item's key,required"`
	By  cwmapi.Actor `json:"by"  jsonschema:"description=The person (a person only),required"`
}

// HoldOutput is the item's hold after the call: nil once taken back.
type HoldOutput struct {
	Hold *Hold `json:"hold,omitempty"`
}

// PersonQueueInput scopes the queue: every module when both are empty, a
// module or an object (a request, a decision) by target or by id (the pane it
// is on); its page shows one action's items when Action is set.
type PersonQueueInput struct {
	Target string `json:"target,omitempty" jsonschema:"description=The module or the object the queue is about; empty: everything"`
	ID     string `json:"id,omitempty"     jsonschema:"description=The module or the object whose pane shows it; used when target is empty"`
	Focus  string `json:"focus,omitempty"  jsonschema:"description=An item's key or object to show first (the Inbox opening its card)"`
	// Action filters the page to one action's items (its tile chosen); every
	// tile keeps its count. Empty: every item (« Tout »).
	Action string `json:"action,omitempty" jsonschema:"description=Only the items asking this action (decide or test or validate or answer or unblock) or those on hold (held); empty = all but those on hold"`
	// Language is the person's (en or fr): the page's labels.
	Language string `json:"language,omitempty" jsonschema:"description=The person's language (en or fr); empty = English"`
	// Kind keeps only the items of that kind of question (overdue: those
	// overdue; conversation: the conversations) - IN-039's switch on, its
	// chip chosen; every count kept.
	Kind string `json:"kind,omitempty" jsonschema:"description=Only the items of this kind of question (or overdue or conversation); every count kept"`
	// Fresh asks the page computed again, never answered from a kept read:
	// a re-read (after a change, after an action) says it; a click does not
	//
	Fresh bool `json:"fresh,omitempty" jsonschema:"description=Compute the queue again: a re-read after a change or an action; a click leaves it out"`
}

// PersonQueueOutput is the queue: its items by action, how many there are by
// action, and its count — every item but the notices and those on hold, which
// come last and are counted apart (Held).
type PersonQueueOutput struct {
	Items    []PersonItem   `json:"items"`
	ByAction map[string]int `json:"by_action"`
	Count    int            `json:"count"`
	Held     int            `json:"held,omitempty"`
	// Unread says why the inbox's questions could not be read (IN-039's
	// switch on): the queue lists its own items and says
	// so, never silently fewer.
	Unread string `json:"unread,omitempty" jsonschema:"description=Why the inbox's questions could not be read; empty: they were"`
	// With IN-039's switch on: whose queue it is, and
	// how many questions wait by kind and overdue, as the inbox counts them.
	Person     string               `json:"person,omitempty"      jsonschema:"description=Whose queue it is: the person's handle"`
	PersonName string               `json:"person_name,omitempty" jsonschema:"description=The person's name"`
	Kinds      []inboxapi.KindCount `json:"kinds,omitempty"       jsonschema:"description=How many questions wait by kind"`
	Overdue    int                  `json:"overdue,omitempty"     jsonschema:"description=How many questions are overdue"`
}

// PersonQueueCountOutput is the queue's count: the rail's badge.
type PersonQueueCountOutput struct {
	Count int `json:"count"`
}

// A hand test's result.
const (
	TestPasses = "passes"
	TestFails  = "fails"
)

// TestInput records a hand test of one deliverable, by a person.
type TestInput struct {
	ID          string       `json:"id"          jsonschema:"description=The request,required"`
	Deliverable string       `json:"deliverable" jsonschema:"description=The deliverable's title,required"`
	Result      string       `json:"result"      jsonschema:"description=passes or fails,enum=passes|fails,required"`
	Note        string       `json:"note,omitempty" jsonschema:"description=What was seen; required when it fails"`
	By          cwmapi.Actor `json:"by"          jsonschema:"description=The person who tested it (a person only),required"`
}
