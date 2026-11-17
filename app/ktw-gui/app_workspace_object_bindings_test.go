// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	actionsapi "github.com/katyoti/katyoti-fwk/internal/extensions/actions/api"
	commentsapi "github.com/katyoti/katyoti-fwk/internal/extensions/comments/api"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	gatesapi "github.com/katyoti/katyoti-fwk/internal/extensions/gates/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	paneapi "github.com/katyoti/katyoti-fwk/internal/extensions/objectpane/api"
	verificationapi "github.com/katyoti/katyoti-fwk/internal/extensions/verification/api"
)

// BINDING-LEVEL TESTS FOR THE OBJECT PANE'S VERBS — lifecycle, delete,
// comments, links, actions, verify. Same contract as the detail tests: the
// stub answers with the core's REAL output type (marshalled, so a renamed json
// tag fails the decode), and every test asserts WHICH service path is called
// WITH WHICH input — a renamed path or a dropped field fails here, not in
// front of the user.

const (
	pathTransitionOptions = "ktw-agents:object-pane/transition-options"
	pathObjectTransition  = "ktw-agents:cwm/object-transition"
	pathGateConfirm       = "ktw-agents:gates/confirm"
	pathObjectDelete      = "ktw-agents:cwm/object-delete"
	pathCommentList       = "ktw-agents:comments/list"
	pathCommentAdd        = "ktw-agents:comments/add"
	pathEdgeCreate        = "ktw-agents:cwm/edge-create"
	pathEdgeDelete        = "ktw-agents:cwm/edge-delete"
	pathActionList        = "ktw-agents:action-surface/list"
	pathVerify            = "ktw-agents:verification/verify"
)

// ── lifecycle ──────────────────────────────────────────────────────────────

func optionsReply() paneapi.TransitionOptionsOutput {
	return paneapi.TransitionOptionsOutput{Lifecycles: []paneapi.LifecycleOptions{{
		Workflow: "feature", WorkflowLabel: "Feature", State: "in-progress",
		States: []cwmapi.WorkflowState{{Name: "todo"}, {Name: "in-progress"}, {Name: "done"}},
		Transitions: []paneapi.TransitionOption{
			{Name: "finish", To: "done", GuardStatus: paneapi.GuardFails, Allowed: false, Reason: "criteria not verified", Force: cwmapi.ForceRule("person")},
			{Name: "reopen", To: "todo", GuardStatus: paneapi.GuardNone, Allowed: true},
		},
	}}}
}

func assertOptionsDrawn(t *testing.T, got WorkspaceObjectLifecycles) {
	t.Helper()
	if len(got.Lifecycles) != 1 {
		t.Fatalf("lifecycles: %+v", got.Lifecycles)
	}
	lc := got.Lifecycles[0]
	if lc.Workflow != "feature" || lc.WorkflowLabel != "Feature" || lc.State != "in-progress" || len(lc.States) != 3 {
		t.Errorf("lifecycle header lost on the wire: %+v", lc)
	}
	if len(lc.Transitions) != 2 {
		t.Fatalf("transitions: %+v", lc.Transitions)
	}
	tr := lc.Transitions[0]
	if tr.Name != "finish" || tr.To != "done" || tr.GuardStatus != paneapi.GuardFails || tr.Allowed || tr.Reason == "" || tr.Force != "person" {
		t.Errorf("a refused transition lost its verdict, reason or force rule: %+v", tr)
	}
}

func TestObjectTransitionOptionsBinding_JudgedForThePerson(t *testing.T) {
	s := &multiStub{t: t, replies: map[string]any{pathTransitionOptions: optionsReply()}}
	got := transitionOptionsVia(context.Background(), s, "billing.us-12", "alice")
	if got.Error != "" || got.Message != "" {
		t.Fatalf("unexpected error/message: %+v", got)
	}
	in, ok := s.inputFor(pathTransitionOptions).(paneapi.TransitionOptionsInput)
	if !ok {
		t.Fatalf("input shape: %T", s.calls[0].input)
	}
	if in.ID != "billing.us-12" || in.By != (cwmapi.Actor{ID: "alice", Kind: cwmapi.ActorPerson}) {
		t.Errorf("options must be judged for the person using the GUI: %+v", in)
	}
	assertOptionsDrawn(t, got)
}

func TestObjectTransitionOptionsBinding_NoPersonIsUser(t *testing.T) {
	s := &multiStub{t: t, replies: map[string]any{pathTransitionOptions: paneapi.TransitionOptionsOutput{}}}
	got := transitionOptionsVia(context.Background(), s, "billing.us-12", "")
	in := s.inputFor(pathTransitionOptions).(paneapi.TransitionOptionsInput)
	if in.By != (cwmapi.Actor{ID: "user", Kind: cwmapi.ActorPerson}) {
		t.Errorf("no profile id must judge as the person \"user\": %+v", in.By)
	}
	if got.Lifecycles == nil {
		t.Errorf("an object with no lifecycle must answer an empty list, not null")
	}
}

func TestObjectTransitionOptionsBinding_ServiceErrorReachesTheUser(t *testing.T) {
	s := &multiStub{t: t, errs: map[string]error{pathTransitionOptions: errors.New("cwm: not found: billing.us-12")}}
	got := transitionOptionsVia(context.Background(), s, "billing.us-12", "alice")
	if !strings.Contains(got.Error, "not found: billing.us-12") {
		t.Errorf("the service error must reach the pane: %+v", got)
	}
}

func TestObjectTransitionBinding_MovesThenReReadsTheOptions(t *testing.T) {
	s := &multiStub{t: t, replies: map[string]any{
		pathObjectTransition:  cwmapi.ObjectTransitionOutput{From: "in-progress", To: "done", Transition: "finish", Forced: true},
		pathTransitionOptions: optionsReply(),
	}}
	got := objectTransitionVia(context.Background(), s, "billing.us-12", "checkable", "done", "finish", "verified by hand", true, "alice")
	// The move, THEN the options after it — the panel redraws from the second.
	if !slices.Equal(s.paths(), []string{pathObjectTransition, pathTransitionOptions}) {
		t.Fatalf("calls: %v, want the move then the options re-read", s.paths())
	}
	in, ok := s.inputFor(pathObjectTransition).(cwmapi.ObjectTransitionInput)
	if !ok {
		t.Fatalf("move input shape: %T", s.calls[0].input)
	}
	want := cwmapi.ObjectTransitionInput{
		ID: "billing.us-12", Capability: "checkable", To: "done", Transition: "finish",
		Note: "verified by hand", Force: true, By: cwmapi.Actor{ID: "alice", Kind: cwmapi.ActorPerson},
	}
	if in != want {
		t.Errorf("move input:\n got %+v\nwant %+v", in, want)
	}
	opt := s.calls[1].input.(paneapi.TransitionOptionsInput)
	if opt.ID != "billing.us-12" || opt.By.ID != "alice" {
		t.Errorf("the re-read must be for the same object and person: %+v", opt)
	}
	if got.Message != "" || got.Error != "" {
		t.Errorf("an accepted move reports nothing: %+v", got)
	}
	assertOptionsDrawn(t, got)
}

func TestObjectTransitionBinding_NoPersonMovesAsUser(t *testing.T) {
	s := &multiStub{t: t, replies: map[string]any{
		pathObjectTransition:  cwmapi.ObjectTransitionOutput{},
		pathTransitionOptions: optionsReply(),
	}}
	objectTransitionVia(context.Background(), s, "billing.us-12", "", "todo", "", "", false, "")
	in := s.inputFor(pathObjectTransition).(cwmapi.ObjectTransitionInput)
	if in.By != (cwmapi.Actor{ID: "user", Kind: cwmapi.ActorPerson}) {
		t.Errorf("actor: %+v, want the person \"user\"", in.By)
	}
}

func TestObjectTransitionBinding_RefusedMoveIsAMessageWithOptionsStillDrawn(t *testing.T) {
	s := &multiStub{t: t,
		replies: map[string]any{pathTransitionOptions: optionsReply()},
		errs:    map[string]error{pathObjectTransition: errors.New("transition refused: criteria not verified")},
	}
	got := objectTransitionVia(context.Background(), s, "billing.us-12", "", "done", "finish", "", false, "alice")
	if got.Message != "transition refused: criteria not verified" {
		t.Errorf("the refusal must come back as Message: %+v", got)
	}
	if got.Error != "" {
		t.Errorf("a refused move is not a broken pane: Error %q", got.Error)
	}
	if !slices.Equal(s.paths(), []string{pathObjectTransition, pathTransitionOptions}) {
		t.Fatalf("calls: %v, the options must still be re-read after a refusal", s.paths())
	}
	assertOptionsDrawn(t, got)
}

func TestObjectGateConfirmBinding_ConfirmsThenReReadsTheOptions(t *testing.T) {
	s := &multiStub{t: t, replies: map[string]any{
		pathGateConfirm:       gatesapi.ConfirmOutput{},
		pathTransitionOptions: optionsReply(),
	}}
	got := gateConfirmVia(context.Background(), s, "billing.us-12", "release", "demoed", "shown on friday", "alice")
	if !slices.Equal(s.paths(), []string{pathGateConfirm, pathTransitionOptions}) {
		t.Fatalf("calls: %v, want the confirmation then the options re-read", s.paths())
	}
	in, ok := s.inputFor(pathGateConfirm).(gatesapi.ConfirmInput)
	if !ok {
		t.Fatalf("confirm input shape: %T", s.calls[0].input)
	}
	want := gatesapi.ConfirmInput{ID: "billing.us-12", Gate: "release", Requirement: "demoed", Note: "shown on friday", By: cwmapi.Actor{ID: "alice", Kind: cwmapi.ActorPerson}}
	if in != want {
		t.Errorf("confirm input:\n got %+v\nwant %+v", in, want)
	}
	if got.Message != "" || got.Error != "" {
		t.Errorf("an accepted confirmation reports nothing: %+v", got)
	}
	assertOptionsDrawn(t, got)
}

func TestObjectGateConfirmBinding_RefusalIsAMessage(t *testing.T) {
	s := &multiStub{t: t,
		replies: map[string]any{pathTransitionOptions: optionsReply()},
		errs:    map[string]error{pathGateConfirm: errors.New("gates: requirement demoed is not a person requirement")},
	}
	got := gateConfirmVia(context.Background(), s, "billing.us-12", "release", "demoed", "", "")
	if !strings.Contains(got.Message, "not a person requirement") {
		t.Errorf("the refusal must reach the user as Message: %+v", got)
	}
	if in := s.inputFor(pathGateConfirm).(gatesapi.ConfirmInput); in.By.ID != "user" {
		t.Errorf("no profile id confirms as \"user\": %+v", in.By)
	}
	assertOptionsDrawn(t, got)
}

// ── delete ─────────────────────────────────────────────────────────────────

func TestObjectDeleteBinding_DeleteDoesNotDetach(t *testing.T) {
	s := &multiStub{t: t, replies: map[string]any{pathObjectDelete: cwmapi.ObjectDeleteOutput{ID: "billing.us-12"}}}
	got := objectDeleteVia(context.Background(), s, "billing.us-12", false)
	in, ok := s.inputFor(pathObjectDelete).(cwmapi.ObjectDeleteInput)
	if !ok {
		t.Fatalf("delete input shape: %T", s.calls[0].input)
	}
	if in != (cwmapi.ObjectDeleteInput{Module: "billing", ID: "billing.us-12"}) {
		t.Errorf("a plain delete must not detach (CWM refuses a linked object): %+v", in)
	}
	if got.Error != "" || got.Object.ID != "billing.us-12" {
		t.Errorf("result: %+v", got)
	}
}

func TestObjectDeleteBinding_DeleteDetachDetaches(t *testing.T) {
	s := &multiStub{t: t, replies: map[string]any{pathObjectDelete: cwmapi.ObjectDeleteOutput{ID: "billing.us-12", Detached: []string{"billing.g1"}}}}
	got := objectDeleteVia(context.Background(), s, "billing.us-12", true)
	in := s.inputFor(pathObjectDelete).(cwmapi.ObjectDeleteInput)
	if in != (cwmapi.ObjectDeleteInput{Module: "billing", ID: "billing.us-12", Detach: true}) {
		t.Errorf("delete-detach must carry Detach: %+v", in)
	}
	if got.Error != "" || got.Object.ID != "billing.us-12" {
		t.Errorf("result: %+v", got)
	}
}

func TestObjectDeleteBinding_RefusalReachesTheUser(t *testing.T) {
	s := &multiStub{t: t, errs: map[string]error{pathObjectDelete: errors.New("cwm: billing.us-12 is linked from billing.g1")}}
	got := objectDeleteVia(context.Background(), s, "billing.us-12", false)
	if !strings.Contains(got.Error, "is linked from billing.g1") {
		t.Errorf("the refusal must reach the user: %+v", got)
	}
	empty := objectDeleteVia(context.Background(), s, "", true)
	if empty.Error == "" {
		t.Errorf("an empty id must be refused before any call")
	}
	if len(s.calls) != 1 {
		t.Errorf("calls: %v, the empty id must not reach the service", s.paths())
	}
}

// ── comments ───────────────────────────────────────────────────────────────

func commentsReply() commentsapi.ListOutput {
	return commentsapi.ListOutput{Comments: []commentsapi.Comment{
		{ID: "c1", By: cwmapi.Actor{ID: "alice", Kind: cwmapi.ActorPerson}, At: "2026-10-01T09:00:00Z", Text: "Is the **total** right?"},
		{ID: "c2", By: cwmapi.Actor{ID: "planner", Kind: "agent"}, At: "2026-10-01T09:05:00Z", Text: "Yes.", ReplyTo: "c1"},
	}}
}

func TestObjectCommentListBinding_DecodesEveryField(t *testing.T) {
	s := &multiStub{t: t, replies: map[string]any{pathCommentList: commentsReply()}}
	got := commentListVia(context.Background(), s, "billing.us-12")
	if in, ok := s.inputFor(pathCommentList).(commentsapi.ListInput); !ok || in.ID != "billing.us-12" {
		t.Errorf("list input: %+v", s.calls[0].input)
	}
	want := []WorkspaceObjectComment{
		{ID: "c1", By: "alice", ByKind: "person", At: "2026-10-01T09:00:00Z", Text: "Is the **total** right?"},
		{ID: "c2", By: "planner", ByKind: "agent", At: "2026-10-01T09:05:00Z", Text: "Yes.", ReplyTo: "c1"},
	}
	if got.Error != "" || !slices.Equal(got.Comments, want) {
		t.Errorf("comments:\n got %+v\nwant %+v", got, want)
	}
}

func TestObjectCommentListBinding_EmptyIsAListAndErrorsReachTheUser(t *testing.T) {
	s := &multiStub{t: t, replies: map[string]any{pathCommentList: commentsapi.ListOutput{}}}
	if got := commentListVia(context.Background(), s, "billing.us-12"); got.Comments == nil || got.Error != "" {
		t.Errorf("no comments must be an empty list: %+v", got)
	}
	s = &multiStub{t: t, errs: map[string]error{pathCommentList: errors.New("comments: no such object")}}
	if got := commentListVia(context.Background(), s, "billing.x"); !strings.Contains(got.Error, "no such object") {
		t.Errorf("the service error must reach the user: %+v", got)
	}
}

func TestObjectCommentAddBinding_AddsAsThePersonThenReReads(t *testing.T) {
	s := &multiStub{t: t, replies: map[string]any{
		pathCommentAdd:  commentsapi.AddOutput{Comment: commentsReply().Comments[1]},
		pathCommentList: commentsReply(),
	}}
	got := commentAddVia(context.Background(), s, "billing.us-12", "Yes.", "c1", "alice")
	if !slices.Equal(s.paths(), []string{pathCommentAdd, pathCommentList}) {
		t.Fatalf("calls: %v, want the add then the list re-read", s.paths())
	}
	in, ok := s.inputFor(pathCommentAdd).(commentsapi.AddInput)
	if !ok {
		t.Fatalf("add input shape: %T", s.calls[0].input)
	}
	want := commentsapi.AddInput{ID: "billing.us-12", Text: "Yes.", ReplyTo: "c1", By: cwmapi.Actor{ID: "alice", Kind: cwmapi.ActorPerson}}
	if in != want {
		t.Errorf("add input:\n got %+v\nwant %+v", in, want)
	}
	if s.calls[1].input.(commentsapi.ListInput).ID != "billing.us-12" {
		t.Errorf("the re-read must be for the same object: %+v", s.calls[1].input)
	}
	if got.Error != "" || len(got.Comments) != 2 {
		t.Errorf("the pane must redraw from the re-read: %+v", got)
	}
}

func TestObjectCommentAddBinding_NoPersonIsUserAndErrorsReachTheUser(t *testing.T) {
	s := &multiStub{t: t, errs: map[string]error{pathCommentAdd: errors.New("comments: text is required")}}
	got := commentAddVia(context.Background(), s, "billing.us-12", "", "", "")
	if in := s.inputFor(pathCommentAdd).(commentsapi.AddInput); in.By != (cwmapi.Actor{ID: "user", Kind: cwmapi.ActorPerson}) {
		t.Errorf("no profile id comments as \"user\": %+v", in.By)
	}
	if !strings.Contains(got.Error, "text is required") {
		t.Errorf("the refusal must reach the user: %+v", got)
	}
	if len(s.calls) != 1 {
		t.Errorf("calls: %v, a refused add must not re-read", s.paths())
	}
}

// ── links ──────────────────────────────────────────────────────────────────

func TestObjectLinkAddBinding_FromKindTo(t *testing.T) {
	s := &multiStub{t: t, replies: map[string]any{pathEdgeCreate: cwmapi.EdgeCreateOutput{}}}
	got := linkAddVia(context.Background(), s, "billing.us-12", "serves", "billing.g1")
	in, ok := s.inputFor(pathEdgeCreate).(cwmapi.EdgeCreateInput)
	if !ok {
		t.Fatalf("edge input shape: %T", s.calls[0].input)
	}
	if in != (cwmapi.EdgeCreateInput{From: "billing.us-12", To: "billing.g1", Kind: "serves"}) {
		t.Errorf("edge create input: %+v", in)
	}
	if got.Error != "" || got.Object.ID != "billing.us-12" {
		t.Errorf("result: %+v", got)
	}
}

func TestObjectLinkAddBinding_RefusalsReachTheUser(t *testing.T) {
	s := &multiStub{t: t, errs: map[string]error{pathEdgeCreate: errors.New("cwm: edge kind serves may not leave a note")}}
	if got := linkAddVia(context.Background(), s, "billing.n1", "serves", "billing.g1"); !strings.Contains(got.Error, "may not leave a note") {
		t.Errorf("the substrate's reason must reach the user: %+v", got)
	}
	if got := linkAddVia(context.Background(), s, "billing.n1", "", "billing.g1"); got.Error == "" {
		t.Errorf("a link without a kind must be refused")
	}
	if len(s.calls) != 1 {
		t.Errorf("calls: %v, an incomplete link must not reach the service", s.paths())
	}
}

func TestObjectLinkDeleteBinding_FromKindTo(t *testing.T) {
	s := &multiStub{t: t, replies: map[string]any{pathEdgeDelete: cwmapi.EdgeDeleteOutput{}}}
	got := linkDeleteVia(context.Background(), s, "billing.us-12", "serves", "billing.g1")
	in, ok := s.inputFor(pathEdgeDelete).(cwmapi.EdgeDeleteInput)
	if !ok {
		t.Fatalf("edge input shape: %T", s.calls[0].input)
	}
	if in != (cwmapi.EdgeDeleteInput{From: "billing.us-12", To: "billing.g1", Kind: "serves"}) {
		t.Errorf("edge delete input: %+v", in)
	}
	if got.Error != "" || got.Object.ID != "billing.us-12" {
		t.Errorf("result: %+v", got)
	}
	s = &multiStub{t: t, errs: map[string]error{pathEdgeDelete: errors.New("cwm: no such edge")}}
	if got := linkDeleteVia(context.Background(), s, "a.b", "serves", "a.c"); !strings.Contains(got.Error, "no such edge") {
		t.Errorf("the service error must reach the user: %+v", got)
	}
}

// kindsCatalog is a catalog reader that records which kind it was asked for.
type kindsCatalog struct {
	asked   []string
	entries []contribapi.Contribution
	err     error
}

func (c *kindsCatalog) List(context.Context) ([]*mbapi.Manifest, error) { return nil, nil }
func (c *kindsCatalog) GetBody(context.Context, string) ([]byte, error) { return nil, nil }
func (c *kindsCatalog) GetContributionsByType(_ context.Context, t string) ([]contribapi.Contribution, error) {
	c.asked = append(c.asked, t)
	if c.err != nil {
		return nil, c.err
	}
	var out []contribapi.Contribution
	for _, e := range c.entries {
		if e.Type == t {
			out = append(out, e)
		}
	}
	return out, nil
}

func edgeType(provider, id, name, desc string) contribapi.Contribution {
	return contribapi.Contribution{Provider: provider, Type: "edge-type", ID: id,
		Metadata: &contribapi.ContributionMeta{Name: name, Description: desc}}
}

func TestObjectLinkKindsBinding_EdgeTypesNamedOnceSorted(t *testing.T) {
	cat := &kindsCatalog{entries: []contribapi.Contribution{
		edgeType("ktw-agents", "serves", "serves", "a story serves a goal"),
		edgeType("ktw-methods", "serves", "serves", "duplicate from another provider"),
		edgeType("ktw-agents", "documents", "documents", "a document documents an object"),
	}}
	got, err := linkKindsVia(context.Background(), cat)
	if err != nil {
		t.Fatalf("linkKindsVia: %v", err)
	}
	if !slices.Equal(cat.asked, []string{"edge-type"}) {
		t.Errorf("the picker must read the edge-type kind: asked %v", cat.asked)
	}
	want := []WorkspaceLinkKind{
		{Name: "documents", Description: "a document documents an object"},
		{Name: "serves", Description: "a story serves a goal"},
	}
	if !slices.Equal(got, want) {
		t.Errorf("kinds:\n got %+v\nwant %+v", got, want)
	}
	cat = &kindsCatalog{err: errors.New("catalog unreachable")}
	if _, err := linkKindsVia(context.Background(), cat); err == nil || !strings.Contains(err.Error(), "catalog unreachable") {
		t.Errorf("a catalog error must surface: %v", err)
	}
}

// ── actions ────────────────────────────────────────────────────────────────

func TestObjectActionsBinding_ObjectScope(t *testing.T) {
	s := &multiStub{t: t, replies: map[string]any{pathActionList: actionsReply()}}
	got, err := objectActionsVia(context.Background(), s, objectActionsInput("billing.us-12"))
	if err != nil {
		t.Fatalf("objectActionsVia: %v", err)
	}
	in, ok := s.inputFor(pathActionList).(actionsapi.ListInput)
	if !ok {
		t.Fatalf("list input shape: %T", s.calls[0].input)
	}
	if in != (actionsapi.ListInput{ID: "billing.us-12", Scope: cwmapi.ActionScopeObject}) {
		t.Errorf("the action bar asks for the object scope: %+v", in)
	}
	if len(got) != 2 || got[0].URI != "p:tasks/refine" || got[0].Label != "Refine" || got[0].Kind != "ai" || got[0].Source != "applies-to" {
		t.Errorf("first action: %+v", got)
	}
	if !got[1].Disabled || got[1].DisabledReason != "condition not met" {
		t.Errorf("a disabled action must carry its reason: %+v", got[1])
	}
}

func TestObjectCollectionActionsBinding_CollectionScope(t *testing.T) {
	s := &multiStub{t: t, replies: map[string]any{pathActionList: actionsReply()}}
	if _, err := objectActionsVia(context.Background(), s, collectionActionsInput("billing", "feature")); err != nil {
		t.Fatalf("objectActionsVia: %v", err)
	}
	in := s.inputFor(pathActionList).(actionsapi.ListInput)
	if in != (actionsapi.ListInput{ID: "billing", Type: "feature", Scope: cwmapi.ActionScopeCollection}) {
		t.Errorf("a list header asks for the collection of one type: %+v", in)
	}
}

func TestObjectActionsBinding_ServiceErrorSurfaces(t *testing.T) {
	s := &multiStub{t: t, errs: map[string]error{pathActionList: errors.New("action-surface: down")}}
	if _, err := objectActionsVia(context.Background(), s, objectActionsInput("billing.us-12")); err == nil || !strings.Contains(err.Error(), "action-surface: down") {
		t.Errorf("the service error must surface: %v", err)
	}
}

// ── verify ─────────────────────────────────────────────────────────────────

// TestObjectVerifyRun_RunsThroughTheActionSurface -: the pane's
// Verify runs through ActionService with the
// object as the run's target; Verify, declared long, answers its run and the
// panel it reports each check into, drawn under the check.
func TestObjectVerifyRun_RunsThroughTheActionSurface(t *testing.T) {
	const uri = "ktw-agents:verification/verify"
	s := &actionSender{out: actionsapi.RunOutput{URI: uri, TaskID: "run-1", Panel: "dp-1"}}
	got := objectVerifyRun(context.Background(), s, "billing.us-12")

	if s.path != "ktw-agents:action/run" {
		t.Errorf("sent to %q, want ActionService.Run (never Verify over its own call)", s.path)
	}
	if s.in.URI != uri || s.in.Context["_target"] != "billing.us-12" {
		t.Errorf("run input = %+v, want Verify's URI and the object as _target", s.in)
	}
	var in verificationapi.VerifyInput
	if err := json.Unmarshal(s.in.Input, &in); err != nil || in.ID != "billing.us-12" {
		t.Errorf("input = %s, want the object to verify", s.in.Input)
	}
	if got.Panel != "dp-1" || got.TaskID != "run-1" || got.Check != nil {
		t.Errorf("a long verification = %+v, want its run and its panel, no outcome yet", got)
	}
}

// TestObjectVerifyRun_AValueIsTheOutcome - a verification that answers its
// value at once is summed up as the old path sums it.
func TestObjectVerifyRun_AValueIsTheOutcome(t *testing.T) {
	value, _ := json.Marshal(verificationapi.VerifyOutput{Objects: 2, Runs: 2, Passed: 1,
		Failed: []verificationapi.VerifyOutcome{{ObjectID: "billing.ac-1", Status: "fail", Evidence: "exit 1"}}})
	s := &actionSender{out: actionsapi.RunOutput{Value: value}}
	got := objectVerifyRun(context.Background(), s, "billing.us-12")
	if got.Panel != "" || got.Check == nil || got.Check.Status != "fail" || !strings.Contains(got.Check.Desc, "billing.ac-1: fail (exit 1)") {
		t.Errorf("a value = %+v, want the outcome summed up", got)
	}
}

// TestObjectVerifyRun_UnreachableIsAHarnessErrorNeverAPass - a run that
// could not start has not passed.
func TestObjectVerifyRun_UnreachableIsAHarnessErrorNeverAPass(t *testing.T) {
	s := &multiStub{t: t, errs: map[string]error{"ktw-agents:action/run": errors.New("no such object")}}
	got := objectVerifyRun(context.Background(), s, "billing.us-12")
	if got.Check == nil || got.Check.Status != "harness-error" || !strings.Contains(got.Check.Error, "no such object") {
		t.Errorf("a check that could not run has not passed: %+v", got)
	}
	empty := &actionSender{out: actionsapi.RunOutput{}}
	if got := objectVerifyRun(context.Background(), empty, "billing.us-12"); got.Check == nil || got.Check.Status != "harness-error" {
		t.Errorf("neither a run nor a value is no verdict: %+v", got)
	}
}

// TestObjectVerifyURI_PassesTheActionSurfaceGrammar -: ActionService
// parses a Run's URI with mbapi.ParseToolURI, whose method token is kebab; the
// Verify run's URI must parse there, or the pane's Verify is refused before it
// runs (the PascalCase "Verify" was).
func TestObjectVerifyURI_PassesTheActionSurfaceGrammar(t *testing.T) {
	uri, err := mbapi.ParseToolURI(objectVerifyURI)
	if err != nil {
		t.Fatalf("ActionService refuses the Verify URI %q: %v", objectVerifyURI, err)
	}
	if uri.Provider != "ktw-agents" || uri.Method != "verify" {
		t.Errorf("the Verify URI names %+v, want ktw-agents:verification/verify", uri)
	}
}
