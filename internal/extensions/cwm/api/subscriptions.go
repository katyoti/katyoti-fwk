// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/cause"
)

// ASYNCHRONOUS SUBSCRIPTIONS. An on_* hook entry that declares `async` is a
// SUBSCRIPTION: CWM never runs it. At the write, CWM records a MATCH — the
// subscription, the event, the object, the coalescing key, the cause and the
// depth — durably (`.katyoti/cwm/matches/`) before the verb returns, and hands
// it over in the background to the MatchTaker an extension contributes (the
// events extension, which puts the reaction in the module's queue). CWM records
// and guards; it never decides what runs next.
//
// The guards, CWM's because only CWM sees every write and its cause:
//   - DEPTH: a write at depth >= ReactionDepthLimit matches no subscription
//     (on_* hooks are not bounded by it);
//   - NO SELF-REACTION: a write caused by a reaction of subscription S never
//     matches S;
//   - CAUSED EVENTS: a write made by a reaction or a hook (depth > 0) matches
//     only a subscription that says AcceptsCaused;
//   - COALESCING: a pending match not yet taken with the same subscription and
//     key gets the new event appended instead of a second match.
//
// THE OUTBOX. A write whose cause is a reaction stages what it can trigger —
// its async matches and its on_* hook calls — under the reaction
// (`.katyoti/cwm/outbox/`), coalesced per object; its object event still goes
// out at once (the journal, the GUI). The reaction's collector releases the
// outbox on success (OutboxRelease), drops it when the reaction is given up
// (OutboxDrop); over failed attempts it is held and accumulates.

// ReactionDepthLimit is the depth at which a write matches no asynchronous
// subscription: a reaction runs one level deeper than the write it reacts to,
// so at most this many reactions chain from one write made with no cause.
const ReactionDepthLimit = 3

// Reaction priorities (an async entry's `priority`); P2 by default.
const (
	PriorityP0 = "P0"
	PriorityP1 = "P1"
	PriorityP2 = "P2"
	PriorityP3 = "P3"
)

// Async defaults.
const (
	DefaultAsyncPriority = PriorityP2
	DefaultAsyncKey      = "${id}"
	DefaultAsyncAttempts = 3
)

// AsyncSpec is what makes an on_* hook entry a subscription: run later, by the
// scheduler, instead of dispatched after the write.
type AsyncSpec struct {
	Priority      string `json:"priority,omitempty"       yaml:"priority,omitempty"       jsonschema:"The reaction's priority: P0 | P1 | P2 | P3; default P2"`
	Key           string `json:"key,omitempty"            yaml:"key,omitempty"            jsonschema:"The coalescing key: an el interpolation over the object; default ${id} (one pending reaction per object)"`
	Attempts      int    `json:"attempts,omitempty"       yaml:"attempts,omitempty"       jsonschema:"How many attempts before the reaction is given up; default 3"`
	AcceptsCaused bool   `json:"accepts_caused,omitempty" yaml:"accepts_caused,omitempty" jsonschema:"True also matches writes made by a reaction or a hook (depth above 0)"`
	Atomic        bool   `json:"atomic,omitempty"         yaml:"atomic,omitempty"         jsonschema:"True stages the reaction's object writes in a change set: committed together after its action and before it reports success; none written when it fails"`
}

// WithDefaults is the spec with every unset field at its default.
func (a AsyncSpec) WithDefaults() AsyncSpec {
	if a.Priority == "" {
		a.Priority = DefaultAsyncPriority
	}
	if a.Key == "" {
		a.Key = DefaultAsyncKey
	}
	if a.Attempts <= 0 {
		a.Attempts = DefaultAsyncAttempts
	}
	return a
}

// SubscriptionID names one async entry in an object's hook table:
// `<source>#<point>#<uri>` — who declared it, where, what it runs.
func SubscriptionID(source string, point HookPoint, uri string) string {
	return source + "#" + string(point) + "#" + uri
}

// ReactionID identifies a reaction: one subscription matched by one event.
// It is the id of the match that records it and the id of the cause its writes
// carry.
type ReactionID struct {
	Subscription string `json:"subscription"`
	EventID      string `json:"event_id"`
}

// String renders `<subscription>@<event-id>`.
func (r ReactionID) String() string { return r.Subscription + "@" + r.EventID }

// ParseReactionID reads String's form back; false when s is not one. The event
// id is the text after the LAST `@` (an event id never holds one).
func ParseReactionID(s string) (ReactionID, bool) {
	i := strings.LastIndex(s, "@")
	if i <= 0 || i == len(s)-1 {
		return ReactionID{}, false
	}
	return ReactionID{Subscription: s[:i], EventID: s[i+1:]}, true
}

// SubscriptionRef is the subscription a match records, as it applied to the
// object at the write.
type SubscriptionRef struct {
	ID       string    `json:"id"       jsonschema:"<source>#<point>#<uri>"`
	Source   string    `json:"source"   jsonschema:"Who declared it: hook:<provider>/<name> | capability:<id> | policy:<name> | type:<name> | object:<id> | module:<subscriber>"`
	Point    HookPoint `json:"point"`
	URI      string    `json:"uri"      jsonschema:"The action the reaction runs"`
	Provider string    `json:"provider,omitempty"`
	// Module is the SUBSCRIBER: the module whose reflex it is — the module
	// written for an entry the emitter's objects carry, the declaring module
	// object for a cross-module one (source module:<subscriber>). Match.Module
	// stays the emitter's.
	Module string `json:"module,omitempty" jsonschema:"The subscriber: the module whose reflex it is"`
	// Writes is, for a cross-module subscription, the modules its reactions
	// may write (default the subscriber's); a write into any other is refused.
	// Empty for a module's own subscription.
	Writes   []string `json:"writes,omitempty" jsonschema:"A cross-module subscription's: the modules its reactions may write"`
	Priority string   `json:"priority"`
	Attempts int      `json:"attempts"`
	// AcceptsCaused is the entry's own: whether caused writes match it.
	AcceptsCaused bool `json:"accepts_caused,omitempty"`
	// Atomic is the entry's own: whether the reaction's writes go to a change
	// set (ChangeSetBegin / ChangeSetCommit) its runner opens and commits.
	Atomic bool `json:"atomic,omitempty"`
	// ResolvedInput is the entry's declared input resolved over the object at
	// the write; empty when it declares none (the taker decides what the
	// action receives — the match's events, typically).
	ResolvedInput map[string]any `json:"resolved_input,omitempty"`
}

// MatchedEvent is one event a match carries: its EventMeta (its id, time,
// revision and cause) and what it was about — an object write (Action
// created / updated / deleted; Event object.created …), a move of its
// lifecycle (Action transitioned; Event object.transitioned), a move its guard
// refused (Action transition-refused; Event object.transition-refused) or an
// event raised on the object (Action event; Event the event type; Payload
// what the raiser sent).
type MatchedEvent struct {
	EventMeta
	Action   string `json:"action"   jsonschema:"created | updated | deleted | transitioned | transition-refused | event"`
	Type     string `json:"type,omitempty"`
	ObjectID string `json:"object_id"`
	Module   string `json:"module"`
	// Event is the event type's name: object.created … for a write,
	// object.transitioned / object.transition-refused for a move, the raised
	// type for EventRaise.
	Event string `json:"event,omitempty"`
	// Payload is the event's own data as JSON: for a write, {operation} when
	// the write is part of an operation (move, split, transition) and empty
	// otherwise; for a move, the transitioned or transition-refused event as
	// published (from, to, transition, workflow, capability, by, note, forced;
	// a refusal's guard, reason and calls); for a raised event, its payload
	// as sent.
	Payload json.RawMessage `json:"payload,omitempty"`
}

// The states of a match.
const (
	// MatchPending: recorded, waiting to be taken (or staged in an outbox).
	MatchPending = "pending"
)

// Match is one async subscription matched by one write — or, coalesced, by
// several writes on the same key while it waited.
type Match struct {
	// ID is the ReactionID of its FIRST event.
	ID           string          `json:"id"`
	Subscription SubscriptionRef `json:"subscription"`
	// Module is the module of the object written: the reaction is its work.
	Module   string `json:"module"`
	ObjectID string `json:"object_id"`
	// Key is the coalescing key, resolved over the object.
	Key string `json:"key"`
	// Depth is the depth of the write that matched; the reaction runs at
	// Depth + 1 (ReactionCause).
	Depth int `json:"depth"`
	// Cause is the first event's cause; absent for a write made with none.
	Cause  *cause.Cause   `json:"cause,omitempty"`
	Events []MatchedEvent `json:"events"`
	At     time.Time      `json:"at"`
	State  string         `json:"state"`
}

// ReactionCause is the cause a reaction's writes carry: {reaction, the match's
// id, the matching write's depth + 1}. Its writes stage what they trigger in
// the outbox keyed by the match's id; one at depth ReactionDepthLimit matches
// nothing, and none matches the reaction's own subscription.
func ReactionCause(m Match) cause.Cause {
	return cause.Cause{Kind: cause.KindReaction, ID: m.ID, Depth: m.Depth + 1}
}

// MatchTaker receives CWM's matches: the events extension contributes one
// (TakeMatches) and puts a reaction per match. Take returning nil
// means the match is taken — CWM forgets it; an error keeps it, retried with
// a backoff, then again at the next start. Take runs in the background, never
// on the write, and may receive a match again after a crash: it is keyed by the
// match's id.
type MatchTaker struct {
	// Name identifies the taker in logs; takers are tried in name order and
	// the first that takes a match takes it.
	Name string
	Take func(ctx context.Context, m Match) error
}

// OutboxReleaseInput releases a reaction's outbox: its staged matches are
// recorded and handed over, its staged on_* hook calls journaled and run.
type OutboxReleaseInput struct {
	Reaction string `json:"reaction" jsonschema:"The reaction's id (<subscription>@<event-id>); required"`
}

// OutboxReleaseOutput says what the release handed on.
type OutboxReleaseOutput struct {
	Matches int `json:"matches" jsonschema:"Matches recorded and handed over"`
	Hooks   int `json:"hooks"   jsonschema:"on_* hook calls journaled and run"`
}

// OutboxDropInput drops a reaction's outbox: the reaction was given up.
type OutboxDropInput struct {
	Reaction string `json:"reaction" jsonschema:"The reaction's id; required"`
}

// OutboxDropOutput lists the writes the reaction made, whose matches and hook
// calls were dropped, and the writes its open change set held, never written.
type OutboxDropOutput struct {
	Writes     []MatchedEvent   `json:"writes"`
	NotWritten []ChangeSetWrite `json:"not_written,omitempty" jsonschema:"The writes the reaction's open change set staged: dropped with it and never written"`
}

// OutboxListInput lists the outboxes held.
type OutboxListInput struct{}

// OutboxSummary is one reaction's outbox.
type OutboxSummary struct {
	Reaction string    `json:"reaction"`
	At       time.Time `json:"at"      jsonschema:"When its first write was staged"`
	Writes   int       `json:"writes"`
	Matches  int       `json:"matches"`
	Hooks    int       `json:"hooks"`
}

// OutboxListOutput is every outbox held, oldest first.
type OutboxListOutput struct {
	Outboxes []OutboxSummary `json:"outboxes"`
}
