// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"

	"github.com/katyoti/katyoti-fwk/internal/core/meta"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// REACTIONS ON THE SCHEDULER. CWM records an asynchronous
// subscription matched by a write — a MATCH — and hands it over; the events
// extension puts it as a to-do of the module written (owner `events`, keyed by
// the match's id), runs it — the internal scheduler through the owner's runner,
// an outside worker through `reactions run` — under the reaction's cause, and
// collects it: success releases what its writes staged in CWM's outbox; a final
// failure holds it until a person retries or drops it. CWM never imports this
// extension; this extension writes CWM state through CWM's verbs only.

// Owner is the scheduler owner of the reaction to-dos.
const Owner = "events"

// Work packet keys of a reaction to-do.
const (
	WorkDoer       = "doer"
	WorkDoerValue  = "reaction"
	WorkURI        = "uri"
	WorkMatch      = "match"
	WorkInstructs  = "instructions"
	ReactionGroup  = "reactions"
	DefaultBackoff = 30 // seconds before a failed reaction is tried again
)

// ReactionRunInput runs a reaction to-do the caller holds.
type ReactionRunInput struct {
	Todo string       `json:"todo" jsonschema:"description=The reaction to-do (pulled by the caller),required"`
	By   cwmapi.Actor `json:"by"   jsonschema:"description=Who runs it: the holder of the to-do,required"`
}

// ReactionRunOutput is what running it gave.
type ReactionRunOutput struct {
	Todo    string `json:"todo"`
	Status  string `json:"status"            jsonschema:"description=success or fail - or started (a task or agent run: reported when it ends)"`
	Summary string `json:"summary"`
	Run     string `json:"run,omitempty"     jsonschema:"description=The task run a task or agent action started"`
}

// ReactionRetryInput sends a reaction given up back to the queue.
type ReactionRetryInput struct {
	Todo string       `json:"todo" jsonschema:"description=The reaction to-do,required"`
	By   cwmapi.Actor `json:"by"   jsonschema:"description=Who asks,required"`
}

// ReactionDropInput gives a reaction up: its outbox dropped, the to-do archived.
type ReactionDropInput struct {
	Todo string       `json:"todo" jsonschema:"description=The reaction to-do,required"`
	By   cwmapi.Actor `json:"by"   jsonschema:"description=Who gives it up,required"`
	Why  string       `json:"why"  jsonschema:"description=Why it is given up,required"`
}

// ReactionDropOutput lists the writes the reaction made without their events.
type ReactionDropOutput struct {
	Todo   string                `json:"todo"`
	Writes []cwmapi.MatchedEvent `json:"writes"`
}

// ReactionListInput lists a module's reactions.
type ReactionListInput struct {
	Module string `json:"module" jsonschema:"description=The module,required"`
	Object string `json:"object,omitempty" jsonschema:"description=Only the reactions to this object's changes"`
}

// Reaction is one reaction to-do as the list shows it.
type Reaction struct {
	Todo         string   `json:"todo"`
	State        string   `json:"state"`
	Subscription string   `json:"subscription"`
	URI          string   `json:"uri"`
	Object       string   `json:"object"`
	Events       []string `json:"events"           jsonschema:"description=The events it reacts to: <event> <event id>"`
	Cause        string   `json:"cause,omitempty"  jsonschema:"description=The first event's cause"`
	Depth        int      `json:"depth"`
	Attempts     int      `json:"attempts,omitempty"`
	Result       string   `json:"result,omitempty"`
}

// ReactionListOutput is a module's open reactions.
type ReactionListOutput struct {
	Reactions []Reaction `json:"reactions"`
}

// ReactionService runs, retries, drops and lists reactions.
type ReactionService interface {
	Run(ctx context.Context, in ReactionRunInput) (ReactionRunOutput, error)
	Retry(ctx context.Context, in ReactionRetryInput) (ReactionRunOutput, error)
	Drop(ctx context.Context, in ReactionDropInput) (ReactionDropOutput, error)
	List(ctx context.Context, in ReactionListInput) (ReactionListOutput, error)
	Subscribers(ctx context.Context, in ReactionSubscribersInput) (cwmapi.ModuleSubscribersOutput, error)
	Events(ctx context.Context, in ReactionEventsInput) (ReactionEventsOutput, error)
}

// ReactionEventsInput asks for the declared event types and their subscribers.
type ReactionEventsInput struct {
	Event string `json:"event,omitempty" jsonschema:"description=One event type; empty: every declared type"`
}

// SourceState is what the events extension's sources last saw: the git HEAD
// (code.changed) and each timer's last firing (timer.elapsed).
type SourceState struct {
	Repo   string            `json:"repo,omitempty"`
	Head   string            `json:"head,omitempty"`
	Timers map[string]string `json:"timers,omitempty" jsonschema:"description=<object>#<timer> to its last firing (RFC 3339)"`
}

// EventView is one event type as the debugger shows it: CWM's subscriptions
// to it, the open reactions of each, and its source's state when the events
// pack raises it.
type EventView struct {
	cwmapi.EventTypeSubscriptions
	Open   map[string]int `json:"open,omitempty"   jsonschema:"description=Open reactions (to-dos not yet ended) per subscription id"`
	Source *SourceState   `json:"source,omitempty" jsonschema:"description=The source's state: the HEAD for code.changed and the timers for timer.elapsed"`
}

// ReactionEventsOutput is every event type asked.
type ReactionEventsOutput struct {
	Events []EventView `json:"events"`
}

// ReactionSubscribersInput lists who subscribes to a module's events.
type ReactionSubscribersInput struct {
	Module string `json:"module"          jsonschema:"description=The emitting module,required"`
	Event  string `json:"event,omitempty" jsonschema:"description=One event type; empty: every type"`
}

// ReactionServiceDef describes the service for KTSP / CLI.
var ReactionServiceDef = meta.ServiceDef{
	Name:    "reactions",
	Short:   "Reactions: an asynchronous subscription matched by a write, as a to-do of its module - run under its cause by the internal scheduler or an outside worker, its events released on success, held on failure",
	Version: "1.0.0",
	Methods: map[string]meta.MethodDef{
		"Run":         {Short: "Run a reaction to-do you hold (todos pull first): its action under the reaction's cause, reported through the contract", TimeoutS: 600},
		"Retry":       meta.Method("Send a reaction given up back to the queue, its attempts counted again"),
		"Drop":        meta.Method("Give a reaction up: its staged events dropped (the writes it made are listed), the to-do archived"),
		"List":        {Short: "A module's open reactions: their subscription, object, events, cause and state", ReadOnly: true},
		"Events":      {Short: "The declared event types and who subscribes to each, in every layer (CWM's EventSubscriptions) - with their open reactions and the sources' state: what the debugger's Events tab shows", ReadOnly: true},
		"Subscribers": {Short: "Who subscribes to a module's events from another module: the event, the action, the modules its reactions may write, whether it applies and why not", ReadOnly: true},
	},
}
