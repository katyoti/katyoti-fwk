// SPDX-License-Identifier: Apache-2.0

// Package ask_user implements the ask_user agent tool for user
// prompts. The tool is channel-agnostic — it dispatches to one of
// three implementations (interactive / inbox / silent) based on the
// resolved `UserComms.Input` value in ctx. Each channel owns its
// own file:
//
//   - interactive_ask.go — HumanGate path (UserCommsInput=`direct` or empty)
//   - inbox_ask.go       — async inbox routing (`inbox`)
//   - silent_ask.go      — ErrNoUserChannel (`none`)
//
// The standalone Tool-API version of ask_user that previously lived
// embedded in the humangate plugin is still the reference
// (internal/extensions/humangate/plugin_ext.go).
package ask_user

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
	inboxapi "github.com/katyoti/katyoti-fwk/internal/extensions/inbox/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
	toolsapi "github.com/katyoti/katyoti-fwk/internal/extensions/tools/api"
)

// AskInput is the input schema for the ask_user tool.
type AskInput struct {
	// Question is the prompt text shown to the user. Empty question
	// is the "just wait for the user's next message" signal — used by
	// chat-style keepalive sessions where the agent has no specific
	// question to ask but needs to yield control back to the user.
	Question string `json:"question" jsonschema:"The question to ask the user (empty = yield to user, wait for next message)"`

	// Prompt is an accepted alias for Question. Several models
	// (observed with GPT-class models) emit the prompt text under
	// `prompt` rather than `question` regardless of the advertised
	// schema; accepting both keeps the question from being silently
	// dropped. Resolve via PromptText(), never read these fields
	// directly.
	Prompt string `json:"prompt,omitempty" jsonschema:"Alias for 'question' — the prompt text shown to the user"`

	// Type selects the prompt style: "text" (free-form), "select"
	// (pick from options), "confirm" (yes/no), "form" (schema-driven).
	// Default: "text". "form" requires the FormDef field and is only
	// available when the UI supports it (GUI mode).
	Type string `json:"type,omitempty" jsonschema:"Prompt type: 'text' (default), 'select', 'confirm', or 'form'"`

	// Options is the list of choices for a select prompt. Required
	// when Type is "select"; ignored otherwise.
	Options []string `json:"options,omitempty" jsonschema:"Available choices (required for 'select' type)"`

	// FormDef is a FormDefinition YAML string for type "form".
	// The YAML defines fields, groups (tabs), validation, and defaults.
	// Only available when the UI supports form prompts.
	FormDef string `json:"form_def,omitempty" jsonschema:"FormDefinition YAML content (required for 'form' type)"`
}

// PromptText returns the question text, accepting either the canonical
// `question` field or the `prompt` alias. The single source of truth
// for "what did the agent ask" — every consumer (the channel impls,
// the prompt_shown event) must go through it so a model's choice of
// field name never strands the question.
func (in AskInput) PromptText() string {
	if in.Question != "" {
		return in.Question
	}
	return in.Prompt
}

// AskOutput is the output of the ask_user tool.
type AskOutput struct {
	// Answer is the user's response. For text prompts, it's the
	// free-form text. For select prompts, it's the selected option.
	// For confirm prompts, it's "true" or "false".
	Answer string `json:"answer" jsonschema:"The user's response"`

	// Type echoes the prompt type that was used.
	Type string `json:"type" jsonschema:"The prompt type that was used"`

	// Dismissed is true when the person dismissed the question without
	// answering it (an inbox request): Answer then says so, it is no answer.
	Dismissed bool `json:"dismissed,omitempty" jsonschema:"True when the person dismissed the question without answering"`
}

// NewAskUserTool creates the ask_user tool. The tool resolves the
// channel implementation (interactive / inbox / silent) from ctx
// per-call via selectChannel; the registered tool object is
// channel-agnostic.
func NewAskUserTool() toolsapi.Tool {
	return toolsapi.NewFunctionTool(
		"ask-user",
		conversationDescription,
		executeAskUser,
		toolsapi.WithReadOnly(),
		toolsapi.WithInteractive(),
		toolsapi.WithIdle(),
		toolsapi.WithDescriber(describeAskUser),
		toolsapi.WithDisplay(toolsapi.ToolDisplay{Icon: "❓", Label: "Ask", Color: "#C792EA", Template: "{{.question}}"}),
	)
}

// executeAskUser is the channel-agnostic dispatcher. Resolution
// order:
//
//  1. Acquire the per-task in-flight slot. A second concurrent call
//     from the same task returns ErrUserAskInFlight (Cline race-bug
//     prevention).
//  2. Select the channel impl from UserCommsInputFromContext(ctx).
//     Empty / "direct" → interactive; "inbox" → inbox stub; "none" →
//     silent; "bubble_to_parent" / "via_leader" → deferred error
//     (channel pending future phase).
//  3. Delegate Ask to the selected impl.
func executeAskUser(ctx context.Context, in AskInput) (AskOutput, error) {
	key := loopIDFromContext(ctx)
	release, acquired := acquireInFlight(key)
	if !acquired {
		return AskOutput{}, ErrUserAskInFlight
	}
	defer release()

	channel, err := selectChannel(ctx)
	if err != nil {
		return AskOutput{}, err
	}
	if err := conversationOnly(ctx, in); err != nil {
		return AskOutput{}, err
	}
	// Called again to check the answer of an inbox request, the ask reads the
	// inbox, whatever the run's channel: a direct ask whose person left fell
	// back to the inbox, and its answer is there — the person is not asked
	// again.
	if resume, ok := agentapi.AwaitResumeFrom(ctx); ok && resume[requestKey] != nil {
		channel = &inboxChannel{}
	}
	return askWithEvents(ctx, channel, in)
}

// askWithEvents wraps a channel's Ask with the tool's own conversation
// events: prompt_shown BEFORE the (potentially blocking) Ask so UIs can
// render the question while the gate waits, user_input after the answer
// arrives. Published directly on the EventBus (not via the loop's emit
// batch) for the same reason: subscribers must see the prompt before
// Ask blocks.
func askWithEvents(ctx context.Context, channel AskUserChannel, in AskInput) (AskOutput, error) {
	turn := agentapi.CurrentTurnFromContext(ctx)
	// An ask called again to check its answer (an awaiting loop) showed its
	// question the first time.
	if _, again := agentapi.AwaitResumeFrom(ctx); !again {
		publishEvent(ctx, agentapi.EventPromptShown, agentapi.PromptShownEvent{
			EventBase:  eventBase(ctx, agentapi.EventPromptShown),
			Turn:       turn,
			Question:   in.PromptText(),
			PromptType: in.Type,
			Options:    in.Options,
			Channel:    agentapi.UserCommsInputFromContext(ctx),
		})
	}

	out, err := channel.Ask(ctx, in)
	if err != nil {
		// A CLOSED user channel (EOF — stdin exhausted in a non-interactive run)
		// is not a retryable tool error: there is no user left to answer, so the
		// session ENDS gracefully instead of the chat keepalive looping it and
		// burning a model call every turn (the headless-chat runaway). A
		// headless chat that must PROGRESS supplies its own user channel (the
		// test-harness user-simulator); absent one, EOF == "user closed the chat".
		if errors.Is(err, io.EOF) {
			return out, &agentapi.ErrRunComplete{Summary: "user channel closed (EOF) — chat session ended"}
		}
		return out, err
	}
	publishEvent(ctx, agentapi.EventUserInput, agentapi.UserInputEvent{
		EventBase: eventBase(ctx, agentapi.EventUserInput),
		Turn:      turn,
		Answer:    out.Answer,
	})
	return out, nil
}

// busFromContext resolves the EventBus from the application context.
// Package seam so tests can substitute an in-memory bus.
var busFromContext = func(ctx context.Context) engine.EventBus {
	bus, err := application.UseService[engine.EventBus](ctx)
	if err != nil {
		return nil
	}
	return bus
}

// publishEvent sends a conversation event directly to the EventBus.
// Missing bus (programmatic callers outside an agent loop, unit tests)
// is fine — the ask still works, it's just unobserved.
func publishEvent(ctx context.Context, topic string, payload any) {
	if bus := busFromContext(ctx); bus != nil {
		_ = bus.Publish(topic, payload)
	}
}

func eventBase(ctx context.Context, eventType string) agentapi.EventBase {
	// Stamp the run id as the emitter so the per-session translator can scope
	// these tool-emitted events to their own session (otherwise an empty
	// emitter leaks the prompt into every parallel chat).
	return agentapi.EventBase{Type: eventType, Time: time.Now(), Emitter: agentapi.RunIDFromContext(ctx)}
}

// ErrNotAConversation refuses an ask that is no conversation once IN-039's
// switch is on (decision): a structured prompt
// (select, confirm, form) is a validation, and a run with no conversation
// (inbox, none) has no one to talk to - both are asked with
// request-validation.
var ErrNotAConversation = errors.New("ask-user: a conversation only")

// conversationOnly is ask-user narrowed to a conversation:
// only a text, and only in a run that has a conversation - the live chat, or
// the run that spawned it. An ask called again for its answer is never
// refused: it was asked already.
func conversationOnly(ctx context.Context, in AskInput) error {
	if _, again := agentapi.AwaitResumeFrom(ctx); again {
		return nil
	}
	switch t := strings.TrimSpace(in.Type); t {
	case "", "text":
	default:
		return fmt.Errorf("%w: a %s is a validation, not a conversation - ask it with %s (kind %s)", ErrNotAConversation, t, RequestValidationTool, validationKindOf(t))
	}
	switch agentapi.UserCommsInputFromContext(ctx) {
	case "inbox", "none":
		return fmt.Errorf("%w: this run has no conversation - ask what you need with %s (kind form for an answer to give, choose for a choice, approve for a yes or no)", ErrNotAConversation, RequestValidationTool)
	}
	return nil
}

// validationKindOf is the kind of question a structured prompt is.
func validationKindOf(promptType string) string {
	switch promptType {
	case "select":
		return inboxapi.KindChoose
	case "confirm":
		return inboxapi.KindApprove
	}
	return inboxapi.KindForm
}

func describeAskUser(raw json.RawMessage) string {
	var in AskInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return "ask-user <invalid>"
	}
	var b strings.Builder
	b.WriteString("ask-user")
	if in.Type != "" {
		b.WriteString(" type=" + in.Type)
	}
	q := in.PromptText()
	if len(q) > 60 {
		fmt.Fprintf(&b, " %q...", q[:60])
	} else if q != "" {
		fmt.Fprintf(&b, " %q", q)
	}
	return b.String()
}

// conversationDescription is ask-user: a conversation only.
const conversationDescription = `Ask the user a question in the conversation and wait for their reply: free text only.

A validation is not a conversation: to have the person approve something, choose between options, fill a form, decide, test by hand or provide data, use request-validation - it is asked once, proven and recorded.
In a run with no conversation (a background or headless run), ask-user is refused: ask with request-validation.
Do not use this tool for routine status updates — only when you genuinely need the user's reply to proceed.
`
