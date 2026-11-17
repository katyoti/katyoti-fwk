// SPDX-License-Identifier: Apache-2.0

package supervisor

import (
	"path/filepath"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
)

// universePaths bundles every host-side path ktw-gui needs to drive a
// ktw-hub spawn for one universe. Computed once per Activate; values mirror
// the resolution `internal/core/application/env.go::newAppEnvironment`
// performs in-process — we replicate that logic here because ktw-gui hosts
// multiple universes concurrently and each one wants its own root.
type universePaths struct {
	// UniverseRoot is the universe folder on disk — the parent of .katyoti/.
	// Stored in catalog.RegistryEntry.HomePath. Distinct from KtwHome.
	UniverseRoot string

	// KtwHome is the per-universe config root `<UniverseRoot>/.katyoti` —
	// the HomeDir the spawned ktw-hub derives from the KTW_WORKSPACE we set
	// (see hubclient.SpawnEnvForWorkspace).
	KtwHome string

	// DataDir is the resolved data root that ktw-hub will write its lockfile
	// under. Must match what the spawned ktw-hub computes, otherwise we look
	// for the lockfile in the wrong place.
	DataDir string
}

// resolveUniversePaths replicates the per-universe DataDir resolution:
//  1. ktw-gui's own KTW_DATA env var (host-wide override; rarely set)
//  2. <katyoti>/config/Settings.yaml `data_dir`
//  3. cenv.ResolveDataDir() (XDG default) + /universes/<universeName>
//
// The result must match what the spawned ktw-hub computes, otherwise we
// look for its lockfile in the wrong place. Both resolvers read the same
// inputs (KTW_DATA env, Settings.yaml, workspace.yaml).
func resolveUniversePaths(universeRoot string) (universePaths, error) {
	universeRoot = filepath.Clean(universeRoot)
	// application.EnvOf is the single source of truth for ANY workspace's
	// path resolution. It runs the same Settings.yaml + Univers.yaml +
	// DataDir/CacheDir resolution chain that application.Env() does for the
	// active workspace.
	env := application.EnvOf(universeRoot)
	return universePaths{
		UniverseRoot: universeRoot,
		KtwHome:      env.HomeDir(),
		DataDir:      env.DataDir(),
	}, nil
}
