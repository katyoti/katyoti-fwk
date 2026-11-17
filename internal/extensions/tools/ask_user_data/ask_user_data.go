// SPDX-License-Identifier: Apache-2.0

// Package ask_user_data implements the `ask-user-data` agent tool — the
// schema-driven sibling of `ask-user`. Where `ask-user` asks a free-text
// question, `ask-user-data` mounts a schema-bound FORM (or wizard) in its
// own ktw-gui tab, blocks until the user submits or cancels, validates the
// submission against the schema, and returns the structured data to the
// agent loop in one tool call.
//
// It reuses the shipped primitives, adding no new event type:
//   - mount = publish the same agentapi.UIArtifactMountEvent the
//     display-ui-component stream plugin emits;
//   - await = the display panel's question (internal/core/panel): the form
//     is asked on a question panel of its own and its answer comes back on
//     the panel's input subject - from a screen following the panel, or
//     from the chat's form card (its ui_event submit/cancel, which the
//     terminal room sends to that input). The panel server keeps the ask,
//     so any screen finds the question still asking;
//   - validate = SchemaService.ValidateAgainst (URI-backed schemas);
//   - freeze  = publish agentapi.UIArtifactCompleteEvent on completion.
//
// The chat composer stays live while the form is pending: a banner button
// (driven by PromptShownEvent{Channel:"form", ArtifactID}) focuses the tab,
// and a typed message is surfaced as a cancel + the typed text.
//
// It is an OPT-IN plugin (`ask-user-data`) — a task adds it to its
// `plugins:` list; it is not in the standard set.
package ask_user_data

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/panel"
	schemacore "github.com/katyoti/katyoti-fwk/internal/core/schema/api"
	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
	hgapi "github.com/katyoti/katyoti-fwk/internal/extensions/humangate/api"
	schemasvc "github.com/katyoti/katyoti-fwk/internal/extensions/schema/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
	toolsapi "github.com/katyoti/katyoti-fwk/internal/extensions/tools/api"
)

// AskUserDataInput is the input schema for the ask-user-data tool.
type AskUserDataInput struct {
	// SchemaURI is a registered data-type URI (e.g.
	// "ktw-example:/data-type/request-form"). Its ui-form
	// extension drives the rendered form, and submissions are validated
	// against it. Mutually exclusive with SchemaInline.
	SchemaURI string `json:"schema_uri,omitempty" jsonschema:"Registered data-type URI to render + validate against (e.g. provider:/data-type/name). Mutually exclusive with schema_inline."`

	// SchemaInline is a full ktw-schema/v1 shape (with optional ui-form
	// extension) supplied inline. Use when no data-type is registered.
	// Not server-validated (no URI to validate against).
	SchemaInline map[string]any `json:"schema_inline,omitempty" jsonschema:"Inline ktw-schema/v1 schema (with ui-form). Mutually exclusive with schema_uri."`

	// InitialValue pre-fills the form (e.g. when editing an existing
	// record, or seeding a create form with known fields).
	InitialValue any `json:"initial_value,omitempty" jsonschema:"Optional initial value to pre-fill the form."`

	// Mode is the CRUD mode for the form widget: "create" (default) or
	// "edit".
	Mode string `json:"mode,omitempty" jsonschema:"Form mode: 'create' (default) or 'edit'."`

	// Wizard renders the form as a multi-step wizard (groups → steps)
	// instead of a single page.
	Wizard bool `json:"wizard,omitempty" jsonschema:"Render as a multi-step wizard (groups become steps) instead of a single-page form."`

	// FormName selects a named ui-form variant from the schema's
	// extensions. Empty uses the default form.
	FormName string `json:"form_name,omitempty" jsonschema:"Named ui-form variant from the schema's extensions (default: the default form)."`

	// Title is the tab label. Defaults to a schema-derived name.
	Title string `json:"title,omitempty" jsonschema:"Optional tab title."`

	// Prompt is the short banner text shown in the chat next to the
	// 'open the form' button (e.g. "Renseigne le nouveau dossier").
	Prompt string `json:"prompt,omitempty" jsonschema:"Short banner text shown beside the 'open form' button in the chat."`
}

// AskUserDataOutput is the result of an ask-user-data call.
type AskUserDataOutput struct {
	// Data is the validated, structured submission (the form's value).
	// Empty when Cancelled is true.
	Data map[string]any `json:"data,omitempty" jsonschema:"The validated structured form submission."`

	// Cancelled is true when the user cancelled the form (Cancel button,
	// or typed a free-text answer in the composer instead of using it).
	Cancelled bool `json:"cancelled,omitempty" jsonschema:"True when the user cancelled instead of submitting."`

	// TypedText carries a free-text answer the user typed in the composer
	// instead of using the form (Cancelled is then true).
	TypedText string `json:"typed_text,omitempty" jsonschema:"Free-text the user typed instead of filling the form (cancel case)."`

	// Type is always "form" — echoes the interaction style.
	Type string `json:"type" jsonschema:"Always 'form'."`

	// ArtifactID is the id of the mounted form artifact.
	ArtifactID string `json:"artifactId" jsonschema:"The mounted form artifact id."`
}

// NewAskUserDataTool creates the ask-user-data tool.
func NewAskUserDataTool() toolsapi.Tool {
	return toolsapi.NewFunctionTool(
		"ask-user-data",
		toolDescription,
		executeAskUserData,
		toolsapi.WithReadOnly(),
		toolsapi.WithInteractive(),
		toolsapi.WithIdle(),
		toolsapi.WithDisplay(toolsapi.ToolDisplay{Icon: "📝", Label: "Ask form", Color: "#82AAFF", Template: "{{.schema_uri}}"}),
	)
}

func executeAskUserData(ctx context.Context, in AskUserDataInput) (AskUserDataOutput, error) {
	if in.SchemaURI == "" && in.SchemaInline == nil {
		return AskUserDataOutput{}, fmt.Errorf("ask-user-data: one of schema_uri or schema_inline is required")
	}
	if in.SchemaURI != "" && in.SchemaInline != nil {
		return AskUserDataOutput{}, fmt.Errorf("ask-user-data: schema_uri and schema_inline are mutually exclusive")
	}

	artifactID := in.artifactID(ctx)

	// Resolve the schema to mount. URI → fetch the AST keeping ui-form
	// extensions; inline → pass through.
	mountSchema, err := resolveSchema(ctx, in)
	if err != nil {
		return AskUserDataOutput{}, err
	}

	gate := resolveGate(ctx)
	channel := agentapi.UserCommsInputFromContext(ctx)

	// GUI path: a form-capable gate, the universe's panel subjects in reach,
	// and not the silent channel.
	if wire, ok := resolveWire(ctx); ok && hasCapability(gate, "form") && channel != "none" {
		return executeGUI(ctx, gate, wire, in, mountSchema)
	}
	// Fallback: no form-capable gate or no panel subjects (CLI/pipe), or the
	// silent channel.
	return executeFallback(ctx, gate, in, artifactID)
}

// executeGUI asks the form on a question panel of its own and shows it as
// an inline card in the chat (the schema AST rides on the prompt event's
// FormSchema, its artifact id the panel's id), waits for the panel's answer,
// validates it, and returns the data. A free-text answer typed in the
// composer meanwhile cancels the form with the text. The chat renders the
// submitted answer as a read-only view card on Next.
func executeGUI(ctx context.Context, gate hgapi.HumanGate, wire panelWire, in AskUserDataInput, mountSchema any) (AskUserDataOutput, error) {
	h, err := panel.OpenPanel(ctx, wire.scope, wire.publish, panel.Open{
		Type: panel.TypeQuestion, Title: in.Title, Owner: agentapi.RunIDFromContext(ctx),
	})
	if err != nil {
		return AskUserDataOutput{}, fmt.Errorf("ask-user-data: open its question panel: %w", err)
	}
	artifactID := h.ID()
	status := "cancelled"
	defer func() { closePanel(h, status) }()

	publish(ctx, agentapi.EventPromptShown, agentapi.PromptShownEvent{
		EventBase:  eventBase(ctx, agentapi.EventPromptShown),
		Turn:       agentapi.CurrentTurnFromContext(ctx),
		Question:   in.Prompt,
		PromptType: "form",
		Channel:    "form",
		ArtifactID: artifactID,
		Title:      in.Title,
		FormSchema: mountSchema,
		FormData:   in.InitialValue,
	})

	ask := panel.Ask{Question: in.Prompt, Form: specOf(mountSchema), Value: in.InitialValue}
	a, err := awaitAnswer(ctx, h, ask, wire.subscribe, typedText(gate))
	if err != nil {
		return AskUserDataOutput{}, fmt.Errorf("ask-user-data: await: %w", err)
	}
	status = a.status()

	// Clear the chat form card now that it resolved (the translator marks
	// the prompt answered on UserInput).
	publish(ctx, agentapi.EventUserInput, agentapi.UserInputEvent{
		EventBase: eventBase(ctx, agentapi.EventUserInput),
		Turn:      agentapi.CurrentTurnFromContext(ctx),
		Answer:    a.summary(),
	})

	switch {
	case a.typed:
		// User typed in the composer instead of filling the form.
		return AskUserDataOutput{Cancelled: true, TypedText: a.text, Type: "form", ArtifactID: artifactID}, nil
	case a.cancelled:
		return AskUserDataOutput{Cancelled: true, Type: "form", ArtifactID: artifactID}, nil
	}
	raw, err := json.Marshal(a.value)
	if err != nil {
		return AskUserDataOutput{}, fmt.Errorf("ask-user-data: submit value: %w", err)
	}
	if a.value == nil {
		raw = nil
	}
	data, err := decodeAndValidate(ctx, in, raw)
	if err != nil {
		return AskUserDataOutput{}, err
	}
	return AskUserDataOutput{Data: cleanStrings(data), Type: "form", ArtifactID: artifactID}, nil
}

// executeFallback degrades to a HumanGate form prompt (rendered inline)
// when the panel subjects are out of reach. Requires the gate to advertise the
// "form" capability; otherwise there is no structured-input channel.
func executeFallback(ctx context.Context, gate hgapi.HumanGate, in AskUserDataInput, artifactID string) (AskUserDataOutput, error) {
	if !hasCapability(gate, "form") {
		return AskUserDataOutput{}, fmt.Errorf("ask-user-data: no form-capable user channel available (no panel subjects and no form-capable gate)")
	}

	formSchema, err := fallbackFormSchema(ctx, in)
	if err != nil {
		return AskUserDataOutput{}, err
	}

	resp, err := gate.Prompt(ctx, hgapi.PromptRequest{
		Type:       "form",
		Question:   in.Prompt,
		FormSchema: formSchema,
	})
	if err != nil {
		return AskUserDataOutput{}, fmt.Errorf("ask-user-data: form prompt failed: %w", err)
	}
	// Validate the submission when we have a URI to validate against.
	if in.SchemaURI != "" {
		body, _ := json.Marshal(resp.Data)
		if err := validateBody(ctx, in.SchemaURI, body); err != nil {
			return AskUserDataOutput{}, err
		}
	}
	return AskUserDataOutput{Data: cleanStrings(resp.Data), Type: "form", ArtifactID: artifactID}, nil
}

// decodeAndValidate parses the submit value into a map and (for URI-backed
// schemas) validates it against the data-type.
func decodeAndValidate(ctx context.Context, in AskUserDataInput, raw json.RawMessage) (map[string]any, error) {
	var data map[string]any
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &data); err != nil {
			return nil, fmt.Errorf("ask-user-data: submit value is not a JSON object: %w", err)
		}
	}
	if in.SchemaURI != "" {
		body, _ := json.Marshal(data)
		if err := validateBody(ctx, in.SchemaURI, body); err != nil {
			return nil, err
		}
	}
	return data, nil
}

// validateBody runs SchemaService.ValidateAgainst and turns a non-Valid
// result into an error listing the error-severity diagnostics.
func validateBody(ctx context.Context, uri string, body json.RawMessage) error {
	svc, err := application.UseService[schemasvc.SchemaService](ctx)
	if err != nil || svc == nil {
		// Schema service unreachable — skip validation rather than block
		// the user; the widget already validated client-side.
		return nil
	}
	out, err := svc.ValidateAgainst(ctx, schemasvc.ValidateAgainstInput{URI: uri, Body: body})
	if err != nil {
		return fmt.Errorf("ask-user-data: validation call failed: %w", err)
	}
	if out.Valid {
		return nil
	}
	msg := "ask-user-data: submission failed schema validation:"
	for _, d := range out.Diagnostics {
		if string(d.Severity) == "warning" {
			continue
		}
		if d.Path != "" {
			msg += fmt.Sprintf("\n  - %s: %s", d.Path, d.Message)
		} else {
			msg += fmt.Sprintf("\n  - %s", d.Message)
		}
	}
	return fmt.Errorf("%s", msg)
}

// resolveSchema returns the CANONICAL schema AST to mount. The SchemaService
// is resolved via application.UseService — in consumer binaries it is the
// bus-backed client (schemalocal.RemoteExtension), in the hub it is the real
// service. Both paths yield the same shape the form runtime understands: the
// URI path fetches the registered AST (ui-form kept); the inline path runs
// the caller's ktw-schema/v1 DSL through the parser so shorthand (e.g.
// `string@enum(...)`) is expanded to canonical type nodes — the renderer
// cannot interpret raw shorthand, so passing the inline map verbatim breaks
// it with "Unknown schema type".
func resolveSchema(ctx context.Context, in AskUserDataInput) (any, error) {
	svc, err := application.UseService[schemasvc.SchemaService](ctx)
	if err != nil || svc == nil {
		return nil, fmt.Errorf("ask-user-data: schema service unavailable")
	}
	if in.SchemaInline != nil {
		body, err := json.Marshal(in.SchemaInline)
		if err != nil {
			return nil, fmt.Errorf("ask-user-data: marshal inline schema: %w", err)
		}
		out, err := svc.Parse(ctx, schemacore.ParseInput{Body: body})
		if err != nil {
			return nil, fmt.Errorf("ask-user-data: inline schema parse failed: %w", err)
		}
		if msg := errorDiagnostics(out.Diagnostics); msg != "" {
			return nil, fmt.Errorf("ask-user-data: inline schema invalid:%s", msg)
		}
		return out.Schema, nil
	}
	exts := []schemasvc.ExtensionRef{{Type: "ui-form", Name: "*"}}
	out, err := svc.GetDataTypeWithExtensions(ctx, schemasvc.GetDataTypeWithExtensionsInput{URI: in.SchemaURI, Extensions: exts})
	if err != nil {
		return nil, fmt.Errorf("ask-user-data: resolve schema %q: %w", in.SchemaURI, err)
	}
	return out.Schema, nil
}

// errorDiagnostics formats the error-severity diagnostics (skipping
// warnings) into a bullet list, or "" when there are none.
func errorDiagnostics(diags []schemacore.Diagnostic) string {
	var b strings.Builder
	for _, d := range diags {
		if string(d.Severity) == "warning" {
			continue
		}
		if d.Path != "" {
			fmt.Fprintf(&b, "\n  - %s: %s", d.Path, d.Message)
		} else {
			fmt.Fprintf(&b, "\n  - %s", d.Message)
		}
	}
	return b.String()
}

// fallbackFormSchema builds the {jsonSchema} FormSchema map a HumanGate
// form prompt expects, from the registered data-type's compiled JSON Schema
// (URI case) or by handing the inline schema through.
func fallbackFormSchema(ctx context.Context, in AskUserDataInput) (map[string]any, error) {
	if in.SchemaInline != nil {
		return map[string]any{"schema": in.SchemaInline, "data": in.InitialValue, "title": in.Title}, nil
	}
	svc, err := application.UseService[schemasvc.SchemaService](ctx)
	if err != nil || svc == nil {
		return nil, fmt.Errorf("ask-user-data: schema service unavailable for fallback")
	}
	js, err := svc.GetJSONSchema(ctx, schemasvc.GetJSONSchemaInput{URI: in.SchemaURI})
	if err != nil {
		return nil, fmt.Errorf("ask-user-data: compile JSON schema for %q: %w", in.SchemaURI, err)
	}
	var jsonSchema any
	_ = json.Unmarshal(js.Schema, &jsonSchema)
	return map[string]any{"jsonSchema": jsonSchema, "data": in.InitialValue, "title": in.Title}, nil
}

// artifactID is the fallback form's id, derived from the run id (single
// in flight per task means one form at a time); the GUI path's is its
// question panel's id.
func (in AskUserDataInput) artifactID(ctx context.Context) string {
	if rid := agentapi.RunIDFromContext(ctx); rid != "" {
		return "ask-user-data:" + rid
	}
	return "ask-user-data"
}

func resolveGate(ctx context.Context) hgapi.HumanGate {
	if g := hgapi.HumanGateFromContext(ctx); g != nil {
		return g
	}
	if g, err := application.UseService[hgapi.HumanGate](ctx); err == nil {
		return g
	}
	return nil
}

func hasCapability(gate hgapi.HumanGate, cap string) bool {
	if gate == nil {
		return false
	}
	for _, c := range gate.Capabilities() {
		if c == cap {
			return true
		}
	}
	return false
}

// eventBase builds the EventBase for a tool-emitted event, stamping the run
// id as the emitter so the per-session translator scopes it to the owning
// session (an empty emitter leaks the artifact into every parallel session).
func eventBase(ctx context.Context, eventType string) agentapi.EventBase {
	return agentapi.EventBase{Type: eventType, Time: time.Now(), Emitter: agentapi.RunIDFromContext(ctx)}
}

// publish emits an event VALUE on the EventBus (matching the
// display-ui-component stream plugin, so the translator's value type-switch
// matches). A missing bus (programmatic callers / tests) is a no-op.
func publish(ctx context.Context, topic string, payload any) {
	bus, err := application.UseService[engine.EventBus](ctx)
	if err != nil || bus == nil {
		return
	}
	_ = bus.Publish(topic, payload)
}

const toolDescription = `Ask the user to fill a structured FORM and return the validated data.

Unlike ask-user (free text), this renders a schema-driven form (or a
multi-step wizard) in its own tab; the user fills and submits it, and you
receive the structured, validated data in one call.

Provide EITHER:
- schema_uri: a registered data-type URI (its ui-form drives the form and
  the submission is validated against it), OR
- schema_inline: an inline ktw-schema/v1 shape (with a ui-form extension).

Optional: initial_value (pre-fill), mode ('create'|'edit'), wizard (multi-step),
form_name (named variant), title, prompt (banner text).

The result is { data, type:"form", artifactId } on submit, or
{ cancelled:true, ... } if the user cancels (or types a free-text answer
instead, returned as typed_text). Use this when you need several related
fields at once (e.g. opening a new dossier, adding a solution).`
