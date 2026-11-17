// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"fmt"
	"sort"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	cwmevents "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/internal/events"
)

// THE STRUCTURE EDITS THAT CHANGE IDENTITY (2026-09-25).
//
// A move WITHIN a module is ObjectUpdate rewriting parent, settled in
// phase 7 and needing no verb. These two exist for what is not that: an object
// id is module-qualified, so refiling into another module ends one identity and
// starts another, and splitting one object into several ends one and starts N.
//
// Both require a reason and publish ONE event (`moved` / `split`) naming the
// old and new ids, which the journal keeps, because the one fact a reader
// cannot recover afterwards is that these ids were the same thing. A refactor
// without that record is indistinguishable from a delete and an unrelated
// create.

// ObjectMove refiles an object, minting a new id when the module changes.
// moveKey marks the writes of an ObjectMove: its new identity is created and
// its old one deleted as the same object (see ObjectMove).
type moveKey struct{}

func moving(ctx stdctx.Context) bool { return ctx.Value(moveKey{}) != nil }

func (s *Service) ObjectMove(ctx stdctx.Context, in api.ObjectMoveInput) (api.ObjectMoveOutput, error) {
	if err := s.refuseInChangeSet(ctx, "ObjectMove"); err != nil {
		return api.ObjectMoveOutput{}, err
	}
	if in.ID == "" {
		return api.ObjectMoveOutput{}, fmt.Errorf("%w: ID required", api.ErrInvalidInput)
	}
	if strings.TrimSpace(in.Reason) == "" {
		// A TRAIL WITH NO REASON records only that someone did something,
		// which is the half of the fact nobody needed.
		return api.ObjectMoveOutput{}, fmt.Errorf("%w: Reason required — the trail exists to answer why", api.ErrInvalidInput)
	}
	ix := s.index()
	if err := ix.ensureLoaded(); err != nil {
		return api.ObjectMoveOutput{}, err
	}
	obj, ok := ix.get(in.ID)
	if !ok {
		return api.ObjectMoveOutput{}, api.ObjectNotFound(in.ID)
	}
	// The module it leaves is written too (the one it lands in is checked by
	// the create).
	if err := s.checkReactionWrites(ctx, s.moduleOfObject(in.ID), "ObjectMove"); err != nil {
		return api.ObjectMoveOutput{}, err
	}
	if in.NewParent == in.ID {
		return api.ObjectMoveOutput{}, fmt.Errorf("%w: an object cannot be its own parent", api.ErrInvalidInput)
	}
	if in.NewLocalID != "" {
		// A local id is one path-safe segment: the module comes from the move.
		if strings.ContainsAny(in.NewLocalID, `./\: `) {
			return api.ObjectMoveOutput{}, fmt.Errorf("%w: new local id %q: one segment, without '.', '/', ':' or spaces", api.ErrInvalidInput, in.NewLocalID)
		}
		if !strings.Contains(in.ID, ".") {
			return api.ObjectMoveOutput{}, fmt.Errorf("%w: %q is a module: its id is its folder, not renamed here", api.ErrInvalidInput, in.ID)
		}
		if in.NewModule == "" && in.NewParent == "" {
			in.NewParent = obj.ParentID() // a rename alone keeps the parent
		}
	}
	if in.NewParent != "" {
		if _, ok := ix.get(in.NewParent); !ok {
			return api.ObjectMoveOutput{}, fmt.Errorf("%w: new parent %q", api.ErrNotFound, in.NewParent)
		}
		// A MOVE UNDER ONE'S OWN DESCENDANT detaches the whole subtree from
		// the tree while leaving every file readable — the worst kind of
		// breakage, because nothing errors and the objects simply stop being
		// reachable.
		for _, anc := range s.ancestorsOf(in.NewParent) {
			if anc == in.ID {
				return api.ObjectMoveOutput{}, fmt.Errorf("%w: %q is a descendant of %q; the move would detach the subtree",
					api.ErrCircularReference, in.NewParent, in.ID)
			}
		}
	}

	module, _ := ix.moduleOf(in.ID)
	newModule := in.NewModule
	if newModule == "" {
		newModule = module
	}
	newID := in.ID
	if newModule != module || in.NewLocalID != "" {
		_, local := splitObjectID(in.ID)
		if in.NewLocalID != "" {
			local = in.NewLocalID
		}
		newID = newModule + "." + local
	}
	// NO PRE-CHECK FOR A TAKEN ID. One was written and then removed: ObjectCreate
	// below already refuses an id that exists, with a message naming it and its
	// module, and it is the FIRST write — so a collision leaves nothing behind
	// either way. Falsifying the pre-check changed nothing observable, which is
	// the definition of a guard that misleads.

	out := api.ObjectMoveOutput{ID: newID}
	if newID != in.ID {
		out.PreviousID = in.ID
	}

	// The children and the inbound references are collected BEFORE anything
	// is written: once the object moves, "what pointed at the old id" is a
	// question the index can no longer answer.
	children := ix.childrenOf(in.ID)
	inbound := ix.edges(in.ID, "", api.DirectionIn)
	fromParent := obj.ParentID()

	// THE MOVED OBJECT IS THE STORED ONE, read from its file: the indexed copy
	// is resident metadata only (no body, no type fields), and writing it back
	// would drop the markdown body of a `.cwm.md` object. A move changes where
	// the object sits — parent, module, local id — and keeps everything else.
	// The file's decode is the move's own copy: nothing it changes reaches the
	// index before the write lands.
	moved, _, storedRev, err := s.fullObjectRev(ctx, in.ID)
	if err != nil {
		return api.ObjectMoveOutput{}, err
	}
	moved.ID = newID
	moved.Parent = in.NewParent

	// The moved object's own writes are marked as the move's: the one `moved`
	// event below reports them, so a subscriber (the journal) records the move
	// once. The re-pointed referrers are writes of their own.
	asMove := api.WithOperation(ctx, api.OperationMove)
	// The new identity's create and the old one's delete are the SAME object
	// moving, not a birth and a death: it keeps its state (no initial-state
	// check) and no create / delete hook fires. The mark is private — a caller
	// cannot claim it; the operation above only labels the events.
	asMove = stdctx.WithValue(asMove, moveKey{}, true)
	var movedRev string // the revision the moved object has once written
	if newID == in.ID {
		// On the revision read: a write landing in between is a conflict, not
		// overwritten.
		upd, err := s.ObjectUpdate(asMove, api.ObjectUpdateInput{Module: module, Object: moved, Revision: storedRev})
		if err != nil {
			return api.ObjectMoveOutput{}, err
		}
		movedRev = upd.Revision
	} else {
		// A NEW IDENTITY: written first, removed second. If the create fails
		// the old object is still there, which is the recoverable order.
		// Its own edges that name itself follow it too.
		for i := range moved.Edges {
			if moved.Edges[i].To == in.ID {
				moved.Edges[i].To = newID
			}
		}
		owned := s.ownedFilePaths(ctx, in.ID)
		created, err := s.ObjectCreate(asMove, api.ObjectCreateInput{Module: newModule, Object: moved})
		if err != nil {
			return api.ObjectMoveOutput{}, err
		}
		movedRev = created.Revision
		// Its owned files (memory, comments) follow it before the old object,
		// which would take them with it, is removed.
		if err := carryOwnedFiles(owned, s.ownedFilePaths(ctx, newID)); err != nil {
			return api.ObjectMoveOutput{}, fmt.Errorf("the new object %s was written but its files could not follow: %w", newID, err)
		}
		if _, err := s.deleteObject(asMove, api.ObjectDeleteInput{Module: module, ID: in.ID}); err != nil {
			return api.ObjectMoveOutput{}, fmt.Errorf("the new object %s was written but the old one could not be removed: %w", newID, err)
		}
	}

	// Re-point everything that named the old id. A move that leaves nine
	// dangling references has not moved anything, it has broken it.
	if newID != in.ID {
		rewritten, dangling := s.repointReferences(ctx, in.ID, newID, children, inbound)
		out.Rewritten, out.Dangling = rewritten, dangling
	}

	// ONE event says what happened, whatever the move took underneath.
	s.Events.PublishObjectMoved(cwmevents.ObjectMovedPayload{
		EventMeta: s.eventMeta(ctx, movedRev),
		ObjectID:  newID, Module: newModule, FromID: in.ID, FromModule: module,
		Parent: in.NewParent, FromParent: fromParent, Type: obj.Type,
		Reason: in.Reason,
	})
	return out, nil
}

// repointReferences rewrites every child's parent and every inbound edge that
// named the old id.
func (s *Service) repointReferences(ctx stdctx.Context, oldID, newID string, children []api.Object, inbound []api.Edge) (rewritten, dangling []string) {
	seen := map[string]bool{}
	note := func(id string, err error) {
		if err != nil {
			dangling = append(dangling, id+": "+err.Error())
			return
		}
		if !seen[id] {
			seen[id] = true
			rewritten = append(rewritten, id)
		}
	}
	for _, child := range children {
		err := s.updateFresh(ctx, child.ID, func(o *api.Object) bool {
			o.Parent = newID
			return true
		})
		note(child.ID, err)
	}
	for _, e := range inbound {
		if e.From == oldID {
			continue // its own edge: rewritten when the new identity was written
		}
		src, ok := s.index().get(e.From)
		if !ok {
			dangling = append(dangling, e.From+": no longer indexed")
			continue
		}
		changed := false
		err := s.updateFresh(ctx, src.ID, func(o *api.Object) bool {
			for i := range o.Edges {
				if o.Edges[i].To == oldID {
					o.Edges[i].To = newID
					changed = true
				}
			}
			return changed
		})
		if changed {
			note(src.ID, err)
		}
	}
	sort.Strings(rewritten)
	sort.Strings(dangling)
	return rewritten, dangling
}

// ObjectSplit replaces one object with several.
func (s *Service) ObjectSplit(ctx stdctx.Context, in api.ObjectSplitInput) (api.ObjectSplitOutput, error) {
	if err := s.refuseInChangeSet(ctx, "ObjectSplit"); err != nil {
		return api.ObjectSplitOutput{}, err
	}
	if in.ID == "" {
		return api.ObjectSplitOutput{}, fmt.Errorf("%w: ID required", api.ErrInvalidInput)
	}
	if len(in.Successors) < 2 {
		return api.ObjectSplitOutput{}, fmt.Errorf("%w: a split needs at least two successors; one is a rename", api.ErrInvalidInput)
	}
	if strings.TrimSpace(in.Reason) == "" {
		return api.ObjectSplitOutput{}, fmt.Errorf("%w: Reason required — the trail exists to answer why", api.ErrInvalidInput)
	}
	ix := s.index()
	if err := ix.ensureLoaded(); err != nil {
		return api.ObjectSplitOutput{}, err
	}
	source, ok := ix.get(in.ID)
	if !ok {
		return api.ObjectSplitOutput{}, api.ObjectNotFound(in.ID)
	}
	module, _ := ix.moduleOf(in.ID)
	parent := source.ParentID()

	// EVERY NAMED CHILD MUST BE ONE, checked before anything is written: a
	// split that half-succeeded because the fourth successor named a typo
	// leaves a shape nobody asked for.
	own := map[string]bool{}
	for _, c := range ix.childrenOf(in.ID) {
		own[c.ID] = true
	}
	claimed := map[string]string{}
	for _, succ := range in.Successors {
		if succ.Type == "" || succ.Title == "" {
			return api.ObjectSplitOutput{}, fmt.Errorf("%w: every successor needs a type and a title", api.ErrInvalidInput)
		}
		for _, child := range succ.Children {
			if !own[child] {
				return api.ObjectSplitOutput{}, fmt.Errorf("%w: %q is not a child of %q", api.ErrInvalidInput, child, in.ID)
			}
			if other, dup := claimed[child]; dup {
				return api.ObjectSplitOutput{}, fmt.Errorf("%w: child %q is claimed by two successors (%q and %q)",
					api.ErrInvalidInput, child, other, succ.Title)
			}
			claimed[child] = succ.Title
		}
	}

	out := api.ObjectSplitOutput{SourceKept: !in.DeleteSource}
	childOwner := map[string]string{}
	for _, succ := range in.Successors {
		o := api.Object{ID: succ.ID, Type: succ.Type, Name: succ.Title, Description: succ.Description, Parent: parent}
		created, err := s.ObjectCreate(ctx, api.ObjectCreateInput{Module: module, Object: o})
		if err != nil {
			return api.ObjectSplitOutput{}, fmt.Errorf("successor %q: %w", succ.Title, err)
		}
		out.Successors = append(out.Successors, created.Object.ID)
		for _, child := range succ.Children {
			childOwner[child] = created.Object.ID
		}
	}

	for child, owner := range childOwner {
		if _, ok := ix.get(child); !ok {
			continue
		}
		if err := s.updateFresh(ctx, child, func(o *api.Object) bool {
			o.Parent = owner
			return true
		}); err != nil {
			return out, fmt.Errorf("re-parenting %s: %w", child, err)
		}
		out.Rewritten = append(out.Rewritten, child)
	}
	sort.Strings(out.Rewritten)

	// A child NO successor adopted stays where it is and is NAMED: a split
	// that quietly stranded three children would look successful.
	for id := range own {
		if _, adopted := childOwner[id]; !adopted {
			out.Orphans = append(out.Orphans, id)
		}
	}
	sort.Strings(out.Orphans)

	var sourceRev string // the source's revision: kept as it is, or removed
	if in.DeleteSource {
		if len(out.Orphans) > 0 {
			return out, fmt.Errorf("%w: %d of %q's children were adopted by no successor, so deleting it would strand them: %s",
				api.ErrInvalidInput, len(out.Orphans), in.ID, strings.Join(out.Orphans, ", "))
		}
		// The source's removal is the split's own: its one event reports it,
		// carrying the revision removed.
		if p, ok := ix.pathOf(in.ID); ok {
			sourceRev, _ = fileRevision(p)
		}
		asSplit := api.WithOperation(ctx, api.OperationSplit)
		if _, err := s.ObjectDelete(asSplit, api.ObjectDeleteInput{Module: module, ID: in.ID}); err != nil {
			return out, fmt.Errorf("the successors were written but the source could not be removed: %w", err)
		}
		out.SourceKept = false
	}

	// ONE event says what happened: the source, its successors, and why. The
	// successors' creates and the children's re-parenting are writes of their
	// own; the source's removal (if any) is reported by this event.
	if out.SourceKept {
		if p, ok := ix.pathOf(in.ID); ok {
			sourceRev, _ = fileRevision(p)
		}
	}
	s.Events.PublishObjectSplit(cwmevents.ObjectSplitPayload{
		EventMeta: s.eventMeta(ctx, sourceRev),
		ObjectID:  in.ID, Module: module, Type: source.Type, Parent: parent,
		Successors: out.Successors, SourceKept: out.SourceKept, Reason: in.Reason,
	})
	return out, nil
}

// splitObjectID splits `<module>.<local>` on the FIRST dot; a bare id has no
// module.
func splitObjectID(id string) (module, local string) {
	if i := strings.Index(id, "."); i > 0 {
		return id[:i], id[i+1:]
	}
	return "", id
}
