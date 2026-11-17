// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/gitcli"
)

// THE EXPORT LEAVES THE MACHINE - after an export moves a
// project's ref, the ref is pushed to the repository's remote (origin, else
// the first one; none: the local ref is the whole copy), never forced, under
// its own refspec: the remote's fetch list gains
// +refs/katyoti/scheduler/*:refs/katyoti/scheduler/*, so a plain fetch of a
// clone brings the refs and `todos import` there sees the to-dos. Only a
// private remote takes it (data-storage S26/S27): a remote anyone can read
// without credentials is public and refused, one whose visibility cannot be
// told this time is not pushed to this time. The pattern is the number-block
// ledger's (internal/extensions/cwm/internal/numberblock/git.go), kept local:
// (a shared ref-commit helper) stays held.

// ExportRefSpec is the refspec a remote's fetch list gains.
const ExportRefSpec = "+" + ExportRefPrefix + "*:" + ExportRefPrefix + "*"

// Visibility is what a remote's probe told.
type Visibility int

const (
	VisibilityUnknown Visibility = iota // the probe could not tell (no network, an odd answer)
	VisibilityPrivate                   // credentials are needed to read it, or it is on this machine
	VisibilityPublic                    // anyone reads it without credentials
)

// remoteVisibility probes a remote's URL. A variable: a test names its own
// (a bare repository on disk is private, a test falsifies a public one).
var remoteVisibility = probeVisibility

// visibilityTTL is how long a probe's answer is kept, per URL.
var visibilityTTL = time.Hour

var (
	pushMu       sync.Mutex
	visibilities = map[string]visibilitySeen{}
	pushedTips   = map[string]string{} // dir + ref -> the commit last pushed
)

type visibilitySeen struct {
	v  Visibility
	at time.Time
}

// pushOutcome is what pushing one export ref came to.
type pushOutcome struct {
	Remote string
	Pushed bool
	Err    string
}

// exportRemote picks the remote the export goes to: origin, else the first
// one; "" when the repository has none.
func exportRemote(dir string) string {
	out, err := gitcli.Run(dir, "remote")
	if err != nil {
		return ""
	}
	names := strings.Fields(out)
	for _, n := range names {
		if n == "origin" {
			return n
		}
	}
	if len(names) > 0 {
		return names[0]
	}
	return ""
}

// pushExport pushes ref at commit to the repository's private remote, once
// per commit and process. Nothing to push (no remote) answers an empty
// outcome; a refusal or a failure is said in Err, never an error of the
// export: the local ref keeps the rows, the next export pushes again.
// rewrite writes the export again over the remote's tip after a push the
// remote rejected (another clone pushed first) and answers the new commit.
func pushExport(dir, ref, commit string, rewrite func() (string, error)) pushOutcome {
	remote := exportRemote(dir)
	if remote == "" || commit == "" {
		return pushOutcome{}
	}
	out := pushOutcome{Remote: remote}
	key := dir + "\x00" + ref
	pushMu.Lock()
	done := pushedTips[key] == commit
	pushMu.Unlock()
	if done {
		out.Pushed = true
		return out
	}
	u, err := gitcli.Run(dir, "remote", "get-url", "--push", remote)
	if err != nil {
		out.Err = err.Error()
		return out
	}
	switch visibilityOf(strings.TrimSpace(u), dir) {
	case VisibilityPublic:
		out.Err = fmt.Sprintf("refused: remote %s is public (readable without credentials); the export goes only to a private remote", remote)
		return out
	case VisibilityUnknown:
		out.Err = fmt.Sprintf("not pushed: the visibility of remote %s could not be told; the next export tries again", remote)
		return out
	}
	ensureExportRefSpec(dir, remote)
	for attempt := 0; attempt < 2; attempt++ {
		res, msg := pushRef(dir, remote, ref)
		switch res {
		case pushed:
			pushMu.Lock()
			pushedTips[key] = commit
			pushMu.Unlock()
			out.Pushed, out.Err = true, ""
			return out
		case pushFailed:
			out.Err = "push failed: " + msg
			return out
		}
		// Rejected: the remote holds commits this ref does not extend. Take
		// them as the parent - the store is the truth, the export a copy of it.
		out.Err = "push rejected: " + msg
		if attempt > 0 || rewrite == nil {
			return out
		}
		if _, err := gitcli.Run(dir, "fetch", "--no-tags", "--quiet", remote, "+"+ref+":"+ref); err != nil {
			out.Err += "; fetch: " + err.Error()
			return out
		}
		if commit, err = rewrite(); err != nil {
			out.Err += "; rewrite: " + err.Error()
			return out
		}
	}
	return out
}

type pushResult int

const (
	pushed pushResult = iota
	pushRejected
	pushFailed
)

// pushRef sends the ref to the remote, never forced.
func pushRef(dir, remote, ref string) (pushResult, string) {
	out, err := gitcli.Run(dir, "push", "--porcelain", "--no-verify", remote, ref+":"+ref)
	if err == nil {
		return pushed, ""
	}
	msg := strings.TrimSpace(out + " " + err.Error())
	for _, s := range []string{"[rejected]", "non-fast-forward", "fetch first", "stale info", "[remote rejected]"} {
		if strings.Contains(msg, s) {
			return pushRejected, msg
		}
	}
	return pushFailed, msg
}

// ensureExportRefSpec adds the export's refspec to the remote's fetch list
// once, so a plain fetch carries the export refs too.
func ensureExportRefSpec(dir, remote string) {
	key := "remote." + remote + ".fetch"
	out, _ := gitcli.Run(dir, "config", "--get-all", key)
	for _, l := range gitcli.Lines(out) {
		if l == ExportRefSpec {
			return
		}
	}
	_, _ = gitcli.Run(dir, "config", "--add", key, ExportRefSpec)
}

// visibilityOf answers a remote URL's visibility, a probe's answer kept
// visibilityTTL; an unknown one is never kept.
func visibilityOf(remoteURL, dir string) Visibility {
	pushMu.Lock()
	seen, ok := visibilities[remoteURL]
	pushMu.Unlock()
	if ok && time.Since(seen.at) < visibilityTTL {
		return seen.v
	}
	v := remoteVisibility(remoteURL, dir)
	if v != VisibilityUnknown {
		pushMu.Lock()
		visibilities[remoteURL] = visibilitySeen{v: v, at: time.Now()}
		pushMu.Unlock()
	}
	return v
}

// scpLike is git's scp-like ssh address: [user@]host:path.
var scpLike = regexp.MustCompile(`^(?:[^@/]+@)?([^:/]+):(.+)$`)

// probeURL answers the URL an anonymous read of the remote goes to, and
// whether the remote is on this machine (a path, file://: private, no probe).
// An ssh address is probed over https on the same host and path: what an
// anonymous person could read there.
func probeURL(remoteURL string) (string, bool) {
	s := strings.TrimSpace(remoteURL)
	if strings.HasPrefix(s, "file://") || filepath.IsAbs(s) || strings.HasPrefix(s, ".") || strings.HasPrefix(s, "/") {
		return "", true
	}
	if u, err := url.Parse(s); err == nil && u.Scheme != "" && len(u.Scheme) > 1 {
		u.User = nil
		switch u.Scheme {
		case "ssh", "git+ssh", "ssh+git":
			u.Scheme, u.Host = "https", u.Hostname()
		}
		return u.String(), false
	}
	if m := scpLike.FindStringSubmatch(s); m != nil {
		return "https://" + m[1] + "/" + strings.TrimPrefix(m[2], "/"), false
	}
	return "", true // a relative path
}

// authNeeded is what git says when a read needs credentials (or hides a
// private repository as a missing one).
var authNeeded = []string{"authentication", "could not read username", "terminal prompts disabled", "repository not found",
	"403", "401", "404", "access denied", "permission denied", "not authorized", "unauthorized", "requested url returned error"}

// probeVisibility reads the remote anonymously - no credential helper, no
// prompt, no askpass, no ssh key: answered, it is public; refused for want of
// credentials, private; anything else unknown.
func probeVisibility(remoteURL, _ string) Visibility {
	probe, local := probeURL(remoteURL)
	if local {
		return VisibilityPrivate
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "-c", "credential.helper=", "-c", "core.askPass=", "ls-remote", "--heads", probe)
	env := []string{}
	for _, e := range gitcliEnv() {
		up := strings.ToUpper(e)
		if strings.HasPrefix(up, "GIT_ASKPASS=") || strings.HasPrefix(up, "SSH_ASKPASS=") {
			continue
		}
		env = append(env, e)
	}
	cmd.Env = append(env, "GIT_TERMINAL_PROMPT=0", "GCM_INTERACTIVE=never", "GIT_CONFIG_NOSYSTEM=1")
	outB, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		return VisibilityUnknown
	}
	if err == nil {
		return VisibilityPublic
	}
	msg := strings.ToLower(string(outB))
	for _, s := range authNeeded {
		if strings.Contains(msg, s) {
			return VisibilityPrivate
		}
	}
	return VisibilityUnknown
}

// gitcliEnv is the process environment without the variables that aim git at
// another repository (a hook's GIT_DIR).
func gitcliEnv() []string {
	out := []string{}
	for _, e := range os.Environ() {
		up := strings.ToUpper(e)
		if strings.HasPrefix(up, "GIT_DIR=") || strings.HasPrefix(up, "GIT_WORK_TREE=") || strings.HasPrefix(up, "GIT_INDEX_FILE=") ||
			strings.HasPrefix(up, "GIT_OBJECT_DIRECTORY=") || strings.HasPrefix(up, "GIT_COMMON_DIR=") || strings.HasPrefix(up, "GIT_PREFIX=") {
			continue
		}
		out = append(out, e)
	}
	return out
}
