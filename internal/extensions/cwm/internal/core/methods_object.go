// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/katyoti/katyoti-fwk/internal/core/cause"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/internal/storage"
)

// ModulesDirName is the Project-root folder holding module folders. A module
// folder lives at modules/<name>/ and holds the module's documents AND its
// objects side by side (modules/cwm/storage-redesign.md).
const ModulesDirName = "modules"

// objectFileName derives an object's filename from its id.
//
// The rule is the one the authored corpus already follows: strip the leading
// "<module>." qualifier, so `cwm.p1` lives in `p1.yaml` and the module object
// `cwm` lives in `cwm.yaml`. Identity still comes from the file CONTENT — this
// only decides where a NEW file is put.
func objectFileName(module, id string) string {
	return strings.TrimPrefix(id, module+".") + ObjectFileSuffix
}

// safeSegment rejects anything that could escape the module folder. An id or
// module name is a single path segment — never a path.
func safeSegment(kind, s string) error {
	switch {
	case s == "":
		return fmt.Errorf("%w: %s required", api.ErrInvalidInput, kind)
	case strings.ContainsAny(s, `/\`), strings.Contains(s, ".."):
		return fmt.Errorf("%w: %s %q must be a single segment", api.ErrInvalidInput, kind, s)
	}
	return nil
}

// objectPath resolves the absolute path of one object's file.
func (s *Service) objectPath(module, id string) (string, error) {
	return s.objectPathIn(module, id, ObjectsDirName)
}

// objectPathIn is objectPath with an explicit storage folder — what a type
// declares via `folder:`. Kept separate so every EXISTING caller keeps writing
// to objects/ unchanged, and only the create path consults the type.
func (s *Service) objectPathIn(module, id, folder string) (string, error) {
	if s.refused != nil {
		return "", s.refused
	}
	if err := api.CheckFolder(folder); err != nil {
		return "", fmt.Errorf("%w: folder %q: %v", api.ErrInvalidInput, folder, err)
	}
	if err := safeSegment("module", module); err != nil {
		return "", err
	}
	if err := safeSegment("id", id); err != nil {
		return "", err
	}
	root := s.Config.ObjectsRoot()
	if root == "" {
		return "", fmt.Errorf("%w: workspace root unset", api.ErrInvalidInput)
	}
	return filepath.Join(root, s.modulesDirName(), module, folder, objectFileName(module, id)), nil
}

// objectPathNamed is objectPathIn with an explicit file name (7c: a slug, a
// `.cwm.md` spelling).
func (s *Service) objectPathNamed(module, id, folder, file string) (string, error) {
	p, err := s.objectPathIn(module, id, folder)
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(p), file), nil
}

// runtimeObjectPath is where a RUNTIME type's object is filed: the project's
// CWM store, same <module>/<folder>/ layout (concepts.md §5, 2026-09-25).
func (s *Service) runtimeObjectPath(module, id, folder string) (string, error) {
	if s.refused != nil {
		return "", s.refused
	}
	if err := api.CheckFolder(folder); err != nil {
		return "", fmt.Errorf("%w: folder %q: %v", api.ErrInvalidInput, folder, err)
	}
	for name, v := range map[string]string{"module": module, "id": id} {
		if err := safeSegment(name, v); err != nil {
			return "", err
		}
	}
	store := s.storeObjectsRoot()
	if store == "" {
		return "", fmt.Errorf("%w: no object store for a runtime object (workspace root unset)", api.ErrInvalidInput)
	}
	return filepath.Join(store, module, folder, objectFileName(module, id)), nil
}

// storeObjectsRoot is the runtime root: <WorkspaceRoot>/cwm/objects. Empty when
// the service has no workspace root (substrate tests build the index alone).
func (s *Service) storeObjectsRoot() string {
	if s.Config == nil || s.Config.WorkspaceRoot == "" {
		return ""
	}
	return filepath.Join(storage.NewPaths(s.Config.WorkspaceRoot).CWMDataDir(), ObjectsDirName)
}

// modulesDirName is the modules folder the root project object declares, or
// the default. Read from the index once it is loaded; the default before.
func (s *Service) modulesDirName() string {
	if ix := s.index(); ix != nil {
		if pid := ix.projectID(); pid != "" {
			if p, ok := ix.get(pid); ok {
				return p.ModulesDir()
			}
		}
	}
	return api.DefaultModulesDir
}

// resolveObjectPath answers "where is this object's file": the index when it
// knows (a runtime object, a type-folder object, the project object), else
// the composed default path. ObjectGet already did this; Update and Delete
// composed the path blindly and could not see a runtime object.
func (s *Service) resolveObjectPath(module, id string) (string, error) {
	if ix := s.index(); ix.ensureLoaded() == nil {
		if p, ok := ix.pathOf(id); ok {
			return p, nil
		}
	}
	return s.objectPath(module, id)
}

// ObjectCreate writes a new object file. Fails if one already exists — Create
// does not overwrite; that is Update's job.
func (s *Service) ObjectCreate(ctx stdctx.Context, in api.ObjectCreateInput) (api.ObjectCreateOutput, error) {
	// An authored id is module-qualified, as a minted one is: `AC-001` given in
	// module todo is — a bare id is a module's own (the module
	// object), and everything that reads the module from an id relies on it.
	// An id naming another module is refused.
	if id := in.Object.ID; id != "" && in.Module != "" && id != in.Module && !s.isModuleLevel(in.Object) {
		if !strings.Contains(id, ".") {
			in.Object.ID = in.Module + "." + id
		} else if m, err := api.ModuleOfObjectID(id); err != nil || m != in.Module {
			return api.ObjectCreateOutput{}, fmt.Errorf("%w: ObjectCreate: id %q names another module than %q", api.ErrInvalidInput, id, in.Module)
		}
	}
	// A cross-module reaction writes only the modules it declares — staged
	// or not (module_subscriptions.go).
	if err := s.checkReactionWrites(ctx, in.Module, "ObjectCreate"); err != nil {
		return api.ObjectCreateOutput{}, err
	}
	// Inside a reaction's change set (changeset.go) the create is checked and
	// staged, not written: its own staged writes exist for it.
	set := s.openSet(ctx)
	in.Object = qualifyEdges(in.Object, s.existsFor(ctx)) // writers write each target's id (7e)
	// The TYPE says where its objects live. A `goal` declaring `folder: goals`
	// lands its objects together without anyone moving them by hand.
	//
	// Only the CREATE path consults it: reads find an object by scanning for the
	// `.cwm.yaml` suffix, so an object written before a type changed its folder
	// is still found exactly where it is. The folder is a default, not a rule.
	t, declared := s.lookupObjectType(in.Object.Type)
	// Minting, the "already exists" check and the write hold the module's
	// create lock: two creates cannot mint the same <PREFIX>-NNN, nor both
	// find an id free and write two files for it.
	createKey := "create:" + in.Module
	if in.Module == "" {
		createKey = "create:" + in.Object.ID
	}
	unlockCreate := s.locks.lock(createKey)
	released := false
	release := func() {
		if !released {
			released = true
			unlockCreate()
		}
	}
	defer release()
	if err := s.holdsStore(); err != nil {
		return api.ObjectCreateOutput{}, err
	}
	// A title and no id: the id is MINTED (7c) and the file carries a slug.
	slug := ""
	if in.Object.ID == "" {
		if in.Module == "" {
			return api.ObjectCreateOutput{}, fmt.Errorf("%w: ObjectCreate: module required to mint an id", api.ErrInvalidInput)
		}
		id, err := s.mintObjectID(in.Module, t, declared)
		if err != nil {
			return api.ObjectCreateOutput{}, err
		}
		in.Object.ID = id
		if m := s.minterID(); m != "" && t.IDPrefix != "" {
			in.Object.Minter = m // set once, here
		}
		slug = slugOf(in.Object.Name)
	}
	path, err := s.createPath(in.Module, in.Object, slug)
	if err != nil {
		return api.ObjectCreateOutput{}, err
	}
	if w, ok := s.stagedOf(ctx, in.Object.ID); ok {
		if w.Op == api.ChangeSetDelete {
			return api.ObjectCreateOutput{}, fmt.Errorf("%w: ObjectCreate: %q is deleted in this change set — update it instead", api.ErrInvalidInput, in.Object.ID)
		}
		return api.ObjectCreateOutput{}, api.ObjectAlreadyExists(in.Object.ID, "staged")
	}
	if _, err := os.Stat(path); err == nil {
		return api.ObjectCreateOutput{}, api.ObjectAlreadyExists(in.Object.ID, "")
	}
	// The id, not only the path: an object of that id filed in another folder
	// (its type's, a subfolder) would otherwise get a second file.
	if ix := s.index(); ix.ensureLoaded() == nil {
		if p, ok := ix.pathOf(in.Object.ID); ok {
			return api.ObjectCreateOutput{}, api.ObjectAlreadyExists(in.Object.ID, s.relPath(p))
		}
	}
	// Claimed: no other create mints or writes this id until this one ends;
	// the lock itself is not held across the checks and hooks below.
	if _, taken := s.locks.reserved.LoadOrStore(in.Object.ID, true); taken {
		return api.ObjectCreateOutput{}, api.ObjectAlreadyExists(in.Object.ID, "being created")
	}
	keepReserved := false // a staged create's id stays reserved for its set's life
	defer func() {
		if !keepReserved {
			s.locks.reserved.Delete(in.Object.ID)
		}
	}()
	release()
	if err := s.checkWriteWith(in.Object, nil, "ObjectCreate", s.existsFor(ctx)); err != nil {
		return api.ObjectCreateOutput{}, err
	}
	if !moving(ctx) { // a move refiles the object as it is
		if err := s.checkSizeBudget(path, in.Object, "ObjectCreate"); err != nil {
			return api.ObjectCreateOutput{}, err
		}
	}
	if err := s.checkSystemModuleName(in.Object, "ObjectCreate"); err != nil {
		return api.ObjectCreateOutput{}, err
	}
	if err := s.checkModuleSubscriptions(in.Object, nil, "ObjectCreate"); err != nil {
		return api.ObjectCreateOutput{}, err
	}
	if !moving(ctx) { // a moved object keeps its state and is not born again
		if err := s.checkCreateLifecycle(in.Object); err != nil {
			return api.ObjectCreateOutput{}, err
		}
		if err := s.runBeforeHooks(ctx, api.HookBeforeCreate, in.Object, nil); err != nil {
			return api.ObjectCreateOutput{}, err
		}
	}
	if set != nil {
		rev, err := s.stageWrite(ctx, set, api.ChangeSetCreate, in.Module, in.Object, path, "", slug)
		if err != nil {
			return api.ObjectCreateOutput{}, err
		}
		keepReserved = true
		return api.ObjectCreateOutput{Object: in.Object, Path: s.relPath(path), Revision: rev}, nil
	}
	rev, err := writeObjectFileRev(path, in.Object)
	if err != nil {
		return api.ObjectCreateOutput{}, err
	}
	// Write-through: reads answer from the index, so without this a caller would
	// write an object and then fail to find it.
	s.index().put(in.Module, path, in.Object)
	// ONE event id per write: the event published and the hooks it fires (a
	// subscription's match) name the same event.
	meta := s.eventMeta(ctx, rev)
	s.publishObjectEvent(ctx, in.Module, in.Object, false, meta)
	if !moving(ctx) {
		s.fireAfterHooks(ctx, api.HookOnCreate, in.Object, nil, writeEvent(ctx, meta, "created", in.Module, in.Object))
	}
	return api.ObjectCreateOutput{Object: in.Object, Path: s.relPath(path), Revision: rev}, nil
}

// ObjectGet loads one object from its file.
func (s *Service) ObjectGet(ctx stdctx.Context, in api.ObjectGetInput) (api.ObjectGetOutput, error) {
	// A reaction reads its own staged writes back (changeset.go).
	if w, ok := s.stagedOf(ctx, in.ID); ok {
		if w.Op == api.ChangeSetDelete {
			return api.ObjectGetOutput{}, api.ObjectNotFound(in.ID)
		}
		return api.ObjectGetOutput{Object: *w.Object, Errors: s.problemsOf(in.ID), Revision: w.Revision}, nil
	}
	// Without a module the index is the only way to the file: an id it does not
	// know (never created, or moved away) is not found — not a missing module.
	if in.Module == "" && in.ID != "" {
		if ix := s.index(); ix.ensureLoaded() == nil {
			if _, ok := ix.pathOf(in.ID); !ok {
				return api.ObjectGetOutput{}, s.notFoundOrEnded(in.ID)
			}
		}
	}
	path, err := s.resolveObjectPath(in.Module, in.ID)
	if err != nil {
		return api.ObjectGetOutput{}, err
	}
	// Identity lives in the file CONTENT, not the filename — so the index, which
	// keys on content, is the authority on where an object actually is. Without
	// this, renaming a file broke Get while List and Tree kept working: the file
	// header promised rename-safety the code did not deliver.
	if ix := s.index(); ix.ensureLoaded() == nil {
		if p, ok := ix.pathOf(in.ID); ok {
			path = p
		}
	}
	obj, rev, err := readObjectFileRev(path)
	if err != nil {
		// errors.Is unwraps; os.IsNotExist does not, and matching the OS message text
		// is locale-dependent (it broke on a French Windows).
		if errors.Is(err, fs.ErrNotExist) {
			return api.ObjectGetOutput{}, s.notFoundOrEnded(in.ID)
		}
		return api.ObjectGetOutput{}, err
	}
	return api.ObjectGetOutput{Object: obj, Errors: s.problemsOf(obj.ID), Revision: rev}, nil
}

// notFoundOrEnded is the refusal of an id the index lacks: an archived one
// says it has ended and where its file went (cwm.objectArchived, a not-found),
// any other is not found.
func (s *Service) notFoundOrEnded(id string) error {
	if path, ended := s.isEnded(id); ended {
		return api.ObjectArchived(id, path)
	}
	return api.ObjectNotFound(id)
}

// ObjectUpdate replaces an existing object. Fails if it does not exist — Update
// does not create; that is Create's job.
func (s *Service) ObjectUpdate(ctx stdctx.Context, in api.ObjectUpdateInput) (api.ObjectUpdateOutput, error) {
	// From outside (CLI, agent tool, KTSP), a write names the revision it read
	// or says force: never a silent overwrite of an edit made outside CWM.
	if err := in.Unconditioned(); err != nil {
		return api.ObjectUpdateOutput{}, err
	}
	// Inside a reaction's change set (changeset.go) the update is checked and
	// staged against the object as the set holds it, not written.
	set := s.openSet(ctx)
	staged, isStaged := s.stagedOf(ctx, in.Object.ID)
	if isStaged {
		if staged.Op == api.ChangeSetDelete {
			return api.ObjectUpdateOutput{}, api.ObjectNotFound(in.Object.ID)
		}
		if in.Module != "" && in.Module != staged.Module {
			return api.ObjectUpdateOutput{}, fmt.Errorf("%w: ObjectUpdate: %q is in module %s, not %s — ObjectMove refiles an object",
				api.ErrInvalidInput, in.Object.ID, staged.Module, in.Module)
		}
		in.Module = staged.Module
	}
	in.Object = qualifyEdges(in.Object, s.existsFor(ctx)) // writers write each target's id (7e)
	// The MODULE is the object's, as the index knows it: an update never
	// refiles (ObjectMove does). An empty module is filled from the index; a
	// different one is refused rather than recorded against the object.
	if ix := s.index(); ix.ensureLoaded() == nil {
		if m, ok := ix.moduleOf(in.Object.ID); ok {
			if in.Module != "" && in.Module != m {
				return api.ObjectUpdateOutput{}, fmt.Errorf("%w: ObjectUpdate: %q is in module %s, not %s — ObjectMove refiles an object",
					api.ErrInvalidInput, in.Object.ID, m, in.Module)
			}
			in.Module = m
		}
	}
	written := in.Module
	if written == "" {
		written = s.moduleOfObject(in.Object.ID)
	}
	if err := s.checkReactionWrites(ctx, written, "ObjectUpdate"); err != nil {
		return api.ObjectUpdateOutput{}, err
	}
	var path string
	var err error
	var priorObj *api.Object
	if isStaged {
		if path, err = s.stagedPath(staged); err != nil {
			return api.ObjectUpdateOutput{}, err
		}
		priorObj = staged.Object
	} else {
		path, err = s.resolveObjectPath(in.Module, in.Object.ID)
		if err != nil {
			return api.ObjectUpdateOutput{}, err
		}
		if _, err := os.Stat(path); err != nil {
			if rerr := s.unreadableRefusal(in.Object.ID, "", "ObjectUpdate"); rerr != nil {
				return api.ObjectUpdateOutput{}, rerr
			}
			return api.ObjectUpdateOutput{}, api.ObjectNotFound(in.Object.ID)
		}
		// An unreadable file is never overwritten: in-process, with a
		// revision or forced, the write is refused until it parses again.
		if err := s.unreadableRefusal(in.Object.ID, path, "ObjectUpdate"); err != nil {
			return api.ObjectUpdateOutput{}, err
		}
		// The previous object, read before the overwrite: the before / after
		// hooks see it.
		if prior, perr := ReadObjectFile(path); perr == nil {
			priorObj = &prior
		}
	}
	// The minter is set once at create and never edited.
	if priorObj != nil {
		in.Object.Minter = priorObj.Minter
	}
	// Update validates too: a type can be changed by an update, and an
	// unvalidated update is a hole straight through a validated create. It
	// refuses what the update ADDS: an object stored with errors is repaired
	// one write at a time.
	if err := s.checkWriteWith(in.Object, priorObj, "ObjectUpdate", s.existsFor(ctx)); err != nil {
		return api.ObjectUpdateOutput{}, err
	}
	if err := s.checkSizeBudget(path, in.Object, "ObjectUpdate"); err != nil {
		return api.ObjectUpdateOutput{}, err
	}
	if err := s.checkParentNotDescendant(in.Object, priorObj, "ObjectUpdate"); err != nil {
		return api.ObjectUpdateOutput{}, err
	}
	if err := s.checkSystemModuleName(in.Object, "ObjectUpdate"); err != nil {
		return api.ObjectUpdateOutput{}, err
	}
	if err := s.checkModuleSubscriptions(in.Object, priorObj, "ObjectUpdate"); err != nil {
		return api.ObjectUpdateOutput{}, err
	}
	if ctx.Value(transitionKey{}) == nil {
		if err := checkRetiredLifecycleFields(priorObj, in.Object); err != nil {
			return api.ObjectUpdateOutput{}, err
		}
	}
	if slot, changed := s.lifecycleStateChanged(ctx, priorObj, in.Object); changed {
		what := "the lifecycle state"
		if slot != "" {
			what = "the state of capability " + slot
		}
		return api.ObjectUpdateOutput{}, fmt.Errorf("%w: %q: %s changes only through ObjectTransition", api.ErrInvalidInput, in.Object.ID, what)
	}
	if err := s.runBeforeHooks(ctx, api.HookBeforeUpdate, in.Object, priorObj); err != nil {
		return api.ObjectUpdateOutput{}, err
	}
	if set != nil {
		rev, err := s.stageWrite(ctx, set, api.ChangeSetUpdate, in.Module, in.Object, path, in.Revision, "")
		if err != nil {
			return api.ObjectUpdateOutput{}, err
		}
		return api.ObjectUpdateOutput{Object: in.Object, Revision: rev}, nil
	}
	// THE WRITE, under the object's lock: the version it replaces is the one
	// the caller read (when it says so), and the file and the index change
	// together. No hook runs in here.
	unlock := s.locks.lock(in.Object.ID)
	if err := s.holdsStore(); err != nil {
		unlock()
		return api.ObjectUpdateOutput{}, err
	}
	cur, err := fileRevision(path)
	if err != nil {
		unlock()
		if errors.Is(err, fs.ErrNotExist) {
			return api.ObjectUpdateOutput{}, api.ObjectNotFound(in.Object.ID)
		}
		return api.ObjectUpdateOutput{}, err
	}
	if in.Revision != "" && in.Revision != cur {
		unlock()
		return api.ObjectUpdateOutput{}, api.ObjectConflict(in.Object.ID, in.Revision)
	}
	rev, err := writeObjectFileRev(path, in.Object)
	if err != nil {
		unlock()
		return api.ObjectUpdateOutput{}, err
	}
	s.index().put(in.Module, path, in.Object)
	unlock()
	meta := s.eventMeta(ctx, rev)
	s.publishObjectEvent(ctx, in.Module, in.Object, true, meta)
	s.fireAfterHooks(ctx, api.HookOnUpdate, in.Object, priorObj, writeEvent(ctx, meta, "updated", in.Module, in.Object))
	return api.ObjectUpdateOutput{Object: in.Object, Revision: rev}, nil
}

// unreadableRefusal refuses a write to an object whose file no longer parses,
// or whose id a second file also carries: the write would replace
// a hand edit CWM cannot read. It names the file and its parse error; the
// refusal lifts once the file parses again (every condition is read from the
// disk now, the last scan only says which files to look at). path is the file
// the write would replace, "" when the index no longer knows the object.
func (s *Service) unreadableRefusal(id, path, verb string) error {
	if path != "" {
		if _, err := ReadObjectFile(path); err != nil {
			return fmt.Errorf("%w: %s: %q: its file %s does not parse (%v) - no write reaches it until it is fixed",
				api.ErrInvalidInput, verb, id, s.relPath(path), err)
		}
	}
	ix := s.index()
	if ix.ensureLoaded() != nil {
		return nil
	}
	local := id
	if _, rest, ok := strings.Cut(id, "."); ok {
		local = rest
	}
	for _, b := range ix.unreadableFiles() {
		abs := b.Path
		if !filepath.IsAbs(abs) {
			abs = filepath.Join(ix.root, filepath.FromSlash(b.Path))
		}
		if path != "" && filepath.Clean(abs) == filepath.Clean(path) {
			continue
		}
		obj, err := ReadObjectFile(abs)
		switch {
		case err == nil && obj.ID == id: // a second file carries the id
			return fmt.Errorf("%w: %s: %q: %s carries the same id (%s) - no write reaches it until one of the files is renumbered or removed",
				api.ErrInvalidInput, verb, id, b.Path, api.DiagnosticDuplicateID)
		case err != nil && b.ID == "" && fileNamesID(abs, local): // the object's file, dropped from the index
			if _, serr := os.Stat(abs); serr == nil {
				return fmt.Errorf("%w: %s: %q: its file %s does not parse (%v) - no write reaches it until it is fixed",
					api.ErrInvalidInput, verb, id, b.Path, err)
			}
		}
	}
	return nil
}

// fileNamesID says whether a file's name is the one an object of that local
// id is filed under: `<local>.cwm.yaml` or `<local>-<slug>.cwm.yaml`.
func fileNamesID(path, local string) bool {
	base := filepath.Base(path)
	return base == local+ObjectFileSuffix || strings.HasPrefix(base, local+"-") && strings.HasSuffix(base, ObjectFileSuffix)
}

// checkParentNotDescendant refuses a new parent that is the object itself or
// one of its descendants: the subtree would leave the tree while every file
// stays readable — nothing errors, the objects simply stop being reachable.
func (s *Service) checkParentNotDescendant(obj api.Object, prior *api.Object, verb string) error {
	p := obj.ParentID()
	if p == "" || (prior != nil && prior.ParentID() == p) {
		return nil
	}
	if p == obj.ID {
		return fmt.Errorf("%w: %s: %q cannot be its own parent", api.ErrCircularReference, verb, obj.ID)
	}
	for _, anc := range s.ancestorsOf(p) {
		if anc == obj.ID {
			return fmt.Errorf("%w: %s: %q is a descendant of %q; the subtree would leave the tree",
				api.ErrCircularReference, verb, p, obj.ID)
		}
	}
	return nil
}

// ObjectDelete removes one object file. It never leaves a link dangling: an
// object with children is refused, naming them, and so is one other objects
// link to — unless Detach, which removes those links first and lists them.
func (s *Service) ObjectDelete(ctx stdctx.Context, in api.ObjectDeleteInput) (api.ObjectDeleteOutput, error) {
	module := in.Module
	if module == "" {
		module = s.moduleOfObject(in.ID)
	}
	if err := s.checkReactionWrites(ctx, module, "ObjectDelete"); err != nil {
		return api.ObjectDeleteOutput{}, err
	}
	// Inside a reaction's change set the delete is staged; with detach it is
	// refused (changeset.go).
	if set := s.openSet(ctx); set != nil {
		return s.stageObjectDelete(ctx, set, in)
	}
	// before_delete may refuse, before anything changes — the links detached
	// included.
	if path, err := s.resolveObjectPath(in.Module, in.ID); err == nil {
		if ix := s.index(); ix.ensureLoaded() == nil {
			if p, ok := ix.pathOf(in.ID); ok {
				path = p
			}
		}
		if prior, err := ReadObjectFile(path); err == nil {
			if err := s.runBeforeHooks(ctx, api.HookBeforeDelete, prior, &prior); err != nil {
				return api.ObjectDeleteOutput{}, err
			}
		}
	}
	detached, err := s.clearReferrers(ctx, in)
	if err != nil {
		return api.ObjectDeleteOutput{}, err
	}
	out, err := s.deleteObject(ctx, in)
	out.Detached = detached
	return out, err
}

// clearReferrers is ObjectDelete's guard: children refuse, inbound links
// refuse unless Detach removes them.
func (s *Service) clearReferrers(ctx stdctx.Context, in api.ObjectDeleteInput) ([]string, error) {
	ix := s.index()
	if err := ix.ensureLoaded(); err != nil {
		return nil, err
	}
	if kids := ix.childrenOf(in.ID); len(kids) > 0 {
		ids := make([]string, 0, len(kids))
		for _, k := range kids {
			ids = append(ids, k.ID)
		}
		sort.Strings(ids)
		return nil, api.ObjectReferenced(fmt.Errorf("%w: ObjectDelete: %q has %d children (%s) — move or delete them first",
			api.ErrReferenced, in.ID, len(ids), strings.Join(ids, ", ")), in.ID, strings.Join(ids, ", "))
	}
	var from []string
	inbound := ix.edges(in.ID, "", api.DirectionIn)
	for _, e := range inbound {
		if e.From != in.ID {
			from = append(from, e.From+" -"+string(e.Kind)+"->")
		}
	}
	if len(from) == 0 {
		return nil, nil
	}
	sort.Strings(from)
	if !in.Detach {
		return nil, api.ObjectReferenced(fmt.Errorf("%w: ObjectDelete: %q is linked from %s — remove the links, or delete with detach",
			api.ErrReferenced, in.ID, strings.Join(from, ", ")), in.ID, strings.Join(from, ", "))
	}
	var detached []string
	seen := map[string]bool{}
	for _, e := range inbound {
		if seen[e.From] || e.From == in.ID {
			continue
		}
		seen[e.From] = true
		src, ok := ix.get(e.From)
		if !ok {
			continue
		}
		changed := false
		if err := s.updateFresh(ctx, src.ID, func(o *api.Object) bool {
			kept := o.Edges[:0:0]
			for _, oe := range o.Edges {
				if oe.To != in.ID {
					kept = append(kept, oe)
				}
			}
			changed = len(kept) != len(o.Edges)
			o.Edges = kept
			return changed
		}); err != nil {
			return detached, fmt.Errorf("ObjectDelete: detaching %s: %w", src.ID, err)
		}
		if changed {
			detached = append(detached, src.ID)
		}
	}
	sort.Strings(detached)
	return detached, nil
}

// deleteObject removes the file, with no referrer guard: ObjectMove calls it
// for the old identity, whose references it re-points right after.
func (s *Service) deleteObject(ctx stdctx.Context, in api.ObjectDeleteInput) (api.ObjectDeleteOutput, error) {
	path, err := s.resolveObjectPath(in.Module, in.ID)
	if err != nil {
		return api.ObjectDeleteOutput{}, err
	}
	// Read BEFORE removing: a node-deleted event has to say what type was
	// deleted, and after os.Remove there is nothing left to ask. A failure to
	// read is not fatal — the delete still proceeds and the node event is
	// simply omitted rather than published with a wrong type.
	prior, priorRev, priorErr := readObjectFileRev(path)
	module := in.Module
	if module == "" {
		if ix := s.index(); ix.ensureLoaded() == nil {
			module, _ = ix.moduleOf(in.ID)
		}
	}
	owned := s.ownedFilePaths(ctx, in.ID)
	unlock := s.locks.lock(in.ID)
	if err := s.holdsStore(); err != nil {
		unlock()
		return api.ObjectDeleteOutput{}, err
	}
	err = os.Remove(path)
	if err == nil {
		s.index().drop(in.ID)
	}
	unlock()
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return api.ObjectDeleteOutput{}, api.ObjectNotFound(in.ID)
		}
		return api.ObjectDeleteOutput{}, fmt.Errorf("cwm/core: delete object %q: %w", in.ID, err)
	}
	removeOwnedFiles(owned)
	// The prior (read before the remove) says the type and whether it was a tree
	// object; without it the item event still goes out, carrying the id.
	// The event carries the revision the delete removed.
	meta := s.eventMeta(ctx, priorRev)
	s.Events.PublishObject("deleted", in.ID, in.Module, prior.Type, prior.ParentID(), "", api.OperationFrom(ctx), meta)
	if priorErr == nil {
		if !moving(ctx) {
			s.fireAfterHooks(ctx, api.HookOnDelete, prior, &prior, writeEvent(ctx, meta, "deleted", module, prior))
		}
	}
	return api.ObjectDeleteOutput{ID: in.ID}, nil
}

// relPath renders a path relative to the Project root for reporting; falls back
// to the absolute path when it cannot.
func (s *Service) relPath(p string) string {
	if rel, err := filepath.Rel(s.Config.ObjectsRoot(), p); err == nil {
		return filepath.ToSlash(rel)
	}
	return p
}

// publishObjectEvent emits an object write on the lifecycle buses.
//
// The subject is `object.<token>.<action>`, what the EventURI
// `cwm:!workspace/object/<token>/updated` resolves to: the inbox
// suspend/resume handshake waits on it.
//
// The subject carries api.SubjectToken(id), NOT the id: an authored id is
// module-qualified, and its dot is the NATS separator (see subject_token.go).
// The subscriber derives the same token from the same id — they never exchange
// it, so both sides MUST call that one function.
func (s *Service) publishObjectEvent(ctx stdctx.Context, module string, obj api.Object, existed bool, meta api.EventMeta) {
	action := "created"
	if existed {
		action = "updated"
	}
	// Logged together so a NATS trace can be tied back to the object without
	// recomputing the hash by hand.
	slog.Debug("cwm/core: object lifecycle event", "id", obj.ID, "token", api.SubjectToken(obj.ID), "action", action)
	// BOTH streams for a tree object: the item subjects are what the EventURI
	// grammar resolves to and what the inbox handshake waits on; the node
	// subjects are what tree views redraw on (and what the node path's
	// on_create subscriber listens to). A tree object IS both. Every payload
	// carries the object id and its module.
	s.Events.PublishObject(action, obj.ID, module, obj.Type, obj.ParentID(), obj.Slug, api.OperationFrom(ctx), meta)
}

// eventMeta is what a write's event says about the write — minted ONCE per
// verb and passed to both the publish and the hooks: a fresh event id
// (UUID v7: time-ordered), the write's time, the revision it left, and the
// cause the write's context carries — none when it carries none, so an
// uncaused write's event reads as it always did.
func (s *Service) eventMeta(ctx stdctx.Context, revision string) api.EventMeta {
	m := api.EventMeta{At: time.Now().UTC(), Revision: revision}
	if u, err := uuid.NewV7(); err == nil {
		m.EventID = u.String()
	} else {
		m.EventID = uuid.NewString()
	}
	if c, ok := cause.CauseFrom(ctx); ok {
		m.Cause = &c
	}
	return m
}
