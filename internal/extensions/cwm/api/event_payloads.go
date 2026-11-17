// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"
	"time"
	"unicode/utf8"

	"github.com/katyoti/katyoti-fwk/internal/core/cause"
)

// THE LIFECYCLE EVENT PAYLOADS — what a subscriber to CWM's events decodes.
// They land in EventEnvelope.Payload as JSON; the subject's entity class
// (item / node / object) says which one. They stay small: enough for a
// subscriber to act without fetching the object again.

// EventMeta is what every write's event says about the write itself, beside
// what it says about the object: which event it is, when the write was made,
// the revision it wrote, and why — the cause the write's context carried, if
// any (internal/core/cause). The journal copies it onto its record.
type EventMeta struct {
	// EventID identifies the event: a UUID v7 minted at the write, so it
	// sorts by time and is unique across processes.
	EventID string `json:"event_id"`
	// At is the write's time (UTC), not when a subscriber received it.
	At time.Time `json:"at"`
	// Revision is the hash of the bytes the write left (ObjectUpdateOutput's);
	// a delete carries the revision it removed.
	Revision string `json:"revision"`
	// Cause is why the write happened; absent on a write made with none.
	Cause *cause.Cause `json:"cause,omitempty"`
}

// ObjectEventPayload accompanies `object.<token>.created|updated|deleted`:
// one per object write. The token in the subject is a hash — ObjectID and
// Module name the object.
type ObjectEventPayload struct {
	EventMeta
	ObjectID string `json:"object_id"`
	Module   string `json:"module"`
	Type     string `json:"type,omitempty"`
	Parent   string `json:"parent,omitempty"`
	Slug     string `json:"slug,omitempty"`
	Action   string `json:"action" jsonschema:"created | updated | deleted"`
	// Operation says the write is part of a larger operation that publishes
	// its own event: `move` for the moved object's own writes, which the one
	// `object.<token>.moved` event reports; `split` for the source's removal;
	// `transition` for a move's own write, which `object.<token>.transitioned`
	// reports. Not the cause: the cause says why, the operation says what
	// this write is a part of. A hook reads it as `event.payload.operation`.
	Operation string `json:"operation,omitempty"`
}

// ObjectTransitionedEventPayload accompanies `object.<token>.transitioned`:
// one per ObjectTransition.
type ObjectTransitionedEventPayload struct {
	EventMeta
	ObjectID string `json:"object_id"`
	Module   string `json:"module"`
	Type     string `json:"type,omitempty"`
	Workflow string `json:"workflow"`
	// Capability names whose lifecycle moved; empty = the object's own.
	Capability string `json:"capability,omitempty"`
	From       string `json:"from"`
	To         string `json:"to"`
	Transition string `json:"transition"`
	By         Actor  `json:"by"`
	Note       string `json:"note,omitempty"`
	Forced     bool   `json:"forced,omitempty"`
	// Calls is, for a forced move, what the functions of the guard it passed
	// answered — the journal keeps them with the move.
	Calls []FunctionAnswer `json:"calls,omitempty"`
}

// ObjectTransitionRefusedEventPayload accompanies
// `object.<token>.transition-refused`: a move its guard refused (a dry run
// publishes nothing), with what each function the guard called answered — a
// planner reads gate('...')'s unmet requirements from it.
type ObjectTransitionRefusedEventPayload struct {
	// EventMeta stamps the refusal like a write's event: the revision is the
	// object's current one (read, not changed — a refused move writes
	// nothing).
	EventMeta
	ObjectID   string           `json:"object_id"`
	Module     string           `json:"module"`
	Type       string           `json:"type,omitempty"`
	Workflow   string           `json:"workflow"`
	Capability string           `json:"capability,omitempty"`
	Transition string           `json:"transition"`
	From       string           `json:"from"`
	To         string           `json:"to"`
	By         Actor            `json:"by"`
	Guard      string           `json:"guard"`
	Reason     string           `json:"reason"`
	Calls      []FunctionAnswer `json:"calls,omitempty"`
}

// ObjectMovedEventPayload accompanies `object.<token>.moved` — ONE event per
// ObjectMove, whatever the move took underneath (an update in place, or a
// create under the new id and a delete of the old one, plus the
// re-pointed referrers, which are writes of their own).
type ObjectMovedEventPayload struct {
	EventMeta
	ObjectID   string `json:"object_id"`
	Module     string `json:"module"`
	FromID     string `json:"from_id"`
	FromModule string `json:"from_module"`
	Parent     string `json:"parent,omitempty"`
	FromParent string `json:"from_parent,omitempty"`
	Type       string `json:"type,omitempty"`
	Reason     string `json:"reason"`
}

// ObjectSplitEventPayload accompanies `object.<token>.split` — ONE event per
// ObjectSplit, on the source's token.
type ObjectSplitEventPayload struct {
	EventMeta
	ObjectID   string   `json:"object_id" jsonschema:"The source"`
	Module     string   `json:"module"`
	Type       string   `json:"type,omitempty"`
	Parent     string   `json:"parent,omitempty"`
	Successors []string `json:"successors"`
	SourceKept bool     `json:"source_kept"`
	Reason     string   `json:"reason"`
}

// ObjectArchivedEventPayload accompanies `object.<token>.archived` — one per
// ObjectArchive that moved a file (an id already archived publishes none).
// Path is where the file went, project-relative; the revision is the
// archived bytes' (unchanged by the move).
type ObjectArchivedEventPayload struct {
	EventMeta
	ObjectID string `json:"object_id"`
	Module   string `json:"module"`
	Type     string `json:"type,omitempty"`
	Parent   string `json:"parent,omitempty"`
	Path     string `json:"path"`
}

// ObjectsCaughtUpEventPayload accompanies `project.caught-up` - ONE event per
// catch-up that found objects moved outside the CWM verbs (a git pull or
// merge, the person's editor, a bulk copy), grouped: the ids added, modified
// and removed, the cause and who made the change. A catch-up writes no
// object, so the event carries no revision.
type ObjectsCaughtUpEventPayload struct {
	// EventID identifies the event: a UUID v7 minted at the catch-up.
	EventID string `json:"event_id"`
	// At is the catch-up's time (UTC).
	At       time.Time     `json:"at"`
	Added    []string      `json:"added,omitempty"    jsonschema:"The ids of the objects that appeared"`
	Modified []string      `json:"modified,omitempty" jsonschema:"The ids of the objects whose file changed"`
	Removed  []string      `json:"removed,omitempty"  jsonschema:"The ids of the objects whose file went"`
	Cause    CaughtUpCause `json:"cause"`
	// By is who made the change: kind outside - the author of the newest
	// commit caught up or the local git user for an edit not committed yet.
	By Actor `json:"by"`
	// Sha is the commit By authored; empty for an edit not committed yet.
	Sha string `json:"sha,omitempty"`
	// Count is how many objects moved: the added, modified and removed ids.
	Count int `json:"count" jsonschema:"How many objects moved"`
	// Held says no per-object event went out: more than CatchUpPerObjectMax
	// objects moved, so their events wait until CatchUpRelease is called
	// with this event's id (the answer to its card).
	Held bool `json:"held,omitempty" jsonschema:"True when the per-object events wait for CatchUpRelease"`
}

// CaughtUpCause is why objects moved outside the verbs: commits between two
// HEADs (a pull, a merge, a commit made by hand), a rewritten history (the
// HEAD CWM saw last is no ancestor of this one, or git no longer has it), or
// the working tree.
type CaughtUpCause struct {
	Kind string `json:"kind" jsonschema:"commits or rewritten or working-tree"`
	// From is the HEAD CWM saw last; empty when the repository had no commit.
	From string `json:"from,omitempty"`
	// To is the HEAD now; empty outside a git repository.
	To string `json:"to,omitempty"`
}

// The kinds of a catch-up's cause (CaughtUpCause.Kind).
const (
	CaughtUpCommits = "commits"
	// CaughtUpRewritten: HEAD moved, but the HEAD CWM saw last is not an
	// ancestor of this one (a rebase, a reset, a force-pull) or git no longer
	// has it (a gc, a shallow clone): no commit range says what moved - the
	// manifest diff does. From and To are the two HEADs, never a range.
	CaughtUpRewritten   = "rewritten"
	CaughtUpWorkingTree = "working-tree"
)

// ObjectRaisedEventPayload accompanies `object.<token>.event` — one per
// EventRaise: an event of a declared type raised on an object, which it does
// not change (Revision is the object's current one). Summary is the payload
// as compact JSON, cut at NoteMax: what the journal shows.
type ObjectRaisedEventPayload struct {
	EventMeta
	ObjectID string `json:"object_id"`
	Module   string `json:"module"`
	Type     string `json:"type,omitempty"`
	Event    string `json:"event"             jsonschema:"The event type's name (code.changed)"`
	Summary  string `json:"summary,omitempty" jsonschema:"The payload as compact JSON; cut at 200 characters"`
}

// NoteMax bounds a short text a record carries about an event or a run (a
// raised event's summary; the note of an invoked action), in bytes.
const NoteMax = 200

// CutNote is s cut to NoteMax bytes on a rune boundary, marked "…" when cut.
func CutNote(s string) string {
	if len(s) <= NoteMax {
		return s
	}
	cut := NoteMax
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + "…"
}

// Operations: the writes an operation's own event already reports.
const (
	// OperationMove marks the moved object's own writes.
	OperationMove = "move"
	// OperationSplit marks the source's removal by a split.
	OperationSplit = "split"
	// OperationTransition marks a transition's own write (the state and the
	// history entry): its on_update still fires, and a subscriber that wants
	// only edits skips it on event.payload.operation.
	OperationTransition = "transition"
	// OperationCaughtUp marks the per-object event of an object a catch-up
	// found moved outside the verbs (a pull, an editor): no verb wrote it,
	// the `project.caught-up` event groups it, and reactions run on it as on
	// a verb write's event.
	OperationCaughtUp = "caught-up"
)

type operationKey struct{}

// WithOperation marks the writes made under ctx as part of an operation that
// publishes its own event (OperationMove, OperationSplit, OperationTransition).
func WithOperation(ctx context.Context, operation string) context.Context {
	return context.WithValue(ctx, operationKey{}, operation)
}

// OperationFrom is the operation WithOperation set, or "".
func OperationFrom(ctx context.Context) string {
	c, _ := ctx.Value(operationKey{}).(string)
	return c
}

// EventObserver is an in-process reaction to CWM's lifecycle events — the
// "react" channel an extension plugs into. CWM attaches every contributed
// observer when its service is created, in that process: the process that
// holds the workspace and makes the writes, so an observer that records (the
// journal) is the one writer and needs no bus.
type EventObserver struct {
	// Name identifies the observer in logs.
	Name string
	// TopicPattern is an EventsSubscribe pattern; "" = every event.
	TopicPattern string
	// Handle receives each event in order. ctx carries the application, so
	// Handle may resolve services (CWM included) with application.UseService.
	// With several projects served, ctx names the project whose store wrote
	// the event (EventProject); each store's events reach its observers once.
	Handle func(ctx context.Context, ev EventEnvelope)
}

type eventProjectCtxKey struct{}

// WithEventProject names, in an observer's ctx, the project key of the store
// whose event it handles (CWM sets it; "" leaves ctx as it is).
func WithEventProject(ctx context.Context, key string) context.Context {
	if key == "" {
		return ctx
	}
	return context.WithValue(ctx, eventProjectCtxKey{}, key)
}

// EventProject is the project key of the store whose event an observer
// handles: its writes in reaction go to that project, qualified
// <key>:<module>. "" when the universe serves one project (write bare).
func EventProject(ctx context.Context) string {
	k, _ := ctx.Value(eventProjectCtxKey{}).(string)
	return k
}
