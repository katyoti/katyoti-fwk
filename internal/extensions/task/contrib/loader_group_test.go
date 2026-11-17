// SPDX-License-Identifier: Apache-2.0

package contrib

import (
	"context"
	"encoding/json"
	"testing"
	"testing/fstest"

	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
)

const groupedTaskMD = `---
name: nightly-index
type: agent
description: Rebuild the index
group: background
---
Rebuild it.
`

// A TASK.md `group:` is the task group its runs are processed in: strict
// parsing accepts it, and the catalog body carries it to other processes.
func TestATaskDeclaresItsGroup(t *testing.T) {
	got, err := Parse([]byte(groupedTaskMD))
	if err != nil {
		t.Fatalf("strict Parse refuses group: %v", err)
	}
	if got.Group != "background" {
		t.Errorf("Group = %q, want background", got.Group)
	}

	p := TaskProviders("ktw-test", loader.NewEmbedFSLoader(fstest.MapFS{
		"embed/tasks/nightly-index/TASK.md": &fstest.MapFile{Data: []byte(groupedTaskMD)},
	}, ""), "embed/tasks")[0]
	entries, err := p.Enumerate(context.Background())
	if err != nil || len(entries) != 1 {
		t.Fatalf("Enumerate: %d entries, %v", len(entries), err)
	}
	var body mbapi.TaskBody
	if err := json.Unmarshal(entries[0].Data(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Group != "background" {
		t.Errorf("TaskBody.Group = %q, want background", body.Group)
	}
}
