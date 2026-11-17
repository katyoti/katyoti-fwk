// SPDX-License-Identifier: Apache-2.0

package panel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/katyoti/katyoti-fwk/internal/core/render"
)

// THE GENERIC PANEL TYPES: what KaTyoti ships for any task -
// steps with their state and a bar, a log that follows the work, an inline
// question whose answer goes back to the task, and the chat. Each is a panel
// type like any pack's (TypeDecl, published as `ktw-agents:/panel-type/<name>`
// by the panel server's pack), its page tree inline; no screen holds code of
// one of them. A panel opened with one of these types and no tree of its own
// shows the type's tree: its open event carries it, so every screen draws it
// with nothing to resolve, and the producer patches it by the nodes named
// here (StepsNode, LogNode, QuestionNode).

// The generic panel types' URIs.
const (
	TypeProgress = "ktw-agents:/panel-type/progress"
	TypeLog      = "ktw-agents:/panel-type/log"
	TypeQuestion = "ktw-agents:/panel-type/question"
	TypeChat     = "ktw-agents:/panel-type/chat"
)

// The nodes of the generic types' trees a producer patches.
const (
	// StepsNode is the progress type's steps: a progress-list drawn as rows
	// under one bar, its stages merged (Handle.Steps).
	StepsNode = "steps"
	// LogNode is the log type's lines: a log node, its lines appended
	// (Handle.Log).
	LogNode = "log"
	// QuestionNode is the question type's context, above the question its
	// ask puts: an empty box a producer replaces.
	QuestionNode = "question"
	// ChatNode and ChatArtifactsNode are the chat type's root and its
	// conversation: the box holding one node per text message and per UI
	// artifact, in the order they came (internal/kts/protocol/terminal/chatpanel).
	ChatNode          = "chat"
	ChatArtifactsNode = "chat:artifacts"
)

// The states of a step.
const (
	StepWaiting = "waiting"
	StepRunning = "running"
	StepDone    = "done"
	StepFailed  = "failed"
	StepSkipped = "skipped"
)

// Step is one step of the work as the progress type shows it - a row of its
// progress-list (the display library's UIProgressStage): its label, its state
// and what it says; Done of Total when it counts.
type Step struct {
	ID      string `json:"id"`
	Label   string `json:"label"`
	State   string `json:"state,omitempty"`
	Done    int64  `json:"done,omitempty"`
	Total   int64  `json:"total,omitempty"`
	Message string `json:"message,omitempty"`
	MS      int64  `json:"ms,omitempty"`
}

// genericTypes are the types, their trees built fresh on each call.
func genericTypes() map[string]TypeDecl {
	return map[string]TypeDecl{
		TypeProgress: {
			Label:       "Étapes",
			Description: "The steps of a piece of work with their state under one bar - an install, a publication - and its question when it asks one.",
			Tree: render.Node{"kind": "vbox", "id": "progress", "children": []any{
				render.Node{"kind": "progress-list", "id": StepsNode, "layout": "rows", "bar": true, "stages": []any{}},
			}},
			Inputs: []string{string(InputSubmit), string(InputAction), string(InputCancel)},
		},
		TypeLog: {
			Label:       "Journal",
			Description: "The lines a piece of work writes as it goes, the last ones shown and a line repeated counted once.",
			Tree: render.Node{"kind": "vbox", "id": "log-panel", "children": []any{
				render.Node{"kind": "log", "id": LogNode, "data": []any{}},
			}},
			Inputs: []string{string(InputCancel)},
		},
		TypeQuestion: {
			Label:       "Question",
			Description: "A question a task puts to the person - its choices or its form - whose answer goes back to the task.",
			Tree:        render.Node{"kind": "vbox", "id": QuestionNode, "children": []any{}},
			Inputs:      []string{string(InputSubmit), string(InputAction), string(InputCancel)},
		},
		TypeChat: {
			Label:       "Conversation",
			Description: "A run's conversation: the messages and the UI artifacts the agent shows, with the box the person writes in.",
			Tree: render.Node{"kind": "vbox", "id": ChatNode, "children": []any{
				render.Node{"kind": "vbox", "id": ChatArtifactsNode, "children": []any{}},
			}},
			Inputs: []string{string(InputMessage), string(InputCancel)},
		},
	}
}

// GenericTypes are the generic panel types by URI.
func GenericTypes() map[string]TypeDecl { return genericTypes() }

// GenericTypeURIs are their URIs, sorted.
func GenericTypeURIs() []string {
	out := make([]string, 0, 4)
	for u := range genericTypes() {
		out = append(out, u)
	}
	sort.Strings(out)
	return out
}

// GenericTree is the page tree of the generic type typeURI, a fresh copy;
// ok false for any other type.
func GenericTree(typeURI string) (render.Node, bool) {
	d, ok := genericTypes()[typeURI]
	if !ok {
		return nil, false
	}
	return render.Node(d.Tree), true
}

// withGenericTree gives an open of a generic type that holds no tree its
// type's tree; any other open is left as it is.
func withGenericTree(o *Open) {
	if o == nil || o.Answer.Tree != nil || o.Answer.Panel != "" {
		return
	}
	if tree, ok := GenericTree(o.Type); ok {
		s := render.InlineSpec(tree)
		o.Answer.Tree = &s
	}
}

// Steps shows the steps of the work on a panel of the progress type: its
// rows replaced by steps, in order, the bar following them.
func (h *Handle) Steps(ctx context.Context, steps ...Step) error {
	stages := make([]any, 0, len(steps))
	for _, s := range steps {
		stages = append(stages, s)
	}
	return h.Patch(ctx, render.Op{Op: render.OpMerge, Target: StepsNode, Value: map[string]any{"stages": stages}})
}

// Log adds lines to a panel of the log type.
func (h *Handle) Log(ctx context.Context, lines ...string) error {
	if len(lines) == 0 {
		return nil
	}
	items := make([]any, 0, len(lines))
	for _, l := range lines {
		items = append(items, l)
	}
	return h.Patch(ctx, render.Op{Op: render.OpAppend, Target: LogNode, Items: items})
}

// ErrCancelled is what Question answers when the person cancels instead.
var ErrCancelled = errors.New("panel: the person cancelled")

// Question puts ask to the person and waits for the answer: the input
// subscribed first (subscribe, over the bus), then the ask sent, then the
// first submit or action taken - the panel runs again (Answered). A cancel
// answers ErrCancelled with the input; an end of ctx its error. A wait that
// must outlive the process is the engine's (a run's durable wait), not this.
func (h *Handle) Question(ctx context.Context, ask Ask, subscribe Subscribe) (Input, error) {
	if subscribe == nil {
		return Input{}, fmt.Errorf("panel %s: a question needs its input subscribed", h.id)
	}
	got := make(chan Input, 1)
	stop, err := subscribe(ctx, h.InputSubject(), func(payload []byte) {
		var in Input
		if json.Unmarshal(payload, &in) != nil || in.Validate() != nil || in.Panel != h.id {
			return
		}
		switch in.Kind {
		case InputSubmit, InputAction, InputCancel:
			select {
			case got <- in:
			default:
			}
		}
	})
	if err != nil {
		return Input{}, fmt.Errorf("panel %s: subscribe to its input: %w", h.id, err)
	}
	defer stop()
	if err := h.Ask(ctx, ask); err != nil {
		return Input{}, err
	}
	select {
	case in := <-got:
		h.Answered()
		if in.Kind == InputCancel {
			return in, ErrCancelled
		}
		return in, nil
	case <-ctx.Done():
		return Input{}, ctx.Err()
	}
}
