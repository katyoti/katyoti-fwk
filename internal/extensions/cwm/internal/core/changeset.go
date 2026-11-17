// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/atomicio"
	"github.com/katyoti/katyoti-fwk/internal/core/cause"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/internal/storage"
)

// A REACTION'S CHANGE SET (the person, 2026-10-07). A
// subscription that declares `atomic: true` has its runner open a change set
// keyed by the reaction's id (ChangeSetBegin) before the action and commit it
// (ChangeSetCommit) after the action and before the reaction reports success.
// While it is open, a write whose cause is that reaction is STAGED instead of
// written: ObjectCreate, ObjectUpdate (and what goes through it —
// transitions, capabilities, edges) and ObjectDelete without detach. Each is
// checked as a write is (validation, size budget, lifecycle, before_* hooks —
// a refusal answers at once) and kept per OBJECT ID with the revision of the
// object at its first touch ("" for a create) and the object staged; the
// revision it answers is the hash of the staged bytes, so a later update in
// the set that passes it compares against it. Refused inside a set: moves,
// splits, renames, named files, attached files and a delete with detach.
//
// The reaction reads its own writes back — ObjectGet, the read-modify-write
// verbs (fullObjectRev), ObjectUpdate's prior and existence checks, the
// existence of an edge's target and the already-exists check of a create.
// Nothing else is overlaid: list, tree, search, children, edges and inbound
// links, effective capabilities and surface, problems, files, and the
// environments of hook `when` and gates answer from what is written.
//
// THE COMMIT re-runs the checks against the current prior (no lock held),
// takes the objects' locks and the create:<module> locks in sorted order,
// verifies each first-touch revision against the current file (a create:
// still absent) — any mismatch releases them all, discards the set and is
// ErrConflict, nothing written — then writes the redo log (commit.json: the
// final bytes of each object and the deletes, fsynced), applies it, updates
// the index, releases the locks, and only then publishes each write's event
// and fires its on_* hooks under the reaction's cause (staged in its outbox,
// as every reaction's write is). A set folder holding commit.json at the next
// start is a commit that did not finish: it is applied again (idempotent),
// re-indexed and its events sent. ChangeSetDiscard drops a set: nothing is
// written, no event goes out.
//
// A CALLER'S CHANGE SET. Any in-process caller opens a set
// under an id it chooses (ChangeSetBegin{Set}) and marks the writes that join
// it with api.WithChangeSet(ctx, id) — the marker wins over a reaction cause.
// It stages and commits as a reaction's set does, keyed set:<id>; its commit
// runs under the cause its first write carried, so a person's or an agent's
// on_* hooks and async matches go out once at the commit, never held in an
// outbox (only a reaction's cause stages them there). The marker is a context
// value: a remote caller's writes do not carry it.
//
// The state is CWM's, uncommitted: `<.katyoti>/cwm/changesets/<hash of the
// set's key>/staged.json`, in memory, loaded at first use. The ids a set's
// creates minted are reserved for its life (locks.reserved), again after a
// restart.

const (
	changeSetFile = "staged.json"
	commitLogFile = "commit.json"
)

// stagedWrite is one object a change set holds.
type stagedWrite struct {
	ID     string `json:"id"`
	Module string `json:"module"`
	Op     string `json:"op"`
	// Base is the object's revision at its first touch; "" for a create (the
	// object was absent).
	Base string `json:"base,omitempty"`
	// Object is the object as staged; nil for a delete.
	Object *api.Object `json:"object,omitempty"`
	// Revision is the hash of the staged bytes; "" for a delete.
	Revision string `json:"revision,omitempty"`
	// Slug is a minted create's file slug.
	Slug string `json:"slug,omitempty"`
	// Sanctioned: a staged write changed a lifecycle state through its verb (a
	// transition, a capability removed) — the commit does not refuse it.
	Sanctioned bool `json:"sanctioned,omitempty"`
	// Operations is the operation marker of each staged write ("" for a plain
	// one): the committed write carries the marker when they all agree.
	Operations []string `json:"operations,omitempty"`
	// Transitions are the moves staged on the object, published at the commit.
	Transitions []stagedTransition `json:"transitions,omitempty"`
}

// stagedTransition is one move of a staged object: its event is published and
// its on_transitioned point fired at the commit.
type stagedTransition struct {
	Payload api.ObjectTransitionedEventPayload `json:"payload"`
	Prior   api.Object                         `json:"prior"`
	Next    api.Object                         `json:"next"`
}

func (w stagedWrite) summary() api.ChangeSetWrite {
	return api.ChangeSetWrite{ID: w.ID, Module: w.Module, Op: w.Op, Revision: w.Revision}
}

// operation is the marker the committed write carries: the one every staged
// write shared, else none.
func (w stagedWrite) operation() string {
	if len(w.Operations) == 0 {
		return ""
	}
	for _, o := range w.Operations[1:] {
		if o != w.Operations[0] {
			return ""
		}
	}
	return w.Operations[0]
}

func (w stagedWrite) clone() stagedWrite {
	cp := w
	if w.Object != nil {
		o := *w.Object
		o.Capabilities = cloneCaps(w.Object.Capabilities)
		o.Edges = append([]api.ObjectEdge(nil), w.Object.Edges...)
		cp.Object = &o
	}
	cp.Operations = append([]string(nil), w.Operations...)
	cp.Transitions = append([]stagedTransition(nil), w.Transitions...)
	return cp
}

// changeSet is one set's staged writes: a reaction's (Reaction) or a
// caller's (Set).
type changeSet struct {
	Reaction string                  `json:"reaction,omitempty"`
	Set      string                  `json:"set,omitempty"`
	At       time.Time               `json:"at"`
	Cause    *cause.Cause            `json:"cause,omitempty"`
	Writes   map[string]*stagedWrite `json:"writes"`
	// committing: the commit took it; no write stages into it any more.
	committing bool
}

// key is the set's key in the store and on disk: a reaction's id, or
// set:<id> for a caller's set.
func (c *changeSet) key() string { return changeSetKeyOf(c.Reaction, c.Set) }

// name is how a message names the set.
func (c *changeSet) name() string { return changeSetName(c.Reaction, c.Set) }

func changeSetKeyOf(reaction, set string) string {
	if set != "" {
		return "set:" + set
	}
	return reaction
}

func changeSetName(reaction, set string) string {
	if set != "" {
		return "set " + set
	}
	return "reaction " + reaction
}

// changeSetKey is the key a verb's input names: exactly one of a reaction and
// a caller's set id.
func changeSetKey(verb, reaction, set string) (string, error) {
	switch {
	case reaction != "" && set != "":
		return "", fmt.Errorf("%w: %s: a reaction or a set, not both", api.ErrInvalidInput, verb)
	case reaction == "" && set == "":
		return "", fmt.Errorf("%w: %s: reaction or set required", api.ErrInvalidInput, verb)
	}
	return changeSetKeyOf(reaction, set), nil
}

func (c *changeSet) sorted() []stagedWrite {
	out := make([]stagedWrite, 0, len(c.Writes))
	for _, w := range c.Writes {
		out = append(out, w.clone())
	}
	sort.Slice(out, func(a, b int) bool { return out[a].ID < out[b].ID })
	return out
}

func summaries(ws []stagedWrite) []api.ChangeSetWrite {
	out := make([]api.ChangeSetWrite, 0, len(ws))
	for _, w := range ws {
		out = append(out, w.summary())
	}
	return out
}

// commitLog is the redo log of a commit: what applying it writes, and what its
// events say. Written whole before anything is applied.
type commitLog struct {
	Reaction string         `json:"reaction,omitempty"`
	Set      string         `json:"set,omitempty"`
	Cause    *cause.Cause   `json:"cause,omitempty"`
	Applies  []appliedWrite `json:"applies"`
}

func (l commitLog) key() string { return changeSetKeyOf(l.Reaction, l.Set) }

// appliedWrite is one object a commit writes or removes.
type appliedWrite struct {
	ID       string `json:"id"`
	Module   string `json:"module"`
	Op       string `json:"op"`
	Path     string `json:"path"`
	Data     []byte `json:"data,omitempty"`
	Revision string `json:"revision"`
	// Object is what a create or an update writes; Prior the object it
	// replaces or removes.
	Object      *api.Object        `json:"object,omitempty"`
	Prior       *api.Object        `json:"prior,omitempty"`
	Operation   string             `json:"operation,omitempty"`
	Owned       map[string]string  `json:"owned,omitempty"`
	Transitions []stagedTransition `json:"transitions,omitempty"`
}

// changeSetStore is every open change set, on disk and in memory. dir "" =
// memory only.
type changeSetStore struct {
	dir  string
	mu   sync.Mutex
	sets map[string]*changeSet
}

// changeSets is the service's change-set store, loaded at first use; a commit
// a previous process logged and did not finish is finished there.
func (s *Service) changeSets() *changeSetStore {
	s.changeSetOnce.Do(func() {
		c := &changeSetStore{sets: map[string]*changeSet{}}
		var logs []commitLog
		if s.Config != nil && s.Config.WorkspaceRoot != "" {
			c.dir = filepath.Join(storage.NewPaths(s.Config.WorkspaceRoot).CWMDataDir(), "changesets")
			logs = c.load()
		}
		for _, set := range c.sets {
			for _, w := range set.Writes {
				if w.Op == api.ChangeSetCreate {
					s.locks.reserved.Store(w.ID, true) // its id stays the set's
				}
			}
		}
		s.changeSetStore = c
		// Never through changeSets() from here: this runs inside its Once.
		for _, l := range logs {
			s.finishCommit(c, l)
		}
	})
	return s.changeSetStore
}

func (c *changeSetStore) folder(reaction string) string {
	return filepath.Join(c.dir, stateFileName(reaction))
}

// load reads the open sets and returns the commits logged and not finished.
func (c *changeSetStore) load() []commitLog {
	ents, err := os.ReadDir(c.dir)
	if err != nil {
		return nil
	}
	var logs []commitLog
	for _, de := range ents {
		if !de.IsDir() {
			continue
		}
		dir := filepath.Join(c.dir, de.Name())
		if raw, err := atomicio.ReadAtomic(filepath.Join(dir, commitLogFile)); err == nil {
			var l commitLog
			if err := json.Unmarshal(raw, &l); err != nil || l.key() == "" {
				slog.Warn("cwm: unreadable change-set commit log", "dir", dir, "err", err)
				continue
			}
			logs = append(logs, l)
			continue
		}
		p := filepath.Join(dir, changeSetFile)
		raw, err := atomicio.ReadAtomic(p)
		var set changeSet
		if err == nil {
			err = json.Unmarshal(raw, &set)
		}
		if err != nil || set.key() == "" {
			slog.Warn("cwm: unreadable change set", "path", p, "err", err)
			continue
		}
		if set.Writes == nil {
			set.Writes = map[string]*stagedWrite{}
		}
		c.sets[set.key()] = &set
	}
	sort.Slice(logs, func(a, b int) bool { return logs[a].key() < logs[b].key() })
	return logs
}

// persist writes set to disk. Caller holds c.mu.
func (c *changeSetStore) persist(set *changeSet) {
	if c.dir == "" {
		return
	}
	dir := c.folder(set.key())
	raw, err := json.MarshalIndent(set, "", "  ")
	if err == nil {
		err = os.MkdirAll(dir, 0o755)
	}
	if err == nil {
		err = atomicio.WriteAtomic(filepath.Join(dir, changeSetFile), raw, 0o644)
	}
	if err != nil {
		slog.Warn("cwm: change-set write", "set", set.key(), "err", err)
	}
}

// writeLog writes a commit's redo log (WriteAtomic: a temp file, fsync,
// rename).
func (c *changeSetStore) writeLog(l commitLog) error {
	if c.dir == "" {
		return nil
	}
	raw, err := json.Marshal(l)
	if err != nil {
		return err
	}
	dir := c.folder(l.key())
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return atomicio.WriteAtomic(filepath.Join(dir, commitLogFile), raw, 0o644)
}

// drop forgets a reaction's set — memory, disk and the ids it reserved — and
// returns what it held.
func (s *Service) dropChangeSet(c *changeSetStore, reaction string) []stagedWrite {
	c.mu.Lock()
	defer c.mu.Unlock()
	var held []stagedWrite
	if set, ok := c.sets[reaction]; ok {
		held = set.sorted()
		delete(c.sets, reaction)
	}
	for _, w := range held {
		if w.Op == api.ChangeSetCreate {
			s.locks.reserved.Delete(w.ID)
		}
	}
	if c.dir != "" {
		_ = os.RemoveAll(c.folder(reaction))
	}
	return held
}

// openSet is the change set ctx writes into: the caller's set ctx carries
// (api.WithChangeSet) when it is open, else its cause's when the cause is a
// reaction whose set is open. nil for every other write — which is written as
// it always was; a write with neither never even loads the store.
func (s *Service) openSet(ctx stdctx.Context) *changeSet {
	key := ""
	if id := api.ChangeSetFrom(ctx); id != "" {
		key = changeSetKeyOf("", id)
	} else if c, ok := cause.CauseFrom(ctx); ok && c.Kind == cause.KindReaction {
		key = c.ID
	} else {
		return nil
	}
	store := s.changeSets()
	store.mu.Lock()
	defer store.mu.Unlock()
	return store.sets[key]
}

// stagedOf is the write ctx's set holds for id, a copy; false when it holds
// none (or ctx has no open set).
func (s *Service) stagedOf(ctx stdctx.Context, id string) (stagedWrite, bool) {
	set := s.openSet(ctx)
	if set == nil {
		return stagedWrite{}, false
	}
	store := s.changeSets()
	store.mu.Lock()
	defer store.mu.Unlock()
	w, ok := set.Writes[id]
	if !ok {
		return stagedWrite{}, false
	}
	return w.clone(), true
}

// existsFor is "is this id an object", as ctx sees it: its set's staged
// creates exist, its staged deletes do not.
func (s *Service) existsFor(ctx stdctx.Context) func(string) bool {
	set := s.openSet(ctx)
	if set == nil {
		return s.objectExists
	}
	return func(id string) bool {
		if w, ok := s.stagedOf(ctx, id); ok {
			return w.Op != api.ChangeSetDelete
		}
		return s.objectExists(id)
	}
}

// objectByIDFor is objectByID as ctx sees it.
func (s *Service) objectByIDFor(ctx stdctx.Context, id string) (api.Object, bool) {
	if w, ok := s.stagedOf(ctx, id); ok {
		if w.Op == api.ChangeSetDelete {
			return api.Object{}, false
		}
		return *w.Object, true
	}
	return s.objectByID(id)
}

// errNotInChangeSet refuses a verb a change set cannot stage.
func (s *Service) refuseInChangeSet(ctx stdctx.Context, verb string) error {
	set := s.openSet(ctx)
	if set == nil {
		return nil
	}
	return fmt.Errorf("%w: %s is not inside a change set (%s): moves, splits, renames, files and a delete with detach are written at once or not at all",
		api.ErrInvalidInput, verb, set.name())
}

// createPath is where a create files obj: its type's folder (the runtime
// store for a runtime type), the file named by its id and slug.
func (s *Service) createPath(module string, obj api.Object, slug string) (string, error) {
	folder := ObjectsDirName
	runtime := false
	if t, declared := s.lookupObjectType(obj.Type); declared {
		folder = t.ObjectsFolder()
		runtime = t.Runtime
	}
	file := objectFileNameFor(module, obj, slug)
	if runtime {
		p, err := s.runtimeObjectPath(module, obj.ID, folder)
		if err != nil {
			return "", err
		}
		return filepath.Join(filepath.Dir(p), file), nil
	}
	return s.objectPathNamed(module, obj.ID, folder, file)
}

// stagedPath is where a staged object's bytes are measured and will land.
func (s *Service) stagedPath(w stagedWrite) (string, error) {
	if w.Op == api.ChangeSetCreate {
		return s.createPath(w.Module, *w.Object, w.Slug)
	}
	return s.resolveObjectPath(w.Module, w.ID)
}

// stageWrite records a create or an update of obj in set, at path, under the
// object's lock. An update not staged yet records the current file's revision
// as its base; a given revision that is not the object's current one — staged
// or stored — is ErrConflict at once. It answers the staged bytes' revision.
func (s *Service) stageWrite(ctx stdctx.Context, set *changeSet, op, module string, obj api.Object, path, given, slug string) (string, error) {
	unlock := s.locks.lock(obj.ID)
	defer unlock()
	if err := s.holdsStore(); err != nil {
		return "", err
	}
	data, err := encodeObjectFile(path, obj)
	if err != nil {
		return "", err
	}
	rev := hashBytes(data)
	store := s.changeSets()
	store.mu.Lock()
	defer store.mu.Unlock()
	if set.committing || store.sets[set.key()] != set {
		return "", fmt.Errorf("%w: the change set of %s is being committed or was dropped", api.ErrConflict, set.name())
	}
	w, ok := set.Writes[obj.ID]
	switch {
	case op == api.ChangeSetCreate:
		if ok {
			return "", api.ObjectAlreadyExists(obj.ID, "staged")
		}
		w = &stagedWrite{ID: obj.ID, Module: module, Op: api.ChangeSetCreate, Slug: slug}
	case ok:
		if w.Op == api.ChangeSetDelete {
			return "", api.ObjectNotFound(obj.ID)
		}
		if given != "" && given != w.Revision {
			return "", api.ObjectConflict(obj.ID, given)
		}
	default:
		cur, err := fileRevision(path)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return "", api.ObjectNotFound(obj.ID)
			}
			return "", err
		}
		if given != "" && given != cur {
			return "", api.ObjectConflict(obj.ID, given)
		}
		w = &stagedWrite{ID: obj.ID, Module: module, Op: api.ChangeSetUpdate, Base: cur}
	}
	cp := obj
	w.Object = &cp
	w.Revision = rev
	w.Operations = append(w.Operations, api.OperationFrom(ctx))
	if ctx.Value(transitionKey{}) != nil {
		w.Sanctioned = true
	}
	set.Writes[obj.ID] = w
	s.stampSetCause(ctx, set)
	store.persist(set)
	return rev, nil
}

// stampSetCause keeps the cause of the set's first write: its commit's events
// carry it. Caller holds the store's lock.
func (s *Service) stampSetCause(ctx stdctx.Context, set *changeSet) {
	if set.Cause != nil {
		return
	}
	if c, ok := cause.CauseFrom(ctx); ok {
		set.Cause = &c
	}
}

// stageDelete records a delete of id in set: a staged create is simply
// forgotten (its id freed), anything else becomes a delete against the
// revision of its first touch.
func (s *Service) stageDelete(ctx stdctx.Context, set *changeSet, module, id, path string) error {
	unlock := s.locks.lock(id)
	defer unlock()
	if err := s.holdsStore(); err != nil {
		return err
	}
	store := s.changeSets()
	store.mu.Lock()
	defer store.mu.Unlock()
	if set.committing || store.sets[set.key()] != set {
		return fmt.Errorf("%w: the change set of %s is being committed or was dropped", api.ErrConflict, set.name())
	}
	w, ok := set.Writes[id]
	switch {
	case ok && w.Op == api.ChangeSetDelete:
		return api.ObjectNotFound(id)
	case ok && w.Op == api.ChangeSetCreate:
		delete(set.Writes, id)
		s.locks.reserved.Delete(id)
	case ok:
		w.Op, w.Object, w.Revision = api.ChangeSetDelete, nil, ""
		w.Operations = append(w.Operations, api.OperationFrom(ctx))
	default:
		cur, err := fileRevision(path)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return api.ObjectNotFound(id)
			}
			return err
		}
		set.Writes[id] = &stagedWrite{ID: id, Module: module, Op: api.ChangeSetDelete, Base: cur,
			Operations: []string{api.OperationFrom(ctx)}}
	}
	s.stampSetCause(ctx, set)
	store.persist(set)
	return nil
}

// stageTransition records a move of a staged object: published at the commit.
func (s *Service) stageTransition(set *changeSet, id string, p api.ObjectTransitionedEventPayload, prior, next api.Object) {
	store := s.changeSets()
	store.mu.Lock()
	defer store.mu.Unlock()
	w, ok := set.Writes[id]
	if !ok || store.sets[set.key()] != set {
		return
	}
	w.Transitions = append(w.Transitions, stagedTransition{Payload: p, Prior: prior, Next: next})
	store.persist(set)
}

// stageObjectDelete is ObjectDelete inside a change set: before_delete asked,
// the referrers judged as the set leaves them (a staged write's links in
// place of its stored ones), then staged.
func (s *Service) stageObjectDelete(ctx stdctx.Context, set *changeSet, in api.ObjectDeleteInput) (api.ObjectDeleteOutput, error) {
	if in.Detach {
		return api.ObjectDeleteOutput{}, s.refuseInChangeSet(ctx, "ObjectDelete with detach")
	}
	ix := s.index()
	if err := ix.ensureLoaded(); err != nil {
		return api.ObjectDeleteOutput{}, err
	}
	module := in.Module
	var prior api.Object
	path := ""
	if w, ok := s.stagedOf(ctx, in.ID); ok {
		if w.Op == api.ChangeSetDelete {
			return api.ObjectDeleteOutput{}, api.ObjectNotFound(in.ID)
		}
		prior, module = *w.Object, w.Module
	} else {
		p, ok := ix.pathOf(in.ID)
		if !ok {
			return api.ObjectDeleteOutput{}, api.ObjectNotFound(in.ID)
		}
		obj, err := ReadObjectFile(p)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return api.ObjectDeleteOutput{}, api.ObjectNotFound(in.ID)
			}
			return api.ObjectDeleteOutput{}, err
		}
		prior, path = obj, p
		if m, ok := ix.moduleOf(in.ID); ok {
			module = m
		}
	}
	if err := s.runBeforeHooks(ctx, api.HookBeforeDelete, prior, &prior); err != nil {
		return api.ObjectDeleteOutput{}, err
	}
	store := s.changeSets()
	store.mu.Lock()
	staged := set.sorted()
	store.mu.Unlock()
	if err := s.referrersIn(in.ID, staged); err != nil {
		return api.ObjectDeleteOutput{}, err
	}
	if err := s.stageDelete(ctx, set, module, in.ID, path); err != nil {
		return api.ObjectDeleteOutput{}, err
	}
	return api.ObjectDeleteOutput{ID: in.ID}, nil
}

// referrersIn is ObjectDelete's referrer guard as the staged writes leave the
// project: a staged object's parent and links replace its stored ones, a
// staged delete refers to nothing.
func (s *Service) referrersIn(id string, staged []stagedWrite) error {
	ix := s.index()
	byID := make(map[string]stagedWrite, len(staged))
	for _, w := range staged {
		byID[w.ID] = w
	}
	kids := map[string]bool{}
	for _, k := range ix.childrenOf(id) {
		if _, ok := byID[k.ID]; !ok {
			kids[k.ID] = true
		}
	}
	from := map[string]bool{}
	for _, e := range ix.edges(id, "", api.DirectionIn) {
		if _, ok := byID[e.From]; !ok && e.From != id {
			from[e.From+" -"+string(e.Kind)+"->"] = true
		}
	}
	for _, w := range staged {
		if w.Op == api.ChangeSetDelete || w.ID == id {
			continue
		}
		if w.Object.ParentID() == id {
			kids[w.ID] = true
		}
		for _, e := range w.Object.Edges {
			if e.To == id {
				from[w.ID+" -"+e.Kind+"->"] = true
			}
		}
	}
	if len(kids) > 0 {
		ids := sortedKeys(kids)
		return api.ObjectReferenced(fmt.Errorf("%w: ObjectDelete: %q has %d children (%s) — move or delete them first",
			api.ErrReferenced, id, len(ids), strings.Join(ids, ", ")), id, strings.Join(ids, ", "))
	}
	if len(from) > 0 {
		f := sortedKeys(from)
		return api.ObjectReferenced(fmt.Errorf("%w: ObjectDelete: %q is linked from %s — remove the links (a delete with detach is not inside a change set)",
			api.ErrReferenced, id, strings.Join(f, ", ")), id, strings.Join(f, ", "))
	}
	return nil
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// --- the verbs ---------------------------------------------------------------

// ChangeSetBegin opens a reaction's or a caller's change set; a leftover one (a failed
// attempt's) is discarded first.
func (s *Service) ChangeSetBegin(_ stdctx.Context, in api.ChangeSetBeginInput) (api.ChangeSetBeginOutput, error) {
	key, err := changeSetKey("ChangeSetBegin", in.Reaction, in.Set)
	if err != nil {
		return api.ChangeSetBeginOutput{}, err
	}
	store := s.changeSets()
	store.mu.Lock()
	if set, ok := store.sets[key]; ok && set.committing {
		store.mu.Unlock()
		return api.ChangeSetBeginOutput{}, fmt.Errorf("%w: ChangeSetBegin: the change set of %s is being committed", api.ErrConflict, changeSetName(in.Reaction, in.Set))
	}
	store.mu.Unlock()
	left := s.dropChangeSet(store, key)
	store.mu.Lock()
	set := &changeSet{Reaction: in.Reaction, Set: in.Set, At: time.Now().UTC(), Writes: map[string]*stagedWrite{}}
	store.sets[key] = set
	store.persist(set)
	store.mu.Unlock()
	return api.ChangeSetBeginOutput{Discarded: summaries(left)}, nil
}

// ChangeSetDiscard drops a reaction's change set: nothing it staged is
// written, no event goes out.
func (s *Service) ChangeSetDiscard(_ stdctx.Context, in api.ChangeSetDiscardInput) (api.ChangeSetDiscardOutput, error) {
	key, err := changeSetKey("ChangeSetDiscard", in.Reaction, in.Set)
	if err != nil {
		return api.ChangeSetDiscardOutput{}, err
	}
	store := s.changeSets()
	store.mu.Lock()
	if set, ok := store.sets[key]; ok && set.committing {
		store.mu.Unlock()
		return api.ChangeSetDiscardOutput{}, fmt.Errorf("%w: ChangeSetDiscard: the change set of %s is being committed", api.ErrConflict, changeSetName(in.Reaction, in.Set))
	}
	store.mu.Unlock()
	return api.ChangeSetDiscardOutput{Discarded: summaries(s.dropChangeSet(store, key))}, nil
}

// ChangeSetList lists the open change sets, oldest first.
func (s *Service) ChangeSetList(_ stdctx.Context, _ api.ChangeSetListInput) (api.ChangeSetListOutput, error) {
	store := s.changeSets()
	store.mu.Lock()
	out := make([]api.ChangeSetSummary, 0, len(store.sets))
	for _, set := range store.sets {
		out = append(out, api.ChangeSetSummary{Reaction: set.Reaction, Set: set.Set, At: set.At, Writes: summaries(set.sorted())})
	}
	store.mu.Unlock()
	sort.Slice(out, func(a, b int) bool {
		if !out[a].At.Equal(out[b].At) {
			return out[a].At.Before(out[b].At)
		}
		return changeSetKeyOf(out[a].Reaction, out[a].Set) < changeSetKeyOf(out[b].Reaction, out[b].Set)
	})
	return api.ChangeSetListOutput{Sets: out}, nil
}

// changeSetHeld is the refusal of a verb that must wait for the reaction's
// open set to be committed (OutboxRelease).
func (s *Service) changeSetHeld(reaction string) bool {
	store := s.changeSets()
	store.mu.Lock()
	defer store.mu.Unlock()
	_, ok := store.sets[reaction]
	return ok
}

// ChangeSetCommit writes a reaction's staged writes together, or none. A
// reaction with no open set commits nothing.
func (s *Service) ChangeSetCommit(ctx stdctx.Context, in api.ChangeSetCommitInput) (api.ChangeSetCommitOutput, error) {
	key, err := changeSetKey("ChangeSetCommit", in.Reaction, in.Set)
	if err != nil {
		return api.ChangeSetCommitOutput{}, err
	}
	store := s.changeSets()
	store.mu.Lock()
	set, ok := store.sets[key]
	if !ok {
		store.mu.Unlock()
		return api.ChangeSetCommitOutput{Writes: []api.ChangeSetWrite{}}, nil
	}
	if set.committing {
		store.mu.Unlock()
		return api.ChangeSetCommitOutput{}, fmt.Errorf("%w: ChangeSetCommit: the change set of %s is already being committed", api.ErrConflict, set.name())
	}
	set.committing = true
	staged := set.sorted()
	var setCause *cause.Cause
	if set.Cause != nil {
		c := *set.Cause
		setCause = &c
	}
	store.mu.Unlock()
	fail := func(err error) (api.ChangeSetCommitOutput, error) {
		s.dropChangeSet(store, key)
		return api.ChangeSetCommitOutput{}, err
	}
	// The commit's checks, events and hooks run under the set's recorded cause
	// and outside any caller's set: what they write is written.
	cctx := api.WithChangeSet(ctx, "")
	if setCause != nil {
		cctx = cause.WithCause(cctx, *setCause)
	}
	if len(staged) == 0 {
		s.dropChangeSet(store, key)
		return api.ChangeSetCommitOutput{Writes: []api.ChangeSetWrite{}}, nil
	}

	// 1. The checks again, against the current prior; no lock held (the
	// before_* hooks run actions).
	plan, err := s.planCommit(cctx, staged)
	if err != nil {
		return fail(err)
	}

	// 2. The locks, in sorted key order: every object, and the create lock of
	// each module a create files into.
	keys := map[string]bool{}
	for _, w := range staged {
		keys[w.ID] = true
		if w.Op == api.ChangeSetCreate {
			keys["create:"+w.Module] = true
		}
	}
	var unlocks []func()
	for _, k := range sortedKeys(keys) {
		unlocks = append(unlocks, s.locks.lock(k))
	}
	release := func() {
		for i := len(unlocks) - 1; i >= 0; i-- {
			unlocks[i]()
		}
		unlocks = nil
	}
	if err := s.holdsStore(); err != nil {
		release()
		return api.ChangeSetCommitOutput{}, err
	}

	// 3. Each first touch's revision against the current file; the final
	// bytes.
	ix := s.index()
	for i := range plan {
		a := &plan[i]
		w := staged[i]
		switch a.Op {
		case api.ChangeSetCreate:
			if _, err := os.Stat(a.Path); err == nil {
				release()
				return fail(api.ObjectConflict(a.ID, "absent"))
			}
			if p, ok := ix.pathOf(a.ID); ok && p != "" {
				release()
				return fail(api.ObjectConflict(a.ID, "absent"))
			}
		default:
			cur, err := fileRevision(a.Path)
			if err != nil || cur != w.Base {
				release()
				return fail(api.ObjectConflict(a.ID, w.Base))
			}
			a.Revision = cur
		}
		if a.Op != api.ChangeSetDelete {
			data, err := encodeObjectFile(a.Path, *a.Object)
			if err != nil {
				release()
				return fail(err)
			}
			a.Data, a.Revision = data, hashBytes(data)
		} else {
			a.Owned = s.ownedFilePaths(cctx, a.ID)
		}
	}

	// 4. The redo log, then the writes.
	log := commitLog{Reaction: in.Reaction, Set: in.Set, Cause: setCause, Applies: plan}
	if err := store.writeLog(log); err != nil {
		release()
		return fail(fmt.Errorf("cwm/core: change-set redo log: %w", err))
	}
	if s.changeSetStopAfterLog != nil && s.changeSetStopAfterLog() {
		release() // a test's stop: everything as a crash leaves it
		return api.ChangeSetCommitOutput{}, errors.New("cwm/core: stopped after the redo log (test)")
	}
	applyErr := s.applyCommit(log)
	release()
	if applyErr != nil {
		// The redo log stays: the next start finishes what is missing.
		store.mu.Lock()
		delete(store.sets, key)
		store.mu.Unlock()
		return api.ChangeSetCommitOutput{}, fmt.Errorf("cwm/core: change-set commit of %s, finished at the next start: %w", set.name(), applyErr)
	}

	// 5. The events, under the reaction's cause; then the set is gone.
	s.commitEvents(cctx, log)
	s.dropChangeSet(store, key)
	out := make([]api.ChangeSetWrite, 0, len(plan))
	for _, a := range plan {
		rev := a.Revision
		if a.Op == api.ChangeSetDelete {
			rev = ""
		}
		out = append(out, api.ChangeSetWrite{ID: a.ID, Module: a.Module, Op: a.Op, Revision: rev})
	}
	return api.ChangeSetCommitOutput{Writes: out}, nil
}

// planCommit re-runs each staged write's checks against the object as it is
// now — validation (the set's creates and deletes counted for links), size
// budget, containment, lifecycle, the before_* hooks, a delete's referrers —
// and resolves where each lands.
func (s *Service) planCommit(ctx stdctx.Context, staged []stagedWrite) ([]appliedWrite, error) {
	ix := s.index()
	if err := ix.ensureLoaded(); err != nil {
		return nil, err
	}
	byID := make(map[string]stagedWrite, len(staged))
	for _, w := range staged {
		byID[w.ID] = w
	}
	known := func(id string) bool {
		if w, ok := byID[id]; ok {
			return w.Op != api.ChangeSetDelete
		}
		return s.objectExists(id)
	}
	plan := make([]appliedWrite, 0, len(staged))
	for _, w := range staged {
		a := appliedWrite{ID: w.ID, Module: w.Module, Op: w.Op, Operation: w.operation(), Transitions: w.Transitions}
		wctx := ctx
		if a.Operation != "" {
			wctx = api.WithOperation(ctx, a.Operation)
		}
		if w.Sanctioned {
			wctx = stdctx.WithValue(wctx, transitionKey{}, true)
		}
		if w.Op == api.ChangeSetCreate {
			path, err := s.createPath(w.Module, *w.Object, w.Slug)
			if err != nil {
				return nil, err
			}
			a.Path, a.Object = path, w.Object
			if err := s.checkWriteWith(*w.Object, nil, "ChangeSetCommit", known); err != nil {
				return nil, err
			}
			if err := s.checkSizeBudget(path, *w.Object, "ChangeSetCommit"); err != nil {
				return nil, err
			}
			if err := s.checkSystemModuleName(*w.Object, "ChangeSetCommit"); err != nil {
				return nil, err
			}
			if err := s.checkCreateLifecycle(*w.Object); err != nil {
				return nil, err
			}
			if err := s.runBeforeHooks(wctx, api.HookBeforeCreate, *w.Object, nil); err != nil {
				return nil, err
			}
			plan = append(plan, a)
			continue
		}
		path, ok := ix.pathOf(w.ID)
		if !ok {
			return nil, api.ObjectConflict(w.ID, w.Base) // gone since it was staged
		}
		prior, err := ReadObjectFile(path)
		if err != nil {
			return nil, api.ObjectConflict(w.ID, w.Base)
		}
		a.Path, a.Prior = path, &prior
		if w.Op == api.ChangeSetDelete {
			if err := s.runBeforeHooks(wctx, api.HookBeforeDelete, prior, &prior); err != nil {
				return nil, err
			}
			if err := s.referrersIn(w.ID, staged); err != nil {
				return nil, err
			}
			plan = append(plan, a)
			continue
		}
		a.Object = w.Object
		obj := *w.Object
		if err := s.checkWriteWith(obj, &prior, "ChangeSetCommit", known); err != nil {
			return nil, err
		}
		if err := s.checkSizeBudget(path, obj, "ChangeSetCommit"); err != nil {
			return nil, err
		}
		if err := s.checkParentNotDescendant(obj, &prior, "ChangeSetCommit"); err != nil {
			return nil, err
		}
		if err := s.checkSystemModuleName(obj, "ChangeSetCommit"); err != nil {
			return nil, err
		}
		if wctx.Value(transitionKey{}) == nil {
			if err := checkRetiredLifecycleFields(&prior, obj); err != nil {
				return nil, err
			}
		}
		if slot, changed := s.lifecycleStateChanged(wctx, &prior, obj); changed {
			what := "the lifecycle state"
			if slot != "" {
				what = "the state of capability " + slot
			}
			return nil, fmt.Errorf("%w: %q: %s changes only through ObjectTransition", api.ErrInvalidInput, obj.ID, what)
		}
		if err := s.runBeforeHooks(wctx, api.HookBeforeUpdate, obj, &prior); err != nil {
			return nil, err
		}
		plan = append(plan, a)
	}
	return plan, nil
}

// applyCommit writes what a redo log says — each file written whole, each
// delete removed (an absent file is a delete already done) — and the index
// with it. Applying a log twice leaves what applying it once did.
func (s *Service) applyCommit(l commitLog) error {
	ix := s.index()
	_ = ix.ensureLoaded()
	var first error
	for _, a := range l.Applies {
		switch a.Op {
		case api.ChangeSetDelete:
			if err := os.Remove(a.Path); err != nil && !errors.Is(err, fs.ErrNotExist) {
				if first == nil {
					first = err
				}
				continue
			}
			ix.drop(a.ID)
			removeOwnedFiles(a.Owned)
		default:
			err := os.MkdirAll(filepath.Dir(a.Path), 0o755)
			if err == nil {
				err = atomicio.WriteAtomic(a.Path, a.Data, 0o644)
			}
			if err != nil {
				if first == nil {
					first = err
				}
				continue
			}
			ix.put(a.Module, a.Path, *a.Object)
		}
	}
	return first
}

// commitEvents sends what each applied write owes, as its own write would
// have: the object event, its on_* point, and the moves staged on it — under
// the reaction's cause, so what they trigger is staged in its outbox.
func (s *Service) commitEvents(ctx stdctx.Context, l commitLog) {
	for _, a := range l.Applies {
		wctx := ctx
		if a.Operation != "" {
			wctx = api.WithOperation(ctx, a.Operation)
		}
		meta := s.eventMeta(wctx, a.Revision)
		switch a.Op {
		case api.ChangeSetCreate:
			s.publishObjectEvent(wctx, a.Module, *a.Object, false, meta)
			s.fireAfterHooks(wctx, api.HookOnCreate, *a.Object, nil, writeEvent(wctx, meta, "created", a.Module, *a.Object))
		case api.ChangeSetUpdate:
			s.publishObjectEvent(wctx, a.Module, *a.Object, true, meta)
			s.fireAfterHooks(wctx, api.HookOnUpdate, *a.Object, a.Prior, writeEvent(wctx, meta, "updated", a.Module, *a.Object))
		case api.ChangeSetDelete:
			prior := api.Object{ID: a.ID}
			if a.Prior != nil {
				prior = *a.Prior
			}
			s.Events.PublishObject("deleted", a.ID, a.Module, prior.Type, prior.ParentID(), "", api.OperationFrom(wctx), meta)
			if a.Prior != nil {
				s.fireAfterHooks(wctx, api.HookOnDelete, prior, &prior, writeEvent(wctx, meta, "deleted", a.Module, prior))
			}
		}
		for _, t := range a.Transitions {
			p := t.Payload
			p.EventMeta = s.eventMeta(ctx, a.Revision)
			s.Events.PublishObjectTransitioned(p)
			s.fireAfterHooks(ctx, api.HookOnTransitioned, t.Next, &t.Prior,
				moveEvent(p.EventMeta, api.TransitionedEventAction, "object.transitioned", a.Module, t.Next, p))
		}
	}
}

// finishCommit finishes a commit a previous process logged: applied again,
// re-indexed, its events sent, its folder removed. It uses c, never
// changeSets(): it runs while the store is being opened.
func (s *Service) finishCommit(c *changeSetStore, l commitLog) {
	if err := s.applyCommit(l); err != nil {
		slog.Warn("cwm: a change-set commit could not be finished; kept for the next start", "set", l.key(), "err", err)
		return
	}
	ctx := stdctx.Background()
	if l.Cause != nil {
		ctx = cause.WithCause(ctx, *l.Cause)
	}
	s.commitEvents(ctx, l)
	for _, a := range l.Applies {
		if a.Op == api.ChangeSetCreate {
			s.locks.reserved.Delete(a.ID)
		}
	}
	c.mu.Lock()
	delete(c.sets, l.key())
	c.mu.Unlock()
	if c.dir != "" {
		_ = os.RemoveAll(c.folder(l.key()))
	}
	slog.Info("cwm: finished a change-set commit a previous process logged", "set", l.key(), "writes", len(l.Applies))
}
