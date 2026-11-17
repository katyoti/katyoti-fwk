// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/reviews/api"
)

// Supersede keeps in.ID and the review before it of its type under the object
// reviewed; each older review of that type is folded into the object's
// reviewable history (one entry: its type, day, commit, rating, score, its
// findings and the open ones by severity), then removed with its children and
// detached — the found-in edges that pointed at it go, the findings stay. The
// history is written with the revision read, so a concurrent write is never
// erased.
func (s *Service) Supersede(ctx context.Context, in api.SupersedeInput) (api.SupersedeOutput, error) {
	c, err := cwmOf(ctx)
	if err != nil {
		return api.SupersedeOutput{}, err
	}
	obj, err := get(ctx, c, in.ID)
	if err != nil {
		return api.SupersedeOutput{}, err
	}
	if obj.Type != api.ReviewType {
		return api.SupersedeOutput{}, fmt.Errorf("%w: %s is a %s, not a review", cwmapi.ErrInvalidInput, obj.ID, obj.Type)
	}
	var fields api.ReviewFields
	decodeFields(obj, &fields)
	if fields.Type == "" || obj.Parent == "" {
		return api.SupersedeOutput{}, fmt.Errorf("%w: %s names no review type or no object reviewed", cwmapi.ErrInvalidInput, obj.ID)
	}
	siblings, err := children(ctx, c, obj.Parent, api.ReviewType)
	if err != nil {
		return api.SupersedeOutput{}, err
	}
	var same []ReviewOf
	for _, o := range siblings {
		r, err := ReadReview(ctx, c, o)
		if err != nil {
			return api.SupersedeOutput{}, err
		}
		if r.Fields.Type == fields.Type {
			same = append(same, r)
		}
	}
	// Newest first; the review just filed wins a tie with an older one.
	sort.SliceStable(same, func(i, j int) bool {
		a, b := same[i], same[j]
		if a.Fields.ReviewedAt != b.Fields.ReviewedAt {
			return a.Fields.ReviewedAt > b.Fields.ReviewedAt
		}
		if (a.Object.ID == obj.ID) != (b.Object.ID == obj.ID) {
			return a.Object.ID == obj.ID
		}
		return a.Object.ID > b.Object.ID
	})
	out := api.SupersedeOutput{Current: same[0].Object.ID}
	if len(same) > 1 {
		out.Previous = same[1].Object.ID
	}
	if len(same) <= 2 {
		return out, nil
	}
	old := same[2:]
	// Oldest first in the history.
	sort.SliceStable(old, func(i, j int) bool { return old[i].Fields.ReviewedAt < old[j].Fields.ReviewedAt })
	if err := appendHistory(ctx, c, obj.Parent, old); err != nil {
		return out, err
	}
	out.History = obj.Parent
	files := newFileSet(ctx, c)
	files.add(obj.Parent)
	detached := map[string]bool{}
	removed := map[string]bool{}
	for _, r := range old {
		if err := remove(ctx, c, r.Object, files, removed, detached); err != nil {
			return out, fmt.Errorf("fold %s: %w", r.Object.ID, err)
		}
		out.Folded = append(out.Folded, r.Object.ID)
	}
	for id := range removed {
		out.Removed = append(out.Removed, id)
	}
	sort.Strings(out.Removed)
	for id := range detached {
		if !removed[id] {
			out.Detached = append(out.Detached, id)
			files.add(id)
		}
	}
	sort.Strings(out.Detached)
	out.Files = files.sorted()
	return out, nil
}

// fileSet gathers the project-relative files of the objects a filing
// changes, resolved while each object still exists.
type fileSet struct {
	ctx   context.Context
	c     cwmapi.CWMService
	paths map[string]bool
}

func newFileSet(ctx context.Context, c cwmapi.CWMService) *fileSet {
	return &fileSet{ctx: ctx, c: c, paths: map[string]bool{}}
}

// add records the file of the object id; an object CWM cannot resolve adds
// nothing (its id is still reported).
func (f *fileSet) add(id string) {
	got, err := f.c.ObjectResolve(f.ctx, cwmapi.ObjectResolveInput{Ref: id})
	if err != nil || got.Path == "" {
		return
	}
	p := got.Path
	if got.ProjectRoot != "" {
		if rel, err := filepath.Rel(got.ProjectRoot, p); err == nil {
			p = rel
		}
	}
	f.paths[filepath.ToSlash(p)] = true
}

func (f *fileSet) merge(paths []string) {
	for _, p := range paths {
		f.paths[p] = true
	}
}

func (f *fileSet) sorted() []string {
	out := make([]string, 0, len(f.paths))
	for p := range f.paths {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// historyEntry is a review as the history keeps it.
func historyEntry(r ReviewOf) api.HistoryEntry {
	e := api.HistoryEntry{Review: r.Object.ID, Type: r.Fields.Type, ReviewedAt: r.Fields.ReviewedAt,
		Commit: r.Fields.Commit, Rating: r.Fields.Rating, Score: r.Fields.Score, Findings: len(r.Findings)}
	for _, f := range r.Findings {
		if f.Open() {
			if e.Open == nil {
				e.Open = map[string]int{}
			}
			e.Open[f.Fields.Severity]++
		}
	}
	return e
}

// appendHistory adds the reviews to the object's reviewable history.
func appendHistory(ctx context.Context, c cwmapi.CWMService, id string, reviews []ReviewOf) error {
	module, err := cwmapi.ModuleOfObjectID(id)
	if err != nil {
		return err
	}
	got, err := c.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: module, ID: id})
	if err != nil {
		return err
	}
	o := got.Object
	var cur api.ReviewableFields
	if g := o.Capabilities[api.Capability]; len(g) > 0 {
		b, _ := json.Marshal(g)
		_ = json.Unmarshal(b, &cur)
	}
	for _, r := range reviews {
		cur.History = append(cur.History, historyEntry(r))
	}
	b, err := json.Marshal(cur)
	if err != nil {
		return err
	}
	var group map[string]any
	if err := json.Unmarshal(b, &group); err != nil {
		return err
	}
	if o.Capabilities == nil {
		o.Capabilities = map[string]map[string]any{}
	}
	o.Capabilities[api.Capability] = group
	_, err = c.ObjectUpdate(ctx, cwmapi.ObjectUpdateInput{Module: module, Object: o, Revision: got.Revision})
	return err
}

// remove deletes a review with its children (its own findings, documents and
// decks), each detached from what links to it; it records each removed
// object, each object whose links it cut, and the removed files.
func remove(ctx context.Context, c cwmapi.CWMService, o cwmapi.Object, files *fileSet, removed, detached map[string]bool) error {
	module, err := cwmapi.ModuleOfObjectID(o.ID)
	if err != nil {
		return err
	}
	kids, err := c.ObjectList(ctx, cwmapi.ObjectListInput{Parent: o.ID})
	if err != nil {
		return err
	}
	for _, k := range kids.Objects {
		if err := remove(ctx, c, k, files, removed, detached); err != nil {
			return err
		}
	}
	files.add(o.ID)
	got, err := c.ObjectDelete(ctx, cwmapi.ObjectDeleteInput{Module: module, ID: o.ID, Detach: true})
	if err != nil {
		return err
	}
	removed[o.ID] = true
	for _, id := range got.Detached {
		detached[id] = true
	}
	return nil
}
