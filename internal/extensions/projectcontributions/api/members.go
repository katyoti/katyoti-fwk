// SPDX-License-Identifier: Apache-2.0

package api

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/katyoti/katyoti-fwk/internal/core/projectfile"
)

// ProjectMember is the folder member of one attached project: its
// contributions folder, served under its provider token.
type ProjectMember struct {
	Token   string `json:"token"`
	Folder  string `json:"folder" jsonschema:"description=The contributions folder; absolute"`
	Project string `json:"project" jsonschema:"description=The project root"`
}

// RefusedMember is an attached project whose contributions are not served:
// its token and why (a finding naming the token and both owners).
type RefusedMember struct {
	Token   string `json:"token"`
	Project string `json:"project"`
	Why     string `json:"why"`
}

// ProjectMembers is the folder member of every attached project, in the
// order attached (projectRoots, cenv.AttachedProjects): a project that does
// not exist on disk, does not read or holds no contributions folder gives
// none. A project is refused when its token is reserved or the name of a
// declared member (projectfile.TokenConflict), or when an earlier project
// already holds it: the first attached wins, never shadowed. The hub calls
// it; it reads only the projects' files.
func ProjectMembers(projectRoots, declared []string) ([]ProjectMember, []RefusedMember) {
	var members []ProjectMember
	var refused []RefusedMember
	held := map[string]string{} // token -> the project holding it
	for _, root := range projectRoots {
		if root == "" {
			continue
		}
		if fi, err := os.Stat(root); err != nil || !fi.IsDir() {
			continue
		}
		f, err := projectfile.Read(root)
		if err != nil {
			slog.Warn("project-contributions: an attached project's settings do not read; its contributions are not served", "project", root, "error", err)
			continue
		}
		folder := f.ContributionsDir(root)
		if folder == "" {
			continue
		}
		token := f.Contributions.Provider
		if why := projectfile.TokenConflict(token, declared); why != "" {
			refused = append(refused, RefusedMember{Token: token, Project: root, Why: fmt.Sprintf("project %s: %s", root, why)})
			continue
		}
		if first, ok := held[token]; ok {
			refused = append(refused, RefusedMember{Token: token, Project: root,
				Why: fmt.Sprintf("the contributions token %q of project %s is already served for project %s; name another in capabilities.contributions.provider", token, root, first)})
			continue
		}
		held[token] = root
		members = append(members, ProjectMember{Token: token, Folder: folder, Project: root})
	}
	return members, refused
}
