// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"strings"
	"testing"
)

// A path or a file name is pandoc's to resolve; only a bare name is looked up
// among the bundled templates, and those are PDF layouts.
func TestResolveTemplatePassesPathsThrough(t *testing.T) {
	for _, name := range []string{"", "my.typ", "dir/report", `dir\report`} {
		got, cleanup, err := resolveTemplate(context.Background(), name, "pdf")
		cleanup()
		if err != nil || got != name {
			t.Errorf("resolveTemplate(%q) = %q, %v; want it unchanged", name, got, err)
		}
	}
	_, cleanup, err := resolveTemplate(context.Background(), "report", "docx")
	cleanup()
	if err == nil || !strings.Contains(err.Error(), "for PDF") {
		t.Fatalf("a bundled template for docx: err = %v, want a PDF-only error", err)
	}
}
