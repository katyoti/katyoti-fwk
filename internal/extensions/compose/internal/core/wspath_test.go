// SPDX-License-Identifier: Apache-2.0

package core

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
)

// On a command line that names no universe, a relative path is the person's
// current folder — `ktw-compose keygen --folder keys` writes ./keys, not into
// the fallback universe's workspace.
func TestARelativePathOnABareCommandLineIsTheCurrentFolder(t *testing.T) {
	for _, k := range []string{"KTW_WORKSPACE", "KTW_UNIVERSE"} {
		t.Setenv(k, "")
		_ = os.Unsetenv(k)
	}
	// An empty global root: no .env there names a workspace.
	t.Setenv("KTW_KATYOTI_ROOT", t.TempDir())
	t.Chdir(t.TempDir())
	cenv.Preflight("katyoti", nil)
	if src := cenv.PreflightInfo().Source; src != "fallback" {
		t.Fatalf("no universe named, yet preflight says %s", src)
	}
	cwd, _ := os.Getwd()
	if got := wsPath("keys"); got != filepath.Join(cwd, "keys") {
		t.Fatalf("wsPath(keys) = %s, want %s", got, filepath.Join(cwd, "keys"))
	}
}
