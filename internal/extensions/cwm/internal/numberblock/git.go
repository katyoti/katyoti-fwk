// SPDX-License-Identifier: Apache-2.0

// Package numberblock gives readable object numbers (RQ-, F-, G-, ...) from
// blocks a minter reserves by a commit in the repository that holds the
// objects (decision: a ledger in each repository). The
// ledger is a dedicated ref, never a file of the working tree: a reservation
// writes git objects and moves the ref by compare-and-swap, and pushes it when
// the repository has a remote. It never touches the working tree or the index,
// which people and agents share.
package numberblock

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

// Ref is the ledger's ref; LedgerFile the one file its commits hold.
const (
	Ref        = "refs/katyoti/number-blocks"
	LedgerFile = "number-blocks.yaml"
)

// Repo is one git repository the ledger lives in.
type Repo struct {
	// Dir is the repository's top level.
	Dir string
	// Remote is the remote the ledger is pushed to and fetched from; empty
	// when the repository has none (the local compare-and-swap is then the
	// whole reservation).
	Remote string
	// readTip and readLedger are the last ledger read, by its commit.
	mu         sync.Mutex
	readTip    string
	readLedger Ledger
	// beforePush, when set, runs between the local compare-and-swap and the
	// push (tests: another clone reserving in between).
	beforePush func()
}

// Open answers the repository whose top level is root, or false when root is
// not the top level of a git repository (a project in no repository, or in a
// subfolder of one, mints as before).
func Open(root string) (*Repo, bool) {
	out, err := git(root, nil, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, false
	}
	top := filepath.Clean(filepath.FromSlash(strings.TrimSpace(out)))
	a, err1 := os.Stat(top)
	b, err2 := os.Stat(root)
	if err1 != nil || err2 != nil || !os.SameFile(a, b) {
		return nil, false
	}
	r := &Repo{Dir: top}
	if remotes, err := git(top, nil, "remote"); err == nil {
		names := strings.Fields(remotes)
		for _, n := range names {
			if n == "origin" {
				r.Remote = n
			}
		}
		if r.Remote == "" && len(names) > 0 {
			r.Remote = names[0]
		}
	}
	return r, true
}

// tip answers the commit the ledger ref points at, "" when there is none.
// The ref is read from the repository's files when it can be (a mint reads it
// on every create: a git process costs a large part of a second on Windows),
// through git otherwise.
func (r *Repo) tip() (string, error) {
	if t, ok := r.tipFromFiles(); ok {
		return t, nil
	}
	out, err := git(r.Dir, nil, "rev-parse", "--verify", "--quiet", Ref+"^{commit}")
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return "", nil // --quiet: a missing ref exits 1 with nothing said
		}
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// tipFromFiles reads the ref in the files backend: its loose file, else its
// line in packed-refs. false when it cannot tell (another ref backend, a git
// folder not found, a symbolic ref).
func (r *Repo) tipFromFiles() (string, bool) {
	gitDir := filepath.Join(r.Dir, ".git")
	st, err := os.Stat(gitDir)
	if err != nil {
		if r.bare() {
			gitDir = r.Dir
		} else {
			return "", false
		}
	} else if !st.IsDir() {
		return "", false // a linked worktree or a submodule: ask git
	}
	if _, err := os.Stat(filepath.Join(gitDir, "reftable")); err == nil {
		return "", false
	}
	if raw, err := os.ReadFile(filepath.Join(gitDir, filepath.FromSlash(Ref))); err == nil {
		t := strings.TrimSpace(string(raw))
		if len(t) < 40 || strings.HasPrefix(t, "ref:") {
			return "", false
		}
		return t, true
	}
	raw, err := os.ReadFile(filepath.Join(gitDir, "packed-refs"))
	if err != nil {
		return "", errors.Is(err, os.ErrNotExist)
	}
	for _, l := range strings.Split(string(raw), "\n") {
		if f := strings.Fields(l); len(f) == 2 && f[1] == Ref {
			return f[0], true
		}
	}
	return "", true
}

// bare says whether Dir is a bare repository's folder.
func (r *Repo) bare() bool {
	_, err := os.Stat(filepath.Join(r.Dir, "HEAD"))
	_, err2 := os.Stat(filepath.Join(r.Dir, "objects"))
	return err == nil && err2 == nil
}

// read answers the ledger at commit (empty at ""), parsed once per commit.
func (r *Repo) read(commit string) (Ledger, error) {
	if commit == "" {
		return Ledger{}, nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if commit == r.readTip {
		return Ledger{Blocks: append([]Block{}, r.readLedger.Blocks...)}, nil
	}
	out, err := git(r.Dir, nil, "cat-file", "blob", commit+":"+LedgerFile)
	if err != nil {
		return Ledger{}, fmt.Errorf("read the number-block ledger: %w", err)
	}
	l, err := parseLedger([]byte(out))
	if err == nil {
		r.readTip, r.readLedger = commit, l
	}
	return Ledger{Blocks: append([]Block{}, l.Blocks...)}, err
}

// ReadLedger answers the ledger as the local ref holds it, and its commit.
func (r *Repo) ReadLedger() (Ledger, string, error) {
	tip, err := r.tip()
	if err != nil {
		return Ledger{}, "", err
	}
	l, err := r.read(tip)
	return l, tip, err
}

// commit writes the ledger as a commit over parent ("" = the first) and
// answers it. Only git objects are written: no working tree, no index.
func (r *Repo) commit(l Ledger, parent, message string) (string, error) {
	data, err := l.encode()
	if err != nil {
		return "", err
	}
	blob, err := git(r.Dir, data, "hash-object", "-w", "--stdin")
	if err != nil {
		return "", err
	}
	tree, err := git(r.Dir, []byte("100644 blob "+strings.TrimSpace(blob)+"\t"+LedgerFile+"\n"), "mktree")
	if err != nil {
		return "", err
	}
	args := []string{"commit-tree", strings.TrimSpace(tree), "-m", message}
	if parent != "" {
		args = append(args, "-p", parent)
	}
	c, err := git(r.Dir, nil, args...)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(c), nil
}

// casRef moves the ref from old ("" = must not exist) to next: false when
// another writer moved it first.
func (r *Repo) casRef(next, old string) bool {
	_, err := git(r.Dir, nil, "update-ref", "-m", "number block", Ref, next, old)
	return err == nil
}

// pushResult is what a push of the ledger came to.
type pushResult int

const (
	pushed       pushResult = iota
	pushRejected            // the remote holds a ledger this one does not extend
	pushFailed              // the remote could not be reached
)

// push sends the ledger ref to the remote, never forced: a remote ledger this
// commit does not extend refuses it.
func (r *Repo) push() (pushResult, string) {
	out, err := git(r.Dir, nil, "push", "--porcelain", r.Remote, Ref+":"+Ref)
	if err == nil {
		return pushed, ""
	}
	msg := out + " " + err.Error()
	if strings.Contains(msg, "[rejected]") || strings.Contains(msg, "non-fast-forward") ||
		strings.Contains(msg, "fetch first") || strings.Contains(msg, "stale info") || strings.Contains(msg, "[remote rejected]") {
		return pushRejected, msg
	}
	return pushFailed, msg
}

// fetch brings the remote's ledger into the local ref (forced: the remote's
// wins over a local block it never accepted). false when the remote could not
// be reached; a remote with no ledger yet leaves the local ref as it is.
func (r *Repo) fetch() bool {
	out, err := git(r.Dir, nil, "ls-remote", r.Remote, Ref)
	if err != nil {
		return false
	}
	if strings.TrimSpace(out) == "" {
		return true // the remote has no ledger yet
	}
	_, err = git(r.Dir, nil, "fetch", "--no-tags", "--quiet", r.Remote, "+"+Ref+":"+Ref)
	return err == nil
}

// ensureRefspec adds the ledger's refspec to the remote's fetch list once, so
// a plain fetch carries the ledger too.
func (r *Repo) ensureRefspec() {
	if r.Remote == "" {
		return
	}
	spec := "+" + Ref + ":" + Ref
	key := "remote." + r.Remote + ".fetch"
	out, _ := git(r.Dir, nil, "config", "--get-all", key)
	for _, l := range strings.Split(out, "\n") {
		if strings.TrimSpace(l) == spec {
			return
		}
	}
	_, _ = git(r.Dir, nil, "config", "--add", key, spec)
}

// git runs one git command in dir, stdin fed when given, and answers its
// standard output. The environment's GIT_* variables are dropped (a hook's
// GIT_DIR or GIT_INDEX_FILE would aim it elsewhere); an author and committer
// are set, so a repository with no user configured still takes the commit.
func git(dir string, stdin []byte, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	env := []string{}
	for _, e := range os.Environ() {
		if !strings.HasPrefix(strings.ToUpper(e), "GIT_") {
			env = append(env, e)
		}
	}
	cmd.Env = append(env,
		"GIT_AUTHOR_NAME=KaTyoti", "GIT_AUTHOR_EMAIL=katyoti@localhost",
		"GIT_COMMITTER_NAME=KaTyoti", "GIT_COMMITTER_EMAIL=katyoti@localhost",
		"GIT_TERMINAL_PROMPT=0")
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return stdout.String(), fmt.Errorf("git %s: %w: %s", args[0], err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}
