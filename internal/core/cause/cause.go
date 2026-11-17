// SPDX-License-Identifier: Apache-2.0

// Package cause is WHY a write happens: the person, agent, hook, action,
// reaction or event that produced it, and how many reactions deep it sits.
//
// A cause travels on the context of the write — set once at an origin (a
// person's CLI or GUI, an agent run), carried across the bus as a header,
// re-set by every reaction (an on_* hook) one level deeper — and CWM stamps
// it on the object event it publishes and the journal on the record it
// keeps, so an object's history answers "why did this change?" and the
// reactive loop can see cascades and stop them at a depth.
//
// A write with no cause behaves exactly as before: nothing is stamped, nothing
// is refused. The kind is an open string: the constants here are the kinds the
// origins built today set; a reaction, an event or a request will set theirs.
//
// A neutral leaf: nothing but the standard library, so CWM, the bus, the
// journal and every origin can share it without one importing another.
package cause

import (
	"context"
	"fmt"
)

// The kinds of cause the built origins set.
const (
	// KindPerson: a person acting through a CLI or the GUI; the id is the
	// profile's handle.
	KindPerson = "person"
	// KindAgent: an agent run; the id is the run id.
	KindAgent = "agent"
	// KindHook: an on_* hook dispatched after a write; the id is the hook
	// journal entry's id, and the depth is the write's depth plus one.
	KindHook = "hook"
	// KindAction: an action run through the ActionService.
	KindAction = "action"
	// KindReaction: a reaction — one asynchronous subscription matched by one
	// event; the id is the reaction's id (`<subscription>@<event-id>`), the
	// depth the triggering write's depth plus one.
	KindReaction = "reaction"
)

// Cause is who or what produced a write, and how many reactions deep.
type Cause struct {
	Kind  string `json:"kind"`
	ID    string `json:"id"`
	Depth int    `json:"depth"`
	// Relay is set when an agent's session relays a person's own act, with
	// how the person confirmed it: what the guarded verbs accept
	// (person.AcceptRelayed) and record as it is.
	Relay *Relay `json:"relay,omitempty"`
	// Session is the agent's session a command runs from (Claude Code's
	// session id), "" when unknown: what tells two sessions acting as one
	// actor apart.
	Session string `json:"session,omitempty"`
}

// How a relayed act was confirmed, weakest first.
const (
	// ConfirmedPrompt: the person answered the agent client's permission
	// prompt on their own device (Claude Code's always-ask tool) — it proves
	// the prompt was answered, not who answered it.
	ConfirmedPrompt = "prompt"
	// ConfirmedTOTP: a code from the person's authenticator app, checked by
	// KaTyoti.
	ConfirmedTOTP = "totp"
	// ConfirmedEntra: a Microsoft Entra push approval on the person's phone.
	ConfirmedEntra = "entra"
	// ConfirmedPasskey: the one-tap approval on the person's phone, their
	// passkey signing the act's challenge with Face ID.
	ConfirmedPasskey = "passkey"
)

// Relay is a person's act relayed by an agent's session: whose act it is,
// which act (its key, `<verb>:<object id>`, the one a proof binds to), how it
// was confirmed and when, and the proof a code or a push carries.
type Relay struct {
	Person    string `json:"person"`
	Act       string `json:"act,omitempty"`
	Confirmed string `json:"confirmed"`
	At        string `json:"at,omitempty"`
	Proof     string `json:"proof,omitempty"`
	// Via is the act this one is part of, already accepted where it was asked
	// (an Inbox choice deciding its decision): set only by the serving code
	// (person.ForAct), never from a command's environment.
	Via string `json:"via,omitempty"`
}

// IsZero reports a cause that says nothing.
func (c Cause) IsZero() bool {
	return c.Kind == "" && c.ID == "" && c.Depth == 0 && c.Relay == nil && c.Session == ""
}

// String renders `kind:id@depth` — how a history line shows it — and, for a
// relayed act, ` relaying <person> (<confirmed>)`.
func (c Cause) String() string {
	if c.IsZero() {
		return ""
	}
	s := fmt.Sprintf("%s:%s@%d", c.Kind, c.ID, c.Depth)
	if c.Relay != nil {
		s += fmt.Sprintf(" relaying %s (%s)", c.Relay.Person, c.Relay.Confirmed)
	}
	return s
}

type ctxKey struct{}

// WithCause returns ctx carrying c: every write made under it is caused by c.
// A zero cause leaves ctx as it is.
func WithCause(ctx context.Context, c Cause) context.Context {
	if c.IsZero() {
		return ctx
	}
	return context.WithValue(ctx, ctxKey{}, c)
}

// Without returns ctx carrying no cause, whatever its parents carry: a
// process serving others (the hub, a sidecar serving the bus) and a call it
// serves are caused by their caller, never by the person whose profile the
// process happened to read at its start.
func Without(ctx context.Context) context.Context {
	if _, ok := CauseFrom(ctx); !ok {
		return ctx
	}
	return context.WithValue(ctx, ctxKey{}, Cause{})
}

// CauseFrom is the cause ctx carries, and whether it carries one.
func CauseFrom(ctx context.Context) (Cause, bool) {
	if ctx == nil {
		return Cause{}, false
	}
	c, ok := ctx.Value(ctxKey{}).(Cause)
	return c, ok && !c.IsZero()
}

// DepthFrom is the depth of ctx's cause, 0 when it carries none: what a
// reaction adds one to.
func DepthFrom(ctx context.Context) int {
	c, _ := CauseFrom(ctx)
	return c.Depth
}
