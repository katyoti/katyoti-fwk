// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/katyoti/katyoti-fwk/internal/core/atomicio"
	"github.com/katyoti/katyoti-fwk/internal/core/gitcli"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// THE CATCH-UP (its seeing half): a file edited outside the CWM
// verbs - a git pull or merge, the person's editor, a bulk copy - is caught up
// when CWM opens or scans the repository.
//
// The index is in memory and every scan reads every file, so a read already
// sees a hand edit. What a scan alone cannot say is WHAT MOVED SINCE CWM LAST
// SAW THE REPOSITORY: after a restart every object reads as new. The cursor
// answers it - the last HEAD seen plus the path -> hash manifest of the
// objects as the last scan read them - persisted beside the index in the
// universe's CWM data folder (index only, never committed).
//
// THE MANIFEST DECIDES what moved: the scan has just hashed every file, so
// comparing its hashes with the cursor's is a pass over two maps. An unchanged
// tree, or a dirty file already announced, announces nothing after a restart -
// HEAD alone would re-announce every dirty file - and a dirty file reverted
// since is announced. Git is read for the HEAD only (the cause: the commits
// between the cursor's HEAD and this one). Git does NOT narrow the comparison:
// measured, `git diff --name-only <HEAD>` over 1,187 edited files takes 1.7 s
// on Windows (git hashes each one again) against no time for the manifest,
// because the in-memory index re-reads every file at open anyway; narrowing by
// git pays once the index itself is persisted, and the cursor's HEAD is kept
// for it.
//
// A catch-up never writes an object file: it reads the index and writes the
// cursor (and the events it holds) only. What it found moved is published as
// ONE grouped event,
// `project.caught-up` (api.ObjectsCaughtUpEventPayload): the ids added,
// modified and removed, the cause - the commits between the cursor's HEAD and
// this one, a rewritten history when the cursor's HEAD is no ancestor of this
// one (or git no longer has it), or the working tree when HEAD did not move or
// a moved file is not committed - and the actor, of
// kind outside: the author of the newest commit with its sha, or the local
// git user with no sha for an edit not committed yet.
//
// THE THRESHOLD (api.CatchUpPerObjectMax, 50): beside the grouped event, a
// catch-up of 50 moved objects or fewer publishes each one's object event -
// created, updated or deleted, operation caught-up - so the reactions that
// listen to object events run as for a verb write. Above 50 the grouped event
// says held and carries the count, and the per-object events are kept in the
// held file beside the cursor until CatchUpRelease is called with the grouped
// event's id (the answer to its card): a pull of hundreds of objects never
// floods the reactions unasked.

// catchUpCursorFile is the cursor's file in the universe's CWM data folder.
const catchUpCursorFile = "catchup-cursor.json"

// ONE CURSOR PER REPOSITORY: every root the index scans (the project, the
// universe's object store, the further roots a universe opens) belongs to the
// git repository whose top level holds it (`git rev-parse --show-toplevel` - a
// root may be a subfolder of its repository), and each repository keeps its own
// HEAD and its own manifest. Two roots in one repository share one cursor; a
// root in a repository of its own is caught up from its own HEAD, never
// another's. A root outside any repository is kept under the key "": the
// manifest diff alone, no HEAD - its catch-up names the working tree and an
// outside actor with no sha. A repository the cursor never saw is a baseline,
// as a first open is.

// catchUpCursor is what CWM last saw of the repositories its roots are in.
type catchUpCursor struct {
	// Repos is each repository's cursor, by its top level (slash form); ""
	// holds the roots outside any git repository.
	Repos map[string]repoCursor `json:"repos,omitempty"`

	// Head and Files are the cursor of one repository as it was written
	// before the cursor was kept per repository: read once and taken as the
	// project root's repository's HEAD and the files of every root.
	Head  string                `json:"head,omitempty"`
	Files map[string]cursorFile `json:"files,omitempty"`
}

// repoCursor is what CWM last saw of one repository.
type repoCursor struct {
	// Head is the repository's HEAD at the last catch-up; "" outside git or
	// before a first commit.
	Head string `json:"head,omitempty"`
	// Files is every indexed object file of the repository's roots, by its
	// index path.
	Files map[string]cursorFile `json:"files"`
}

// cursorFile is one object file as the last catch-up saw it.
type cursorFile struct {
	ID   string `json:"id"`
	Hash string `json:"hash"`
}

// catchUp is what a catch-up found moved since the cursor.
type catchUp struct {
	// Repo is the repository's top level the catch-up compared, "" for the
	// roots outside any git repository; its git commands run there.
	Repo                     string
	From, To                 string // the cursor's HEAD and the HEAD now
	Added, Modified, Removed []string
	// Paths are the files of the ids that moved, as the cursor and the scan
	// name them: the ones whose state in git says the cause.
	Paths []string
	// Revisions is each moved id's revision: the hash its file has now, or
	// for a removed id the one it had - what its object event carries.
	Revisions map[string]string
}

// empty reports whether nothing moved.
func (c catchUp) empty() bool { return len(c.Added)+len(c.Modified)+len(c.Removed) == 0 }

// catchUpState is the index's catch-up bookkeeping.
type catchUpState struct {
	mu sync.Mutex // one catch-up at a time
	// on, when set, receives every catch-up that found something moved
	// (the grouped event is published from it). Nil: none.
	on func(catchUp)
	// last is the last catch-up run, one per repository, for the tests and
	// the reports.
	last []catchUp
	// tops is each root's repository top level once known ("" outside git):
	// a root does not change repository while the index lives.
	tops map[string]string

	// THE VERBS' OWN WRITES KEEP THE CURSOR: a file a verb wrote or removed is
	// no outside change, so it must not be caught up at the next open. Each
	// put / drop records its file here (by index path; nil: the file went);
	// the cursor takes them a moment later (cursorFlushDelay), at Close, and
	// before any catch-up compares. pendMu is a leaf lock: put / drop take it
	// under the index's lock, the catch-up under its own.
	pendMu  sync.Mutex
	pending map[string]*cursorFile
	flushT  *time.Timer
}

// cursorFlushDelay batches the verbs' writes into one cursor write.
const cursorFlushDelay = 2 * time.Second

// cursorPath is the cursor's file: beside the object store, in the universe's
// CWM data folder. "" when the index has no store (substrate tests).
func (ix *objectIndex) cursorPath() string {
	if ix.storeRoot == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(ix.storeRoot), catchUpCursorFile)
}

// catchUp compares the index as the last scan read it with the cursor, one
// repository at a time, hands what moved in each to the catch-up hook (one
// call per repository that moved), and saves the cursor. The first catch-up
// of a universe - or of a repository the cursor never saw - saves its cursor
// and announces nothing: a first open is a baseline, not a change.
func (ix *objectIndex) catchUp() ([]catchUp, error) {
	path := ix.cursorPath()
	if path == "" {
		return nil, nil
	}
	ix.catch.mu.Lock()
	defer ix.catch.mu.Unlock()

	now := ix.byRepo(ix.manifest())
	pend := ix.takePending()
	prev, found, err := ix.readRepoCursors(path)
	if err != nil {
		found = false // an unreadable cursor is a lost one: a new baseline
	}
	if found && len(pend) > 0 {
		// The verbs' own writes are what CWM saw: never an outside change.
		prev = ix.withPending(prev, pend)
	}
	next := map[string]repoCursor{}
	for _, top := range ix.rootTops() {
		next[top] = repoCursor{Files: map[string]cursorFile{}}
	}
	for top, files := range now {
		next[top] = repoCursor{Files: files}
	}
	for top, was := range prev.Repos {
		if _, ok := next[top]; !ok && len(was.Files) > 0 {
			// every object of it went: announced once, then the repository
			// is forgotten with its last file
			next[top] = repoCursor{Files: map[string]cursorFile{}}
		}
	}
	var out []catchUp
	changed := !found || len(pend) > 0 || prev.legacy() || len(prev.Repos) != len(next)
	for _, top := range slices.Sorted(maps.Keys(next)) {
		rc := next[top]
		rc.Head = gitHeadIn(top)
		next[top] = rc
		was, seen := prev.Repos[top]
		if !seen || rc.Head != was.Head || !maps.Equal(was.Files, rc.Files) {
			changed = true
		}
		if !found || !seen {
			continue // a baseline for this repository
		}
		c := catchUp{Repo: top, From: was.Head, To: rc.Head}
		c.Added, c.Modified, c.Removed = diffManifests(was.Files, rc.Files)
		if c.empty() {
			continue
		}
		c.Paths = movedPaths(was.Files, rc.Files, c)
		c.Revisions = movedRevisions(was.Files, rc.Files, c)
		out = append(out, c)
	}
	// AN UNCHANGED CURSOR IS NOT WRITTEN AGAIN: a scan of a quiet tree
	// touches no file.
	if changed {
		if err := writeCursor(path, catchUpCursor{Repos: next}); err != nil {
			return out, err
		}
	}
	ix.catch.last = out
	if ix.catch.on != nil {
		for _, c := range out {
			ix.catch.on(c)
		}
	}
	return out, nil
}

// rebaseline writes the cursor from the index as the last scan read it and
// announces nothing: what moved since was written by CWM itself (a
// conversion), never outside the verbs.
func (ix *objectIndex) rebaseline() error {
	path := ix.cursorPath()
	if path == "" {
		return nil
	}
	ix.catch.mu.Lock()
	defer ix.catch.mu.Unlock()
	ix.takePending()
	next := map[string]repoCursor{}
	for _, top := range ix.rootTops() {
		next[top] = repoCursor{Files: map[string]cursorFile{}}
	}
	for top, files := range ix.byRepo(ix.manifest()) {
		next[top] = repoCursor{Files: files}
	}
	for top, rc := range next {
		rc.Head = gitHeadIn(top)
		next[top] = rc
	}
	ix.catch.last = nil
	return writeCursor(path, catchUpCursor{Repos: next})
}

// legacy reports whether the cursor was read in its one-repository shape.
func (c catchUpCursor) legacy() bool { return c.Repos == nil && c.Files != nil }

// readRepoCursors reads the cursor, per repository. A cursor written in the
// one-repository shape is split over the repositories of today's roots: its
// HEAD is the project root's repository's.
func (ix *objectIndex) readRepoCursors(path string) (catchUpCursor, bool, error) {
	c, found, err := readCursor(path)
	if err != nil || !found {
		return c, found, err
	}
	if c.legacy() {
		c.Repos = map[string]repoCursor{}
		for top, files := range ix.byRepo(c.Files) {
			c.Repos[top] = repoCursor{Files: files}
		}
		top := ix.rootTop(ix.root)
		rc := c.Repos[top]
		if rc.Files == nil {
			rc.Files = map[string]cursorFile{}
		}
		rc.Head = c.Head
		c.Repos[top] = rc
	}
	if c.Repos == nil {
		c.Repos = map[string]repoCursor{}
	}
	return c, true, nil
}

// withPending puts the verbs' writes into the cursor, each in the repository
// of its root.
func (ix *objectIndex) withPending(c catchUpCursor, pend map[string]*cursorFile) catchUpCursor {
	for p, f := range pend {
		top := ix.repoOf(p)
		rc := c.Repos[top]
		if rc.Files == nil {
			rc.Files = map[string]cursorFile{}
		}
		if f == nil {
			delete(rc.Files, p)
		} else {
			rc.Files[p] = *f
		}
		c.Repos[top] = rc
	}
	return c
}

// byRepo splits a manifest by the repository of each file's root.
func (ix *objectIndex) byRepo(files map[string]cursorFile) map[string]map[string]cursorFile {
	out := map[string]map[string]cursorFile{}
	for p, f := range files {
		top := ix.repoOf(p)
		if out[top] == nil {
			out[top] = map[string]cursorFile{}
		}
		out[top][p] = f
	}
	return out
}

// roots are the folders the index scans: the project root and the
// universe's object store.
func (ix *objectIndex) roots() []string {
	out := []string{ix.root}
	if ix.storeRoot != "" {
		out = append(out, ix.storeRoot)
	}
	return out
}

// rootTops are the repositories of the roots the index scans, each once.
func (ix *objectIndex) rootTops() []string {
	var out []string
	for _, r := range ix.roots() {
		if top := ix.rootTop(r); !slices.Contains(out, top) {
			out = append(out, top)
		}
	}
	return out
}

// repoOf is the repository of an index path: its root's - the deepest root
// that holds it - or the project root's.
func (ix *objectIndex) repoOf(indexPath string) string {
	abs := filepath.Join(ix.root, filepath.FromSlash(indexPath))
	best, depth := ix.root, -1
	for _, r := range ix.roots() {
		rel, err := filepath.Rel(r, abs)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue
		}
		if d := len(filepath.Clean(r)); d > depth {
			best, depth = r, d
		}
	}
	return ix.rootTop(best)
}

// rootTop is a root's repository top level (slash form), "" when it is in
// no git repository. Known once per root: a root existing is asked once.
func (ix *objectIndex) rootTop(root string) string {
	if top, ok := ix.catch.tops[root]; ok {
		return top
	}
	top, final := gitTopLevel(root)
	if final {
		if ix.catch.tops == nil {
			ix.catch.tops = map[string]string{}
		}
		ix.catch.tops[root] = top
	}
	return top
}

// gitTopLevel is the top level of the git repository holding dir (slash
// form), "" outside any. final is false while dir does not exist yet: its
// answer may change once it does.
func gitTopLevel(dir string) (top string, final bool) {
	if !inGitWorkTree(dir) {
		return "", true // no git process for a folder outside any repository
	}
	if _, err := os.Stat(dir); err != nil {
		return "", false
	}
	out, err := gitcli.Run(dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", true // a folder git does not see as a work tree (a .git inside)
	}
	return filepath.ToSlash(filepath.Clean(strings.TrimSpace(out))), true
}

// manifest is the index's object files as the last scan read them.
func (ix *objectIndex) manifest() map[string]cursorFile {
	ix.mu.RLock()
	defer ix.mu.RUnlock()
	out := make(map[string]cursorFile, len(ix.byID))
	for id, e := range ix.byID {
		out[e.Path] = cursorFile{ID: id, Hash: e.Hash}
	}
	return out
}

// gitHeadIn is the HEAD of the repository at top, "" for the roots outside
// git (top "") or a repository with no commit yet.
func gitHeadIn(top string) string {
	if top == "" {
		return ""
	}
	out, err := gitcli.Run(filepath.FromSlash(top), "rev-parse", "--verify", "-q", "HEAD")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

// canonicalPath is an existing folder's path in its one resolved form - the
// long name of each element, symlinks followed - or the cleaned path when it
// cannot be resolved.
func canonicalPath(p string) string {
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return filepath.Clean(r)
	}
	return filepath.Clean(p)
}

// inGitWorkTree reports whether dir or one of its parents holds a `.git`.
func inGitWorkTree(dir string) bool {
	for d := filepath.Clean(dir); ; {
		if _, err := os.Stat(filepath.Join(d, ".git")); err == nil {
			return true
		}
		up := filepath.Dir(d)
		if up == d {
			return false
		}
		d = up
	}
}

// diffManifests says, by id, what moved between two manifests. An id whose
// file moved with the same bytes did not change.
func diffManifests(prev, now map[string]cursorFile) (added, modified, removed []string) {
	oldByID := map[string]string{}
	newByID := map[string]string{}
	touched := map[string]bool{}
	for _, f := range prev {
		oldByID[f.ID] = f.Hash
		touched[f.ID] = true
	}
	for _, f := range now {
		newByID[f.ID] = f.Hash
		touched[f.ID] = true
	}
	for id := range touched {
		o, had := oldByID[id]
		n, has := newByID[id]
		switch {
		case !had && has:
			added = append(added, id)
		case had && !has:
			removed = append(removed, id)
		case o != n:
			modified = append(modified, id)
		}
	}
	sort.Strings(added)
	sort.Strings(modified)
	sort.Strings(removed)
	return added, modified, removed
}

func readCursor(path string) (catchUpCursor, bool, error) {
	raw, err := atomicio.ReadAtomic(path)
	if errors.Is(err, os.ErrNotExist) {
		return catchUpCursor{}, false, nil
	}
	if err != nil {
		return catchUpCursor{}, false, err
	}
	var c catchUpCursor
	if err := json.Unmarshal(raw, &c); err != nil {
		return catchUpCursor{}, false, err
	}
	return c, true, nil
}

func writeCursor(path string, c catchUpCursor) error {
	raw, err := json.Marshal(c)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return atomicio.WriteAtomic(path, raw, 0o644)
}

// caughtUpEvent is the grouped event of a catch-up: the ids, the cause and the
// outside actor who made the change.
func (ix *objectIndex) caughtUpEvent(c catchUp) api.ObjectsCaughtUpEventPayload {
	p := api.ObjectsCaughtUpEventPayload{
		At:       time.Now().UTC(),
		Added:    c.Added,
		Modified: c.Modified,
		Removed:  c.Removed,
		Cause:    api.CaughtUpCause{Kind: api.CaughtUpWorkingTree, To: c.To},
		By:       api.Actor{Kind: api.ActorOutside},
	}
	p.EventID = newEventID()
	switch {
	case c.To != "" && c.To != c.From && ix.anyDirty(c.Repo, c.Paths):
		// A file still differing from HEAD is an edit not committed yet: the
		// working tree is the cause, with no sha (below).
	case c.To != "" && c.To != c.From && !isAncestor(c.Repo, c.From, c.To):
		// HEAD moved, but not forward from the HEAD CWM saw last (a rebase, a
		// reset, a force-pull, or a HEAD git no longer has): no commit range
		// is the cause. The manifest diff still names exactly what moved; the
		// actor is the local git user who rewrote it, with no sha.
		p.Cause = api.CaughtUpCause{Kind: api.CaughtUpRewritten, From: c.From, To: c.To}
	case c.To != "" && c.To != c.From:
		// HEAD moved forward and every file that moved is as HEAD has it: the
		// commits between the two HEADs are the cause, the newest one's
		// author the actor.
		p.Cause = api.CaughtUpCause{Kind: api.CaughtUpCommits, From: c.From, To: c.To}
		p.Sha = c.To
		p.By.ID = gitOutput(c.Repo, "log", "-1", "--format=%an", c.To)
	}
	if p.Cause.Kind != api.CaughtUpCommits && c.To != "" {
		p.By.ID = gitOutput(c.Repo, "config", "user.name")
	}
	if p.By.ID == "" {
		p.By.ID = "unknown"
	}
	return p
}

// isAncestor reports whether the commit from is an ancestor of to in the
// repository at top. No from (a repository with no commit before) is the
// start of every history. A from git does not have (gc, shallow clone) or
// any other git error is no ancestor: no commit range can be named.
func isAncestor(top, from, to string) bool {
	if from == "" {
		return true
	}
	if top == "" {
		return false
	}
	_, err := gitcli.Run(filepath.FromSlash(top), "merge-base", "--is-ancestor", from, to)
	return err == nil
}

// gitOutput is a git command's trimmed answer in the repository at top, ""
// on an error or outside git.
func gitOutput(top string, args ...string) string {
	if top == "" {
		return ""
	}
	out, err := gitcli.Run(filepath.FromSlash(top), args...)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

// movedPaths are the files of the ids a catch-up found moved, in both
// manifests (a removed id's old file, an added id's new one), sorted.
func movedPaths(prev, now map[string]cursorFile, c catchUp) []string {
	moved := map[string]bool{}
	for _, ids := range [][]string{c.Added, c.Modified, c.Removed} {
		for _, id := range ids {
			moved[id] = true
		}
	}
	seen := map[string]bool{}
	for _, m := range []map[string]cursorFile{prev, now} {
		for p, f := range m {
			if moved[f.ID] {
				seen[p] = true
			}
		}
	}
	return slices.Sorted(maps.Keys(seen))
}

// dirtyChunk bounds the paths of one git status, under any command-line limit.
const dirtyChunk = 100

// anyDirty reports whether one of the files differs from HEAD in the working
// tree or the index (modified, added, deleted, untracked). Git is asked only
// when something moved, a chunk of paths at a time, and stops at the first.
func (ix *objectIndex) anyDirty(top string, paths []string) bool {
	if top == "" {
		return true // no git: nothing is as a commit has it
	}
	dir := filepath.FromSlash(top)
	// ONE FORM ON BOTH SIDES: git answers its top level in the long form,
	// while a root may be given in another (an 8.3 short name on Windows, a
	// symlinked folder): both are resolved before they are compared, or every
	// path falls outside the repository and reads as dirty.
	base, canonTop := canonicalPath(ix.root), canonicalPath(dir)
	inRepo := make([]string, 0, len(paths))
	for _, p := range paths { // index paths are the project root's; git takes the repository's
		rel, err := filepath.Rel(canonTop, filepath.Join(base, filepath.FromSlash(p)))
		if err == nil && (rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator))) {
			return true // a file outside the repository is no file of its commits
		}
		if err != nil {
			return true
		}
		inRepo = append(inRepo, filepath.ToSlash(rel))
	}
	for start := 0; start < len(inRepo); start += dirtyChunk {
		chunk := inRepo[start:min(start+dirtyChunk, len(inRepo))]
		args := append([]string{"status", "--porcelain", "--untracked-files=all", "--"}, chunk...)
		out, err := gitcli.Run(dir, args...)
		if err != nil || strings.TrimSpace(out) != "" {
			return true // an unreadable status is no proof of a commit
		}
	}
	return false
}

// movedRevisions is each moved id's revision: its file's hash now, or the one
// a removed id's file had.
func movedRevisions(prev, now map[string]cursorFile, c catchUp) map[string]string {
	all := map[string]string{}
	for _, f := range now {
		all[f.ID] = f.Hash
	}
	for _, f := range prev {
		if _, kept := all[f.ID]; !kept {
			all[f.ID] = f.Hash
		}
	}
	moved := map[string]string{}
	for _, ids := range [][]string{c.Added, c.Modified, c.Removed} {
		for _, id := range ids {
			moved[id] = all[id]
		}
	}
	return moved
}

// catchUpHeldFile keeps, beside the cursor, the per-object events of the
// catch-ups above the threshold, by their grouped event's id, until released.
const catchUpHeldFile = "catchup-held.json"

// objectEvents are the per-object events of a catch-up, stamped at its
// grouped event's time: created for an added id, updated for a modified one,
// deleted for a removed one, each of operation caught-up. An id still indexed
// says its module, type and parent; a removed one its module only.
func (ix *objectIndex) objectEvents(c catchUp, at time.Time) []api.ObjectEventPayload {
	ix.mu.RLock()
	defer ix.mu.RUnlock()
	var out []api.ObjectEventPayload
	for _, g := range []struct {
		action string
		ids    []string
	}{{"created", c.Added}, {"updated", c.Modified}, {"deleted", c.Removed}} {
		for _, id := range g.ids {
			p := api.ObjectEventPayload{
				EventMeta: api.EventMeta{EventID: newEventID(), At: at, Revision: c.Revisions[id]},
				ObjectID:  id,
				Action:    g.action,
				Operation: api.OperationCaughtUp,
			}
			if e, ok := ix.byID[id]; ok && g.action != "deleted" {
				p.Module, p.Type, p.Parent, p.Slug = e.Module, e.Object.Type, e.Object.ParentID(), e.Object.Slug
			} else {
				p.Module, _ = api.ModuleOfObjectID(id)
			}
			out = append(out, p)
		}
	}
	return out
}

// newEventID is a UUID v7 (time-ordered), a v4 when none can be minted.
func newEventID() string {
	if u, err := uuid.NewV7(); err == nil {
		return u.String()
	}
	return uuid.NewString()
}

// publishCatchUp publishes what a catch-up found moved: its grouped event,
// then each object's event at the threshold or under it. Above it the
// object events are held - written before the grouped event goes out, so a
// reaction to it can release them at once.
func (s *Service) publishCatchUp(ix *objectIndex, c catchUp) {
	grouped := ix.caughtUpEvent(c)
	per := ix.objectEvents(c, grouped.At)
	grouped.Count = len(per)
	if len(per) > api.CatchUpPerObjectMax {
		grouped.Held = true
		if err := ix.holdEvents(grouped.EventID, per); err != nil {
			slog.Warn("cwm: the catch-up's per-object events could not be held", "event", grouped.EventID, "err", err)
		}
		s.Events.PublishCaughtUp(grouped)
		return
	}
	s.Events.PublishCaughtUp(grouped)
	s.publishObjectEvents(per)
}

// publishObjectEvents publishes caught-up object events, one each.
func (s *Service) publishObjectEvents(per []api.ObjectEventPayload) {
	for _, p := range per {
		s.Events.PublishObject(p.Action, p.ObjectID, p.Module, p.Type, p.Parent, p.Slug, p.Operation, p.EventMeta)
	}
}

// CatchUpRelease publishes the per-object events a catch-up above the
// threshold held, by its grouped event's id, and forgets them: a second
// release of the same id is refused (not found).
func (s *Service) CatchUpRelease(_ stdctx.Context, in api.CatchUpReleaseInput) (api.CatchUpReleaseOutput, error) {
	if in.EventID == "" {
		return api.CatchUpReleaseOutput{}, fmt.Errorf("%w: event_id is required", api.ErrInvalidInput)
	}
	per, err := s.index().takeHeld(in.EventID)
	if err != nil {
		return api.CatchUpReleaseOutput{}, err
	}
	s.publishObjectEvents(per)
	return api.CatchUpReleaseOutput{Published: len(per)}, nil
}

// heldPath is the held file, beside the cursor; "" with no store.
func (ix *objectIndex) heldPath() string {
	if ix.storeRoot == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(ix.storeRoot), catchUpHeldFile)
}

// holdEvents keeps a catch-up's per-object events under its grouped event's
// id. Called from the catch-up hook, under the catch-up's lock.
func (ix *objectIndex) holdEvents(eventID string, per []api.ObjectEventPayload) error {
	path := ix.heldPath()
	if path == "" {
		return errors.New("no store to hold the events in")
	}
	held, err := readHeld(path)
	if err != nil {
		return err
	}
	held[eventID] = per
	return writeHeld(path, held)
}

// takeHeld removes and answers the events held under eventID.
func (ix *objectIndex) takeHeld(eventID string) ([]api.ObjectEventPayload, error) {
	ix.catch.mu.Lock()
	defer ix.catch.mu.Unlock()
	path := ix.heldPath()
	held := map[string][]api.ObjectEventPayload{}
	if path != "" {
		var err error
		if held, err = readHeld(path); err != nil {
			return nil, err
		}
	}
	per, ok := held[eventID]
	if !ok {
		return nil, fmt.Errorf("%w: no catch-up holds events under %q", api.ErrNotFound, eventID)
	}
	delete(held, eventID)
	if len(held) == 0 {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
	} else if err := writeHeld(path, held); err != nil {
		return nil, err
	}
	return per, nil
}

func readHeld(path string) (map[string][]api.ObjectEventPayload, error) {
	held := map[string][]api.ObjectEventPayload{}
	raw, err := atomicio.ReadAtomic(path)
	if errors.Is(err, os.ErrNotExist) {
		return held, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(raw, &held); err != nil {
		return nil, err
	}
	return held, nil
}

func writeHeld(path string, held map[string][]api.ObjectEventPayload) error {
	raw, err := json.Marshal(held)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return atomicio.WriteAtomic(path, raw, 0o644)
}

// noteVerbWrite records a file a verb wrote (f) or removed (nil) for the
// cursor, and schedules its flush. Caller holds ix.mu.
func (ix *objectIndex) noteVerbWrite(path string, f *cursorFile) {
	if ix.storeRoot == "" || path == "" {
		return
	}
	c := &ix.catch
	c.pendMu.Lock()
	defer c.pendMu.Unlock()
	if c.pending == nil {
		c.pending = map[string]*cursorFile{}
	}
	c.pending[path] = f
	if c.flushT == nil {
		c.flushT = time.AfterFunc(cursorFlushDelay, ix.flushPending)
	}
}

// takePending answers the verbs' writes not in the cursor yet and forgets them.
func (ix *objectIndex) takePending() map[string]*cursorFile {
	c := &ix.catch
	c.pendMu.Lock()
	defer c.pendMu.Unlock()
	p := c.pending
	c.pending = nil
	if c.flushT != nil {
		c.flushT.Stop()
		c.flushT = nil
	}
	return p
}

// flushPending writes the verbs' writes into the cursor. With no cursor yet
// they are dropped: the first catch-up takes the whole tree as its baseline.
func (ix *objectIndex) flushPending() {
	path := ix.cursorPath()
	if path == "" {
		return
	}
	ix.catch.mu.Lock()
	defer ix.catch.mu.Unlock()
	pend := ix.takePending()
	if len(pend) == 0 {
		return
	}
	prev, found, err := ix.readRepoCursors(path)
	if err != nil || !found {
		return
	}
	prev = ix.withPending(prev, pend)
	prev.Head, prev.Files = "", nil // written per repository from now on
	if err := atomicio.WriteAtomic(path, mustJSON(prev), 0o644); err != nil {
		slog.Warn("cwm: the catch-up cursor could not take the verbs' writes", "err", err)
	}
}

// mustJSON is a cursor's bytes (a cursor always marshals).
func mustJSON(c catchUpCursor) []byte {
	raw, _ := json.Marshal(c)
	return raw
}
