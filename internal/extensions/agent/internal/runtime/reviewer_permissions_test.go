// SPDX-License-Identifier: Apache-2.0

package runtime

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/marshal"
	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
)

// The reviewer persona (ktw-methods) reads everything, files its own findings
// and changes nothing it judges: its shipped config, decoded as the agent
// decodes it, gives the permission hook an allow list that lets the reads and
// its own objects through and stops every write, script and delegation.
func TestTheReviewerChangesNothingItJudges(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "..", "app", "ktw-methods", "embed", "personas", "reviewer", "TASK.md"))
	if err != nil {
		t.Fatal(err)
	}
	var front struct {
		Config map[string]any `yaml:"config"`
	}
	if err := config.DecodeBytes(raw, marshal.Frontmatter, &front); err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(front.Config)
	if err != nil {
		t.Fatal(err)
	}
	var req agentapi.AgentRunRequest
	if err := json.Unmarshal(b, &req); err != nil {
		t.Fatal(err)
	}
	if req.SpawnPermissions == nil || len(req.SpawnPermissions.AllowedTools) == 0 {
		t.Fatalf("the reviewer declares no allowed tools: %+v", req.SpawnPermissions)
	}
	hook := NewSpawnPermissionHook(req.SpawnPermissions)
	for _, tool := range []string{"file-read", "grep", "cwm-object-get", "cwm-object-search", "cwm-object-create", "cwm-edge-create", "create-document", "complete"} {
		if d, _ := hook.OnPreToolExec(nil, tool, nil); d != nil && d.Decision == agentapi.Deny {
			t.Errorf("the reviewer cannot %s: %s", tool, d.Reason)
		}
	}
	for _, tool := range []string{"file-write", "file-edit", "bash", "run-script", "delegate", "cwm-object-update", "cwm-object-delete", "cwm-object-move", "cwm-object-file-write", "cwm-edge-delete"} {
		if d, _ := hook.OnPreToolExec(nil, tool, nil); d == nil || d.Decision != agentapi.Deny {
			t.Errorf("the reviewer may %s, want it denied", tool)
		}
	}
}
