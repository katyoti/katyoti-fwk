// SPDX-License-Identifier: Apache-2.0

package storeset

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/internal/core"
)

// Served is the CWM service of a universe serving several projects
// : the one api.CWMService in front of the set, and CWM's one
// write seam - the port the universe database later sits
// behind. It embeds no store: it declares every verb of api.CWMService itself
// (served_verbs.go), so a new verb does not compile until it is routed. Each
// verb goes where its object lives:
//
//   - a verb of one object, to the store holding it (an edge, to its source's;
//     a move stays in its project);
//   - a verb of a module, to the store holding the module;
//   - a verb across objects - list, search, scan, tree, errors, outbox,
//     change sets, migrations, events - to every store, the answers joined;
//   - a type, aspect or declaration read, to the first store (types are one
//     namespace: every store reads the same declarations);
//   - project creation is refused: each member has its project already.
//
// Each lifecycle step (observers, match takers, bootstrap hooks, type
// refresh, close) runs on every store.
//
// An id is written bare (the only project holding it) or qualified
// <key>:<module>.<local>; a module bare or <key>:<module>. New input naming
// the one object or module a verb acts on - an object verb's id, an edge's
// source, a tree's root, a resolve's ref, a create's module, a module's
// subscribers - written bare when several projects hold it is refused
// (Ambiguous), naming each candidate's qualified form; written
// qualified it goes through. What names an object beside one already placed
// (an edge's target, a move's new parent, a split's successors, a create's
// parent under its module) is read in that object's project. A filter of a
// joined answer - a list's parent, a search's within, a module of the error
// list or of run hooks - matches in every project holding it, each row naming
// its project; qualified, in its key's project alone. The joined answers
// of list, search and errors key an object by its qualified form (States,
// InError, ObjectErrorList) and name each object's project (Projects), as a
// read names its object's (ObjectGetOutput.Project), so two
// projects holding one bare id never blur there; the other joined answers
// (edges listed with no object, the rootless tree, change-set and outbox
// writes) still carry bare ids.
type Served struct {
	primary *core.Service // the first store: the type namespace's reader
	set     *Set
}

var _ api.CWMService = (*Served)(nil)

// NewServed fronts the set; its first store is the primary.
func NewServed(set *Set) (*Served, error) {
	if set == nil || len(set.keys) == 0 {
		return nil, fmt.Errorf("%w: served store set: no store", api.ErrInvalidInput)
	}
	return &Served{primary: set.byKey[set.keys[0]], set: set}, nil
}

// Set is the store set served.
func (s *Served) Set() *Set { return s.set }

// splitID reads an id written bare or qualified: its project key ("" when
// bare) and its bare id.
func splitID(id string) (string, string, error) {
	if !strings.Contains(id, ":") {
		return "", id, nil
	}
	return api.ParseQualifiedID(id)
}

// splitModule reads a module written bare or <key>:<module>.
func splitModule(module string) (string, string, error) {
	i := strings.Index(module, ":")
	if i < 0 {
		return "", module, nil
	}
	key, m := module[:i], module[i+1:]
	if key == "" || m == "" || strings.Contains(m, ":") {
		return "", "", fmt.Errorf("%w: module %q: write it <key>:<module>", api.ErrInvalidInput, module)
	}
	return key, m, nil
}

// keyedModule takes the project key off a module written <key>:<module>
// beside an object id whose key is key: a module keyed in another project
// than the id is refused, an equal key or a bare module goes through, a key on
// the module alone names the store. verb words the refusal (read, updated, deleted).
func keyedModule(key, id, module, verb string) (string, string, error) {
	mk, m, err := splitModule(module)
	if err != nil {
		return "", "", err
	}
	if mk == "" {
		return key, module, nil
	}
	if key != "" && key != mk {
		return "", "", fmt.Errorf("%w: object %q is %s in project %q, its module in %q", api.ErrInvalidInput, id, verb, key, mk)
	}
	return mk, m, nil
}

// --- One object -------------------------------------------------------------

// ObjectGet reads one object from the store holding it and names that store's
// project key (Project), a bare id's as a qualified one's.
func (s *Served) ObjectGet(ctx context.Context, in api.ObjectGetInput) (api.ObjectGetOutput, error) {
	key, id, err := splitID(in.ID)
	if err != nil {
		return api.ObjectGetOutput{}, err
	}
	in.ID = id
	if key, in.Module, err = keyedModule(key, in.ID, in.Module, "read"); err != nil {
		return api.ObjectGetOutput{}, err
	}
	key, out, err := s.set.ObjectGet(ctx, key, in)
	out.Project = key
	return out, err
}

func (s *Served) ObjectCreate(ctx context.Context, in api.ObjectCreateInput) (api.ObjectCreateOutput, error) {
	key, m, err := splitModule(in.Module)
	if err != nil {
		return api.ObjectCreateOutput{}, err
	}
	in.Module = m
	if strings.Contains(in.Object.ID, ":") {
		k, id, err := api.ParseQualifiedID(in.Object.ID)
		if err != nil {
			return api.ObjectCreateOutput{}, err
		}
		if key != "" && k != key {
			return api.ObjectCreateOutput{}, fmt.Errorf("%w: object %q is created in project %q, its module in %q", api.ErrInvalidInput, in.Object.ID, k, key)
		}
		key, in.Object.ID = k, id
	}
	if strings.Contains(in.Object.Parent, ":") {
		k, parent, err := api.ParseQualifiedID(in.Object.Parent)
		if err != nil {
			return api.ObjectCreateOutput{}, err
		}
		if key != "" && k != key {
			return api.ObjectCreateOutput{}, fmt.Errorf("%w: object %q is created in project %q, its parent %q in %q: a write never crosses projects", api.ErrInvalidInput, in.Object.ID, key, in.Object.Parent, k)
		}
		key, in.Object.Parent = k, parent
	}
	_, out, err := s.set.ObjectCreate(ctx, key, in)
	return out, err
}

func (s *Served) ObjectUpdate(ctx context.Context, in api.ObjectUpdateInput) (api.ObjectUpdateOutput, error) {
	key, id, err := splitID(in.Object.ID)
	if err != nil {
		return api.ObjectUpdateOutput{}, err
	}
	in.Object.ID = id
	if key, in.Module, err = keyedModule(key, id, in.Module, "updated"); err != nil {
		return api.ObjectUpdateOutput{}, err
	}
	_, out, err := s.set.ObjectUpdate(ctx, key, in)
	return out, err
}

func (s *Served) ObjectDelete(ctx context.Context, in api.ObjectDeleteInput) (api.ObjectDeleteOutput, error) {
	key, id, err := splitID(in.ID)
	if err != nil {
		return api.ObjectDeleteOutput{}, err
	}
	in.ID = id
	if key, in.Module, err = keyedModule(key, id, in.Module, "deleted"); err != nil {
		return api.ObjectDeleteOutput{}, err
	}
	_, out, err := s.set.ObjectDelete(ctx, key, in)
	return out, err
}

// ObjectResolve resolves a reference with no container named: a qualified id
// or a full name in its project's store, a bare id in the only store holding
// it.
func (s *Served) ObjectResolve(ctx context.Context, in api.ObjectResolveInput) (api.ObjectResolveOutput, error) {
	_, out, err := s.set.Resolve(ctx, "", in.Ref)
	return out, err
}

// --- Across the stores ------------------------------------------------------

// ObjectList lists every store's objects; Projects names each one's project,
// in the order of Objects; InError names them by their qualified ids.
func (s *Served) ObjectList(ctx context.Context, in api.ObjectListInput) (api.ObjectListOutput, error) {
	got, err := s.set.ObjectList(ctx, in)
	if err != nil {
		return api.ObjectListOutput{}, err
	}
	out := api.ObjectListOutput{Total: got.Total, Truncated: got.Truncated}
	out.Objects, out.Projects, out.InError = unlocate(got.Objects)
	return out, nil
}

// ObjectSearch searches every store; Projects names each object's project, in
// the order of Objects. States and InError key each object by its qualified
// id: two projects may hold one bare id.
func (s *Served) ObjectSearch(ctx context.Context, in api.ObjectSearchInput) (api.ObjectSearchOutput, error) {
	got, err := s.set.ObjectSearch(ctx, in)
	if err != nil {
		return api.ObjectSearchOutput{}, err
	}
	out := api.ObjectSearchOutput{Total: got.Total, Capped: got.Capped}
	out.Objects, out.Projects, out.InError = unlocate(got.Objects)
	for _, l := range got.Objects {
		if l.Hit != nil {
			out.Hits = append(out.Hits, *l.Hit)
		}
		if l.State != nil {
			if out.States == nil {
				out.States = map[string]api.ObjectState{}
			}
			out.States[api.QualifiedID(l.Project, l.Object.ID)] = *l.State
		}
	}
	if len(out.Hits) != len(out.Objects) {
		out.Hits = nil // hits come in the order of objects, or not at all
	}
	return out, nil
}

func unlocate(ls []Located) (objs []api.Object, projects []string, inError []string) {
	objs = make([]api.Object, 0, len(ls))
	projects = make([]string, 0, len(ls))
	for _, l := range ls {
		objs = append(objs, l.Object)
		projects = append(projects, l.Project)
		if l.InError {
			inError = append(inError, api.QualifiedID(l.Project, l.Object.ID))
		}
	}
	return objs, projects, inError
}

// --- Lifecycle: every store -------------------------------------------------

func (s *Served) each(fn func(key string, st *core.Service)) {
	for _, k := range s.set.keys {
		fn(k, s.set.byKey[k])
	}
}

// AttachObservers attaches the observers to every store.
func (s *Served) AttachObservers(ctx context.Context, observers []api.EventObserver) {
	s.each(func(_ string, st *core.Service) { st.AttachObservers(ctx, observers) })
}

// AttachMatchTakers attaches the match takers to every store.
func (s *Served) AttachMatchTakers(ctx context.Context, takers []api.MatchTaker) {
	s.each(func(_ string, st *core.Service) { st.AttachMatchTakers(ctx, takers) })
}

// RunBootstrapHooks runs each store's bootstrap hooks on its own project
// object; an error names its project.
func (s *Served) RunBootstrapHooks(ctx context.Context, part core.BootstrapPart) []error {
	var errs []error
	s.each(func(k string, st *core.Service) {
		for _, e := range st.RunBootstrapHooks(ctx, part) {
			errs = append(errs, fmt.Errorf("project %q: %w", k, e))
		}
	})
	return errs
}

// EnsureProject gives every store's project its project object when it has
// none; true when one was created.
func (s *Served) EnsureProject(ctx context.Context) (bool, error) {
	created := false
	var errs []error
	s.each(func(k string, st *core.Service) {
		c, err := st.EnsureProject(ctx)
		created = created || c
		if err != nil {
			errs = append(errs, fmt.Errorf("project %q: %w", k, err))
		}
	})
	return created, errors.Join(errs...)
}

// RefreshCatalogTypes adds the types newly contributed to every store.
func (s *Served) RefreshCatalogTypes(cts core.CatalogTypeSet) {
	s.each(func(_ string, st *core.Service) { st.RefreshCatalogTypes(cts) })
}

// Close closes every store.
func (s *Served) Close() error {
	var errs []error
	s.each(func(k string, st *core.Service) {
		if err := st.Close(); err != nil {
			errs = append(errs, fmt.Errorf("project %q: %w", k, err))
		}
	})
	return errors.Join(errs...)
}
