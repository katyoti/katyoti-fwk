// SPDX-License-Identifier: Apache-2.0

package contrib

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/loader"
)

// TestFolderSkillEnumeration verifies that a SkillResourceProvider backed by a
// plain on-disk FSLoader (the ktw-host generic-host path) enumerates a folder's
// skills under a runtime provider name — the risk-1 mechanism.
func TestFolderSkillEnumeration(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "skills"), 0o755); err != nil {
		t.Fatal(err)
	}
	skill := "---\nname: flow-spec\ndescription: demo skill from a folder\n---\n# Flow\nbody\n"
	if err := os.WriteFile(filepath.Join(dir, "skills", "flow-spec.md"), []byte(skill), 0o644); err != nil {
		t.Fatal(err)
	}

	p := SkillProvider("acme-flows", loader.NewFSLoader(dir), "skills")
	got, err := p.Enumerate(context.Background())
	if err != nil {
		t.Fatalf("Enumerate error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 skill enumerated, got %d: %+v", len(got), got)
	}
	if got[0].Provider != "acme-flows" {
		t.Fatalf("want provider acme-flows, got %q", got[0].Provider)
	}
	t.Logf("enumerated: provider=%s id=%s type=%s", got[0].Provider, got[0].ID, got[0].Type)
}
