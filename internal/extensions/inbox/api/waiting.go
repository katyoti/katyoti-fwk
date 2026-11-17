// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"

	"github.com/katyoti/katyoti-fwk/internal/core/meta"
	"github.com/katyoti/katyoti-fwk/internal/core/render"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// « À FAIRE », THE ONE PLACE (decision) - everything
// that waits on a person is one list, read, never stored: the open questions
// for them, then for each holding list they are a member of; within each,
// overdue first, then by kind (its order among the kinds), then priority, then
// age. Its sources are the human tasks (the inbox's own to-dos), the inbox's
// pending entries - a gate's confirmation and an ask-user question included,
// a notice listed apart under Recent and never counted - and what the askers
// not yet asking through a human task contribute (WaitingSource):
// planification's person steps, decisions, hand tests, unblocks and
// acceptances. Each question shows once: an entry a source answers for is
// listed through that source's item only. The rail's Inbox opens it with its
// count.

// WaitingItem is one question waiting on a person: its kind first (what is
// expected), what it asks, what it is about, what its answer settles, whose it
// is and by when.
type WaitingItem struct {
	// Key is its stable key: its source and the object or to-do it stands for.
	Key string `json:"key" jsonschema:"description=Its stable key: its source and what it stands for"`
	// Source is who lists it: inbox, or a contributed source's name.
	Source string `json:"source" jsonschema:"description=Who lists it: inbox or the asker's source"`
	// Kind is what is expected (an ask-kind's name); Label its short label.
	Kind  string `json:"kind" jsonschema:"description=What is expected: an ask-kind's name (approve or decide or test-by-hand ...)"`
	Label string `json:"label,omitempty" jsonschema:"description=The kind's short label"`
	Title string `json:"title" jsonschema:"description=The question in one line"`
	// Asks is what is expected in the asker's words, when it says more.
	Asks string `json:"asks,omitempty" jsonschema:"description=What is expected in the asker's words"`
	// Options and FormDef are what a choose or a form question offers: its
	// card draws them.
	Options []string `json:"options,omitempty" jsonschema:"description=The choices of a choose question"`
	FormDef string   `json:"form_def,omitempty" jsonschema:"description=A form question's FormDefinition (YAML)"`
	// About is the object it is about: « Ouvrir » goes there.
	About   string `json:"about,omitempty" jsonschema:"description=The object it is about"`
	Request string `json:"request,omitempty" jsonschema:"description=The request it moves on"`
	Module  string `json:"module,omitempty"`
	// Satisfies is what its answer settles.
	Satisfies []string `json:"satisfies,omitempty" jsonschema:"description=What its answer settles"`
	// List is the holding list it waits on; empty: the person's own.
	List string `json:"list,omitempty" jsonschema:"description=The holding list it waits on; empty for the person's own"`
	// ClaimedBy is who took it, when someone did.
	ClaimedBy    *cwmapi.Actor `json:"claimed_by,omitempty" jsonschema:"description=Who took it"`
	AnswerableBy string        `json:"answerable_by,omitempty" jsonschema:"description=Who may answer: person or agent or any"`
	Proof        string        `json:"proof,omitempty" jsonschema:"description=The proof the answer requires"`
	Priority     string        `json:"priority,omitempty" jsonschema:"description=P0 to P3"`
	Due          string        `json:"due,omitempty" jsonschema:"description=By when it must be answered (RFC 3339)"`
	Overdue      bool          `json:"overdue,omitempty" jsonschema:"description=Past its due date and unanswered"`
	Since        string        `json:"since,omitempty" jsonschema:"description=Since when it waits (RFC 3339)"`
	// Todo is the to-do its answer reports; Entry the inbox entry it answers.
	Todo  string `json:"todo,omitempty"`
	Entry string `json:"entry,omitempty"`
	// Ref is the source's own key of the item.
	Ref string `json:"ref,omitempty" jsonschema:"description=The source's own key of the item"`
	// AskedBy is who asks it: the human task's asker, the entry's requester.
	AskedBy string `json:"asked_by,omitempty" jsonschema:"description=Who asks it: the human task's asker or the entry's requester"`
	// Answer are the nodes answering it, by its kind - its forms, its
	// « Pourquoi ? » modal - when the list was asked with answers (the
	// inbox's own questions).
	Answer []render.Node `json:"answer,omitempty" jsonschema:"description=The nodes answering it by its kind (asked with answers)"`
	// Own is the source's own item, kept in the process from its Items to
	// its Controls; never sent.
	Own any `json:"-"`
}

// WaitingGroup is one part of the list: the person's own questions (Mine), or
// one holding list's with its members.
type WaitingGroup struct {
	List    string        `json:"list,omitempty" jsonschema:"description=The holding list; empty for the person's own"`
	Title   string        `json:"title,omitempty" jsonschema:"description=The holding list's name"`
	Mine    bool          `json:"mine,omitempty" jsonschema:"description=The person's own questions"`
	Members []Member      `json:"members,omitempty" jsonschema:"description=The holding list's members"`
	Items   []WaitingItem `json:"items"`
}

// KindCount is how many open questions of one kind wait.
type KindCount struct {
	Kind  string `json:"kind"`
	Label string `json:"label,omitempty"`
	Count int    `json:"count"`
}

// WaitingInput is whose list and which of its questions.
type WaitingInput struct {
	// Person is whose list; empty: the profile's.
	Person string `json:"person,omitempty" jsonschema:"description=Whose list: a person's handle; empty for the profile's"`
	// Kind keeps only that kind's questions in the list (overdue: only those
	// overdue; conversation: only the conversations); the counts stay whole.
	// The page lists them all: its header filters by kind.
	Kind string `json:"kind,omitempty" jsonschema:"description=Only this kind's questions (or overdue - or conversation: only the conversations); the counts stay whole"`
	// Language is the person's (en or fr): the page's labels.
	Language string `json:"language,omitempty" jsonschema:"description=The person's language (en or fr); empty for the profile's"`
	// Answers gives each question the nodes answering it (Answer) - the
	// inbox's own and those of a contributed source (its Controls): a screen
	// that is not the inbox's draws them as they are (planification's « À
	// faire », RQ-1017).
	Answers bool `json:"answers,omitempty" jsonschema:"description=Each question carries the nodes answering it (answer): the inbox's own and a contributed source's"`
}

// WaitingOutput is the list: its groups (Mine first), its counts and what was
// only for information (Recent, never counted).
type WaitingOutput struct {
	Person  string         `json:"person"`
	Name    string         `json:"name,omitempty" jsonschema:"description=The person's name"`
	Count   int            `json:"count" jsonschema:"description=How many questions wait: the rail's badge"`
	Overdue int            `json:"overdue,omitempty"`
	Kinds   []KindCount    `json:"kinds,omitempty" jsonschema:"description=How many questions of each kind wait (in the kinds' order)"`
	Groups  []WaitingGroup `json:"groups"`
	Recent  []WaitingItem  `json:"recent,omitempty" jsonschema:"description=What is only for information: notices"`
	// Conversations are the chats running in the person's sessions, those
	// waiting on their reply first: listed, never counted.
	Conversations []Conversation `json:"conversations,omitempty" jsonschema:"description=The chats running in the person's sessions: those waiting on their reply first; never counted"`
}

// ChatPanelType is the display panel type of a run's conversation
const ChatPanelType = "ktw-agents:/panel-type/chat"

// KindConversation is the filter keeping only the conversations.
const KindConversation = "conversation"

// Conversation is a chat running in the person's sessions: its display panel,
// whether it waits on their reply, and when it last moved. Opening it opens
// its chat.
type Conversation struct {
	Panel   string `json:"panel" jsonschema:"description=The chat's display panel (dp-…)"`
	Title   string `json:"title,omitempty" jsonschema:"description=The chat's title"`
	Owner   string `json:"owner,omitempty" jsonschema:"description=The run it is the conversation of"`
	Waiting bool   `json:"waiting,omitempty" jsonschema:"description=It waits on the person's reply"`
	Updated string `json:"updated,omitempty" jsonschema:"description=When it last moved (RFC 3339)"`
	// Last is the chat's last word to the person while it waits on them: what
	// it asks.
	Last string `json:"last,omitempty" jsonschema:"description=What the chat asks the person while it waits on them"`
}

// WaitingCountOutput is the list's count: the rail's badge.
type WaitingCountOutput struct {
	Count int `json:"count"`
}

// WaitingServiceDef declares « À faire » for KTSP publication
// (`<provider>:waiting/<Verb>`).
var WaitingServiceDef = meta.ServiceDef{
	Name:    "waiting",
	Short:   "« À faire »: everything waiting on a person, in one place - their questions, then their holding lists'",
	Version: "1.0.0",
	Methods: map[string]meta.MethodDef{
		"List":  {Short: "Everything waiting on a person: theirs then their holding lists', overdue first, by kind; notices apart", ReadOnly: true},
		"Count": {Short: "How many questions wait on a person (notices left out): the rail's badge", ReadOnly: true},
		"Inbox": {Short: "What the rail's Inbox opens: « À faire »'s page and count", ReadOnly: true},
	},
}

// THE RAIL'S INBOX, NAMED BY THE BACKEND (the person's choice
// 2026-11-02): the GUI's rail Inbox opens what the inbox answers here - no
// inbox screen code in the GUI (decision). It is « À faire »: its
// page, its count as the rail's badge, and the Global strip's panels it
// stands in for.

// InboxInput asks what the rail's Inbox opens.
type InboxInput struct{}

// InboxScreen is what the rail's Inbox opens.
type InboxScreen struct {
	// Page is the page source the Inbox draws; empty: the built-in Inbox.
	Page string `json:"page,omitempty" jsonschema:"description=The page source the rail's Inbox draws (a method answering a page); empty: the built-in Inbox"`
	// Count is the method answering the rail's badge ({count}).
	Count string `json:"count,omitempty" jsonschema:"description=The method answering the rail's badge ({count}); empty: no badge"`
	// Replaces are the Global strip's declared panels it stands in for.
	Replaces []string `json:"replaces,omitempty" jsonschema:"description=The declared global panels it stands in for: the Global strip leaves them out"`
}

// HumanTaskCard draws one human task's card - « À faire »'s own card, the
// one frame of a human task wherever it shows: the
// chat that asks it draws it with it. it carries its answer
// nodes (Answer) while it waits; answered, answered says who answered and
// what (the inbox's words), shown in place of the answer area. The asker that
// draws « À faire » sets it (planification); nil, a chat shows the question's
// title alone.
var HumanTaskCard func(ctx context.Context, it WaitingItem, answered, language string) render.Node

// WaitingSourceInbox is the inbox's own source in the waiting list: its
// human tasks and its entries (WaitingItem.Source).
const WaitingSourceInbox = "inbox"

// The sources of « À faire » and the Global strip's panel it replaces. The
// page is planification's previous « À faire » reused, every waiting item in
// it (the person's choice 2026-11-02): the inbox's own
// page is no longer what the rail opens.
const (
	PersonQueuePageURI  = "ktw-agents:views/person-queue-page"
	PersonQueueCountURI = "ktw-agents:views/person-queue-count"
	WaitingCountURI     = "ktw-agents:waiting/count"
	// FormerPersonQueuePanel is planification's former « À faire » panel.
	FormerPersonQueuePanel = "person-queue"
)

// WaitingService is « À faire ».
type WaitingService interface {
	List(ctx context.Context, in WaitingInput) (WaitingOutput, error)
	Count(ctx context.Context, in WaitingInput) (WaitingCountOutput, error)
	Inbox(ctx context.Context, in InboxInput) (InboxScreen, error)
}

// WaitingSource is what an asker that does not yet ask through a human task
// contributes to « À faire » (application.Contribute): its questions waiting
// on the person, the inbox entries it answers for - never listed by the inbox
// itself, so a question shows once - and the controls answering one of its
// items on the page. The point is the inbox's (inboxlocal.WaitingExtension).
type WaitingSource struct {
	// Name is the source's name: its items' Source.
	Name string
	// Items are its questions waiting on the person.
	Items func(ctx context.Context) ([]WaitingItem, error)
	// Answers says an inbox entry is the source's: shown through its items
	// only, or not at all once none carries it.
	Answers func(e Entry) bool
	// Controls are the page nodes answering one of its items, in the
	// person's language.
	Controls func(ctx context.Context, it WaitingItem, language string) []render.Node
}

// withoutSourcesKey carries the sources a list leaves out.
type withoutSourcesKey struct{}

// WithoutSources is ctx with the waiting list leaving out the items of the
// sources named - still their entries' owners (Answers) -: a source reading
// the list for the inbox's own questions only, never its own again
// . Its counts are then those of the items listed.
func WithoutSources(ctx context.Context, names ...string) context.Context {
	return context.WithValue(ctx, withoutSourcesKey{}, names)
}

// SourceLeftOut says ctx's waiting list leaves the source's items out.
func SourceLeftOut(ctx context.Context, name string) bool {
	names, _ := ctx.Value(withoutSourcesKey{}).([]string)
	for _, n := range names {
		if n == name {
			return true
		}
	}
	return false
}
