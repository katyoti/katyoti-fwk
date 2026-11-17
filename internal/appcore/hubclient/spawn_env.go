// SPDX-License-Identifier: Apache-2.0

package hubclient

import (
	"os"
	"path/filepath"
	"strings"
)

// SpawnEnvForWorkspace returns the env slice for ConnectOptions.SpawnEnv that
// scopes a spawned ktw-hub to one universe: the caller's environment with
// `KTW_WORKSPACE` set to the universe folder (the parent of `.katyoti/`). The
// hub then derives HomeDir = `<KTW_WORKSPACE>/.katyoti` itself. KTW_DATA /
// KTW_CACHE / other host overrides flow through unchanged, so the spawned hub
// resolves the same DataDir (and lockfile) as application.EnvOf(workspace).
//
// Any inherited `KTW_WORKSPACE` and legacy `KTW_HOME` are dropped so each
// spawn is scoped cleanly.
func SpawnEnvForWorkspace(workspace string) []string {
	workspace = filepath.Clean(workspace)
	env := os.Environ()
	out := make([]string, 0, len(env)+1)
	for _, kv := range env {
		if strings.HasPrefix(kv, "KTW_HOME=") || strings.HasPrefix(kv, "KTW_WORKSPACE=") {
			continue
		}
		out = append(out, kv)
	}
	return append(out, "KTW_WORKSPACE="+workspace)
}
