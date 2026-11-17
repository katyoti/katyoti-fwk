// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// A MODULE UPDATE IS COMMITTED WITH WHAT IT RECORDS —
// the acceptance checks and the change-log entry a request's module update
// names are objects the step wrote: in a project kept in git, a module-update
// report is not collected while one of their files is untracked, and the
// module-updated gate refuses the end the same way. A worker commits by
// explicit pathspec from what it believes it touched; a criterion left out
// leaves HEAD with verdicts whose parent is no object. A project with no git,
// or git missing, is not checked.

// recordedObjects are the objects a request's module update names: its checks
// and its change-log entry.
func recordedObjects(o cwmapi.Object) []string {
	mu := fieldsOf(o).ModuleUpdate
	if mu == nil {
		return nil
	}
	ids := append([]string(nil), mu.Checks...)
	if isChangeID(mu.Changelog) {
		ids = append(ids, mu.Changelog)
	}
	return ids
}

// untracked answers, for each of ids whose file the project's git does not
// track, what to commit: "<id> (<path>)". Objects CWM cannot resolve, and a
// project outside git, answer nothing.
func untracked(ctx context.Context, cwm cwmapi.CWMService, ids []string) []string {
	var out []string
	for _, id := range ids {
		res, err := cwm.ObjectResolve(ctx, cwmapi.ObjectResolveInput{Ref: id})
		if err != nil || res.Path == "" || res.ProjectRoot == "" || !inGitRepo(res.ProjectRoot) {
			continue
		}
		rel, err := filepath.Rel(res.ProjectRoot, res.Path)
		if err != nil {
			continue
		}
		rel = filepath.ToSlash(rel)
		// ls-files lists what the index holds (staged counts): empty is
		// untracked; an error is no repository, or no git.
		got, err := exec.CommandContext(ctx, "git", "-C", res.ProjectRoot, "ls-files", "--", rel).Output()
		if err != nil || strings.TrimSpace(string(got)) != "" {
			continue
		}
		out = append(out, fmt.Sprintf("%s (%s)", id, rel))
	}
	return out
}

// checkCommitted refuses a module-update report while an object the request's
// module update names is untracked in git.
func checkCommitted(ctx context.Context, cwm cwmapi.CWMService, request string) error {
	o, err := load(ctx, cwm, request)
	if err != nil {
		return err
	}
	if missing := untracked(ctx, cwm, recordedObjects(o)); len(missing) > 0 {
		return invalid("module-update: commit what the module update records before reporting it - untracked in git: %s", strings.Join(missing, ", "))
	}
	return nil
}

// inGitRepo reports a folder inside a git work tree: a `.git` in it or in one
// of its parents. Checked on the file system first, so a project outside git
// runs no git command.
func inGitRepo(dir string) bool {
	for d := filepath.Clean(dir); ; {
		if _, err := os.Stat(filepath.Join(d, ".git")); err == nil {
			return true
		}
		parent := filepath.Dir(d)
		if parent == d {
			return false
		}
		d = parent
	}
}
