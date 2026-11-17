// SPDX-License-Identifier: Apache-2.0

// Package panel is the display panels' API: a screen with a
// unique id that a running task opens, updates by events and reads the
// person's input from, shown by any screen by that id. A panel's events carry
// the one answer and the one op set of the page DSL (render.Answer, render.Op,
// ) - this package adds the panel's identity, its lifecycle and
// the transport around them, no second answer shape and no other op.
//
// Events go out on subjects.PanelOutSubject, each with the panel's sequence
// number; the person's input comes back on subjects.PanelInSubject. A screen
// that joins late follows from a snapshot with the events package's
// PanelFollower.
//
// It imports only render and the subject builders, so a sidecar shows a panel
// with this package alone.
package panel

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/katyoti/katyoti-fwk/internal/core/render"
)

// IDPrefix starts every panel id.
const IDPrefix = "dp-"

// NewID is a fresh panel id: dp- and a time-ordered UUID (v7), a valid
// subject token.
func NewID() string {
	u, err := uuid.NewV7()
	if err != nil {
		u = uuid.New()
	}
	return IDPrefix + u.String()
}

// ValidID reports whether id is a panel id this package makes.
func ValidID(id string) bool {
	rest, ok := strings.CutPrefix(id, IDPrefix)
	if !ok {
		return false
	}
	_, err := uuid.Parse(rest)
	return err == nil && strings.ToLower(rest) == rest
}

// State is where a panel is: open (running), asking (waiting for the
// person's input), complete (its work ended), closed.
type State string

const (
	StateOpen     State = "open"
	StateAsking   State = "asking"
	StateComplete State = "complete"
	StateClosed   State = "closed"
)

// Kind is what an event out of a panel does.
type Kind string

const (
	// KindOpen opens the panel: its type and title, its owner, its answer.
	KindOpen Kind = "open"
	// KindOps patches what the panel shows, by node id (render.Op).
	KindOps Kind = "ops"
	// KindProgress says how far the work is.
	KindProgress Kind = "progress"
	// KindAsk asks the person: the panel waits for an input.
	KindAsk Kind = "ask"
	// KindComplete ends the work: its result and status.
	KindComplete Kind = "complete"
	// KindClose closes the panel.
	KindClose Kind = "close"
)

// Open is what an open event carries: the panel type (a panel-type
// contribution URI) or an inline answer, its title, who owns it, the object
// its work is on and what it first shows.
//
// Object is the CWM object the run works on (its id): when the panel's type
// keeps its outcome (TypeDecl.Keep), or the person asks to keep it, the
// panel server writes the outcome as a document attached to that object
// once the panel ends. No object, nothing is kept.
type Open struct {
	Type   string        `json:"type,omitempty"`
	Title  string        `json:"title,omitempty"`
	Owner  string        `json:"owner,omitempty"`
	Parent string        `json:"parent,omitempty"`
	Object string        `json:"object,omitempty"`
	Answer render.Answer `json:"answer"`
}

// Progress is how far the work is: a label, current of total (total 0 when
// it is not known).
type Progress struct {
	Label   string `json:"label,omitempty"`
	Current int64  `json:"current"`
	Total   int64  `json:"total,omitempty"`
}

// AskAction is one answer an ask offers as a button.
type AskAction struct {
	Name  string `json:"name"`
	Label string `json:"label,omitempty"`
}

// Ask is a question the panel puts to the person: its text, a form (a data
// type's spec, render.Spec) and the actions offered. The panel waits for an
// input; the owning run waits on it durably.
type Ask struct {
	Question string       `json:"question,omitempty"`
	Form     *render.Spec `json:"form,omitempty"`
	Value    any          `json:"value,omitempty"`
	Actions  []AskAction  `json:"actions,omitempty"`
}

// Complete is how the work ended: its status (ok, failed, cancelled) and
// its result.
type Complete struct {
	Status string `json:"status"`
	Result any    `json:"result,omitempty"`
}

// Event is one event out of a panel, in the order of its Seq (1 for open).
type Event struct {
	Panel    string      `json:"panel"`
	Seq      int64       `json:"seq"`
	Kind     Kind        `json:"kind"`
	Open     *Open       `json:"open,omitempty"`
	Ops      []render.Op `json:"ops,omitempty"`
	Progress *Progress   `json:"progress,omitempty"`
	Ask      *Ask        `json:"ask,omitempty"`
	Complete *Complete   `json:"complete,omitempty"`
}

// Validate refuses an event that is not one of its kind: a bad panel id, a
// sequence below 1, an open that is not the first or holds no type and no
// answer, ops off the op set, a progress or an ask or a complete without
// its body.
func (e Event) Validate() error {
	if !ValidID(e.Panel) {
		return fmt.Errorf("panel: event: %q is no panel id", e.Panel)
	}
	if e.Seq < 1 {
		return fmt.Errorf("panel %s: event seq %d: sequences start at 1", e.Panel, e.Seq)
	}
	switch e.Kind {
	case KindOpen:
		if e.Seq != 1 || e.Open == nil {
			return fmt.Errorf("panel %s: open is its first event and carries its open", e.Panel)
		}
		if e.Open.Type == "" {
			if err := e.Open.Answer.Validate(); err != nil {
				return fmt.Errorf("panel %s: open: a type or an answer: %w", e.Panel, err)
			}
		}
	case KindOps:
		if len(e.Ops) == 0 {
			return fmt.Errorf("panel %s: ops without an op", e.Panel)
		}
		for _, o := range e.Ops {
			if err := o.Validate(); err != nil {
				return fmt.Errorf("panel %s: %w", e.Panel, err)
			}
		}
	case KindProgress:
		if e.Progress == nil {
			return fmt.Errorf("panel %s: progress without its progress", e.Panel)
		}
	case KindAsk:
		if e.Ask == nil || (e.Ask.Question == "" && e.Ask.Form == nil && len(e.Ask.Actions) == 0) {
			return fmt.Errorf("panel %s: an ask asks something: a question, a form or actions", e.Panel)
		}
	case KindComplete:
		if e.Complete == nil || e.Complete.Status == "" {
			return fmt.Errorf("panel %s: complete says its status", e.Panel)
		}
	case KindClose:
	default:
		return fmt.Errorf("panel %s: unknown event %q", e.Panel, e.Kind)
	}
	if e.Seq == 1 && e.Kind != KindOpen {
		return fmt.Errorf("panel %s: the first event opens the panel", e.Panel)
	}
	return nil
}

// InputKind is what the person sent back to a panel.
type InputKind string

const (
	// InputSubmit is a form's value.
	InputSubmit InputKind = "submit"
	// InputAction is a button: its name and value.
	InputAction InputKind = "action"
	// InputMessage is a chat turn.
	InputMessage InputKind = "message"
	// InputCancel asks the owning run to stop its work.
	InputCancel InputKind = "cancel"
)

// Input is the person's input to a panel, on subjects.PanelInSubject.
type Input struct {
	Panel string    `json:"panel"`
	Kind  InputKind `json:"kind"`
	Name  string    `json:"name,omitempty"`
	Value any       `json:"value,omitempty"`
	Text  string    `json:"text,omitempty"`
	By    string    `json:"by,omitempty"`
}

// Validate refuses an input that is not one of its kind.
func (in Input) Validate() error {
	if !ValidID(in.Panel) {
		return fmt.Errorf("panel: input: %q is no panel id", in.Panel)
	}
	switch in.Kind {
	case InputSubmit, InputCancel:
	case InputAction:
		if in.Name == "" {
			return fmt.Errorf("panel %s: an action input names its action", in.Panel)
		}
	case InputMessage:
		if in.Text == "" {
			return fmt.Errorf("panel %s: a message input carries its text", in.Panel)
		}
	default:
		return fmt.Errorf("panel %s: unknown input %q", in.Panel, in.Kind)
	}
	return nil
}

// DecodeInput reads an input from its payload, validated.
func DecodeInput(payload []byte) (Input, error) {
	var in Input
	if err := json.Unmarshal(payload, &in); err != nil {
		return Input{}, fmt.Errorf("panel: input: %w", err)
	}
	return in, in.Validate()
}

// DecodeEvent reads an event from its payload, validated.
func DecodeEvent(payload []byte) (Event, error) {
	var e Event
	if err := json.Unmarshal(payload, &e); err != nil {
		return Event{}, fmt.Errorf("panel: event: %w", err)
	}
	return e, e.Validate()
}

// SeqOf is an event's sequence number - what a PanelFollower reads.
func SeqOf(e Event) int64 { return e.Seq }
