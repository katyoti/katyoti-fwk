// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"testing"

	"encoding/json"

	actionsapi "github.com/katyoti/katyoti-fwk/internal/extensions/actions/api"
	commentsapi "github.com/katyoti/katyoti-fwk/internal/extensions/comments/api"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	paneapi "github.com/katyoti/katyoti-fwk/internal/extensions/objectpane/api"
	verificationapi "github.com/katyoti/katyoti-fwk/internal/extensions/verification/api"
)

// : the GUI opens, saves and lists the actions of another
// project's object by its qualified id <key>:<module>.<local>. The Go bridge
// adds nothing and drops nothing: the id the frontend sends reaches the pane's
// Detail and the action surface unchanged, and a save or a delete names the
// project on the module (<key>:<module>), which cwm's store set reads
// (keyedModule) - never the bare id alone, which two projects may hold.

const qualified = "crm:billing.us-12"

func TestTheBridgePassesAQualifiedIDUnchanged_DetailAndActions(t *testing.T) {
	reply := detailReply()
	reply.Project = "crm"
	s := &multiStub{t: t, replies: map[string]any{"ktw-agents:object-pane/detail": reply, "ktw-agents:action-surface/list": actionsReply()}}
	got, err := objectDetailVia(context.Background(), s, qualified)
	if err != nil {
		t.Fatalf("objectDetailVia: %v", err)
	}
	if in := s.inputFor("ktw-agents:object-pane/detail").(paneapi.DetailInput); in.ID != qualified {
		t.Errorf("Detail asked for %q, want the qualified id %q unchanged", in.ID, qualified)
	}
	if in := s.inputFor("ktw-agents:action-surface/list").(actionsapi.ListInput); in.ID != qualified || in.Scope != cwmapi.ActionScopeObject {
		t.Errorf("action-surface/List asked for %+v, want the qualified id %q in the object scope", in, qualified)
	}
	// The pane draws the bare id the answer carries, and the project holding it.
	if got.ID != "billing.us-12" || got.Project != "crm" {
		t.Errorf("detail: id %q project %q, want billing.us-12 in crm", got.ID, got.Project)
	}
	if in := objectActionsInput(qualified); in.ID != qualified {
		t.Errorf("WorkspaceObjectActions asks for %q, want %q", in.ID, qualified)
	}
}

func TestTheBridgePassesAQualifiedIDUnchanged_SaveAndDelete(t *testing.T) {
	stored := cwmapi.Object{ID: "billing.us-12", Type: "feature"} // the store answers bare
	s := &multiStub{t: t, replies: map[string]any{
		"ktw-agents:cwm/object-get":    cwmapi.ObjectGetOutput{Object: stored, Project: "crm"},
		"ktw-agents:cwm/object-update": cwmapi.ObjectUpdateOutput{Object: stored},
		"ktw-agents:cwm/object-delete": cwmapi.ObjectDeleteOutput{},
	}}
	if _, err := objectUpdateVia(context.Background(), s, qualified, `{"content":{"status":"done"}}`, "", "r1"); err != nil {
		t.Fatalf("objectUpdateVia: %v", err)
	}
	get := s.inputFor("ktw-agents:cwm/object-get").(cwmapi.ObjectGetInput)
	if get.ID != qualified || get.Module != "crm:billing" {
		t.Errorf("ObjectGet %+v, want id %q in module crm:billing", get, qualified)
	}
	// The object read back is bare: the project travels on the module.
	upd := s.inputFor("ktw-agents:cwm/object-update").(cwmapi.ObjectUpdateInput)
	if upd.Module != "crm:billing" || upd.Revision != "r1" {
		t.Errorf("ObjectUpdate module %q revision %q, want crm:billing and r1", upd.Module, upd.Revision)
	}
	if r := objectDeleteVia(context.Background(), s, qualified, false); r.Error != "" {
		t.Fatalf("objectDeleteVia: %s", r.Error)
	}
	del := s.inputFor("ktw-agents:cwm/object-delete").(cwmapi.ObjectDeleteInput)
	if del.ID != qualified || del.Module != "crm:billing" {
		t.Errorf("ObjectDelete %+v, want id %q in module crm:billing", del, qualified)
	}
}

// A bare id - one project served - is sent as it is: today's calls.
func TestTheBridgeKeepsABareIDBare(t *testing.T) {
	stored := cwmapi.Object{ID: "billing.us-12", Type: "feature"}
	s := &multiStub{t: t, replies: map[string]any{
		"ktw-agents:cwm/object-get":    cwmapi.ObjectGetOutput{Object: stored},
		"ktw-agents:cwm/object-update": cwmapi.ObjectUpdateOutput{Object: stored},
	}}
	if _, err := objectUpdateVia(context.Background(), s, "billing.us-12", `{"content":{}}`, "", ""); err != nil {
		t.Fatalf("objectUpdateVia: %v", err)
	}
	if get := s.inputFor("ktw-agents:cwm/object-get").(cwmapi.ObjectGetInput); get.ID != "billing.us-12" || get.Module != "billing" {
		t.Errorf("ObjectGet %+v, want the bare id in module billing", get)
	}
}

// : the pane's remaining calls - comments, Verify, « + New »'s
// choices and the create under the object - reach the universe with the
// qualified id the frontend sends; cwm's store set routes each (comments read
// and write the thread through ObjectNamedFileRead/Write, routed verbs).
func TestTheBridgePassesAQualifiedIDUnchanged_CommentsVerifyAndCreate(t *testing.T) {
	ctx := context.Background()
	s := &multiStub{t: t, replies: map[string]any{
		"ktw-agents:comments/list":               commentsapi.ListOutput{},
		"ktw-agents:comments/add":                commentsapi.AddOutput{},
		"ktw-agents:action/run":                  actionsapi.RunOutput{Panel: "p1"},
		"ktw-agents:cwm/object-allowed-children": cwmapi.ObjectAllowedChildrenOutput{},
		"ktw-agents:cwm/object-aspect-list":      cwmapi.ObjectAspectListOutput{},
		"ktw-agents:cwm/object-create":           cwmapi.ObjectCreateOutput{Object: cwmapi.Object{ID: "billing.us-13", Type: "feature"}},
		"ktw-agents:cwm/object-type-get":         cwmapi.ObjectTypeGetOutput{},
	}}

	if r := commentListVia(ctx, s, qualified); r.Error != "" {
		t.Fatalf("commentListVia: %s", r.Error)
	}
	if in := s.inputFor("ktw-agents:comments/list").(commentsapi.ListInput); in.ID != qualified {
		t.Errorf("comments/List asked for %q, want %q", in.ID, qualified)
	}
	if r := commentAddVia(ctx, s, qualified, "hello", "", "ana"); r.Error != "" {
		t.Fatalf("commentAddVia: %s", r.Error)
	}
	if in := s.inputFor("ktw-agents:comments/add").(commentsapi.AddInput); in.ID != qualified {
		t.Errorf("comments/Add asked for %q, want %q", in.ID, qualified)
	}

	if r := objectVerifyRun(ctx, s, qualified); r.Panel != "p1" {
		t.Fatalf("objectVerifyRun: %+v", r)
	}
	run := s.inputFor("ktw-agents:action/run").(actionsapi.RunInput)
	var vin verificationapi.VerifyInput
	if err := json.Unmarshal(run.Input, &vin); err != nil || vin.ID != qualified || run.Context["_target"] != qualified {
		t.Errorf("verify run input %s target %v, want id and _target %q", run.Input, run.Context["_target"], qualified)
	}

	if _, err := objectChildTypesVia(ctx, s, qualified); err != nil {
		t.Fatalf("objectChildTypesVia: %v", err)
	}
	if in := s.inputFor("ktw-agents:cwm/object-allowed-children").(cwmapi.ObjectAllowedChildrenInput); in.ParentID != qualified {
		t.Errorf("ObjectAllowedChildren asked under %q, want %q", in.ParentID, qualified)
	}

	if _, err := objectCreateVia(ctx, s, qualified, "feature", "", "A child", "", ""); err != nil {
		t.Fatalf("objectCreateVia: %v", err)
	}
	// Created in the parent's project: its module keyed, its parent qualified.
	if in := s.inputFor("ktw-agents:cwm/object-create").(cwmapi.ObjectCreateInput); in.Module != "crm:billing" || in.Object.Parent != qualified {
		t.Errorf("ObjectCreate module %q parent %q, want crm:billing under %q", in.Module, in.Object.Parent, qualified)
	}
}
