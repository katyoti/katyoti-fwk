// SPDX-License-Identifier: Apache-2.0

package contrib

import (
	"encoding/json"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
)

// A task's I/O contracts are files beside it — a flat task's
// <name>.input.dsl.yaml, a folder task's TASK.output.dsl.yaml — read by the
// engine's discovery as the inline form was; the keys take the catalog's
// names (permissionProfile, sessionMode), the old ones still read.
func TestDiscovery_ReadsTheContractsBesideATaskAndTheNewKeys(t *testing.T) {
	files := map[string]string{
		"tasks/greet.md":                    "---\nname: greet\ntype: agent\ndescription: d\npermissionProfile: read-only\nsessionMode: chat\n---\nSay hello. Call complete when done.\n",
		"tasks/greet.input.dsl.yaml":        "name: greet-input\nroot: In\ntypes:\n  In:\n    type: object\n    fields:\n      who: {type: string}\n",
		"tasks/review/TASK.md":              "---\nname: review\ntype: agent\ndescription: d\npermission_profile: full\nsession:\n  mode: session\n---\nReview. Call complete when done.\n",
		"tasks/review/TASK.output.dsl.yaml": "name: review-output\nroot: Out\ntypes:\n  Out:\n    type: object\n    fields:\n      ok: {type: boolean}\n",
	}
	tasks, drops := DiscoverTasksReporting([]taskapi.TaskLoader{{Loader: loader.NewMapLoader(files), Root: "tasks", Provider: "ktw-test"}})
	if len(drops) != 0 {
		t.Fatalf("drops = %+v", drops)
	}
	greet, review := tasks["ktw-test:greet"], tasks["ktw-test:review"]
	if greet == nil || review == nil {
		t.Fatalf("tasks = %v; want greet and review", tasks)
	}
	var in map[string]any
	if err := json.Unmarshal(greet.InputSchema, &in); err != nil || in["name"] != "greet-input" {
		t.Errorf("greet input schema = %s, %v; want greet.input.dsl.yaml as JSON", greet.InputSchema, err)
	}
	if len(greet.OutputSchema) != 0 {
		t.Errorf("greet declares no output contract, got %s", greet.OutputSchema)
	}
	var out map[string]any
	if err := json.Unmarshal(review.OutputSchema, &out); err != nil || out["name"] != "review-output" {
		t.Errorf("review output schema = %s, %v; want TASK.output.dsl.yaml as JSON", review.OutputSchema, err)
	}
	if greet.PermissionProfile != "read-only" || greet.SessionMode != "chat" {
		t.Errorf("greet = %q / %q; want the new keys read", greet.PermissionProfile, greet.SessionMode)
	}
	if review.PermissionProfile != "full" || review.SessionMode != "session" {
		t.Errorf("review = %q / %q; want the old keys still read", review.PermissionProfile, review.SessionMode)
	}
}
