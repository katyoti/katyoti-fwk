// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/katyoti/katyoti-fwk/internal/core/usermsg"
	"time"

	actionsapi "github.com/katyoti/katyoti-fwk/internal/extensions/actions/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp"
)

// The shapes and bindings the object pane's actions use: the action row
// (QuickAction / Action), running one through ActionService, describing one
// before its launch dialog, and the type-picker row.

// WorkspacePolicy is a rule that applies to the object (api.Policy): its name,
// what it says, and whether it carries a check.
type WorkspacePolicy struct {
	Name        string `json:"name"`
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
	Icon        string `json:"icon,omitempty"`
	HasCheck    bool   `json:"hasCheck,omitempty"`
}

// WorkspaceBadge is one resolved header badge (api.Badge): what it says, its
// icon and colour, its tooltip, and the capability that set it.
type WorkspaceBadge struct {
	ID         string `json:"id"`
	Label      string `json:"label"`
	Icon       string `json:"icon,omitempty"`
	Color      string `json:"color,omitempty"`
	Tooltip    string `json:"tooltip,omitempty"`
	Capability string `json:"capability,omitempty"`
}

// WorkspaceNodeDiagnostic mirrors api.ObjectDiagnostic for the GUI binding (an
// object detail's diagnostics).
type WorkspaceNodeDiagnostic struct {
	Code        string `json:"code"`
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
}

// WorkspaceQuickAction mirrors api.Action for the GUI binding. The
// URI is the dispatch handle: "system:/tasks/<name>" for substrate-
// handled URIs (chat etc.), or a regular task / KTSP URI for
// runner-handled actions.
type WorkspaceQuickAction struct {
	URI            string         `json:"uri"`
	Label          string         `json:"label"`
	Description    string         `json:"description,omitempty"`
	Icon           string         `json:"icon,omitempty"`
	Kind           string         `json:"kind"`
	Source         string         `json:"source"`
	Confirm        bool           `json:"confirm,omitempty"`
	Preview        bool           `json:"preview,omitempty"`
	Quick          bool           `json:"quick,omitempty"`
	Disabled       bool           `json:"disabled,omitempty"`
	DisabledReason string         `json:"disabledReason,omitempty"`
	BoundInput     map[string]any `json:"boundInput,omitempty"`
}

// WorkspaceAction is the GUI shape of one action in an object's action
// list (action-surface/List). Same shape as QuickAction.
type WorkspaceAction = WorkspaceQuickAction

// actionRunHop is the hop GUI → ActionService on top of the bound
// ActionService puts on the target method, and that bound itself when the
// method declares none (the bus's default, a few seconds).
const actionRunHop = 5 * time.Second

// actionRunBudget is how long the GUI waits for an action run: its target
// declares the bound in the catalog - a method its TimeoutS, which
// ActionService puts on it (else the bus's default), a script the run bound
// its engine applies (scripts.yaml timeout_s, else the scripts' default) - so
// the GUI waits that, plus the hop, never a blanket ceiling of its own. A
// long method and a task answer at once with their run (and panel).
func actionRunBudget(ctx context.Context, manifestOf mbapi.ManifestOf, uri string) time.Duration {
	inner := mbapi.MethodTimeout(ctx, manifestOf, uri)
	if inner <= 0 {
		inner = actionRunHop
	}
	return inner + actionRunHop
}

// actionRunContext is a's context bounded by actionRunBudget for uri, read
// from universeName's catalog (unreachable: the undeclared bound).
func (a *App) actionRunContext(universeName, uri string) (context.Context, context.CancelFunc) {
	var manifestOf mbapi.ManifestOf
	if c, err := a.universeCatalog(universeName); err == nil {
		manifestOf = c.ManifestSlots
	}
	return context.WithTimeout(a.ctx, actionRunBudget(a.ctx, manifestOf, uri))
}

// WorkspaceRunActionResult is the GUI-facing return of WorkspaceRunAction.
// cwm-v2-providers US-15 (2026-06-08). When the action ran successfully,
// `TaskID` is populated for async tasks and `Value` for sync method/script
// calls. `Error` carries the substrate's actual error message inline.
type WorkspaceRunActionResult struct {
	URI    string `json:"uri"`
	TaskID string `json:"taskId,omitempty"`
	// Panel is the display panel a method declared long reports into
	// (with TaskID); empty otherwise.
	Panel    string       `json:"panel,omitempty"`
	Value    string       `json:"value,omitempty"`
	Error    string       `json:"error,omitempty"`
	ErrorMsg *usermsg.Msg `json:"errorMsg,omitempty"` // the error by key (errors namespace)
}

// WorkspaceRunAction dispatches an action URI through ActionService.
// Used by the ActionsPanel to launch every action an object exposes
// (the actions its type and capabilities declare, tasks, scripts).
// The dispatcher (`internal/extensions/actions/`) routes by URI slot
// prefix: `tasks/` → TaskService, `scripts/` → ScriptService.
//
// `inputJSON` is the JSON-encoded input payload (empty string sends no
// input — equivalent to `{}`). For tasks expecting structured input,
// pass an object that matches the task's declared input-schema.
//
// Wails-bound: window.go.main.App.WorkspaceRunAction(universeName, uri, inputJSON).
func (a *App) WorkspaceRunAction(universeName, uri, inputJSON string) WorkspaceRunActionResult {
	if uri == "" {
		return WorkspaceRunActionResult{Error: "Action URI is required."}
	}
	sender, err := a.universeSender(universeName)
	if err != nil {
		return WorkspaceRunActionResult{URI: uri, Error: fmt.Sprintf("Cannot reach universe %q: %v", universeName, err)}
	}
	in := actionsapi.RunInput{URI: uri}
	if inputJSON != "" {
		in.Input = json.RawMessage(inputJSON)
	}
	// The GUI waits as long as the target declares (actionRunBudget).
	ctx, cancel := a.actionRunContext(universeName, uri)
	defer cancel()
	return runActionCall(ctx, sender, uri, in)
}

// WorkspaceRunActionAs runs an action as WorkspaceRunAction does, under
// callID: WorkspaceCancelCall(callID) gives it up, and the cancel reaches the
// method on the server - what a section's load made
// obsolete calls.
//
// Wails-bound: window.go.main.App.WorkspaceRunActionAs(universeName, uri, inputJSON, callID).
func (a *App) WorkspaceRunActionAs(universeName, uri, inputJSON, callID string) WorkspaceRunActionResult {
	if uri == "" {
		return WorkspaceRunActionResult{Error: "Action URI is required."}
	}
	sender, err := a.universeSender(universeName)
	if err != nil {
		return WorkspaceRunActionResult{URI: uri, Error: fmt.Sprintf("Cannot reach universe %q: %v", universeName, err)}
	}
	in := actionsapi.RunInput{URI: uri}
	if inputJSON != "" {
		in.Input = json.RawMessage(inputJSON)
	}
	ctx, cancel := a.actionRunContext(universeName, uri)
	defer cancel()
	ctx, done := guiCalls.begin(ctx, callID)
	defer done()
	return runActionCall(ctx, sender, uri, in)
}

// WorkspaceCancelCall gives up the call WorkspaceRunActionAs runs under
// callID; one already ended, or never begun, is ignored.
//
// Wails-bound: window.go.main.App.WorkspaceCancelCall(callID).
func (a *App) WorkspaceCancelCall(callID string) {
	guiCalls.cancel(callID)
}

// runActionCall is one ActionService.Run over the bus, its answer read.
func runActionCall(ctx context.Context, sender ktsp.Sender, uri string, in actionsapi.RunInput) WorkspaceRunActionResult {
	respRaw, err := sender.Call(ctx, "ktw-agents:action/run", in)
	if err != nil {
		return WorkspaceRunActionResult{URI: uri, Error: err.Error(), ErrorMsg: errorMsgOf(err)}
	}
	var out actionsapi.RunOutput
	if err := json.Unmarshal(respRaw, &out); err != nil {
		return WorkspaceRunActionResult{URI: uri, Error: fmt.Sprintf("Decode response failed: %v", err)}
	}
	value := ""
	if len(out.Value) > 0 {
		value = string(out.Value)
	}
	return WorkspaceRunActionResult{
		URI:    out.URI,
		TaskID: out.TaskID,
		Panel:  out.Panel,
		Value:  value,
	}
}

// WorkspaceDescribeActionResult is the GUI-facing return of
// WorkspaceDescribeAction. InputSchema is the action's inline JSON
// Schema (string; empty when the action declares no input) — the
// ActionsPanel uses it to auto-generate the parameter dialog before
// dispatching Run.
type WorkspaceDescribeActionResult struct {
	URI         string `json:"uri"`
	Label       string `json:"label,omitempty"`
	Description string `json:"description,omitempty"`
	// Details is the action's long form — a task's markdown body. Read on
	// demand when a card's info popup opens, never in a listing.
	Details     string       `json:"details,omitempty"`
	InputSchema string       `json:"inputSchema,omitempty"`
	IsAsync     bool         `json:"isAsync"`
	Error       string       `json:"error,omitempty"`
	ErrorMsg    *usermsg.Msg `json:"errorMsg,omitempty"` // the error by key (errors namespace)
}

// WorkspaceDescribeAction inspects an action URI through
// ActionService.Describe — label, description, input schema, async
// flag — so the GUI can render a schema-driven launch form before
// invoking WorkspaceRunAction.
//
// Wails-bound: window.go.main.App.WorkspaceDescribeAction(universeName, uri).
func (a *App) WorkspaceDescribeAction(universeName, uri string) WorkspaceDescribeActionResult {
	if uri == "" {
		return WorkspaceDescribeActionResult{Error: "Action URI is required."}
	}
	sender, err := a.universeSender(universeName)
	if err != nil {
		return WorkspaceDescribeActionResult{URI: uri, Error: fmt.Sprintf("Cannot reach universe %q: %v", universeName, err)}
	}
	respRaw, err := sender.Call(a.ctx, "ktw-agents:action/describe", actionsapi.DescribeInput{URI: uri})
	if err != nil {
		return WorkspaceDescribeActionResult{URI: uri, Error: err.Error(), ErrorMsg: errorMsgOf(err)}
	}
	var out actionsapi.DescribeOutput
	if err := json.Unmarshal(respRaw, &out); err != nil {
		return WorkspaceDescribeActionResult{URI: uri, Error: fmt.Sprintf("Decode response failed: %v", err)}
	}
	schema := ""
	if len(out.InputSchema) > 0 {
		schema = string(out.InputSchema)
	}
	return WorkspaceDescribeActionResult{
		URI:         out.URI,
		Label:       out.Label,
		Description: out.Description,
		Details:     out.Details,
		InputSchema: schema,
		IsAsync:     out.IsAsync,
	}
}

// WorkspaceActionTaskResult is the task behind a `<provider>:tasks/<name>`
// action, as the universe's catalog lists it: its run name and, when it
// declares an input, the URI of its registered form.
type WorkspaceActionTaskResult struct {
	Task           string `json:"task,omitempty"`
	Title          string `json:"title,omitempty"`
	Description    string `json:"description,omitempty"`
	InputSchemaURI string `json:"inputSchemaURI,omitempty"`
	// Tags are the task's behavioural markers: `no-initial-input` starts its
	// run at once, with no initial-message box.
	Tags  []string `json:"tags,omitempty"`
	Error string   `json:"error,omitempty"`
}

// WorkspaceActionTask finds the task an interactive action runs, so the GUI
// shows its form before the chat starts on the object. It reads the catalog,
// like the task gallery: the task may live in any sidecar of the universe.
//
// Wails-bound: window.go.main.App.WorkspaceActionTask(universeName, uri).
func (a *App) WorkspaceActionTask(universeName, uri string) WorkspaceActionTaskResult {
	t, err := mbapi.ParseToolURI(uri)
	if err != nil || t.SlotType != "tasks" {
		return WorkspaceActionTaskResult{Error: fmt.Sprintf("%s is not a task action", uri)}
	}
	// A task hidden from the gallery (advertised: false) is still the one
	// the action runs: its form is asked first all the same.
	tasks, err := a.tasksKTSP(universeName, true)
	if err != nil {
		return WorkspaceActionTaskResult{Error: err.Error()}
	}
	for _, ti := range tasks {
		if ti.Name == t.Method && (t.Provider == "" || ti.Provider == t.Provider) {
			return WorkspaceActionTaskResult{Task: ti.URI, Title: ti.Title, Description: ti.Description, InputSchemaURI: ti.InputSchemaURI, Tags: ti.Tags}
		}
	}
	return WorkspaceActionTaskResult{Error: fmt.Sprintf("task %s not found in universe %q", uri, universeName)}
}

// WorkspaceNodeTypeChoice is one row in the "+ New" type picker
// (WorkspaceObjectChildTypes, from cwm/ObjectAllowedChildren).
type WorkspaceNodeTypeChoice struct {
	Name        string `json:"name"`
	Label       string `json:"label,omitempty"`
	Icon        string `json:"icon,omitempty"`
	Description string `json:"description,omitempty"`
	// CreateTask is the task that creates an object of this type instead of
	// the plain form — the dialog opens it as an assistant chat.
	CreateTask string `json:"createTask,omitempty"`
	// Named: the parent's type names it, or it names the parents it
	// accepts - « + New » here offers only these.
	Named bool `json:"named,omitempty"`
}

// CapCheckResult is the outcome of verifying an object (WorkspaceObjectVerifyRun).
type CapCheckResult struct {
	Status string `json:"status"` // pass | fail | unproven | harness-error
	Desc   string `json:"desc"`
	Error  string `json:"error,omitempty"`
}
