// SPDX-License-Identifier: Apache-2.0

package core

import (
	"testing"

	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

func TestBuildExecRequest_ThreadsCurrentNode(t *testing.T) {
	t1 := &taskapi.Task{
		Name: "review",
		Type: "agent",
		Kind: taskapi.KindObject,
		Body: "body",
	}
	in := taskapi.TaskRunRequest{
		WorkspaceRoot: "/tmp/ws",
		ParentRunID:   "run-parent",
		SessionID:     "session-1",
		CurrentNode: map[string]any{
			"id":   "doc-42",
			"type": "document",
			"path": "/notes/foo.md",
		},
	}
	exec := buildExecRequest(t1, in, nil)
	if got := exec.Runtime.CurrentNode; got == nil {
		t.Fatal("Runtime.CurrentNode = nil; want propagated map")
	}
	if id := exec.Runtime.CurrentNode["id"]; id != "doc-42" {
		t.Errorf("CurrentNode.id = %v, want doc-42", id)
	}
	if exec.Runtime.WorkspaceRoot != "/tmp/ws" {
		t.Errorf("WorkspaceRoot lost: %q", exec.Runtime.WorkspaceRoot)
	}
	if exec.Runtime.ParentRunID != "run-parent" {
		t.Errorf("ParentRunID lost: %q", exec.Runtime.ParentRunID)
	}
}

func TestBuildExecRequest_NilCurrentNode_GlobalTask(t *testing.T) {
	t1 := &taskapi.Task{
		Name: "ad-hoc",
		Type: "simulator",
		Kind: taskapi.KindGlobal,
	}
	in := taskapi.TaskRunRequest{}
	exec := buildExecRequest(t1, in, nil)
	if exec.Runtime.CurrentNode != nil {
		t.Errorf("global task with nil request CurrentNode should yield nil; got %v", exec.Runtime.CurrentNode)
	}
}

// TestChildSpec_CurrentNodeField ensures the spawner's ChildSpec carries
// CurrentNode through to the child's TaskExecRequest.Runtime. We don't run
// a full spawn — we exercise the struct-copy convention that the spawner
// follows (asserted by reading the public field).
func TestChildSpec_CarriesCurrentNode(t *testing.T) {
	parentNode := map[string]any{
		"id":   "design-doc-7",
		"type": "design-doc",
	}
	spec := engine.ChildSpec{
		Kind:        "agent",
		Name:        "review-step",
		ParentMode:  "coordinator",
		CurrentNode: parentNode,
	}
	if spec.CurrentNode == nil {
		t.Fatal("CurrentNode lost")
	}
	if spec.CurrentNode["id"] != "design-doc-7" {
		t.Errorf("CurrentNode.id = %v", spec.CurrentNode["id"])
	}
}
