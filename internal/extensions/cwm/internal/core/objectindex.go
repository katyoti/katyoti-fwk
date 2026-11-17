// SPDX-License-Identifier: Apache-2.0

package core

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"

	"github.com/bmatcuk/doublestar/v4"

	"github.com/katyoti/katyoti-fwk/internal/core/atomicio"
	"github.com/katyoti/katyoti-fwk/internal/core/projectfile"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// The rebuildable index over the object files (storage-redesign.md P2).
//
// Authored objects live in modules/<name>/objects/*.yaml and are the ONLY source
// of truth. This index is a derived read model: it makes tree and list reads pure
// data with no per-object file access (G5), and its content hashes let a scan say
// what changed since last time (G4 — change detection).
//
// It is deliberately IN-MEMORY. Persisting it would add an invalidation problem
// to buy a startup cost we do not have, and "deleting the index loses nothing"
// stays true by construction rather than by discipline.

// indexEntry is one object plus where it came from and what it hashed to.
type indexEntry struct {
	Object api.Object
	Module string
	Path   string // repo-relative, for reporting
	Hash   string // sha256 of the file bytes — the change-detection basis
}

// objectIndex holds the derived read model. The zero value is not usable; call
// newObjectIndex.
type objectIndex struct {
	mu   sync.RWMutex
	root string // the project root: holds project.cwm.yaml and the modules folder
	// refused is the project's refusal (Service.refused): the
	// index never loads a refused project, every read answers it.
	refused error
	// storeRoot is the project's CWM store for runtime objects
	// (`.katyoti/cwm/objects/`) — the SECOND root (concepts.md §5, 2026-09-25).
	// Empty = no store (substrate tests).
	storeRoot string
	byID      map[string]indexEntry
	// byParent is the tree, resident: parent id → child ids, sorted. Rebuilt
	// by scan, kept by put / drop, so childrenOf is a lookup and a whole drawn
	// tree costs one pass — measured 2026-09-25: 367 ms → single-digit ms at
	// 514 objects once this replaced the per-node scan of every entry.
	byParent map[string][]string
	loaded   bool
	// ft is the full-text index over the same objects, bodies included
	// (fulltext.go): rebuilt by scan, kept by put / drop, under mu.
	ft *ftIndex
	// project is the root project object's id when one is filed, "" otherwise.
	project string
	// renumber names, of two objects carrying one id, the one to renumber:
	// "a", "b" or "" when it cannot tell (number_blocks.go). nil: never.
	renumber func(a, b api.Object) string

	// The problems bookkeeping (problems.go): which objects a write touched
	// since the last read of problems, whether everything is stale (a scan),
	// and the files the scan could not file under an id.
	dirty      map[string]bool
	allStale   bool
	unreadable []api.InvalidObject

	// gen counts the changes to the MODULE objects (a scan, a put or a drop
	// of a bare id): what is derived from them (the cross-module
	// subscriptions, module_subscriptions.go) is cached per generation.
	gen uint64

	// hooked is the objects whose own `hooks:` is non-empty: rebuilt by scan,
	// kept by put / drop, so listing every object's own subscriptions
	// (event_subscriptions.go) reads them without walking every entry.
	hooked map[string]bool

	// THE WRITES A SCAN MUST NOT LOSE. A scan reads every file outside the
	// lock and swaps under it, so a put or a drop landing between the read
	// and the swap would be reverted until the next scan. Every put / drop
	// bumps wseq; while a scan runs (scanning > 0) it also records the id
	// in scanWrites with its seq, and the swap keeps the live entry (or the
	// absence) of every id written after the scan began.
	wseq       uint64
	scanning   int
	scanWrites map[string]uint64
	// afterRead, when set, runs between a scan's read and its swap — the
	// test seam that lands a write in that window. Nil in production.
	afterRead func()

	// incoming is "what points at this object": target id → the ids whose
	// authored edges name it, each with how many of its edges do. Built on the
	// first incoming question, kept by put / drop, dropped (nil) when a scan
	// replaces the entries; inMu serialises the build among readers holding
	// mu.RLock (a writer holds mu exclusively). Without it, every incoming
	// lookup walked every entry, and a walk over the corpus was quadratic
	//
	inMu     sync.Mutex
	incoming map[string]map[string]int

	// catch is the catch-up's bookkeeping (catchup.go): what moved outside
	// the verbs since CWM last saw the repository.
	catch catchUpState
}

func newObjectIndex(root, storeRoot string) *objectIndex {
	return &objectIndex{root: root, storeRoot: storeRoot, byID: map[string]indexEntry{}, byParent: map[string][]string{}, ft: newFTIndex(), hooked: map[string]bool{}}
}

// hashBytes is the content-hash basis. Hashing the FILE BYTES (not the parsed
// object) means any authored change — including one that round-trips to the same
// struct, like a reordered key or an edited comment — counts as a modification.
// A verdict's basis changed even if the value did not.
func hashBytes(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// objectsGlob walks every module's object files and returns (module, path)
// pairs. It is the one place that knows the on-disk layout. projectErr is the
// project object's refusal (projectfile.Read: an invalid scan exclusion, …),
// reported on its file; the walk then runs on the defaults.
func (ix *objectIndex) objectsGlob() (_ []struct{ Module, Path string }, projectErr error, _ error) {
	var out []struct{ Module, Path string }

	// THE PROJECT OBJECT FIRST (identity-and-index.md §7): it lives at the
	// project root, beside the modules folder, because it says WHERE the
	// modules are, and what the scan leaves out. Module "" — it belongs to no
	// module.
	modulesDir := api.DefaultModulesDir
	var ex *scanExclusions
	rootFile := filepath.Join(ix.root, api.ProjectFileName)
	if _, err := os.Stat(rootFile); err == nil {
		out = append(out, struct{ Module, Path string }{Module: "", Path: rootFile})
		f, err := projectfile.Read(ix.root)
		if err != nil {
			projectErr = err
		} else {
			modulesDir = f.ModulesDir
			if len(f.ScanExclude) > 0 {
				ex = &scanExclusions{root: ix.root, globs: f.ScanExclude}
			}
		}
	}
	// The authored root: <project>/<modules_dir>/<module>/...
	if err := walkModules(filepath.Join(ix.root, modulesDir), ex, &out); err != nil {
		return nil, nil, fmt.Errorf("cwm/core: read modules dir: %w", err)
	}
	// The runtime root: <store>/<module>/... — what runs produce, same layout,
	// same id space, one index (concepts.md §5). The project's exclusions
	// are authored paths: they never reach the store.
	if ix.storeRoot != "" {
		if err := walkModules(ix.storeRoot, nil, &out); err != nil {
			return nil, nil, fmt.Errorf("cwm/core: read object store: %w", err)
		}
	}
	return out, projectErr, nil
}

// scanExclusions is the project's `scan.exclude`: doublestar globs matched
// against a path relative to the project root, slash-separated. A nil
// *scanExclusions excludes nothing.
type scanExclusions struct {
	root  string
	globs []string
}

// excluded reports whether path (absolute, under root) matches a glob. The
// globs were validated by projectfile.Read, so a match error cannot occur.
func (ex *scanExclusions) excluded(path string) bool {
	if ex == nil {
		return false
	}
	rel, err := filepath.Rel(ex.root, path)
	if err != nil {
		return false
	}
	rel = filepath.ToSlash(rel)
	for _, g := range ex.globs {
		if ok, _ := doublestar.Match(g, rel); ok {
			return true
		}
	}
	return false
}

// walkModules walks one modules root: every subfolder is a module, unless
// the project excludes it.
func walkModules(modulesDir string, ex *scanExclusions, out *[]struct{ Module, Path string }) error {
	entries, err := os.ReadDir(modulesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // no modules yet is not an error — it is an empty index
		}
		return err
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(modulesDir, e.Name())
		if ex.excluded(dir) {
			continue
		}
		if err := walkObjectDir(dir, e.Name(), true, ex, out); err != nil {
			return err
		}
	}
	return nil
}

// scan rebuilds the index from disk and reports the diff against what was there.
// A file that fails to parse is REPORTED, not silently skipped — a corrupt object
// that vanishes from the index would read as "removed", which is a lie.
func (ix *objectIndex) scan() (api.ObjectScanOutput, error) {
	ix.mu.Lock()
	start := ix.wseq
	ix.scanning++
	ix.mu.Unlock()
	swapped := false
	defer func() {
		if !swapped { // the read failed: no swap, still end the scan
			ix.mu.Lock()
			ix.endScanLocked()
			ix.mu.Unlock()
		}
	}()

	found, projectErr, err := ix.objectsGlob()
	if err != nil {
		return api.ObjectScanOutput{}, err
	}

	next := make(map[string]indexEntry, len(found))
	ft := newFTIndex()
	var bad []api.InvalidObject
	for _, f := range found {
		raw, err := atomicio.ReadAtomic(f.Path)
		if err != nil {
			bad = append(bad, api.InvalidObject{Path: ix.rel(f.Path), Problems: []string{api.DiagnosticUnreadable + ": " + err.Error()}})
			continue
		}
		obj, err := ReadObjectFile(f.Path)
		if err != nil {
			bad = append(bad, api.InvalidObject{Path: ix.rel(f.Path), Problems: []string{api.DiagnosticUnreadable + ": " + err.Error()}})
			continue
		}
		// A DUPLICATE ID IS AN ERROR, naming both files.
		//
		// This was `next[obj.ID] = ...`, a straight assignment: two files
		// carrying one id meant the second silently overwrote the first, and
		// WHICH ONE WON depended on directory read order. A flat folder made
		// that catchable because filenames mirror ids; subfolders hide it
		// completely, so the guard has to exist before the folders do.
		if prior, dup := next[obj.ID]; dup {
			problem := fmt.Sprintf("%s: %q is already the id of %s; this file is not loaded", api.DiagnosticDuplicateID, obj.ID, prior.Path)
			// Two minters on one number (a provisional block): the ledger
			// names the one to renumber, by the move verb.
			if ix.renumber != nil {
				switch ix.renumber(prior.Object, obj) {
				case "a":
					problem += fmt.Sprintf("; renumber %s: its minter does not own the number block holding the number", prior.Path)
				case "b":
					problem += fmt.Sprintf("; renumber %s: its minter does not own the number block holding the number", ix.rel(f.Path))
				}
			}
			bad = append(bad, api.InvalidObject{ID: obj.ID, Path: ix.rel(f.Path), Problems: []string{problem}})
			continue
		}
		next[obj.ID] = indexEntry{
			Object: obj,
			Module: f.Module,
			Path:   ix.rel(f.Path),
			Hash:   hashBytes(raw),
		}
	}
	// TWO PASSES: a bare edge target is resolved against every id of the
	// project (qualifyTarget), so the edges are read once all ids are known.
	exists := func(id string) bool { _, ok := next[id]; return ok }
	for id, e := range next {
		full := qualifyEdges(e.Object, exists) // a bare `to: g1` reads as module.g1, or as a module (7e)
		ft.add(full)
		e.Object = resident(full)
		next[id] = e
	}
	// THE PROJECT OBJECT'S REFUSAL (an invalid scan exclusion, …) is listed on
	// its file, and the scan ran on the defaults.
	if projectErr != nil {
		problem := api.DiagnosticUnreadable + ": " + projectErr.Error()
		i := slices.IndexFunc(bad, func(b api.InvalidObject) bool { return b.Path == api.ProjectFileName })
		if i < 0 {
			bad = append(bad, api.InvalidObject{Path: api.ProjectFileName, Problems: []string{problem}})
		} else {
			bad[i].Problems = append(bad[i].Problems, problem)
		}
	}
	// READ REPORTS. A file that does not parse, or a second file for one id,
	// is listed (unreadableFiles, the global Errors list) and the rest of the
	// project loads: one bad file must never make a project unopenable.
	sort.Slice(bad, func(i, j int) bool { return bad[i].Path < bad[j].Path })

	if ix.afterRead != nil {
		ix.afterRead()
	}

	ix.mu.Lock()
	defer ix.mu.Unlock()
	swapped = true
	// A WRITE MADE WHILE THE SCAN READ WINS: what the scan read for that id
	// may predate the write, the live entry does not.
	for id, seq := range ix.scanWrites {
		if seq <= start {
			continue
		}
		if live, ok := ix.byID[id]; ok {
			next[id] = live
		} else {
			delete(next, id)
		}
		ft.adopt(ix.ft, id)
	}
	ix.endScanLocked()

	var out api.ObjectScanOutput
	var moved []string // same id and bytes, another file or module
	for id, e := range next {
		prev, existed := ix.byID[id]
		switch {
		case !existed:
			out.Added = append(out.Added, id)
		case prev.Hash != e.Hash:
			out.Modified = append(out.Modified, id)
		case prev.Path != e.Path || prev.Module != e.Module:
			moved = append(moved, id)
		}
	}
	for id := range ix.byID {
		if _, still := next[id]; !still {
			out.Removed = append(out.Removed, id)
		}
	}
	sort.Strings(out.Added)
	sort.Strings(out.Modified)
	sort.Strings(out.Removed)

	// AN EMPTY DIFF RECOMPUTES NOTHING. The problems are a function of the
	// files and the declarations (a declaration change marks everything
	// itself, markAllStale): when no id came, went or changed bytes, the
	// marks already pending stand and only a moved file is re-checked.
	changed := !ix.loaded || len(out.Added)+len(out.Modified)+len(out.Removed) > 0
	ix.byID = next
	ix.incoming = nil
	ix.ft = ft
	ix.unreadable = bad
	if changed {
		ix.allStale = true
		ix.dirty = nil
	} else {
		for _, id := range moved {
			ix.markLocked(id)
		}
	}
	ix.byParent = map[string][]string{}
	for id, e := range next {
		p := parentOf(e.Object)
		ix.byParent[p] = append(ix.byParent[p], id)
	}
	for p := range ix.byParent {
		sort.Strings(ix.byParent[p])
	}
	ix.hooked = map[string]bool{}
	for id, e := range next {
		if len(e.Object.Hooks) > 0 {
			ix.hooked[id] = true
		}
	}
	ix.loaded = true
	if changed {
		ix.gen++
	}
	// The project is the object filed at the project root (its level, not
	// its type's name: a software project, a dossier).
	ix.project = ""
	for id, e := range next {
		if e.Module == "" && e.Path == api.ProjectFileName {
			ix.project = id
		}
	}
	out.Total = len(next)
	return out, nil
}

// ensureLoaded scans once, lazily. Reads go through it so a caller never sees an
// empty index just because nothing scanned yet.
func (ix *objectIndex) ensureLoaded() error {
	if ix.refused != nil {
		return ix.refused
	}
	ix.mu.RLock()
	done := ix.loaded
	ix.mu.RUnlock()
	if done {
		return nil
	}
	if _, err := ix.scan(); err != nil {
		return err
	}
	// THE CATCH-UP AT OPEN: what moved outside the verbs since CWM last saw
	// the repository. A catch-up that fails costs the announcement, never the
	// read: the index is loaded.
	_, _ = ix.catchUp()
	return nil
}

// put records a written object — the write-through that makes create-then-read
// work without a rescan. Without it a caller would write a file and then fail to
// find it, because reads answer from the index.
func (ix *objectIndex) put(module, path string, obj api.Object) {
	raw, err := atomicio.ReadAtomic(path)
	hash := ""
	if err == nil {
		hash = hashBytes(raw)
	}
	ix.mu.Lock()
	defer ix.mu.Unlock()
	ix.noteWriteLocked(obj.ID)
	prior, existed := ix.byID[obj.ID]
	if existed {
		ix.unlink(parentOf(prior.Object), obj.ID)
		ix.unlinkIncomingLocked(prior.Object)
	} else {
		ix.markReferrersLocked(obj.ID) // an id appeared: links to it now resolve
	}
	ix.markLocked(obj.ID)
	if isModuleObjectID(obj.ID) {
		ix.gen++
	}
	rel := ix.rel(path)
	if existed && prior.Path != rel {
		ix.noteVerbWrite(prior.Path, nil) // the object's file moved
	}
	if hash != "" {
		ix.noteVerbWrite(rel, &cursorFile{ID: obj.ID, Hash: hash})
	}
	ix.byID[obj.ID] = indexEntry{Object: resident(obj), Module: module, Path: rel, Hash: hash}
	ix.linkIncomingLocked(obj)
	if len(obj.Hooks) > 0 {
		ix.hooked[obj.ID] = true
	} else {
		delete(ix.hooked, obj.ID)
	}
	ix.ft.add(obj)
	p := parentOf(obj)
	ix.byParent[p] = append(ix.byParent[p], obj.ID)
	sort.Strings(ix.byParent[p])
}

// noteWriteLocked records a put or a drop of id for the scans running now.
// Caller holds mu.
func (ix *objectIndex) noteWriteLocked(id string) {
	ix.wseq++
	if ix.scanning == 0 {
		return
	}
	if ix.scanWrites == nil {
		ix.scanWrites = map[string]uint64{}
	}
	ix.scanWrites[id] = ix.wseq
}

// endScanLocked ends one scan; the last one out forgets the writes. Caller
// holds mu.
func (ix *objectIndex) endScanLocked() {
	ix.scanning--
	if ix.scanning == 0 {
		ix.scanWrites = nil
	}
}

// isModuleObjectID reports a module object's id: bare, no module prefix.
func isModuleObjectID(id string) bool { return id != "" && !strings.Contains(id, ".") }

// generation is the module objects' change counter (gen).
func (ix *objectIndex) generation() uint64 {
	ix.mu.RLock()
	defer ix.mu.RUnlock()
	return ix.gen
}

// moduleObjects is every indexed MODULE object — a bare id filed in a module
// — with its module, under one read lock.
func (ix *objectIndex) moduleObjects() (gen uint64, objs []indexEntry) {
	ix.mu.RLock()
	defer ix.mu.RUnlock()
	for id, e := range ix.byID {
		if e.Module != "" && isModuleObjectID(id) {
			objs = append(objs, e)
		}
	}
	return ix.gen, objs
}

// moduleOf returns the module an indexed object belongs to.
//
// THE INDEX KNOWS, so nothing re-derives it from the id. A module object's own
// id is BARE (`cwm`, not `cwm.cwm`), so splitting its id on the dot yields no
// module and would deny a module ownership of its own folder — which is where
// this was caught. The scan recorded the folder each object came from; that is
// the answer.
func (ix *objectIndex) moduleOf(id string) (string, bool) {
	ix.mu.RLock()
	defer ix.mu.RUnlock()
	e, ok := ix.byID[id]
	if !ok {
		return "", false
	}
	return e.Module, true
}

// get returns one indexed object by id.
func (ix *objectIndex) get(id string) (api.Object, bool) {
	ix.mu.RLock()
	defer ix.mu.RUnlock()
	e, ok := ix.byID[id]
	return e.Object, ok
}

// pathOf returns the ABSOLUTE path of an indexed object's file. The index
// stores it repo-relative for reporting, so it is rejoined to the root here.
func (ix *objectIndex) pathOf(id string) (string, bool) {
	ix.mu.RLock()
	defer ix.mu.RUnlock()
	e, ok := ix.byID[id]
	if !ok || e.Path == "" {
		return "", false
	}
	return filepath.Join(ix.root, filepath.FromSlash(e.Path)), true
}

// drop removes an object from the index after its file is deleted.
func (ix *objectIndex) drop(id string) {
	ix.mu.Lock()
	defer ix.mu.Unlock()
	ix.noteWriteLocked(id)
	if prior, ok := ix.byID[id]; ok {
		ix.unlink(parentOf(prior.Object), id)
		ix.unlinkIncomingLocked(prior.Object)
		ix.noteVerbWrite(prior.Path, nil)
	}
	ix.markReferrersLocked(id) // an id went: links to it now dangle
	ix.markLocked(id)
	if isModuleObjectID(id) {
		ix.gen++
	}
	delete(ix.byID, id)
	delete(ix.hooked, id)
	ix.ft.remove(id)
}

// objectsWithHooks is every indexed object whose own `hooks:` is non-empty,
// by id, under one read lock.
func (ix *objectIndex) objectsWithHooks() []api.Object {
	ix.mu.RLock()
	defer ix.mu.RUnlock()
	ids := make([]string, 0, len(ix.hooked))
	for id := range ix.hooked {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]api.Object, 0, len(ids))
	for _, id := range ids {
		if e, ok := ix.byID[id]; ok {
			out = append(out, e.Object)
		}
	}
	return out
}

// fulltext ranks every indexed object against query (fulltext.go). It calls
// nothing back under its lock: a caller filters the hits after, through the
// index's own accessors — re-entering the read lock here deadlocks as soon as
// a writer waits (a pending Lock blocks new RLocks).
func (ix *objectIndex) fulltext(query string) []api.ObjectSearchHit {
	ix.mu.RLock()
	defer ix.mu.RUnlock()
	return ix.ft.search(query, func(string) bool { return true })
}

// subtree returns id and every object under it in the tree.
func (ix *objectIndex) subtree(id string) map[string]bool {
	ix.mu.RLock()
	defer ix.mu.RUnlock()
	out := map[string]bool{}
	if _, ok := ix.byID[id]; !ok {
		return out
	}
	stack := []string{id}
	for len(stack) > 0 {
		cur := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if out[cur] {
			continue
		}
		out[cur] = true
		stack = append(stack, ix.byParent[cur]...)
	}
	return out
}

// unlink removes id from its parent's child list. Caller holds the lock.
func (ix *objectIndex) unlink(parent, id string) {
	kids := ix.byParent[parent]
	for i, k := range kids {
		if k == id {
			ix.byParent[parent] = append(kids[:i:i], kids[i+1:]...)
			return
		}
	}
}

// parentOf reads the containment field. The tree is a FIELD (parent);
// every other reference is an edge (concepts.md § the reference rule).
//
// Delegates to the typed accessor so the index and any consumer read the tree
// backbone through ONE implementation — TestParentIDMatchesContainer pins that
// they agree.
func parentOf(obj api.Object) string { return obj.ParentID() }

// projectID returns the root project object's id, "" when none is filed.
func (ix *objectIndex) projectID() string {
	ix.mu.RLock()
	defer ix.mu.RUnlock()
	return ix.project
}

// edges returns the object-authored edges the index holds, filtered like
// EdgeList filters the graph: incident to `id` (empty = every edge), of
// `kind` (empty = any), in `dir` ("" both, "out", "in").
//
// THE INDEX IS THE EDGE STORE (phase 7). An edge is authored on its SOURCE
// object (`edges: [{kind, to}]`, concepts.md §3) and lives in that one file;
// "what points at this object" — the incoming direction — is therefore a
// question only the index can answer, which is why EdgeList reads it here and
// not from the files. Incident to an id, only that object and the objects
// pointing at it are read (incoming); every edge is a walk of every entry.
func (ix *objectIndex) edges(id string, kind api.EdgeType, dir api.Direction) []api.Edge {
	ix.mu.RLock()
	defer ix.mu.RUnlock()
	var out []api.Edge
	// The authored edges are read in place - no copy of each object, no edge
	// built unless it matches (: a view asks this many times).
	entries := func(yield func(indexEntry) bool) {
		if id == "" {
			for _, e := range ix.byID {
				if !yield(e) {
					return
				}
			}
			return
		}
		if dir != api.DirectionIn {
			if e, ok := ix.byID[id]; ok && !yield(e) {
				return
			}
		}
		if dir != api.DirectionOut {
			for _, from := range ix.referrers(id) {
				if from == id && dir != api.DirectionIn {
					continue // read above, as the source
				}
				if e, ok := ix.byID[from]; ok && !yield(e) {
					return
				}
			}
		}
	}
	for e := range entries {
		from := e.Object.ID
		for i := range e.Object.Edges {
			edge := &e.Object.Edges[i]
			if kind != "" && api.EdgeType(edge.Kind) != kind {
				continue
			}
			if id != "" {
				outgoing := from == id && dir != api.DirectionIn
				incoming := edge.To == id && dir != api.DirectionOut
				if !outgoing && !incoming {
					continue
				}
			}
			out = append(out, api.Edge{From: from, To: edge.To, Kind: api.EdgeType(edge.Kind)})
		}
	}
	return out
}

// list applies the filters and returns objects in stable id order.
func (ix *objectIndex) list(in api.ObjectListInput) []api.Object {
	ix.mu.RLock()
	defer ix.mu.RUnlock()

	var out []api.Object
	for _, e := range ix.byID {
		if in.Module != "" && e.Module != in.Module {
			continue
		}
		if in.Type != "" && e.Object.Type != in.Type {
			continue
		}
		if in.Parent != "" && parentOf(e.Object) != in.Parent {
			continue
		}
		out = append(out, e.Object)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// childrenOf returns the direct children of id, in stable order.
func (ix *objectIndex) childrenOf(id string) []api.Object {
	kids := ix.byParent[id]
	out := make([]api.Object, 0, len(kids))
	for _, k := range kids { // already sorted by id
		if e, ok := ix.byID[k]; ok {
			out = append(out, e.Object)
		}
	}
	return out
}

// tree builds the containment subtree. depth <= 0 means unlimited.
//
// A cycle in parent would otherwise recurse forever, so the walk
// carries the ids on the current path and stops rather than hanging. Authored
// ids make a cycle a real possibility, not a theoretical one.
func (ix *objectIndex) tree(in api.ObjectTreeInput) (api.ObjectTreeOutput, error) {
	ix.mu.RLock()
	defer ix.mu.RUnlock()

	var total int
	var build func(id string, depth int, onPath map[string]bool) []api.ObjectTreeNode
	build = func(id string, depth int, onPath map[string]bool) []api.ObjectTreeNode {
		if in.Depth > 0 && depth >= in.Depth {
			return nil
		}
		var nodes []api.ObjectTreeNode
		for _, child := range ix.childrenOf(id) {
			if onPath[child.ID] {
				continue // cycle — do not descend through it again
			}
			onPath[child.ID] = true
			total++
			nodes = append(nodes, api.ObjectTreeNode{
				Object:   child,
				Children: build(child.ID, depth+1, onPath),
			})
			delete(onPath, child.ID)
		}
		return nodes
	}

	var out api.ObjectTreeOutput
	if in.Root != "" {
		e, ok := ix.byID[in.Root]
		if !ok {
			return api.ObjectTreeOutput{}, api.ObjectNotFound(in.Root)
		}
		total++
		out.Roots = []api.ObjectTreeNode{{
			Object:   e.Object,
			Children: build(in.Root, 0, map[string]bool{in.Root: true}),
		}}
	} else {
		// Every object whose declared parent is absent from the index is a root.
		// Using "parent not found" rather than "parent empty" means an object
		// pointing at a deleted parent still surfaces instead of vanishing.
		var roots []api.Object
		for _, e := range ix.byID {
			if e.Object.ID == ix.project && ix.project != "" {
				continue // the project is the root itself, below
			}
			p := parentOf(e.Object)
			if p == "" {
				roots = append(roots, e.Object)
				continue
			}
			if _, ok := ix.byID[p]; !ok {
				roots = append(roots, e.Object)
			}
		}
		sort.Slice(roots, func(i, j int) bool { return roots[i].ID < roots[j].ID })
		// THE PROJECT IS THE ROOT (identity-and-index.md §7): when a project
		// object is filed, everything parentless hangs under it — modules by
		// doctrine (2026-09-21: the project holds every parentless item), and
		// what runs produce. The project level is derived, never authored. An
		// object whose `parent:` names the project (a study, whose type
		// accepts only the dossier d'architecture as its parent) hangs there
		// too, among the parentless ones by id.
		if pe, ok := ix.byID[ix.project]; ok && ix.project != "" {
			total++
			node := api.ObjectTreeNode{Object: pe.Object}
			for _, r := range roots {
				total++
				node.Children = append(node.Children, api.ObjectTreeNode{
					Object:   r,
					Children: build(r.ID, 1, map[string]bool{r.ID: true, ix.project: true}),
				})
			}
			node.Children = append(node.Children, build(ix.project, 0, map[string]bool{ix.project: true})...)
			sort.SliceStable(node.Children, func(i, j int) bool { return node.Children[i].Object.ID < node.Children[j].Object.ID })
			out.Roots = []api.ObjectTreeNode{node}
			out.Total = total
			return out, nil
		}
		for _, r := range roots {
			total++
			out.Roots = append(out.Roots, api.ObjectTreeNode{
				Object:   r,
				Children: build(r.ID, 0, map[string]bool{r.ID: true}),
			})
		}
	}
	out.Total = total
	return out, nil
}

func (ix *objectIndex) rel(p string) string {
	if r, err := filepath.Rel(ix.root, p); err == nil {
		return filepath.ToSlash(r)
	}
	return p
}

// walkObjectDir collects every `*.cwm.yaml` under a module, RECURSING so objects
// can be grouped wherever they read best.
//
// The SUFFIX is what makes an object an object — a format marker, like `.go`,
// carrying neither identity nor type. That is what frees an object from living
// in `objects/`: a doc-behaviour object can sit beside the documents it governs.
//
// Until 2026-09-20 this skipped directories outright (`if f.IsDir() { continue }`),
// which did not error — it made any object filed in a subfolder DISAPPEAR. A
// module with 47 objects in one flat folder has no way to organise them, and the
// obvious attempt silently lost work.
//
// THE PATH IS NEVER SEMANTIC. Identity lives in the file's CONTENT, so a folder
// is filing and nothing reads it. Grouping by whatever helps a reader (goals/,
// phases/) is fine; mirroring the tree is not, because re-parenting an object
// would then require moving the file and the path would become load-bearing —
// re-introducing the path-as-identity this model rejects.
//
// Entries are SORTED at every level. Directory order is not guaranteed, and an
// index whose order varies between runs moves hashes for no reason — which has
// already cost this refactor two false census diffs.
//
// HIDDEN FOLDERS are never scanned either — except the module's planning space
// (`.ktw`, api.PlanningFolder), and only directly under the module (`moduleRoot`).
//
// THE PROJECT'S EXCLUSIONS (`scan.exclude`, ex) come on top: a folder that
// matches is PRUNED — never read, so nothing under it is walked — and a file
// that matches is skipped.
func walkObjectDir(dir, module string, moduleRoot bool, ex *scanExclusions, out *[]struct{ Module, Path string }) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil // an unreadable subfolder contributes nothing, like an absent one
	}
	names := make([]string, 0, len(entries))
	dirs := make([]string, 0, len(entries))
	for _, e := range entries {
		// `_`-prefixed is NEVER scanned, at file or folder level. One convention
		// covers `_generated/` (machine-written artifacts are not objects),
		// `_archive/` and `_backup/` without needing a list of reserved names.
		if strings.HasPrefix(e.Name(), "_") {
			continue
		}
		if e.IsDir() && strings.HasPrefix(e.Name(), ".") && !(moduleRoot && e.Name() == api.PlanningFolder) {
			continue
		}
		if ex.excluded(filepath.Join(dir, e.Name())) {
			continue
		}
		if e.IsDir() {
			dirs = append(dirs, e.Name())
			continue
		}
		if api.IsObjectFile(e.Name()) { // .cwm.yaml or .cwm.md (7c)
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	sort.Strings(dirs)
	for _, n := range names {
		*out = append(*out, struct{ Module, Path string }{module, filepath.Join(dir, n)})
	}
	for _, d := range dirs {
		if err := walkObjectDir(filepath.Join(dir, d), module, false, ex, out); err != nil {
			return err
		}
	}
	return nil
}

// resident is the copy of an object the index keeps: everything but the BODY
// and the type's FIELDS (identity-and-index.md §4 — metadata and capabilities
// resident, content on demand). ObjectGet hydrates from the file; every other read answers
// from this. The hash is of the FILE, so change detection is untouched.
func resident(obj api.Object) api.Object {
	obj.Body, obj.Fields = "", nil
	return obj
}

// markLocked marks one object's problems for recomputation. Caller holds mu.
func (ix *objectIndex) markLocked(id string) {
	if ix.dirty == nil {
		ix.dirty = map[string]bool{}
	}
	ix.dirty[id] = true
}

// markReferrersLocked marks the objects pointing at id — its children and the
// sources of edges to it — whose links change when id appears or goes. Caller
// holds mu.
func (ix *objectIndex) markReferrersLocked(id string) {
	for _, child := range ix.byParent[id] {
		ix.markLocked(child)
	}
	for _, src := range ix.referrers(id) {
		ix.markLocked(src)
	}
}

// referrers is the ids whose authored edges name id, built once from every
// entry and kept after (incoming). Caller holds mu, read or write.
func (ix *objectIndex) referrers(id string) []string {
	ix.inMu.Lock()
	defer ix.inMu.Unlock()
	if ix.incoming == nil {
		in := map[string]map[string]int{}
		for src, e := range ix.byID {
			for _, edge := range e.Object.Edges {
				if in[edge.To] == nil {
					in[edge.To] = map[string]int{}
				}
				in[edge.To][src]++
			}
		}
		ix.incoming = in
	}
	out := make([]string, 0, len(ix.incoming[id]))
	for src := range ix.incoming[id] {
		out = append(out, src)
	}
	return out
}

// linkIncomingLocked counts obj's authored edges in incoming, once built.
// Caller holds mu for writing.
func (ix *objectIndex) linkIncomingLocked(obj api.Object) {
	if ix.incoming == nil {
		return
	}
	for _, edge := range obj.Edges {
		if ix.incoming[edge.To] == nil {
			ix.incoming[edge.To] = map[string]int{}
		}
		ix.incoming[edge.To][obj.ID]++
	}
}

// unlinkIncomingLocked takes obj's authored edges out of incoming, once
// built. Caller holds mu for writing.
func (ix *objectIndex) unlinkIncomingLocked(obj api.Object) {
	if ix.incoming == nil {
		return
	}
	for _, edge := range obj.Edges {
		srcs := ix.incoming[edge.To]
		if srcs == nil {
			continue
		}
		if srcs[obj.ID]--; srcs[obj.ID] <= 0 {
			delete(srcs, obj.ID)
		}
		if len(srcs) == 0 {
			delete(ix.incoming, edge.To)
		}
	}
}

// markAllStale makes the next read of problems recompute every object (new
// declarations can clear or reveal problems anywhere).
func (ix *objectIndex) markAllStale() {
	ix.mu.Lock()
	ix.allStale = true
	ix.mu.Unlock()
}

// takeDirty hands the pending marks to the problem store and clears them.
func (ix *objectIndex) takeDirty() (all bool, ids []string) {
	ix.mu.Lock()
	defer ix.mu.Unlock()
	all = ix.allStale
	for id := range ix.dirty {
		ids = append(ids, id)
	}
	ix.allStale, ix.dirty = false, nil
	return all, ids
}

// unreadableFiles lists the files the last scan could not file under an id.
func (ix *objectIndex) unreadableFiles() []api.InvalidObject {
	ix.mu.RLock()
	defer ix.mu.RUnlock()
	return append([]api.InvalidObject(nil), ix.unreadable...)
}
