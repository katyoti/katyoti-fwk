// SPDX-License-Identifier: Apache-2.0

package contribution_test

import (
	"slices"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/contribution"
	"github.com/katyoti/katyoti-fwk/internal/extensions/task/contrib/taskkind"
)

// The task data-type declares audience beside kind and types:
// the catalog's task data carries the key the TASK.md author writes. The
// task kind's home is its pack: the schema is the task leaf's.
func TestTaskDataTypeAudience(t *testing.T) {
	dsl, err := taskkind.Leaf().Schema.Document()
	if err != nil {
		t.Fatal(err)
	}
	fields, _, err := contribution.FieldPaths(dsl)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Task.kind", "Task.types", "Task.audience"} {
		if !slices.Contains(fields, want) {
			t.Errorf("the task data-type has no %s (fields: %v)", want, fields)
		}
	}
}
