// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"

	actionsapi "github.com/katyoti/katyoti-fwk/internal/extensions/actions/api"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	paneapi "github.com/katyoti/katyoti-fwk/internal/extensions/objectpane/api"
	ktsapi "github.com/katyoti/katyoti-fwk/internal/kts/api"
)

// BINDING-LEVEL TESTS FOR THE DETAIL SURFACE (the GUI port, 2026-09-25).
//
// What these catch is what a compile cannot: the WIRE. Every one of them
// marshals the core's real output type, hands it to the binding as bytes, and
// asserts on the rows the pane draws — so a renamed json tag, a map that
// arrives as `[]any`, or an id taken from the wrong space fails here instead
// of in front of the user.
//
// They also assert the CALLS: which verb, with which input. The port's whole
// risk is a binding that quietly asks for less than the pane shows.

// multiStub answers by path and records every call, so a binding that makes
// two round trips is visible as two round trips.
type multiStub struct {
	t       *testing.T
	replies map[string]any
	errs    map[string]error
	calls   []stubCall
}

type stubCall struct {
	path  string
	input any
}

func (s *multiStub) Call(_ context.Context, path string, params any, _ ...ktsapi.CallOption) (json.RawMessage, error) {
	s.calls = append(s.calls, stubCall{path: path, input: params})
	if err, ok := s.errs[path]; ok {
		return nil, err
	}
	reply, ok := s.replies[path]
	if !ok {
		s.t.Fatalf("binding called %q, which the stub was not given an answer for (calls so far: %d)", path, len(s.calls))
	}
	raw, err := json.Marshal(reply)
	if err != nil {
		s.t.Fatalf("stub marshal %s: %v", path, err)
	}
	return raw, nil
}

func (s *multiStub) Notify(context.Context, string, any, ...ktsapi.CallOption) error { return nil }

func (s *multiStub) inputFor(path string) any {
	for _, c := range s.calls {
		if c.path == path {
			return c.input
		}
	}
	s.t.Fatalf("no call to %q; got %v", path, s.paths())
	return nil
}

func (s *multiStub) paths() []string {
	out := make([]string, 0, len(s.calls))
	for _, c := range s.calls {
		out = append(out, c.path)
	}
	return out
}

// detailReply is the pane the core would answer with: a module carrying a
// type-declared capability, an instance-added one, a declared aspect and an
// undeclared one, a link out and a link in, and two child groups.
// actionsReply is the object's action surface.
func actionsReply() actionsapi.ListOutput {
	return actionsapi.ListOutput{Actions: []cwmapi.Action{{URI: "p:tasks/refine", Label: "Refine", Kind: "ai", Source: "applies-to"}, {URI: "p:tasks/archive", Label: "Archive", Kind: "sync", Source: "type", Disabled: true, DisabledReason: "condition not met"}}}
}

func detailReply() paneapi.DetailOutput {
	o := cwmapi.Object{ID: "billing.us-12", Type: "feature", Aspects: []string{"isa", "ghost"}}
	o.Name = "Invoice run"
	o.Description = "the nightly run"
	o.Parent = "billing"
	o.Name = "Invoice run"
	o.Tags = []string{"billing"}
	o.SetLifecycle(cwmapi.Lifecycle{State: "in-progress"})
	o.Capabilities["checkable"] = map[string]any{"kind": "command", "probe": "make test"}
	return paneapi.DetailOutput{
		Object:     o,
		ObjectType: cwmapi.ObjectType{Name: "feature", Label: "Feature", Icon: "book"},
		Effective: []cwmapi.CapabilityRef{
			{ID: "container", Label: "Container", Source: "type"},
			{ID: "content", Label: "Content", Source: "type"},
			{ID: "marker", Source: "type"}, // a marker: no panel
			{ID: "checkable", Label: "Checkable", Verify: true, Schema: "ktw-methods:/data-type/checkable", Source: "object"},
		},
		Capabilities: []string{"container", "content", "marker", "checkable"},
		// The strip the MODEL declares: the marker contributes none.
		Panels: []paneapi.ObjectPanel{
			{ID: "cap:container", Name: "Container", Kind: paneapi.PanelKindFields, Capabilities: []string{"container"}, Schema: "s:/container", Origin: "type"},
			{ID: "cap:content", Name: "Content", Kind: paneapi.PanelKindFields, Capabilities: []string{"content"}, Schema: "s:/content", Origin: "type"},
			{ID: "cap:checkable", Name: "Checkable", Kind: paneapi.PanelKindFields, Capabilities: []string{"checkable"}, Schema: "ktw-methods:/data-type/checkable", Verify: true, Origin: "object"},
		},
		Aspects: []paneapi.AspectRef{
			{Name: "isa", Label: "ISA", Capabilities: []string{"want", "checkable"}, Declared: true},
			{Name: "ghost", Declared: false},
		},
		Badges:       []cwmapi.Badge{{ID: "lifecycle.state", Label: "in-progress", Icon: "progress", Color: "blue", Tooltip: "where it stands", Capability: "lifecycle"}},
		QuickActions: []cwmapi.Action{{URI: cwmapi.SystemTaskChatURI, Label: "Start chat", Kind: "sync", Source: "quick"}},
		Links: []paneapi.ObjectLink{
			{Kind: "serves", Label: "serves", Direction: "out", TargetID: "billing.g1", TargetType: "goal", TargetTitle: "One invoice run", Resolved: true},
			{Kind: "serves", Label: "served-by", Direction: "in", TargetID: "billing.uc-3", TargetType: "use-case", TargetTitle: "Run invoices", Resolved: true},
			{Kind: "documents", Label: "documents", Direction: "out", TargetID: "docs/spec.md", Resolved: false},
		},
		Children: []paneapi.ObjectChildGroup{{
			Type: "acceptance-criterion", Label: "Acceptance criteria", Count: 2,
			Objects: []cwmapi.Object{objWithTitle("billing.ac-1", "acceptance-criterion", "Totals match"), objWithTitle("billing.ac-2", "acceptance-criterion", "No duplicate lines")},
		}},
		Diagnostics: []cwmapi.ObjectDiagnostic{{Code: "aspect-not-declared", Label: "aspect 'ghost' is not declared"}},
	}
}

func objWithTitle(id, typ, title string) cwmapi.Object {
	o := cwmapi.Object{ID: id, Type: typ}
	o.Name = title
	return o
}

func TestObjectDetailBinding_DrawsTheWholePane(t *testing.T) {
	s := &multiStub{t: t, replies: map[string]any{"ktw-agents:object-pane/detail": detailReply(), "ktw-agents:action-surface/list": actionsReply()}}
	got, err := objectDetailVia(context.Background(), s, "billing.us-12")
	if err != nil {
		t.Fatalf("objectDetailVia: %v", err)
	}
	// TWO round trips: the object, and what it offers.
	if len(s.calls) != 2 || s.calls[0].path != "ktw-agents:object-pane/detail" || s.calls[1].path != "ktw-agents:action-surface/list" {
		t.Errorf("calls: %v, want ObjectDetail then action-surface/List", s.paths())
	}
	// The body is asked for HERE and nowhere else — the tree walk never sets it.
	in, ok := s.inputFor("ktw-agents:object-pane/detail").(paneapi.DetailInput)
	if !ok || !in.Body {
		t.Errorf("the detail pane renders the body; the call must ask for it: %+v", s.calls[0].input)
	}
	if got.ID != "billing.us-12" || got.Title != "Invoice run" || got.Module != "billing" {
		t.Errorf("header: %+v", got)
	}
	if got.ParentID != "billing" {
		t.Errorf("parent: %q, want billing (container.parent IS the tree)", got.ParentID)
	}
	if got.TypeLabel != "Feature" || got.TypeIcon != "book" {
		t.Errorf("type: %q / %q", got.TypeLabel, got.TypeIcon)
	}
	if got.Status != "in-progress" || len(got.Tags) != 1 {
		t.Errorf("content fields: status %q tags %v", got.Status, got.Tags)
	}
	if len(got.Badges) != 1 || got.Badges[0] != (WorkspaceBadge{ID: "lifecycle.state", Label: "in-progress", Icon: "progress", Color: "blue", Tooltip: "where it stands", Capability: "lifecycle"}) {
		t.Errorf("badges: %+v", got.Badges)
	}
	// THE STRIP IS THE MODEL'S, not one tab per capability: four effective
	// capabilities, three panels, because `marker` holds no fields and no panel.
	if len(got.Tabs) != 3 {
		t.Fatalf("tabs: %d, want 3 — the model declared 3 panels for 4 capabilities: %+v", len(got.Tabs), got.Tabs)
	}
	for _, tab := range got.Tabs {
		if slices.Contains(tab.Capabilities, "marker") {
			t.Errorf("a marker capability must draw no tab: %+v", tab)
		}
	}
	// ...but the marker is still REPORTED, so nothing is hidden.
	if !hasCapability(got.Capabilities, "marker") {
		t.Errorf("the marker capability vanished from the pane entirely: %v", got.Capabilities)
	}
	var checkable *WorkspaceObjectTab
	for i := range got.Tabs {
		if slices.Contains(got.Tabs[i].Capabilities, "checkable") {
			checkable = &got.Tabs[i]
		}
	}
	if checkable == nil {
		t.Fatalf("no checkable tab: %+v", got.Tabs)
	}
	if !checkable.Verify || checkable.Schema == "" || checkable.Origin != "object" {
		t.Errorf("the verifiable instance capability lost its verify / schema / source: %+v", *checkable)
	}
	// The panel's FILTER is resolved against the object: its values are keyed
	// by the capability they came from.
	if checkable.Values["checkable"]["probe"] != "make test" {
		t.Errorf("the panel carries no values for its capability: %+v", checkable.Values)
	}
	// The chip row, undeclared aspect included.
	if len(got.Aspects) != 2 {
		t.Fatalf("aspects: %+v, want both the declared and the undeclared one", got.Aspects)
	}
	for _, tr := range got.Aspects {
		if tr.Name == "ghost" && tr.Declared {
			t.Errorf("an undeclared aspect must arrive as undeclared: %+v", tr)
		}
	}
	// Links, named from THIS side: the incoming one reads by its inverse.
	if len(got.Links) != 3 {
		t.Fatalf("links: %+v", got.Links)
	}
	if got.Links[1].Direction != "in" || got.Links[1].Label != "served-by" {
		t.Errorf("the incoming link must read by the inverse name: %+v", got.Links[1])
	}
	if got.Links[2].Resolved {
		t.Errorf("a path target is not an object and must arrive unresolved: %+v", got.Links[2])
	}
	// Children by type, with their titles.
	if len(got.Children) != 1 || got.Children[0].Count != 2 || len(got.Children[0].Objects) != 2 {
		t.Fatalf("children: %+v", got.Children)
	}
	if got.Children[0].Objects[0].Title != "Totals match" {
		t.Errorf("a child row lost its title: %+v", got.Children[0].Objects[0])
	}
	// Actions, with the greyed one greyed AND its reason.
	if len(got.Actions) != 2 || len(got.QuickActions) != 1 {
		t.Fatalf("actions: %+v / quick: %+v", got.Actions, got.QuickActions)
	}
	if !got.Actions[1].Disabled || got.Actions[1].DisabledReason == "" {
		t.Errorf("a disabled action must carry its reason to the tooltip: %+v", got.Actions[1])
	}
	if len(got.Errors) != 1 {
		t.Errorf("diagnostics: %+v", got.Errors)
	}
}

// « + New » lists the types allowed under the parent, then the aspects that
// name the task creating an object carrying them: an aspect with
// no create task is no choice.
func TestObjectChildTypesBinding_TypesThenCreatableAspects(t *testing.T) {
	s := &multiStub{t: t, replies: map[string]any{
		"ktw-agents:cwm/object-allowed-children": cwmapi.ObjectAllowedChildrenOutput{Choices: []cwmapi.TypeChoice{
			{Name: "acceptance-criterion", Label: "Acceptance criterion", Icon: "check"},
			{Name: "note", Label: "Note"},
		}},
		"ktw-agents:cwm/object-aspect-list": cwmapi.ObjectAspectListOutput{Aspects: []cwmapi.Aspect{
			{Name: "etude-existant", Label: "Étude de l'existant", Capabilities: []string{"asis-study"}, CreateTask: "ktw-acme:tasks/new-study"},
			{Name: "isa", Label: "ISA", Capabilities: []string{"want", "checkable"}},
		}},
	}}
	got, err := objectChildTypesVia(context.Background(), s, "billing.us-12")
	if err != nil {
		t.Fatalf("objectChildTypesVia: %v", err)
	}
	// Two calls: the allowed types, then the aspects.
	if len(s.calls) != 2 {
		t.Errorf("calls: %v, want two", s.paths())
	}
	if len(got) != 3 || got[0].Name != "acceptance-criterion" || got[0].Label != "Acceptance criterion" {
		t.Fatalf("picker rows: %+v", got)
	}
	if a := got[2]; a.Name != "etude-existant" || a.Label != "Étude de l'existant" || a.CreateTask != "ktw-acme:tasks/new-study" {
		t.Fatalf("the creatable aspect: %+v", a)
	}
}

// ── create ─────────────────────────────────────────────────────────────────

func TestObjectCreateBinding_TheParentTravelsInsideTheObject(t *testing.T) {
	created := objWithTitle("billing.ac-3", "acceptance-criterion", "Totals reconcile")
	s := &multiStub{t: t, replies: map[string]any{
		"ktw-agents:cwm/object-create":   cwmapi.ObjectCreateOutput{Object: created, Path: "modules/billing/objects/ac-3.cwm.yaml"},
		"ktw-agents:cwm/object-type-get": cwmapi.ObjectTypeGetOutput{}, // below the module level
	}}
	got, err := objectCreateVia(context.Background(), s,
		"billing.us-12", "acceptance-criterion", "", "Totals reconcile", "totals must reconcile", "")
	if err != nil {
		t.Fatalf("objectCreateVia: %v", err)
	}
	in, ok := s.inputFor("ktw-agents:cwm/object-create").(cwmapi.ObjectCreateInput)
	if !ok {
		t.Fatalf("create input shape: %T", s.calls[0].input)
	}
	// THE TREE IS A FIELD. The parent is inside the object, not a second
	// argument — if this regresses, objects are written as roots.
	c, has := in.Object, true
	if !has || c.Parent != "billing.us-12" {
		t.Errorf("container.parent: %+v, want billing.us-12", c)
	}
	if c.Name != "Totals reconcile" || c.Description != "totals must reconcile" {
		t.Errorf("container fields: %+v", c)
	}
	// The module is where the id says it is — derived from the parent when the
	// caller authored no id.
	if in.Module != "billing" {
		t.Errorf("module: %q, want billing", in.Module)
	}
	if got.Object.ID != "billing.ac-3" {
		t.Errorf("result: %+v", got.Object)
	}
}

func TestObjectCreateBinding_CapabilityGroupsFromTheFormSurvive(t *testing.T) {
	s := &multiStub{t: t, replies: map[string]any{
		"ktw-agents:cwm/object-create": cwmapi.ObjectCreateOutput{Object: objWithTitle("billing.g2", "goal", "Ship it")},
	}}
	caps := `{"want":{"title":"Ship it","reachability":"reachable"},"content":{"status":"todo"}}`
	if _, err := objectCreateVia(context.Background(), s, "billing", "goal", "billing.g2", "Ship it", "", caps); err != nil {
		t.Fatalf("objectCreateVia: %v", err)
	}
	in := s.inputFor("ktw-agents:cwm/object-create").(cwmapi.ObjectCreateInput)
	if in.Object.Capabilities["want"]["reachability"] != "reachable" {
		t.Errorf("the form's capability groups did not reach the write: %+v", in.Object.Capabilities)
	}
	// And the container the binding fills does not erase them.
	if _, ok := in.Object.Capabilities["content"]; !ok {
		t.Errorf("filling container dropped another group: %+v", in.Object.Capabilities)
	}
}

// TestObjectCreateBinding_BeforeCreateRefusalReachesTheUser — the binding
// runs no before_create of its own: CWM's create runs the type's hooks, and a
// refusal comes back as the create's error. The user sees the hook's reason,
// and the binding calls nothing besides the create - and the type's level
// read (a module-level type makes a module) - no action.
func TestObjectCreateBinding_BeforeCreateRefusalReachesTheUser(t *testing.T) {
	s := &multiStub{t: t, errs: map[string]error{
		"ktw-agents:cwm/object-create": fmt.Errorf("%w: before_create x:goals/check: a goal needs a reachability", cwmapi.ErrRefusedByHook),
	}, replies: map[string]any{"ktw-agents:cwm/object-type-get": cwmapi.ObjectTypeGetOutput{}}}
	_, err := objectCreateVia(context.Background(), s, "billing", "goal", "", "X", "", "")
	if err == nil {
		t.Fatal("a before_create refusal must fail the create")
	}
	if !strings.Contains(err.Error(), "a goal needs a reachability") {
		t.Errorf("the refusal's REASON is what the user needs; got %q", err)
	}
	if got := s.paths(); len(got) != 2 || got[0] != "ktw-agents:cwm/object-type-get" || got[1] != "ktw-agents:cwm/object-create" {
		t.Errorf("the binding must call the create and nothing else; got %v", got)
	}
}

// ── update ─────────────────────────────────────────────────────────────────

// TestObjectUpdateBinding_MergesRatherThanReplacing — a whole-object write
// that sent only the edited panel would silently delete every other capability
// group. The binding reads the object first for exactly that reason.
func TestObjectUpdateBinding_MergesRatherThanReplacing(t *testing.T) {
	stored := cwmapi.Object{ID: "billing.us-12", Type: "feature"}
	stored.Name = "Invoice run"
	stored.Parent = "billing"
	stored.Name = "Invoice run"
	stored.SetLifecycle(cwmapi.Lifecycle{State: "todo"})
	stored.Capabilities["checkable"] = map[string]any{"probe": "make test"}

	s := &multiStub{t: t, replies: map[string]any{
		"ktw-agents:cwm/object-get":    cwmapi.ObjectGetOutput{Object: stored},
		"ktw-agents:cwm/object-update": cwmapi.ObjectUpdateOutput{Object: stored},
	}}
	if _, err := objectUpdateVia(context.Background(), s, "billing.us-12", `{"content":{"status":"done"}}`, `["isa"]`, ""); err != nil {
		t.Fatalf("objectUpdateVia: %v", err)
	}
	in, ok := s.inputFor("ktw-agents:cwm/object-update").(cwmapi.ObjectUpdateInput)
	if !ok {
		t.Fatalf("update input shape: %T", s.calls[1].input)
	}
	if in.Module != "billing" {
		t.Errorf("module: %q", in.Module)
	}
	if in.Object.Capabilities["content"]["status"] != "done" {
		t.Errorf("the edited panel did not reach the write: %+v", in.Object.Capabilities["content"])
	}
	if _, ok := in.Object.Capabilities["checkable"]; !ok {
		t.Errorf("a group the pane did not send was DELETED: %+v", in.Object.Capabilities)
	}
	if c := in.Object; c.Parent != "billing" {
		t.Errorf("the update moved the object out of its parent: %+v", c)
	}
	if len(in.Object.Aspects) != 1 || in.Object.Aspects[0] != "isa" {
		t.Errorf("the + Aspect chip did not reach the write: %+v", in.Object.Aspects)
	}
}

// TestObjectUpdateWithBody_WritesGroupAndBodyTogether — a deck object saves its
// settings (the `deck` group) and its slides (the body) in one write, and
// keeps every group it did not send.
func TestObjectUpdateWithBody_WritesGroupAndBodyTogether(t *testing.T) {
	stored := cwmapi.Object{ID: "talks.d1", Type: "slide-deck", Body: "# old"}
	stored.Name = "Deck"
	stored.Capabilities = map[string]map[string]any{}
	stored.Capabilities["deck"] = map[string]any{"theme": "katyoti", "author": "me"}
	stored.Capabilities["checkable"] = map[string]any{"probe": "x"}

	s := &multiStub{t: t, replies: map[string]any{
		"ktw-agents:cwm/object-get":    cwmapi.ObjectGetOutput{Object: stored},
		"ktw-agents:cwm/object-update": cwmapi.ObjectUpdateOutput{Object: stored},
	}}
	body := "<!-- slide: layout=cover -->\n# New"
	if _, err := objectWriteVia(context.Background(), s, "talks.d1", `{"deck":{"theme":"acme","aspect":"4:3"}}`, "", &body, ""); err != nil {
		t.Fatalf("objectWriteVia: %v", err)
	}
	in := s.inputFor("ktw-agents:cwm/object-update").(cwmapi.ObjectUpdateInput)
	if in.Object.Body != body {
		t.Errorf("body: %q", in.Object.Body)
	}
	if in.Object.Capabilities["deck"]["theme"] != "acme" || in.Object.Capabilities["deck"]["aspect"] != "4:3" {
		t.Errorf("deck group: %+v", in.Object.Capabilities["deck"])
	}
	if _, ok := in.Object.Capabilities["checkable"]; !ok {
		t.Errorf("a group not sent was deleted: %+v", in.Object.Capabilities)
	}
}

// ── find ───────────────────────────────────────────────────────────────────

func TestObjectFindBinding_ResolvesAFullNameToTheObject(t *testing.T) {
	s := &multiStub{t: t, replies: map[string]any{
		"ktw-agents:cwm/object-resolve": cwmapi.ObjectResolveOutput{ID: "cwm.g1", Type: "goal", ProjectID: "0193c2f4"},
	}}
	got, err := objectFindVia(context.Background(), s, "0193c2f4.cwm.g1")
	if err != nil {
		t.Fatalf("objectFindVia: %v", err)
	}
	if got.ResolvedID != "cwm.g1" {
		t.Errorf("a full name pasted from a log must OPEN the object: %+v", got)
	}
	// And it did not also run a text search for it.
	for _, p := range s.paths() {
		if p == "ktw-agents:cwm/object-search" {
			t.Errorf("a resolved reference must not fall through to a search: %v", s.paths())
		}
	}
}

func TestObjectFindBinding_SaysWhenTheProjectIsNotMounted(t *testing.T) {
	s := &multiStub{t: t,
		replies: map[string]any{"ktw-agents:cwm/object-search": cwmapi.ObjectSearchOutput{}},
		errs: map[string]error{"ktw-agents:cwm/object-resolve": fmt.Errorf(
			"cwm: not found: 0193c2f4.cwm.g1 belongs to project 0193c2f4, not to this one (aaaa1111)")},
	}
	got, err := objectFindVia(context.Background(), s, "0193c2f4.cwm.g1")
	if err != nil {
		t.Fatalf("objectFindVia: %v", err)
	}
	if got.NotMounted != "0193c2f4" {
		t.Errorf("a reference to another project must say WHICH project: %+v", got)
	}
	// "not mounted here" and "no such thing" are different answers, so it must
	// NOT degrade into an empty search result.
	for _, p := range s.paths() {
		if p == "ktw-agents:cwm/object-search" {
			t.Errorf("it fell through to a search and lost the answer: %v", s.paths())
		}
	}
}

func TestObjectFindBinding_PlainTextIsASearch(t *testing.T) {
	s := &multiStub{t: t, replies: map[string]any{
		"ktw-agents:cwm/object-search": cwmapi.ObjectSearchOutput{
			Objects: []cwmapi.Object{objWithTitle("billing.us-12", "feature", "Invoice run"), objWithTitle("sys-inbox.IB-001", "inbox-item", "Invoice run failed")}, Total: 2},
		// The system modules, by their capability.
		"ktw-agents:cwm/object-list": cwmapi.ObjectListOutput{Objects: []cwmapi.Object{{ID: "sys-inbox", Type: "system-module"}}},
	}}
	got, err := objectFindVia(context.Background(), s, "invoice run")
	if err != nil {
		t.Fatalf("objectFindVia: %v", err)
	}
	if got.ResolvedID != "" || len(got.Objects) != 2 || got.Objects[0].Title != "Invoice run" {
		t.Errorf("search result: %+v", got)
	}
	if got.Objects[0].System || !got.Objects[1].System {
		t.Errorf("an object in a system module is flagged system, a user one not: %+v", got.Objects)
	}
	if len(s.calls) == 0 || s.calls[0].path != "ktw-agents:cwm/object-search" {
		t.Errorf("a phrase with a space is not a reference; calls: %v", s.paths())
	}
}

func hasCapability(list []string, want string) bool {
	for _, c := range list {
		if c == want {
			return true
		}
	}
	return false
}

// : a declared panel's count action reaches the GUI's tab
// which calls it and draws the answer as the tab's badge.
func TestObjectDetailBinding_TheTabCarriesItsCountAction(t *testing.T) {
	reply := detailReply()
	reply.Panels = append(reply.Panels, paneapi.ObjectPanel{ID: "tab:open", Name: "Open", Kind: paneapi.PanelKindSource, Source: "ktw-x:open/board", Count: "ktw-x:open/count", Origin: "panel"})
	s := &multiStub{t: t, replies: map[string]any{"ktw-agents:object-pane/detail": reply, "ktw-agents:action-surface/list": actionsReply()}}
	got, err := objectDetailVia(context.Background(), s, "billing.us-12")
	if err != nil {
		t.Fatalf("objectDetailVia: %v", err)
	}
	for _, tab := range got.Tabs {
		if tab.ID == "tab:open" {
			if tab.Count != "ktw-x:open/count" {
				t.Errorf("the tab lost its count action: %q", tab.Count)
			}
			return
		}
	}
	t.Fatalf("no tab:open in %+v", got.Tabs)
}
