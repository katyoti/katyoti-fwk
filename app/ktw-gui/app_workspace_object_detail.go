// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/katyoti/katyoti-fwk/internal/core/usermsg"
	"strings"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/render"
	actionsapi "github.com/katyoti/katyoti-fwk/internal/extensions/actions/api"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	paneapi "github.com/katyoti/katyoti-fwk/internal/extensions/objectpane/api"
	verificationapi "github.com/katyoti/katyoti-fwk/internal/extensions/verification/api"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp"
)

// THE OBJECT WORKSPACE, bound (the GUI port, 2026-09-25).
//
// Every binding here is the object-side twin of one in
// app_workspace_items.go, and each is split in two: the App method the
// frontend calls, and a `…Via(ctx, sender, …)` half that takes the sender as
// an argument. The split is not decoration — it is what makes the ONE JSON
// round trip testable with a stub, which is where the port's real risks live:
// a renamed field, a `[]any` where the frontend expects `[]string`, an id from
// the wrong id space. The node tree's bindings had no such test, and the wire
// shape is exactly what a compile cannot check.
//
// The page these serve: modules/gui/objects-what-the-user-sees.md.

// WorkspaceObjectDetail is the detail pane's shape. Flatter than the object
// pane's DetailOutput on purpose: the frontend reads titles and ids, not
// capability maps, and anything it does not render does not cross the bridge.
type WorkspaceObjectDetail struct {
	ID string `json:"id"`
	// Revision is the stored version the pane read; every save sends it back.
	Revision string `json:"revision,omitempty"`
	Type     string `json:"type"`
	Title    string `json:"title"`
	Module   string `json:"module,omitempty"`
	// Project is the key of the project holding the object when the universe
	// serves several; empty with one.
	Project string `json:"project,omitempty"`

	Description string   `json:"description,omitempty"`
	ParentID    string   `json:"parentId,omitempty"`
	Status      string   `json:"status,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	// Icon is the object's own icon (the header edits it); empty = its type's.
	Icon string `json:"icon,omitempty"`
	// Body is the object's body, shown and edited in its type's section as the
	// type declares it (the section's tab carries the declaration).
	Body string `json:"body,omitempty"`

	TypeLabel string `json:"typeLabel,omitempty"`
	TypeIcon  string `json:"typeIcon,omitempty"`

	// Tabs are the pane's PANELS — what each effective capability declares or
	// derives, NOT one per capability. A marker capability holds no
	// fields and declares no panel, so it contributes no
	// tab; a capability may contribute several.
	Tabs []WorkspaceObjectTab `json:"tabs"`
	// Bare says the pane draws no object header: the type's tabs are its
	// whole pane, their panels drawing its head (pane.only).
	Bare bool `json:"bare,omitempty"`
	// Capabilities is the effective set as bare ids — what the object IS,
	// including the markers that draw no tab, so nothing is hidden.
	Capabilities []string `json:"capabilities,omitempty"`
	// CapabilityInfo is the same set as badges: label, icon, description, who
	// brings it.
	CapabilityInfo []WorkspaceCapabilityInfo `json:"capabilityInfo,omitempty"`
	// The header's line: when the object was made and last changed (derived
	// by the journal — no object stores them), and what it was derived from
	// (its outgoing derived-from edge).
	CreatedAt   string `json:"createdAt,omitempty"`
	UpdatedAt   string `json:"updatedAt,omitempty"`
	DerivedFrom string `json:"derivedFrom,omitempty"`
	// Aspects is the chip row. An undeclared aspect comes through with
	// declared=false rather than being dropped.
	Aspects []WorkspaceObjectAspect `json:"aspects,omitempty"`
	Badges  []WorkspaceBadge        `json:"badges,omitempty"`
	// Policies are the rules that apply to the object — not capabilities.
	Policies []WorkspacePolicy `json:"policies,omitempty"`

	Actions      []WorkspaceAction `json:"actions"`
	QuickActions []WorkspaceAction `json:"quickActions,omitempty"`

	Links    []WorkspaceObjectLink       `json:"links,omitempty"`
	Children []WorkspaceObjectChildGroup `json:"children,omitempty"`

	Errors   []WorkspaceNodeDiagnostic `json:"errors,omitempty"`
	Error    string                    `json:"error,omitempty"`
	ErrorMsg *usermsg.Msg              `json:"errorMsg,omitempty"` // the error by key, in the person's language (errors namespace)
}

// WorkspaceObjectTab is one capability panel: its id, its header and the
// schema that drives its form.
type WorkspaceObjectTab struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	// Icon is the tabler name; Emoji the text-surface fallback.
	Icon  string `json:"icon,omitempty"`
	Emoji string `json:"emoji,omitempty"`
	Order int    `json:"order"`
	// Kind is "content" (render Content), "source" (call Source with the
	// object id and render the definition it returns) or "fields" (the
	// capability's own field-group, rendered from Schema).
	Kind string `json:"kind"`
	// ChildType is the child type a "children" tab lists.
	ChildType string      `json:"childType,omitempty"`
	Content   *render.Def `json:"content,omitempty"`
	Source    string      `json:"source,omitempty"`
	Schema    string      `json:"schema,omitempty"`
	// Refresh is a declared panel's: which object changes re-read it while
	// it is shown (usePanelRefresh).
	Refresh *paneapi.PanelRefresh `json:"refresh,omitempty"`
	// Input is a declared panel's input, laid under what its source is
	// called with.
	Input map[string]any `json:"input,omitempty"`
	// Capabilities are the capabilities the tab is about.
	Capabilities []string `json:"capabilities,omitempty"`
	// Verify is true when the capability declares a verify hook, which is
	// what puts a Run check button in a fields tab.
	Verify bool `json:"verify,omitempty"`
	// Origin is "type", "object" or "panel" — a capability the instance
	// carries is shown as the instance's, so a user can tell what it added.
	Origin string `json:"origin,omitempty"`
	// Section: shown as a section of the Information tab, not as a tab.
	Section bool `json:"section,omitempty"`
	// Header: the object's header instead of a tab - drawn above its tabs
	// (the page's isHeaderPanel).
	Header bool `json:"header,omitempty"`
	// More: not one of the object's main tabs (an aspect lists them, this
	// one is not listed) — the strip shows it behind « More… ».
	More bool `json:"more,omitempty"`
	// Body is the type's body declaration, on the type's own section.
	Body *WorkspaceObjectBody `json:"body,omitempty"`
	// Badge is a declared panel's tab badge read from the object, in its
	// tone; none when its value is empty.
	Badge *cwmapi.Badge `json:"badge,omitempty"`
	// Count is a declared panel's count action ({count: n; color?}): the
	// GUI calls it with {id} and the input and draws the answer as the
	// tab's badge when no stored Badge is set.
	Count string `json:"count,omitempty"`
	// Values are the stored fields a fields tab displays, keyed by capability
	// id then field name.
	Values map[string]map[string]any `json:"values,omitempty"`
}

// WorkspaceObjectBody is what the object's type says its body is.
type WorkspaceObjectBody struct {
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
	// Format is markdown, text, html, or one a panel owns (deck).
	Format string `json:"format"`
	// Panel names the panel that shows the body, when one does.
	Panel string `json:"panel,omitempty"`
}

// WorkspaceCapabilityInfo is one capability as the Information tab shows it.
type WorkspaceCapabilityInfo struct {
	ID          string `json:"id"`
	Label       string `json:"label,omitempty"`
	Description string `json:"description,omitempty"`
	Icon        string `json:"icon,omitempty"`
	Source      string `json:"source,omitempty"`
}

// WorkspaceObjectAspect is one chip.
type WorkspaceObjectAspect struct {
	Name         string   `json:"name"`
	Label        string   `json:"label,omitempty"`
	Description  string   `json:"description,omitempty"`
	Capabilities []string `json:"capabilities,omitempty"`
	Declared     bool     `json:"declared"`
}

// WorkspaceObjectLink is one row of the Links section.
type WorkspaceObjectLink struct {
	Kind        string `json:"kind"`
	Label       string `json:"label"`
	Direction   string `json:"direction"`
	TargetID    string `json:"targetId"`
	TargetType  string `json:"targetType,omitempty"`
	TargetTitle string `json:"targetTitle,omitempty"`
	Resolved    bool   `json:"resolved"`
	// Ended: the end is an archived object — a valid link, shown as ended.
	Ended bool `json:"ended,omitempty"`
}

// WorkspaceObjectChildGroup is one list under the object.
type WorkspaceObjectChildGroup struct {
	Type    string            `json:"type"`
	Label   string            `json:"label,omitempty"`
	Icon    string            `json:"icon,omitempty"`
	Count   int               `json:"count"`
	Drawn   bool              `json:"drawn,omitempty"`
	Objects []WorkspaceObject `json:"objects"`
	// Rows are what a children tab shows of each object beside its name (a
	// group not drawn in the tree): its state and its key fields, by id.
	Rows []WorkspaceObjectChildRow `json:"rows,omitempty"`
}

// WorkspaceObjectChildRow is one child as its parent's tab lists it.
type WorkspaceObjectChildRow struct {
	ID     string           `json:"id"`
	State  *WorkspaceBadge  `json:"state,omitempty"`
	Badges []WorkspaceBadge `json:"badges,omitempty"`
}

// WorkspaceObjectDetailFetch is the detail pane in ONE call.
//
// Wails-bound: window.go.main.App.WorkspaceObjectDetailFetch(universeName, objectID).
func (a *App) WorkspaceObjectDetailFetch(universeName, objectID string) WorkspaceObjectDetail {
	sender, err := a.universeSender(universeName)
	if err != nil {
		return WorkspaceObjectDetail{Error: fmt.Sprintf("Cannot reach universe %q: %v", universeName, err)}
	}
	out, err := objectDetailVia(a.ctx, sender, objectID)
	if err != nil {
		return WorkspaceObjectDetail{Error: err.Error(), ErrorMsg: errorMsgOf(err)}
	}
	return out
}

// objectDetailVia is the binding minus the App: the object pane's Detail and the
// object's action surface (action-surface/List), decoded into what the pane
// draws. Whole or nothing: a pane missing its actions without saying so would
// look right and be quietly poorer.
func objectDetailVia(ctx context.Context, sender ktsp.Sender, objectID string) (WorkspaceObjectDetail, error) {
	if objectID == "" {
		return WorkspaceObjectDetail{}, fmt.Errorf("object id is required")
	}
	// Body: true — the pane renders the body, so it is asked for here and
	// nowhere else. The tree walk never sets it.
	raw, err := sender.Call(ctx, "ktw-agents:object-pane/detail",
		paneapi.DetailInput{ID: objectID, Body: true})
	if err != nil {
		return WorkspaceObjectDetail{}, fmt.Errorf("ktw-agents:cwm/object-detail: %w", err)
	}
	var resp paneapi.DetailOutput
	if err := json.Unmarshal(raw, &resp); err != nil {
		return WorkspaceObjectDetail{}, fmt.Errorf("decode response: %w", err)
	}
	out := objectDetailView(resp)
	out.Project = projectOf(raw)
	if out.Actions, err = objectActionsVia(ctx, sender, actionsapi.ListInput{ID: objectID, Scope: cwmapi.ActionScopeObject}); err != nil {
		return WorkspaceObjectDetail{}, err
	}
	return out, nil
}

// objectDetailView is the projection, kept apart from the call so the shape
// contract is tested without a transport at all.
func objectDetailView(resp paneapi.DetailOutput) WorkspaceObjectDetail {
	o := resp.Object
	out := WorkspaceObjectDetail{
		ID:       o.ID,
		Revision: resp.Revision,
		Type:     o.Type,
		Title:    o.DisplayTitle(),
		Module:   moduleOf(o.ID),
		ParentID: o.ParentID(),
		Tabs:     []WorkspaceObjectTab{},
		Actions:  []WorkspaceAction{},
		Bare:     resp.Bare,
	}
	out.Description = o.Description
	out.Tags = o.Tags
	out.Icon = o.Icon
	out.Body = o.Body
	if life, ok := o.Lifecycle(); ok {
		out.Status = life.State // a state is the lifecycle's
	}
	out.TypeLabel = resp.ObjectType.Label
	if out.TypeLabel == "" {
		out.TypeLabel = resp.ObjectType.Name
	}
	out.TypeIcon = resp.ObjectType.Icon
	if out.TypeIcon == "" {
		out.TypeIcon = resp.ObjectType.IconName
	}
	// The panels the MODEL declares, with each panel's filter resolved against
	// the object. The GUI decides nothing about which tabs exist — a pane that
	// invented one per capability is what put a "Tree node" tab on screen with
	// nothing in it.
	out.Capabilities = resp.Capabilities
	for _, c := range resp.CapabilityInfo {
		out.CapabilityInfo = append(out.CapabilityInfo, WorkspaceCapabilityInfo{
			ID: c.ID, Label: c.Label, Description: c.Description, Icon: c.Icon, Source: c.Source,
		})
	}
	if !resp.CreatedAt.IsZero() {
		out.CreatedAt = resp.CreatedAt.Format(time.RFC3339)
	}
	if !resp.UpdatedAt.IsZero() {
		out.UpdatedAt = resp.UpdatedAt.Format(time.RFC3339)
	}
	for _, l := range resp.Links {
		if l.Kind == string(cwmapi.EdgeTypeDerivedFrom) && l.Direction == "out" {
			out.DerivedFrom = firstNonEmpty(l.TargetTitle, l.TargetID)
			break
		}
	}
	for _, p := range resp.Panels {
		tab := WorkspaceObjectTab{
			ID: p.ID, Name: p.Name, Description: p.Description, Order: p.Order, Kind: p.Kind, ChildType: p.ChildType,
			Content: p.Content, Source: p.Source, Schema: p.Schema, Refresh: p.Refresh, Input: p.Input,
			Capabilities: p.Capabilities, Verify: p.Verify, Origin: p.Origin, Section: p.Section, More: p.More,
			Badge: p.Badge, Header: p.Header, Count: p.Count,
		}
		if p.Icon != nil {
			tab.Icon, tab.Emoji = p.Icon.Tabler, p.Icon.Emoji
		}
		if p.Body != nil {
			tab.Body = &WorkspaceObjectBody{Label: p.Body.Label, Description: p.Body.Description, Format: p.Body.Format, Panel: p.Body.Panel}
		}
		if tab.Name == "" {
			tab.Name = p.ID
		}
		for _, capID := range p.Capabilities {
			if capID == cwmapi.FieldsPanel { // the type's own fields
				if len(o.Fields) > 0 {
					if tab.Values == nil {
						tab.Values = map[string]map[string]any{}
					}
					tab.Values[capID] = o.Fields
				}
				continue
			}
			if group, ok := o.Capability(capID); ok && len(group) > 0 {
				if tab.Values == nil {
					tab.Values = map[string]map[string]any{}
				}
				tab.Values[capID] = group
			}
		}
		out.Tabs = append(out.Tabs, tab)
	}
	for _, tr := range resp.Aspects {
		out.Aspects = append(out.Aspects, WorkspaceObjectAspect{
			Name: tr.Name, Label: tr.Label, Description: tr.Description,
			Capabilities: tr.Capabilities, Declared: tr.Declared,
		})
	}
	for _, b := range resp.Badges {
		out.Badges = append(out.Badges, workspaceBadge(b))
	}
	for _, p := range resp.Policies {
		out.Policies = append(out.Policies, WorkspacePolicy{Name: p.Name, Label: p.DisplayName(), Description: p.Description, Icon: p.Icon, HasCheck: len(p.Hooks[cwmapi.HookVerify]) > 0})
	}
	out.QuickActions = workspaceActions(resp.QuickActions)
	for _, l := range resp.Links {
		out.Links = append(out.Links, WorkspaceObjectLink{
			Kind: l.Kind, Label: l.Label, Direction: l.Direction,
			TargetID: l.TargetID, TargetType: l.TargetType, TargetTitle: l.TargetTitle, Resolved: l.Resolved, Ended: l.Ended,
		})
	}
	for _, g := range resp.Children {
		group := WorkspaceObjectChildGroup{
			Type: g.Type, Label: g.Label, Icon: g.Icon, Count: g.Count, Drawn: g.Drawn,
			Objects: make([]WorkspaceObject, 0, len(g.Objects)),
		}
		for _, child := range g.Objects {
			group.Objects = append(group.Objects, WorkspaceObject{
				ID: child.ID, Type: child.Type, Title: child.DisplayTitle(), Module: moduleOf(child.ID),
			})
		}
		for _, r := range g.Rows {
			row := WorkspaceObjectChildRow{ID: r.ID}
			if r.State != nil {
				b := workspaceBadge(*r.State)
				row.State = &b
			}
			for _, b := range r.Badges {
				row.Badges = append(row.Badges, workspaceBadge(b))
			}
			group.Rows = append(group.Rows, row)
		}
		out.Children = append(out.Children, group)
	}
	for _, d := range resp.Diagnostics {
		out.Errors = append(out.Errors, WorkspaceNodeDiagnostic{Code: d.Code, Label: d.Label, Description: d.Description})
	}
	return out
}

// workspaceBadge is a resolved badge as the GUI draws it.
func workspaceBadge(b cwmapi.Badge) WorkspaceBadge {
	return WorkspaceBadge{ID: b.ID, Label: b.Label, Icon: b.Icon, Color: b.Color, Tooltip: b.Tooltip, Capability: b.Capability}
}

// workspaceActions projects the core's actions onto the GUI's shape. One
// function for both lists, so the bar and the card grid can never drift.
func workspaceActions(in []cwmapi.Action) []WorkspaceAction {
	out := make([]WorkspaceAction, 0, len(in))
	for _, a := range in {
		out = append(out, WorkspaceAction{
			URI: a.URI, Label: a.Label, Description: a.Description, Icon: a.Icon,
			Kind: a.Kind, Source: a.Source, Confirm: a.Confirm, Preview: a.Preview, Quick: a.Quick,
			Disabled: a.Disabled, DisabledReason: a.DisabledReason, BoundInput: a.BoundInput,
		})
	}
	return out
}

// ── the pickers ────────────────────────────────────────────────────────────

// WorkspaceObjectChildTypes is the "+ new" picker under a container: ONE call
// where the node side made two (child types and item kinds).
//
// Wails-bound: window.go.main.App.WorkspaceObjectChildTypes(universeName, parentID).
func (a *App) WorkspaceObjectChildTypes(universeName, parentID string) []WorkspaceNodeTypeChoice {
	sender, err := a.universeSender(universeName)
	if err != nil {
		a.emitError(fmt.Sprintf("WorkspaceObjectChildTypes %q: %v", universeName, err))
		return nil
	}
	out, err := objectChildTypesVia(a.ctx, sender, parentID)
	if err != nil {
		a.emitError(fmt.Sprintf("WorkspaceObjectChildTypes %q: %v", universeName, err))
		return nil
	}
	return out
}

func objectChildTypesVia(ctx context.Context, sender ktsp.Sender, parentID string) ([]WorkspaceNodeTypeChoice, error) {
	raw, err := sender.Call(ctx, "ktw-agents:cwm/object-allowed-children",
		cwmapi.ObjectAllowedChildrenInput{ParentID: parentID})
	if err != nil {
		return nil, fmt.Errorf("ktw-agents:cwm/object-allowed-children: %w", err)
	}
	var resp cwmapi.ObjectAllowedChildrenOutput
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	out := make([]WorkspaceNodeTypeChoice, 0, len(resp.Choices))
	for _, c := range resp.Choices {
		out = append(out, WorkspaceNodeTypeChoice{Name: c.Name, Label: c.Label, Icon: c.Icon, Description: c.Description, CreateTask: c.CreateTask, Named: c.Named})
	}
	return append(out, aspectChoicesVia(ctx, sender)...), nil
}

// aspectChoicesVia offers, beside the object types, every aspect that names
// the task creating an object carrying it (an Étude de l'existant): « + New »
// opens that task. The universe's view filters them by name like
// the types (new_types). An aspect list that cannot be read offers none.
func aspectChoicesVia(ctx context.Context, sender ktsp.Sender) []WorkspaceNodeTypeChoice {
	raw, err := sender.Call(ctx, "ktw-agents:cwm/object-aspect-list", cwmapi.ObjectAspectListInput{})
	if err != nil {
		return nil
	}
	var resp cwmapi.ObjectAspectListOutput
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil
	}
	var out []WorkspaceNodeTypeChoice
	for _, asp := range resp.Aspects {
		if asp.CreateTask == "" {
			continue
		}
		out = append(out, WorkspaceNodeTypeChoice{Name: asp.Name, Label: asp.Label, Description: asp.Description, CreateTask: asp.CreateTask})
	}
	return out
}

// WorkspaceObjectAspects is the "+ Aspect" picker: every declared aspect.
//
// Wails-bound: window.go.main.App.WorkspaceObjectAspects(universeName).
func (a *App) WorkspaceObjectAspects(universeName string) []WorkspaceObjectAspect {
	sender, err := a.universeSender(universeName)
	if err != nil {
		a.emitError(fmt.Sprintf("WorkspaceObjectAspects %q: %v", universeName, err))
		return nil
	}
	raw, err := sender.Call(a.ctx, "ktw-agents:cwm/object-aspect-list", cwmapi.ObjectAspectListInput{})
	if err != nil {
		a.emitError(fmt.Sprintf("WorkspaceObjectAspects %q: %v", universeName, err))
		return nil
	}
	var resp cwmapi.ObjectAspectListOutput
	if err := json.Unmarshal(raw, &resp); err != nil {
		a.emitError(fmt.Sprintf("WorkspaceObjectAspects %q: decode: %v", universeName, err))
		return nil
	}
	out := make([]WorkspaceObjectAspect, 0, len(resp.Aspects))
	for _, tr := range resp.Aspects {
		out = append(out, WorkspaceObjectAspect{
			Name: tr.Name, Label: tr.Label, Description: tr.Description,
			Capabilities: tr.Capabilities, Declared: true,
		})
	}
	return out
}

// WorkspaceObjectTypeGet fetches one declared type — the create form's schema
// source, read after the picker and before the form.
//
// Wails-bound: window.go.main.App.WorkspaceObjectTypeGet(universeName, typeName).
func (a *App) WorkspaceObjectTypeGet(universeName, typeName string) WorkspaceObjectTypeDetail {
	sender, err := a.universeSender(universeName)
	if err != nil {
		return WorkspaceObjectTypeDetail{Error: fmt.Sprintf("Cannot reach universe %q: %v", universeName, err)}
	}
	raw, err := sender.Call(a.ctx, "ktw-agents:cwm/object-type-get", cwmapi.ObjectTypeGetInput{Name: typeName})
	if err != nil {
		return WorkspaceObjectTypeDetail{Error: err.Error()}
	}
	var resp cwmapi.ObjectTypeGetOutput
	if err := json.Unmarshal(raw, &resp); err != nil {
		return WorkspaceObjectTypeDetail{Error: fmt.Sprintf("Decode response failed: %v", err)}
	}
	t := resp.ObjectType
	label := t.Label
	if label == "" {
		label = t.Name
	}
	return WorkspaceObjectTypeDetail{
		Name: t.Name, Label: label, Description: t.Description, Icon: t.Icon,
		Capabilities: t.Capabilities, FieldsDataTypeURI: t.Fields,
		AllowedChildren: t.AllowedChildren, Abstract: t.Abstract,
	}
}

// WorkspaceObjectTypeDetail is a declared type as the create form reads it.
type WorkspaceObjectTypeDetail struct {
	Name              string   `json:"name"`
	Label             string   `json:"label,omitempty"`
	Description       string   `json:"description,omitempty"`
	Icon              string   `json:"icon,omitempty"`
	Capabilities      []string `json:"capabilities,omitempty"`
	FieldsDataTypeURI string   `json:"fieldsDataTypeUri,omitempty"`
	AllowedChildren   []string `json:"allowedChildren,omitempty"`
	Abstract          bool     `json:"abstract,omitempty"`
	Error             string   `json:"error,omitempty"`
}

// ── create / update / delete ───────────────────────────────────────────────

// WorkspaceObjectMutationResult is the envelope every write returns: the
// object on success, the substrate's own message on failure. The message is
// passed through verbatim — "type 'module' is not allowed under 'note'" is
// what the user needs, and a binding that rewrites it into "create failed"
// destroys the only useful part.
type WorkspaceObjectMutationResult struct {
	Object WorkspaceObject `json:"object"`
	// Revision is the version just written: the pane's next save sends it.
	Revision string       `json:"revision,omitempty"`
	Error    string       `json:"error,omitempty"`
	ErrorMsg *usermsg.Msg `json:"errorMsg,omitempty"` // the error by key, in the person's language (errors namespace)
}

// WorkspaceObjectCreate writes a new object under a parent.
//
// `capabilitiesJSON` is the schema-driven form's output: a map of capability
// id → field group, exactly the shape the object stores. The container group
// is filled here from the name / parent the dialog collected, so the caller
// never has to know that `container.parent` IS the tree.
//
// Wails-bound: window.go.main.App.WorkspaceObjectCreate(universeName, parentID,
//
//	objectType, id, name, description, capabilitiesJSON).
func (a *App) WorkspaceObjectCreate(universeName, parentID, objectType, id, name, description, capabilitiesJSON string) WorkspaceObjectMutationResult {
	sender, err := a.universeSender(universeName)
	if err != nil {
		return WorkspaceObjectMutationResult{Error: fmt.Sprintf("Cannot reach universe %q: %v", universeName, err)}
	}
	out, err := objectCreateVia(a.ctx, sender, parentID, objectType, id, name, description, capabilitiesJSON)
	if err != nil {
		return WorkspaceObjectMutationResult{Error: err.Error(), ErrorMsg: errorMsgOf(err)}
	}
	return out
}

// objectCreateVia builds the object and asks CWM to create it. The type's
// `before_create` hooks run inside that create: a refusal comes back as its
// error, with the hook's reason, and nothing is written.
func objectCreateVia(ctx context.Context, sender ktsp.Sender,
	parentID, objectType, id, name, description, capabilitiesJSON string) (WorkspaceObjectMutationResult, error) {
	if objectType == "" {
		return WorkspaceObjectMutationResult{}, fmt.Errorf("a type is required")
	}
	if name == "" && id == "" {
		return WorkspaceObjectMutationResult{}, fmt.Errorf("a name or an id is required")
	}
	obj := cwmapi.Object{ID: id, Type: objectType}
	if capabilitiesJSON != "" {
		var caps map[string]map[string]any
		if err := json.Unmarshal([]byte(capabilitiesJSON), &caps); err != nil {
			return WorkspaceObjectMutationResult{}, fmt.Errorf("invalid capabilities JSON: %w", err)
		}
		obj.Capabilities = caps
	}
	// The container group carries the tree. Merged rather than replaced: a form
	// that already filled container fields keeps them.
	if name != "" {
		obj.Name = name
	}
	if description != "" {
		obj.Description = description
	}
	obj.Parent = parentID

	// The module is the id's own first segment when the caller authored one,
	// else the parent's. An object belongs to the module its id names — that
	// is the identity rule, and deriving the folder from anything else would
	// file it where its id says it is not.
	module := moduleOf(id)
	if module == "" {
		// A module-level type (its level capability, never its name -
		// ) makes a MODULE: its id is its own bare name and it is
		// its own module. Filed in the parent's module, CWM
		// would mint <parent's module>.<uuid> - an object of the project
		// root typed as a module, which the objects tree never lists.
		// An undeclared type is no module (CWM files it as a plain object);
		// any other failure to read the type is refused here, its reason
		// given - guessing « not a module » would file a module where the
		// tree never lists it.
		level, err := typeLevelVia(ctx, sender, objectType)
		if err != nil && !errors.Is(cwmapi.Restore(err), cwmapi.ErrNotFound) {
			return WorkspaceObjectMutationResult{}, err
		}
		if level == cwmapi.CapModule {
			// CWM's one slug spelling: accents transliterated, at most
			// cwmapi.SlugMax characters.
			slug := cwmapi.Slug(name)
			if slug == "" {
				return WorkspaceObjectMutationResult{}, fmt.Errorf("a module's name must hold a letter or a digit: %q", name)
			}
			obj.ID, module = slug, slug
			// Another project's root: the module is created in that
			// project, its key on the module (cwm's store set reads it).
			if key, _, err := cwmapi.ParseQualifiedID(parentID); err == nil {
				module = cwmapi.QualifiedID(key, slug)
			}
			// A module is parentless, as every module is: the project holds
			// every parentless object (identity-and-index.md §7, the
			// project level derived, never authored) - the tree draws an
			// object whose parent is the project object nowhere.
			obj.Parent = ""
		}
	}
	if module == "" {
		module = moduleOf(parentID)
	}
	if module == "" {
		return WorkspaceObjectMutationResult{}, fmt.Errorf("cannot tell which module this object belongs to: neither %q nor its parent %q names one", id, parentID)
	}
	in := cwmapi.ObjectCreateInput{Module: module, Object: obj}
	raw, err := sender.Call(ctx, "ktw-agents:cwm/object-create", in)
	if err != nil {
		return WorkspaceObjectMutationResult{}, err
	}
	var resp cwmapi.ObjectCreateOutput
	if err := json.Unmarshal(raw, &resp); err != nil {
		return WorkspaceObjectMutationResult{}, fmt.Errorf("decode response: %w", err)
	}
	return WorkspaceObjectMutationResult{Object: WorkspaceObject{
		ID: resp.Object.ID, Type: resp.Object.Type, Title: resp.Object.DisplayTitle(), Module: moduleOf(resp.Object.ID),
	}}, nil
}

// typeLevelVia reads a type's level - project, module or "" - from CWM, which
// owns the type registry.
func typeLevelVia(ctx context.Context, sender ktsp.Sender, typeName string) (string, error) {
	raw, err := sender.Call(ctx, "ktw-agents:cwm/object-type-get", cwmapi.ObjectTypeGetInput{Name: typeName})
	if err != nil {
		return "", fmt.Errorf("type %q: %w", typeName, err)
	}
	var resp cwmapi.ObjectTypeGetOutput
	if err := json.Unmarshal(raw, &resp); err != nil {
		return "", fmt.Errorf("decode type %q: %w", typeName, err)
	}
	return resp.ObjectType.Level(), nil
}

// WorkspaceObjectUpdate replaces an object's capability groups and aspects.
//
// WHOLE-OBJECT WRITE, deliberately: the file is the record, so the pane sends
// what it has rather than a patch. That is why it reads the object first —
// sending only the panel the user edited would silently drop every other
// capability group.
//
// Wails-bound: window.go.main.App.WorkspaceObjectUpdate(universeName, objectID,
//
//	capabilitiesJSON, aspectsJSON, revision).
//
// revision is the version the pane read (its detail, or its last save): a
// change made elsewhere since refuses the save with a message asking to
// reload (cwm.objectConflict) - never silently overwritten.
func (a *App) WorkspaceObjectUpdate(universeName, objectID, capabilitiesJSON, aspectsJSON, revision string) WorkspaceObjectMutationResult {
	sender, err := a.universeSender(universeName)
	if err != nil {
		return WorkspaceObjectMutationResult{Error: fmt.Sprintf("Cannot reach universe %q: %v", universeName, err)}
	}
	out, err := objectUpdateVia(a.ctx, sender, objectID, capabilitiesJSON, aspectsJSON, revision)
	if err != nil {
		return WorkspaceObjectMutationResult{Error: err.Error(), ErrorMsg: errorMsgOf(err)}
	}
	return out
}

// WorkspaceObjectUpdateWithBody is WorkspaceObjectUpdate that also replaces
// the object's body, in the same write — an editor whose object is composed
// from capability fields and the body (a slide deck: its settings in the
// `deck` group, its slides in the body) saves both at once.
//
// Wails-bound: window.go.main.App.WorkspaceObjectUpdateWithBody(universeName,
//
//	objectID, capabilitiesJSON, body, revision).
func (a *App) WorkspaceObjectUpdateWithBody(universeName, objectID, capabilitiesJSON, body, revision string) WorkspaceObjectMutationResult {
	sender, err := a.universeSender(universeName)
	if err != nil {
		return WorkspaceObjectMutationResult{Error: fmt.Sprintf("Cannot reach universe %q: %v", universeName, err)}
	}
	out, err := objectWriteVia(a.ctx, sender, objectID, capabilitiesJSON, "", &body, revision)
	if err != nil {
		return WorkspaceObjectMutationResult{Error: err.Error(), ErrorMsg: errorMsgOf(err)}
	}
	return out
}

func objectUpdateVia(ctx context.Context, sender ktsp.Sender, objectID, capabilitiesJSON, aspectsJSON, revision string) (WorkspaceObjectMutationResult, error) {
	return objectWriteVia(ctx, sender, objectID, capabilitiesJSON, aspectsJSON, nil, revision)
}

// objectWriteVia reads the object, merges the capability groups sent, sets
// the aspects when sent and the body when body is not nil, and writes it on
// condition of revision - the version the pane read: when the stored object
// changed since (by hand, an agent, git), CWM refuses the write with a
// conflict instead of the merge overwriting that change.
func objectWriteVia(ctx context.Context, sender ktsp.Sender, objectID, capabilitiesJSON, aspectsJSON string, body *string, revision string) (WorkspaceObjectMutationResult, error) {
	obj, module, err := readObjectVia(ctx, sender, objectID)
	if err != nil {
		return WorkspaceObjectMutationResult{}, err
	}
	if capabilitiesJSON != "" {
		var caps map[string]map[string]any
		if err := json.Unmarshal([]byte(capabilitiesJSON), &caps); err != nil {
			return WorkspaceObjectMutationResult{}, fmt.Errorf("invalid capabilities JSON: %w", err)
		}
		// Merge, group by group: a pane that sent two panels must not delete
		// the third.
		if obj.Capabilities == nil {
			obj.Capabilities = map[string]map[string]any{}
		}
		for id, group := range caps {
			if id == cwmapi.IdentityPanel { // the header saves the object core
				if err := setIdentityFrom(&obj, group); err != nil {
					return WorkspaceObjectMutationResult{}, err
				}
				continue
			}
			if id == cwmapi.FieldsPanel { // the type's fields tab saves the type's fields
				obj.Fields = group
				continue
			}
			obj.Capabilities[id] = group
		}
	}
	if aspectsJSON != "" {
		var aspects []string
		if err := json.Unmarshal([]byte(aspectsJSON), &aspects); err != nil {
			return WorkspaceObjectMutationResult{}, fmt.Errorf("invalid aspects JSON: %w", err)
		}
		obj.Aspects = aspects
	}
	if body != nil {
		obj.Body = *body
	}
	raw, err := sender.Call(ctx, "ktw-agents:cwm/object-update", cwmapi.ObjectUpdateInput{Module: module, Object: obj, Revision: revision})
	if err != nil {
		return WorkspaceObjectMutationResult{}, err
	}
	var resp cwmapi.ObjectUpdateOutput
	if err := json.Unmarshal(raw, &resp); err != nil {
		return WorkspaceObjectMutationResult{}, fmt.Errorf("decode response: %w", err)
	}
	return WorkspaceObjectMutationResult{Object: WorkspaceObject{
		ID: resp.Object.ID, Type: resp.Object.Type, Title: resp.Object.DisplayTitle(), Module: moduleOf(resp.Object.ID),
	}, Revision: resp.Revision}, nil
}

// setIdentityFrom writes what the header saved into the object core; the
// body, which is its type's, is left as it is.
func setIdentityFrom(o *cwmapi.Object, values map[string]any) error {
	raw, err := json.Marshal(values)
	if err != nil {
		return fmt.Errorf("identity: %w", err)
	}
	var id cwmapi.Identity
	if err := json.Unmarshal(raw, &id); err != nil {
		return fmt.Errorf("identity: %w", err)
	}
	o.SetIdentity(id)
	return nil
}

// readObjectVia loads one object and the module that owns it.
func readObjectVia(ctx context.Context, sender ktsp.Sender, objectID string) (cwmapi.Object, string, error) {
	if objectID == "" {
		return cwmapi.Object{}, "", fmt.Errorf("object id is required")
	}
	module := moduleOf(objectID)
	raw, err := sender.Call(ctx, "ktw-agents:cwm/object-get", cwmapi.ObjectGetInput{Module: module, ID: objectID})
	if err != nil {
		return cwmapi.Object{}, "", err
	}
	var resp cwmapi.ObjectGetOutput
	if err := json.Unmarshal(raw, &resp); err != nil {
		return cwmapi.Object{}, "", fmt.Errorf("decode response: %w", err)
	}
	return resp.Object, module, nil
}

// WorkspaceObjectDelete removes one object.
//
// Wails-bound: window.go.main.App.WorkspaceObjectDelete(universeName, objectID).
func (a *App) WorkspaceObjectDelete(universeName, objectID string) WorkspaceObjectMutationResult {
	sender, err := a.universeSender(universeName)
	if err != nil {
		return WorkspaceObjectMutationResult{Error: fmt.Sprintf("Cannot reach universe %q: %v", universeName, err)}
	}
	return objectDeleteVia(a.ctx, sender, objectID, false)
}

// ── actions ────────────────────────────────────────────────────────────────

// WorkspaceObjectActions is the action bar for one object.
//
// Wails-bound: window.go.main.App.WorkspaceObjectActions(universeName, objectID).
func (a *App) WorkspaceObjectActions(universeName, objectID string) []WorkspaceAction {
	return a.objectActionList(universeName, objectActionsInput(objectID))
}

// objectActionsInput asks the action surface for one object's action bar.
func objectActionsInput(objectID string) actionsapi.ListInput {
	return actionsapi.ListInput{ID: objectID, Scope: cwmapi.ActionScopeObject}
}

// WorkspaceObjectCollectionActions is the action row on a list header — the
// actions that belong to a COLLECTION of one type rather than to any object
// in it.
//
// Wails-bound: window.go.main.App.WorkspaceObjectCollectionActions(universeName, objectID, objectType).
func (a *App) WorkspaceObjectCollectionActions(universeName, objectID, objectType string) []WorkspaceAction {
	return a.objectActionList(universeName, collectionActionsInput(objectID, objectType))
}

// collectionActionsInput asks the action surface for a list header's row.
func collectionActionsInput(objectID, objectType string) actionsapi.ListInput {
	return actionsapi.ListInput{ID: objectID, Type: objectType, Scope: cwmapi.ActionScopeCollection}
}

func (a *App) objectActionList(universeName string, in actionsapi.ListInput) []WorkspaceAction {
	sender, err := a.universeSender(universeName)
	if err != nil {
		a.emitError(fmt.Sprintf("object actions %q: %v", universeName, err))
		return nil
	}
	out, err := objectActionsVia(a.ctx, sender, in)
	if err != nil {
		a.emitError(fmt.Sprintf("object actions %q: %v", universeName, err))
		return nil
	}
	return out
}

func objectActionsVia(ctx context.Context, sender ktsp.Sender, in actionsapi.ListInput) ([]WorkspaceAction, error) {
	raw, err := sender.Call(ctx, "ktw-agents:action-surface/list", in)
	if err != nil {
		return nil, fmt.Errorf("ktw-agents:action-surface/list: %w", err)
	}
	var resp actionsapi.ListOutput
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return workspaceActions(resp.Actions), nil
}

// ── links ──────────────────────────────────────────────────────────────────

// WorkspaceObjectLinkAdd authors an edge FROM this object.
//
// Wails-bound: window.go.main.App.WorkspaceObjectLinkAdd(universeName, fromID, kind, toID).
func (a *App) WorkspaceObjectLinkAdd(universeName, fromID, kind, toID string) WorkspaceObjectMutationResult {
	sender, err := a.universeSender(universeName)
	if err != nil {
		return WorkspaceObjectMutationResult{Error: fmt.Sprintf("Cannot reach universe %q: %v", universeName, err)}
	}
	return linkAddVia(a.ctx, sender, fromID, kind, toID)
}

func linkAddVia(ctx context.Context, sender ktsp.Sender, fromID, kind, toID string) WorkspaceObjectMutationResult {
	if fromID == "" || toID == "" || kind == "" {
		return WorkspaceObjectMutationResult{Error: "A link needs a source, a kind and a target."}
	}
	if _, err := sender.Call(ctx, "ktw-agents:cwm/edge-create",
		cwmapi.EdgeCreateInput{From: fromID, To: toID, Kind: cwmapi.EdgeType(kind)}); err != nil {
		return WorkspaceObjectMutationResult{Error: err.Error(), ErrorMsg: errorMsgOf(err)}
	}
	return WorkspaceObjectMutationResult{Object: WorkspaceObject{ID: fromID}}
}

// WorkspaceObjectLinkDelete removes one authored edge.
//
// Wails-bound: window.go.main.App.WorkspaceObjectLinkDelete(universeName, fromID, kind, toID).
func (a *App) WorkspaceObjectLinkDelete(universeName, fromID, kind, toID string) WorkspaceObjectMutationResult {
	sender, err := a.universeSender(universeName)
	if err != nil {
		return WorkspaceObjectMutationResult{Error: fmt.Sprintf("Cannot reach universe %q: %v", universeName, err)}
	}
	return linkDeleteVia(a.ctx, sender, fromID, kind, toID)
}

func linkDeleteVia(ctx context.Context, sender ktsp.Sender, fromID, kind, toID string) WorkspaceObjectMutationResult {
	if _, err := sender.Call(ctx, "ktw-agents:cwm/edge-delete",
		cwmapi.EdgeDeleteInput{From: fromID, To: toID, Kind: cwmapi.EdgeType(kind)}); err != nil {
		return WorkspaceObjectMutationResult{Error: err.Error(), ErrorMsg: errorMsgOf(err)}
	}
	return WorkspaceObjectMutationResult{Object: WorkspaceObject{ID: fromID}}
}

// ── find ───────────────────────────────────────────────────────────────────

// WorkspaceObjectFindResult is one row of the search box, or the answer that a
// pasted id belongs to a project this machine does not have.
type WorkspaceObjectFindResult struct {
	Objects []WorkspaceObject `json:"objects"`
	// ResolvedID is set when the query was an id or a full name and it named
	// an object here — the GUI opens it instead of showing a list.
	ResolvedID string `json:"resolvedId,omitempty"`
	// NotMounted carries the project id when the reference names a project
	// this universe has not mounted. That is a different answer from "no
	// match" and the user must be able to tell them apart.
	NotMounted string `json:"notMounted,omitempty"`
	Error      string `json:"error,omitempty"`
}

// WorkspaceObjectFind is the one search box (objects-what-the-user-sees §9).
//
// A query that LOOKS like a reference — it has a dot and no spaces — is
// resolved first, because a full name pasted from a log must open the object
// rather than fuzzy-match its id as text. Anything else, and anything that
// fails to resolve, is a search.
//
// Wails-bound: window.go.main.App.WorkspaceObjectFind(universeName, query).
func (a *App) WorkspaceObjectFind(universeName, query string) WorkspaceObjectFindResult {
	sender, err := a.universeSender(universeName)
	if err != nil {
		return WorkspaceObjectFindResult{Error: fmt.Sprintf("Cannot reach universe %q: %v", universeName, err)}
	}
	out, err := objectFindVia(a.ctx, sender, query)
	if err != nil {
		return WorkspaceObjectFindResult{Error: err.Error()}
	}
	return out
}

func objectFindVia(ctx context.Context, sender ktsp.Sender, query string) (WorkspaceObjectFindResult, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return WorkspaceObjectFindResult{Objects: []WorkspaceObject{}}, nil
	}
	if looksLikeAReference(query) {
		raw, err := sender.Call(ctx, "ktw-agents:cwm/object-resolve", cwmapi.ObjectResolveInput{Ref: query})
		if err == nil {
			var resp cwmapi.ObjectResolveOutput
			if jerr := json.Unmarshal(raw, &resp); jerr == nil && resp.ID != "" {
				return WorkspaceObjectFindResult{
					ResolvedID: resp.ID,
					Objects:    []WorkspaceObject{{ID: resp.ID, Type: resp.Type, Title: resp.ID, Module: moduleOf(resp.ID)}},
				}, nil
			}
		} else if projectID, ok := otherProjectFromResolveError(err); ok {
			// The reference is well-formed but names ANOTHER project: say so
			// with the project id, which is what a user pastes to someone who
			// has it. Falling through to a text search would answer "no
			// results" to a question that has an answer.
			return WorkspaceObjectFindResult{NotMounted: projectID, Objects: []WorkspaceObject{}}, nil
		}
	}
	raw, err := sender.Call(ctx, "ktw-agents:cwm/object-search", cwmapi.ObjectSearchInput{Query: query, Limit: 50})
	if err != nil {
		return WorkspaceObjectFindResult{}, fmt.Errorf("ktw-agents:cwm/object-search: %w", err)
	}
	// Each found object with the project holding it, when the universe
	// serves several.
	found, err := locatedObjects(raw)
	if err != nil {
		return WorkspaceObjectFindResult{}, fmt.Errorf("decode response: %w", err)
	}
	system := systemModulesVia(ctx, sender)
	out := WorkspaceObjectFindResult{Objects: make([]WorkspaceObject, 0, len(found))}
	for _, f := range found {
		o := f.Object
		out.Objects = append(out.Objects, WorkspaceObject{
			ID: o.ID, Type: o.Type, Title: o.DisplayTitle(), Module: moduleOf(o.ID),
			System:  system[o.ID] || system[moduleOf(o.ID)],
			Project: f.Project,
		})
	}
	return out, nil
}

// systemModulesVia is the set of the universe's system modules - the objects
// carrying the system-module capability, never a name test; empty when CWM
// cannot answer.
func systemModulesVia(ctx context.Context, sender ktsp.Sender) map[string]bool {
	out := map[string]bool{}
	raw, err := sender.Call(ctx, "ktw-agents:cwm/object-list", cwmapi.ObjectListInput{Capability: cwmapi.CapSystemModule})
	if err != nil {
		return out
	}
	var resp cwmapi.ObjectListOutput
	if json.Unmarshal(raw, &resp) == nil {
		for _, o := range resp.Objects {
			out[o.ID] = true
		}
	}
	return out
}

// looksLikeAReference is deliberately loose: a dot and no whitespace. It only
// decides which call to TRY first — a wrong guess costs one resolve that fails
// and falls through to the search, never a wrong answer.
func looksLikeAReference(q string) bool {
	return strings.Contains(q, ".") && !strings.ContainsAny(q, " \t")
}

// otherProjectFromResolveError reads the project id out of the resolver's
// "belongs to another project" refusal. Text, because that is what crosses
// the wire: KTSP carries an error string, not a typed error, so the binding
// either parses the message or loses the distinction between "not here" and
// "nowhere".
func otherProjectFromResolveError(err error) (string, bool) {
	msg := err.Error()
	const marker = "belongs to project "
	i := strings.Index(msg, marker)
	if i < 0 {
		return "", false
	}
	rest := msg[i+len(marker):]
	if j := strings.IndexAny(rest, " ,;:\n"); j > 0 {
		rest = rest[:j]
	}
	rest = strings.Trim(rest, "\"'")
	if rest == "" {
		return "", false
	}
	return rest, true
}

// ── capability checks ──────────────────────────────────────────────────────

// ObjectVerifyRunResult is a verification run through the action surface:
// Verify, a long method, answers at once with its run and the display panel
// it reports each check into - drawn under the check, its outcome said there
// once it completes; a verification that answers its value at once, or one
// that could not run, gives the pane's check result (Check).
type ObjectVerifyRunResult struct {
	TaskID string          `json:"taskId,omitempty"`
	Panel  string          `json:"panel,omitempty"`
	Check  *CapCheckResult `json:"check,omitempty"`
}

// WorkspaceObjectVerifyRun verifies an object and everything under it - every
// verify hook in force runs (a criterion's records its verdict) - through
// ActionService: the object is the run's target and the
// call answers with the run's panel instead of waiting for every check - no
// bus fallback, no client deadline on the checks. A failure to reach the
// service is a harness-error, never a pass.
//
// Wails-bound: window.go.main.App.WorkspaceObjectVerifyRun(universeName, objectID).
func (a *App) WorkspaceObjectVerifyRun(universeName, objectID string) ObjectVerifyRunResult {
	sender, err := a.universeSender(universeName)
	if err != nil {
		return ObjectVerifyRunResult{Check: &CapCheckResult{Status: "harness-error", Error: err.Error()}}
	}
	ctx, cancel := a.actionRunContext(universeName, objectVerifyURI)
	defer cancel()
	return objectVerifyRun(ctx, sender, objectID)
}

// objectVerifyURI is the method a Verify run calls.
const objectVerifyURI = "ktw-agents:verification/verify"

// objectVerifyRun is one Verify run through ActionService.Run, its answer
// read: a run with its panel, or the outcome summed up.
func objectVerifyRun(ctx context.Context, sender ktsp.Sender, objectID string) ObjectVerifyRunResult {
	const uri = objectVerifyURI
	raw, err := json.Marshal(verificationapi.VerifyInput{ID: objectID})
	if err != nil {
		return ObjectVerifyRunResult{Check: &CapCheckResult{Status: "harness-error", Error: err.Error()}}
	}
	r := runActionCall(ctx, sender, uri, actionsapi.RunInput{URI: uri, Input: raw, Context: map[string]any{"_target": objectID}})
	switch {
	case r.Error != "":
		return ObjectVerifyRunResult{Check: &CapCheckResult{Status: "harness-error", Error: "verify failed: " + r.Error}}
	case r.Panel != "":
		return ObjectVerifyRunResult{TaskID: r.TaskID, Panel: r.Panel}
	}
	var out verificationapi.VerifyOutput
	if r.Value == "" || json.Unmarshal([]byte(r.Value), &out) != nil {
		return ObjectVerifyRunResult{Check: &CapCheckResult{Status: "harness-error", Error: "decode the verification"}}
	}
	res := verifyResult(out)
	return ObjectVerifyRunResult{Check: &res}
}

// verifyResult sums a verification up for the pane.
func verifyResult(out verificationapi.VerifyOutput) CapCheckResult {
	counts := fmt.Sprintf("%d run, %d passed, %d failed, %d unproven", out.Runs, out.Passed, len(out.Failed), len(out.Unproven))
	var first *verificationapi.VerifyOutcome
	status := "pass"
	switch {
	case out.Runs == 0:
		status = "unproven"
		counts = "nothing to verify"
	case len(out.Failed) > 0:
		status, first = "fail", &out.Failed[0]
	case len(out.Unproven) > 0:
		status, first = "unproven", &out.Unproven[0]
	}
	if first != nil {
		counts += " — " + first.ObjectID + ": " + first.Status
		if first.Evidence != "" {
			counts += " (" + first.Evidence + ")"
		}
	}
	return CapCheckResult{Status: status, Desc: counts}
}
