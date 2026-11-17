// SPDX-License-Identifier: Apache-2.0

package projectfile

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
)

// HubToken is the provider name the hub publishes its own contributions
// under: no project may take it.
const HubToken = "ktw-hub"

// TokenConflict says why a project's contributions cannot be published under
// token in a universe whose members are named members: the token is the
// system's or the hub's, or a member answers to it. "" when they can. One
// rule, asked by the hub before it adds the project as a member, by the
// attach before it links one, and by lint-universe.
func TokenConflict(token string, members []string) string {
	if token == contribapi.SystemProvider || token == HubToken {
		return fmt.Sprintf("the project's contributions token %q is reserved: %s publishes under it; name another in capabilities.contributions.provider", token, token)
	}
	for _, m := range members {
		if m == token {
			return fmt.Sprintf("the project's contributions token %q is the name of member %s; name another in capabilities.contributions.provider", token, m)
		}
	}
	return ""
}

// ContributionsDir is the project's contributions folder, under projectRoot,
// when it exists; "" when the project holds none (absent is normal: a
// project that contributes nothing).
func (f File) ContributionsDir(projectRoot string) string {
	if projectRoot == "" {
		return ""
	}
	dir := filepath.Join(projectRoot, filepath.FromSlash(f.Contributions.Root))
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		return ""
	}
	return dir
}

// CWMFiles lists the `*.cwm.*` files under a contributions folder, relative
// to it and slash-separated. An object file there is in the wrong place: the
// folder is read as a pack, which leaves it out, and the lint says so. The
// folder's own `objects/` is not: when the folder is the sys-contributions
// module, its module object lives there.
func CWMFiles(dir string) ([]string, error) {
	var out []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && d.Name() == "objects" && filepath.Dir(p) == filepath.Clean(dir) {
			return filepath.SkipDir
		}
		if !d.IsDir() && strings.Contains(d.Name(), ".cwm.") {
			rel, _ := filepath.Rel(dir, p)
			out = append(out, filepath.ToSlash(rel))
		}
		return nil
	})
	sort.Strings(out)
	return out, err
}

// OverwritesDir is where the project overrides a provider's embedded content:
// `<project>/<contributions root>/overwrites/<provider>/`, laid out as the
// provider's own tree (`embed/<kind folder>/<name>`). One composition for every
// reader of it — the sidecar kit's content overlay and CWM's floor alike
// (decision); the folder may not exist.
func (f File) OverwritesDir(projectRoot, provider string) string {
	return filepath.Join(projectRoot, filepath.FromSlash(f.Contributions.Root), "overwrites", provider)
}
