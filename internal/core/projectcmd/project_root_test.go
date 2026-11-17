// SPDX-License-Identifier: Apache-2.0

package projectcmd

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
)

// TestRunRunsInTheUniverseNotTheWorkingFolder: a deliverable check runs in the
// project the universe serves — for a universe that is its own project, its
// workspace — even when the process runs elsewhere (a served ktw-agents runs
// in its installation folder): `go test ./...` there would find no go.mod.
func TestRunRunsInTheUniverseNotTheWorkingFolder(t *testing.T) {
	requireBash(t)
	t.Setenv("KTW_KATYOTI_ROOT", t.TempDir())
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, ".katyoti"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "marker.txt"), []byte("the-project"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KTW_WORKSPACE", workspace)
	t.Chdir(t.TempDir())
	application.ResetGlobals()
	t.Cleanup(application.ResetGlobals)

	out, err := Run(context.Background(), "cat marker.txt", time.Minute)
	if err != nil || strings.TrimSpace(out) != "the-project" {
		t.Fatalf("cat marker.txt: out %q err %v, want it run in the universe's project %s", out, err, workspace)
	}
}
