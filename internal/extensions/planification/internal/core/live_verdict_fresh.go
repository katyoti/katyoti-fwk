// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"os/exec"
	"path"
	"strings"
	"time"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// A LIVE VERDICT COUNTS ONLY WHEN NEWER THAN ITS SCRIPT:
// a tests/live deliverable is verified from its criterion's passed verdict
// (verdictFor) only when that verdict ran at or after the last commit that
// touched the script it runs - a pass recorded before the walk changed proves
// the old walk, so the deliverable waits on the batch again. A project outside
// git, a script git does not know, or git missing set no such bound.

// scriptChangedAt is when the last commit touching the tests/live script of
// probe was made (RFC 3339), in the project holding criterion; "" when it
// cannot be told.
func scriptChangedAt(ctx context.Context, cwm cwmapi.CWMService, criterion, probe string) string {
	rel := probeScriptPath(probe)
	if rel == "" {
		return ""
	}
	res, err := cwm.ObjectResolve(ctx, cwmapi.ObjectResolveInput{Ref: criterion})
	if err != nil || res.ProjectRoot == "" || !inGitRepo(res.ProjectRoot) {
		return ""
	}
	out, err := exec.CommandContext(ctx, "git", "-C", res.ProjectRoot, "log", "-1", "--format=%cI", "--", rel).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// probeScriptPath is the project-relative path of the tests/live script a
// probe runs - joined to the folder a `cd <dir> &&` before it enters, so
// `cd app/ktw-gui/frontend && bash tests/live/x.sh` names
// app/ktw-gui/frontend/tests/live/x.sh; "" with none.
func probeScriptPath(probe string) string {
	dir := ""
	words := strings.Fields(strings.ReplaceAll(probe, "\\", "/"))
	for i, w := range words {
		w = strings.Trim(w, "\"'`;")
		if w == "cd" && i+1 < len(words) {
			dir = strings.Trim(words[i+1], "\"'`;&")
			continue
		}
		if strings.Contains(w, liveGUITest) {
			if dir != "" && !path.IsAbs(w) {
				w = path.Join(dir, w)
			}
			return strings.TrimPrefix(w, "./")
		}
	}
	return ""
}

// laterOf is the later of two RFC 3339 times; either when the other cannot
// be read.
func laterOf(a, b string) string {
	ta, errA := time.Parse(time.RFC3339, a)
	tb, errB := time.Parse(time.RFC3339, b)
	switch {
	case errA != nil:
		if errB != nil {
			return a
		}
		return b
	case errB != nil:
		return a
	case tb.After(ta):
		return b
	default:
		return a
	}
}
