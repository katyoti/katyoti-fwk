// SPDX-License-Identifier: Apache-2.0

// Package api is the inbox service contract: file a request that needs a
// person, answer it, triage what waits, dismiss what is dealt with. Every verb
// is implemented over the CWM Object API — the service keeps no storage and no
// index of its own. The live inbox is the `inbox` object in the `sys-inbox`
// module — created by Ensure, which the service calls whenever it needs the
// inbox and which the `bootstrap` hook calls when a project opens; requests are
// its children.
package api

import (
	"context"
	"errors"
	"fmt"

	"github.com/katyoti/katyoti-fwk/internal/core/meta"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// Object types the inbox works with.
const (
	InboxType   = "inbox"
	RequestType = "approval-request"
)

// Statuses an Entry reports. Any status other than pending and dismissed is an
// answer — the request's decision; approved is the default one.
const (
	StatusPending   = "pending"
	StatusApproved  = "approved"
	StatusRejected  = "rejected"
	StatusDismissed = "dismissed"
)

// What a request expects as its answer (FileInput.Expects). A verdict is
// approved — a note optional — or rejected, saying what to change; text is
// the person's answer itself, sent or dismissed, never rejected; a choice is
// one of its options (approved with that choice, which decides the decision
// it is about) or none — rejected, with a note that sends the decision back.
// A request filed without it (an older one) takes any answer, but a rejection
// still says what to change.
//
// A notice expects nothing: it is an information (what a demand became), read
// and dismissed - no answer is asked, and it counts in no pending total.
const (
	ExpectsVerdict = "verdict"
	ExpectsText    = "text"
	ExpectsChoice  = "choice"
	ExpectsNotice  = "notice"
)

// ErrGone is a request that is no longer in the inbox: deleted, or moved out
// of it. No answer will come to it.
var ErrGone = errors.New("inbox: the request is gone")

// A request's lifecycle states (its workflow, approval-request).
const (
	StatePending   = "pending"
	StateAnswered  = "answered"
	StateDismissed = "dismissed"
)

// UpdatedEventURI is the event a request's update publishes — what a task
// waiting on the request subscribes to. The subject carries the token DERIVED
// from the id (cwmapi.SubjectToken), never the id itself: an object id is
// module-qualified and its dot is the NATS separator.
func UpdatedEventURI(requestID string) string {
	return fmt.Sprintf("cwm:!workspace/object/%s/updated", cwmapi.SubjectToken(requestID))
}

// InboxServiceDef describes the service for KTSP/CLI auto-generation.
var InboxServiceDef = meta.ServiceDef{
	Name:    "inbox",
	Short:   "What needs a person: file a request, answer it, triage the inbox, dismiss",
	Version: "1.0.0",
	Methods: map[string]meta.MethodDef{
		"File":     meta.Method("File an approval request in the inbox, pending"),
		"Get":      meta.Method("Read one request"),
		"Answer":   meta.Method("Answer a pending request; the task waiting on it resumes"),
		"Triage":   meta.Method("List what waits in the inbox: every entry but answered requests"),
		"Dismiss":  meta.Method("Clear an entry: a request is closed as dismissed, anything else leaves the inbox"),
		"Ensure":   meta.Method("Give the project its inbox: the one there is, else sys-inbox and its inbox, created (idempotent; the bootstrap hook)"),
		"Relocate": meta.Method("Move an inbox kept outside sys-inbox, with its entries, into sys-inbox (dry-run unless apply)"),
	},
}

// InboxService manages the inbox's entries.
type InboxService interface {
	File(ctx context.Context, in FileInput) (FileOutput, error)
	Get(ctx context.Context, in GetInput) (GetOutput, error)
	Answer(ctx context.Context, in AnswerInput) (AnswerOutput, error)
	Triage(ctx context.Context, in TriageInput) (TriageOutput, error)
	Dismiss(ctx context.Context, in DismissInput) (DismissOutput, error)
	// Ensure gives the project its inbox — idempotent. It is the inbox
	// extension's `bootstrap` hook, and every verb that needs the inbox calls it.
	Ensure(ctx context.Context, in EnsureInput) (EnsureOutput, error)
	// Relocate moves an inbox that lives outside sys-inbox — a migrated
	// project's — and its entries into sys-inbox, so every project keeps its
	// inbox in the same place. Dry-run unless Apply.
	Relocate(ctx context.Context, in RelocateInput) (RelocateOutput, error)
}

// RelocateInput: without Apply the move is only planned.
type RelocateInput struct {
	Apply bool `json:"apply,omitempty" jsonschema:"Move; without it the move is only reported"`
}

// Relocation is one object's move, old id to new.
type Relocation struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// RelocateOutput is the move, planned or made. Empty Moves: the inbox is
// already in sys-inbox, or there is none.
type RelocateOutput struct {
	Inbox   string       `json:"inbox"             jsonschema:"The inbox after the move (or where it is)"`
	Moves   []Relocation `json:"moves,omitempty"   jsonschema:"The inbox first, then each entry"`
	Applied bool         `json:"applied,omitempty"`
}

// EnsureInput carries nothing the service reads; a bootstrap hook sends its
// {point, object} payload, which is ignored.
type EnsureInput struct{}

// EnsureOutput names the inbox and says whether this call created it.
type EnsureOutput struct {
	Inbox   string `json:"inbox"`
	Created bool   `json:"created,omitempty"`
}

// Entry is one thing in the inbox. An approval-request carries the request
// fields; any other kind carries its id, type, title and status.
type Entry struct {
	ID           string `json:"id"`
	Inbox        string `json:"inbox"                   jsonschema:"The inbox object it sits under"`
	Type         string `json:"type"                    jsonschema:"Object type; approval-request or any other kind filed for triage"`
	Title        string `json:"title"`
	Description  string `json:"description,omitempty"`
	RequestedBy  string `json:"requested_by,omitempty"  jsonschema:"The task that asked"`
	RequestedAt  string `json:"requested_at,omitempty"  jsonschema:"RFC3339"`
	Status       string `json:"status,omitempty"`
	Urgency      string `json:"urgency,omitempty"`
	BlocksTask   bool   `json:"blocks_task,omitempty"`
	DecisionNote string `json:"decision_note,omitempty" jsonschema:"The answer's text"`
	ResolvedBy   string `json:"resolved_by,omitempty"`
	ResolvedAt   string `json:"resolved_at,omitempty"   jsonschema:"RFC3339"`
	// Relayed is set when an agent's session relayed the person's answer.
	Relayed *Relayed `json:"relayed,omitempty" jsonschema:"Set when an agent's session relayed the person's answer: which session and how the person confirmed it"`
	About   string   `json:"about,omitempty"         jsonschema:"The object it is about (its about edge): the Inbox opens it"`
	Panel   string   `json:"panel,omitempty"         jsonschema:"The panel of what it is about the Inbox opens it on: where it is answered; empty: its first"`
	Expects string   `json:"expects,omitempty"       jsonschema:"What its answer is: verdict (approved or rejected with a note) or text (a note; dismissed otherwise) or choice (one of its options; none rejected with a note) or notice (an information read and dismissed; nothing to answer); empty on an older request"`
	// Options are a choice's labels; Choice is the one the person chose.
	Options []string `json:"options,omitempty" jsonschema:"A choice's options: the labels of the decision it is about"`
	Choice  string   `json:"choice,omitempty"  jsonschema:"The option chosen; empty while pending and when none was"`
}

// FileInput files a request.
type FileInput struct {
	Title       string `json:"title"                  jsonschema:"The question in one line; required"`
	Description string `json:"description,omitempty"  jsonschema:"The full context of the question"`
	RequestedBy string `json:"requested_by,omitempty" jsonschema:"The task asking"`
	Urgency     string `json:"urgency,omitempty"      jsonschema:"low, medium (default) or high"`
	BlocksTask  bool   `json:"blocks_task,omitempty"  jsonschema:"The asking task waits for the answer"`
	// About is the object the question is about (a request): the Inbox opens
	// it, where the question is answered with its whole context.
	About string `json:"about,omitempty" jsonschema:"The object the question is about (an object id); the Inbox opens it"`
	// Panel is the panel of About the Inbox opens it on — where it is
	// answered (a person step's « À faire » card): the entry is a
	// notification, its text lives there.
	Panel string `json:"panel,omitempty" jsonschema:"The panel of the object it is about the Inbox opens it on; empty: its first"`
	// Expects is what the answer is: a verdict, text, a choice, or a notice.
	Expects string `json:"expects,omitempty" jsonschema:"verdict (approved or rejected with what to change) or text (the person's answer) or choice (one of the options - or none with a note) or notice (an information: nothing to answer); empty: any answer"`
	// Options are the labels a choice is answered with — the options of the
	// decision About names, which an option resolves. A choice only.
	Options []string `json:"options,omitempty" jsonschema:"A choice's options: at least two unique labels of the decision it is about (about required); refused on any other kind"`
}

// FileOutput returns the filed request.
type FileOutput struct {
	Request Entry  `json:"request"`
	Path    string `json:"path" jsonschema:"Repo-relative path of the request's file"`
}

// GetInput names one entry.
type GetInput struct {
	ID string `json:"id" jsonschema:"Entry id; required"`
}

// GetOutput returns it.
type GetOutput struct {
	Entry Entry `json:"entry"`
}

// AnswerInput answers a pending request.
type AnswerInput struct {
	ID       string `json:"id"                 jsonschema:"Request id; required"`
	Decision string `json:"decision,omitempty" jsonschema:"The resolution status (approved, rejected ...); default approved. A rejection takes a note; a request expecting text is never rejected"`
	Note     string `json:"note,omitempty"     jsonschema:"The answer's text; what the waiting task receives when set. Required to reject, and to answer a request expecting text"`
	// Choice answers a request expecting a choice: one of its options, with
	// approved — the decision it is about is resolved with it, as the person,
	// before the answer is written. None is rejected with a note, no choice.
	Choice string `json:"choice,omitempty" jsonschema:"The option chosen, on a request expecting a choice (approved); none is rejected with a note"`
	By     string `json:"by,omitempty"       jsonschema:"Who answered; default user"`
}

// AnswerOutput returns the answered request. An entry already answered or
// dismissed is not answered again: it comes back as it stands, OK false and
// Message saying by whom, when and what - a page's button shows it.
type AnswerOutput struct {
	Request Entry  `json:"request"`
	OK      *bool  `json:"ok,omitempty"      jsonschema:"false when the entry was already answered or dismissed: nothing changed"`
	Message string `json:"message,omitempty" jsonschema:"Why nothing changed: already answered - by whom and when and what"`
}

// TriageInput is empty: there is one inbox per project.
type TriageInput struct{}

// TriageOutput lists what waits, oldest request first.
type TriageOutput struct {
	Inbox   string  `json:"inbox"   jsonschema:"The inbox object; empty when the project has none"`
	Entries []Entry `json:"entries"`
}

// DismissInput names the entry to clear.
type DismissInput struct {
	ID string `json:"id" jsonschema:"Entry id; required"`
}

// DismissOutput reports what happened to it.
type DismissOutput struct {
	ID      string `json:"id"`
	Removed bool   `json:"removed" jsonschema:"true: the entry was deleted; false: a request kept as a record, closed as dismissed"`
}

// Relayed records a person's answer an agent's session relayed: which session,
// how the person confirmed it (prompt or totp or entra) and when.
type Relayed struct {
	By        string `json:"by"           jsonschema:"The agent session that relayed it"`
	Confirmed string `json:"confirmed"    jsonschema:"How the person confirmed it: prompt or totp or entra"`
	At        string `json:"at,omitempty" jsonschema:"When the person confirmed it (RFC 3339)"`
}
