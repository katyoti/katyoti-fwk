// SPDX-License-Identifier: Apache-2.0

// Package storeset joins several CWM stores - one per project - into one
// place to read and write.
//
// A CWM store (core.Service) serves ONE project folder: one corpus root, one
// store root, one index. A universe serving several projects - two project
// folders of one repository, or two repositories - opens one store per
// project and joins them HERE, above the core: the set knows each store by
// its project key (the project object's id), lists and
// searches across every store, and routes a read or a write of one object to
// the store holding it. No root is ever passed through the core to another
// store's folder: the join is this package's, the core stays single-root
// (internal/test/arch TestCWMCoreKeepsOneRootPerStore).
//
// A bare id or module that several projects hold is refused here (Ambiguous,
// ), naming the projects and each candidate's qualified form.
// A reference is resolved by Resolve: <key>:<module>.<local> in
// the store of its key, a bare id in the store of the project that wrote it.
package storeset

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/internal/core"
)

// Set is several stores, one per project, joined: the dispatcher above them.
// Its stores are fixed at New; it never opens or closes one.
type Set struct {
	keys  []string // in the order given
	byKey map[string]*core.Service
}

// New joins the stores given. Each must serve a project of its own: a project
// object naming its key, a corpus root and a store root no other store of the
// set serves - two stores over one folder would write each other's files.
func New(stores ...*core.Service) (*Set, error) {
	s := &Set{byKey: map[string]*core.Service{}}
	corpus := map[string]string{}
	store := map[string]string{}
	for _, st := range stores {
		if st == nil {
			return nil, fmt.Errorf("%w: store set: a nil store", api.ErrInvalidInput)
		}
		key := st.ProjectKey()
		if key == "" {
			return nil, fmt.Errorf("%w: store set: the store at %s serves no project object, so it has no key to address it by (cwm project-init)", api.ErrInvalidInput, st.CorpusRoot())
		}
		if _, dup := s.byKey[key]; dup {
			return nil, fmt.Errorf("%w: store set: two stores serve the project %q", api.ErrInvalidInput, key)
		}
		c := cleanRoot(st.CorpusRoot())
		if other, dup := corpus[c]; dup {
			return nil, fmt.Errorf("%w: store set: projects %q and %q are served from one folder %s", api.ErrInvalidInput, other, key, st.CorpusRoot())
		}
		corpus[c] = key
		if r := cleanRoot(st.StoreRoot()); r != "" {
			if other, dup := store[r]; dup {
				return nil, fmt.Errorf("%w: store set: projects %q and %q keep their runtime objects in one store %s", api.ErrInvalidInput, other, key, st.StoreRoot())
			}
			store[r] = key
		}
		s.byKey[key] = st
		s.keys = append(s.keys, key)
	}
	// Each store's observers name its project from now on (they already hear
	// only its events): a reaction writes back into the project it came from.
	for _, st := range stores {
		st.ServeAsMember()
	}
	return s, nil
}

func cleanRoot(p string) string {
	if p == "" {
		return ""
	}
	if abs, err := filepath.Abs(p); err == nil {
		p = abs
	}
	p = filepath.Clean(p)
	if runtime.GOOS == "windows" {
		p = strings.ToLower(p)
	}
	return p
}

// Keys are the projects of the set, in the order their stores were given.
func (s *Set) Keys() []string { return append([]string(nil), s.keys...) }

// Store is the store serving the project key.
func (s *Set) Store(key string) (*core.Service, bool) {
	st, ok := s.byKey[key]
	return st, ok
}

// Located is one object and the project whose store holds it.
type Located struct {
	Project string     `json:"project"`
	Object  api.Object `json:"object"`
	// Hit is why it matched, for a fulltext search.
	Hit *api.ObjectSearchHit `json:"hit,omitempty"`
	// State is its lifecycle state, for a search asked WithStates.
	State *api.ObjectState `json:"state,omitempty"`
	// InError says the object carries errors (ObjectGet gives them).
	InError bool `json:"in_error,omitempty"`
}

// --- Routing ----------------------------------------------------------------

// byProject answers the store of an explicit key.
func (s *Set) byProject(key string) (string, *core.Service, error) {
	st, ok := s.byKey[key]
	if !ok {
		return "", nil, fmt.Errorf("%w: project %q is not served here (served: %s)", api.ErrNotFound, key, strings.Join(s.keys, ", "))
	}
	return key, st, nil
}

// holding answers the one store holding what `holds` says it holds: the
// store of `project` when one is named, else the only store that holds it.
// Several holders are refused: the refusal names each candidate
// as qualify(key) - the qualified form to write instead.
func (s *Set) holding(project, what string, holds func(*core.Service) (bool, error), qualify func(key string) string) (string, *core.Service, error) {
	if project != "" {
		return s.byProject(project)
	}
	var found []string
	for _, k := range s.keys {
		ok, err := holds(s.byKey[k])
		if err != nil {
			return "", nil, fmt.Errorf("project %q: %w", k, err)
		}
		if ok {
			found = append(found, k)
		}
	}
	switch len(found) {
	case 0:
		return "", nil, fmt.Errorf("%w: %s is in no served project (%s)", api.ErrNotFound, what, strings.Join(s.keys, ", "))
	case 1:
		return found[0], s.byKey[found[0]], nil
	default:
		return "", nil, Ambiguous(what, found, qualify)
	}
}

// Ambiguous is the refusal of a bare id or module that several served
// projects hold: invalid input naming each project and each
// candidate's qualified form, in the set's order - what the CLI, the GUI and
// an agent's tool all meet, since each goes through the one CWM service.
func Ambiguous(what string, keys []string, qualify func(key string) string) error {
	cands := make([]string, len(keys))
	for i, k := range keys {
		cands[i] = qualify(k)
	}
	return fmt.Errorf("%w: %s is in several projects (%s): name the project - write one of %s", api.ErrInvalidInput, what, strings.Join(keys, ", "), strings.Join(cands, ", "))
}

func (s *Set) holdingID(project, id string) (string, *core.Service, error) {
	return s.holding(project, fmt.Sprintf("object %q", id), func(st *core.Service) (bool, error) { return st.Holds(id) },
		func(k string) string { return api.QualifiedID(k, id) })
}

func (s *Set) holdingModule(project, module string) (string, *core.Service, error) {
	return s.holding(project, fmt.Sprintf("module %q", module), func(st *core.Service) (bool, error) { return st.HoldsModule(module) },
		func(k string) string { return k + ":" + module })
}

// qualifiedAt is the served project of a qualified id <key>:<id>, and the id
// bare.
func (s *Set) qualifiedAt(id string) (string, string, error) {
	key, bare, err := api.ParseQualifiedID(id)
	if err != nil {
		return "", "", err
	}
	if _, _, err := s.byProject(key); err != nil {
		return "", "", err
	}
	return key, bare, nil
}

// --- One object -------------------------------------------------------------

// ObjectGet reads one object from the store holding it: project's when named,
// else the only one whose index holds the id.
func (s *Set) ObjectGet(ctx context.Context, project string, in api.ObjectGetInput) (string, api.ObjectGetOutput, error) {
	key, st, err := s.holdingID(project, in.ID)
	if err != nil {
		return "", api.ObjectGetOutput{}, err
	}
	out, err := st.ObjectGet(ctx, in)
	return key, out, err
}

// ObjectCreate writes a new object in project's store when named, else in the
// only store whose project has the module.
func (s *Set) ObjectCreate(ctx context.Context, project string, in api.ObjectCreateInput) (string, api.ObjectCreateOutput, error) {
	key, st, err := s.holdingModule(project, in.Module)
	if err != nil {
		return "", api.ObjectCreateOutput{}, err
	}
	out, err := st.ObjectCreate(ctx, in)
	return key, out, err
}

// ObjectUpdate rewrites an object in the store holding it.
func (s *Set) ObjectUpdate(ctx context.Context, project string, in api.ObjectUpdateInput) (string, api.ObjectUpdateOutput, error) {
	key, st, err := s.holdingID(project, in.Object.ID)
	if err != nil {
		return "", api.ObjectUpdateOutput{}, err
	}
	out, err := st.ObjectUpdate(ctx, in)
	return key, out, err
}

// ObjectDelete removes an object from the store holding it.
func (s *Set) ObjectDelete(ctx context.Context, project string, in api.ObjectDeleteInput) (string, api.ObjectDeleteOutput, error) {
	key, st, err := s.holdingID(project, in.ID)
	if err != nil {
		return "", api.ObjectDeleteOutput{}, err
	}
	out, err := st.ObjectDelete(ctx, in)
	return key, out, err
}

// --- References -------------------------------------------------

// Resolve answers which object, in which project, a reference written in an
// object of project `from` names - the "which file is this" of ObjectResolve,
// joined over the stores:
//
//   - a qualified id <key>:<module>.<local> resolves in the store of its key;
//   - a full name <project-uuid>.<module>.<local> in the store of that uuid;
//   - a bare id is relative to its container: it resolves in from's store
//     only - never in another project holding the same id - so every edge
//     written before several projects existed keeps naming what it named.
//     With no container named it resolves in the only store holding it, and
//     several holders are refused.
func (s *Set) Resolve(ctx context.Context, from, ref string) (string, api.ObjectResolveOutput, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", api.ObjectResolveOutput{}, fmt.Errorf("%w: ref required", api.ErrInvalidInput)
	}
	var (
		key string
		st  *core.Service
		err error
	)
	switch {
	case api.IsFullName(ref):
		pid, _, _ := api.ParseFullName(ref)
		key, st, err = s.byUUID(pid)
	case strings.Contains(ref, ":"):
		var k string
		if k, _, err = api.ParseQualifiedID(ref); err == nil {
			key, st, err = s.byProject(k)
		}
	case from != "":
		key, st, err = s.byProject(from)
	default:
		key, st, err = s.holdingID("", ref)
	}
	if err != nil {
		return "", api.ObjectResolveOutput{}, err
	}
	out, err := st.ObjectResolve(ctx, api.ObjectResolveInput{Ref: ref})
	if err != nil {
		if from != "" && key == from && !strings.Contains(ref, ":") && errors.Is(err, api.ErrNotFound) {
			err = fmt.Errorf("%w (a bare id names an object of its own project %q; one of another project is written <key>:%s)", err, from, ref)
		}
		return "", api.ObjectResolveOutput{}, err
	}
	return key, out, nil
}

// Reference is how an object of project `from` writes a reference to the
// object id of project `to`: bare inside its project, <to>:<id> across.
func (s *Set) Reference(from, to, id string) (string, error) {
	if _, _, err := s.byProject(to); err != nil {
		return "", err
	}
	if from != "" {
		if _, _, err := s.byProject(from); err != nil {
			return "", err
		}
	}
	if _, err := api.ModuleOfObjectID(id); err != nil {
		return "", err
	}
	return api.Reference(from, to, id), nil
}

// byUUID answers the store whose project carries the uuid.
func (s *Set) byUUID(uuid string) (string, *core.Service, error) {
	for _, k := range s.keys {
		if st := s.byKey[k]; strings.EqualFold(st.ProjectUUID(), uuid) {
			return k, st, nil
		}
	}
	return "", nil, fmt.Errorf("%w: project uuid %s is not served here (served: %s)", api.ErrNotFound, uuid, strings.Join(s.keys, ", "))
}

// --- Across the stores ------------------------------------------------------

// ListOutput is a list across the stores.
type ListOutput struct {
	Objects   []Located `json:"objects"`
	Total     int       `json:"total"`
	Truncated bool      `json:"truncated,omitempty"`
}

// ObjectList lists every store's matching objects, in id order then project
// order; Limit cuts the joined list (each store lists whole). A Parent is a
// filter: bare, it matches in every project (each row names its project);
// qualified <key>:<id>, in its key's store alone.
func (s *Set) ObjectList(ctx context.Context, in api.ObjectListInput) (ListOutput, error) {
	limit := in.Limit
	in.Limit = 0
	keys := s.keys
	if strings.Contains(in.Parent, ":") {
		key, bare, err := s.qualifiedAt(in.Parent)
		if err != nil {
			return ListOutput{}, fmt.Errorf("parent: %w", err)
		}
		keys, in.Parent = []string{key}, bare
	}
	var out ListOutput
	for _, k := range keys {
		got, err := s.byKey[k].ObjectList(ctx, in)
		if err != nil {
			return ListOutput{}, fmt.Errorf("project %q: %w", k, err)
		}
		bad := set(got.InError)
		for _, o := range got.Objects {
			out.Objects = append(out.Objects, Located{Project: k, Object: o, InError: bad[o.ID]})
		}
	}
	s.sortByID(out.Objects)
	out.Total = len(out.Objects)
	if limit > 0 && len(out.Objects) > limit {
		out.Objects = out.Objects[:limit]
		out.Truncated = true
	}
	return out, nil
}

// SearchOutput is a search across the stores.
type SearchOutput struct {
	Objects []Located `json:"objects"`
	Total   int       `json:"total"`
	Capped  bool      `json:"capped,omitempty"`
}

// ObjectSearch searches every store - only those holding Within when it is
// given - and joins the answers: a substring search in id order then project
// order, a fulltext one by most words matched then score (each store scores
// against its own corpus). Limit cuts the joined answer.
func (s *Set) ObjectSearch(ctx context.Context, in api.ObjectSearchInput) (SearchOutput, error) {
	limit := in.Limit
	in.Limit = 0
	keys := s.keys
	switch {
	case strings.Contains(in.Within, ":"):
		key, bare, err := s.qualifiedAt(in.Within)
		if err != nil {
			return SearchOutput{}, fmt.Errorf("within: %w", err)
		}
		keys, in.Within = []string{key}, bare
	case in.Within != "":
		keys = nil
		for _, k := range s.keys {
			ok, err := s.byKey[k].Holds(in.Within)
			if err != nil {
				return SearchOutput{}, fmt.Errorf("project %q: %w", k, err)
			}
			if ok {
				keys = append(keys, k)
			}
		}
		if len(keys) == 0 {
			return SearchOutput{}, fmt.Errorf("%w: within: object %q is in no served project", api.ErrNotFound, in.Within)
		}
	}
	var out SearchOutput
	for _, k := range keys {
		got, err := s.byKey[k].ObjectSearch(ctx, in)
		if err != nil {
			return SearchOutput{}, fmt.Errorf("project %q: %w", k, err)
		}
		out.Capped = out.Capped || got.Capped
		bad := set(got.InError)
		for i, o := range got.Objects {
			l := Located{Project: k, Object: o, InError: bad[o.ID]}
			if i < len(got.Hits) {
				h := got.Hits[i]
				l.Hit = &h
			}
			if st, ok := got.States[o.ID]; ok {
				l.State = &st
			}
			out.Objects = append(out.Objects, l)
		}
	}
	if in.Mode == api.SearchFullText {
		order := s.order()
		sort.SliceStable(out.Objects, func(i, j int) bool {
			a, b := out.Objects[i].Hit, out.Objects[j].Hit
			if a != nil && b != nil {
				if len(a.Matched) != len(b.Matched) {
					return len(a.Matched) > len(b.Matched)
				}
				if a.Score != b.Score {
					return a.Score > b.Score
				}
			}
			return order[out.Objects[i].Project] < order[out.Objects[j].Project]
		})
	} else {
		s.sortByID(out.Objects)
	}
	out.Total = len(out.Objects)
	if limit > 0 && len(out.Objects) > limit {
		out.Objects = out.Objects[:limit]
	}
	return out, nil
}

// order is each project's rank in the set.
func (s *Set) order() map[string]int {
	o := make(map[string]int, len(s.keys))
	for i, k := range s.keys {
		o[k] = i
	}
	return o
}

// sortByID sorts located objects by id, then by the set's project order.
func (s *Set) sortByID(objs []Located) {
	order := s.order()
	sort.SliceStable(objs, func(i, j int) bool {
		if objs[i].Object.ID != objs[j].Object.ID {
			return objs[i].Object.ID < objs[j].Object.ID
		}
		return order[objs[i].Project] < order[objs[j].Project]
	})
}

func set(ids []string) map[string]bool {
	m := make(map[string]bool, len(ids))
	for _, id := range ids {
		m[id] = true
	}
	return m
}
