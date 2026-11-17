// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"fmt"
	"sort"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// The read side of the object substrate. These answer from the index, never by
// walking the object files per call (goals.md G5).

// index returns the service's object index, building it on first use. The index
// is derived state, so creating it lazily costs nothing and keeps New() cheap.
func (s *Service) index() *objectIndex {
	s.objIndexOnce.Do(func() {
		s.objIndex = newObjectIndex(s.Config.ObjectsRoot(), s.storeObjectsRoot())
		s.objIndex.renumber = s.renumberOf
		s.objIndex.refused = s.refused
		// What a catch-up found moved outside the verbs is published as ONE
		// grouped event, with each object's event under the threshold
		// (catchup.go).
		ix := s.objIndex
		ix.catch.on = func(c catchUp) { s.publishCatchUp(ix, c) }
	})
	return s.objIndex
}

// ObjectList returns the objects matching the filters, in stable id order.
func (s *Service) ObjectList(_ stdctx.Context, in api.ObjectListInput) (api.ObjectListOutput, error) {
	ix := s.index()
	if err := ix.ensureLoaded(); err != nil {
		return api.ObjectListOutput{}, err
	}
	objs := ix.list(in)
	if in.Capability != "" {
		kept := objs[:0:0]
		for _, o := range objs {
			if s.hasEffective(o, in.Capability) {
				kept = append(kept, o)
			}
		}
		objs = kept
	}
	out := api.ObjectListOutput{Total: len(objs)}
	if in.Limit > 0 && len(objs) > in.Limit {
		objs = objs[:in.Limit]
		out.Truncated = true
	}
	if len(in.Fields) > 0 {
		kept := make([]api.Object, len(objs))
		for i, o := range objs {
			kept[i] = selectObjectFields(o, in.Fields)
		}
		objs = kept
	}
	out.Objects = objs
	out.InError = s.inError(objectIDs(objs))
	return out, nil
}

// selectObjectFields keeps of o its id, its type and the parts fields names: a
// core field by its json name, fields or capabilities whole, or a path into
// them - fields.<name>, capabilities.<capability>[.<field>]. A name that
// matches nothing keeps nothing. A part taken in pieces goes into maps of its
// own: the index's maps are shared and never written.
func selectObjectFields(o api.Object, fields []string) api.Object {
	out := api.Object{ID: o.ID, Type: o.Type}
	var (
		allFields, allCaps bool
		typeFields         []string
		wholeCaps          = map[string]bool{}
		capFields          = map[string][]string{}
	)
	for _, f := range fields {
		head, rest, _ := strings.Cut(strings.TrimSpace(f), ".")
		switch head {
		case "name":
			out.Name = o.Name
		case "description":
			out.Description = o.Description
		case "icon":
			out.Icon = o.Icon
		case "parent":
			out.Parent = o.Parent
		case "order":
			out.Order = o.Order
		case "slug":
			out.Slug = o.Slug
		case "minter":
			out.Minter = o.Minter
		case "tags":
			out.Tags = o.Tags
		case "badges":
			out.Badges = o.Badges
		case "edges":
			out.Edges = o.Edges
		case "aspects":
			out.Aspects = o.Aspects
		case "files":
			out.Files = o.Files
		case "hooks":
			out.Hooks = o.Hooks
		case "agent":
			out.Agent = o.Agent
		case "body":
			out.Body = o.Body
		case "fields":
			if rest == "" {
				allFields = true
			} else {
				typeFields = append(typeFields, rest)
			}
		case "capabilities":
			capID, field, _ := strings.Cut(rest, ".")
			switch {
			case capID == "":
				allCaps = true
			case field == "":
				wholeCaps[capID] = true
			default:
				capFields[capID] = append(capFields[capID], field)
			}
		}
	}
	if allFields {
		out.Fields = o.Fields
	} else {
		for _, name := range typeFields {
			if v, ok := o.Fields[name]; ok {
				if out.Fields == nil {
					out.Fields = map[string]any{}
				}
				out.Fields[name] = v
			}
		}
	}
	if allCaps {
		out.Capabilities = o.Capabilities
		return out
	}
	keep := func(capID string, group map[string]any) {
		if out.Capabilities == nil {
			out.Capabilities = map[string]map[string]any{}
		}
		out.Capabilities[capID] = group
	}
	for capID := range wholeCaps {
		if group, ok := o.Capabilities[capID]; ok {
			keep(capID, group)
		}
	}
	for capID, names := range capFields {
		group, ok := o.Capabilities[capID]
		if !ok || wholeCaps[capID] {
			continue
		}
		part := map[string]any{}
		for _, name := range names {
			if v, ok := group[name]; ok {
				part[name] = v
			}
		}
		if len(part) > 0 {
			keep(capID, part)
		}
	}
	return out
}

// ObjectSearch answers from the resident metadata with zero file reads (7b):
// a case-insensitive substring over id, name, title, description and tags,
// narrowed by type, module and effective capabilities — and, last, by an
// ObjectFilter, evaluated on at most api.SearchFilterCap candidates.
func (s *Service) ObjectSearch(_ stdctx.Context, in api.ObjectSearchInput) (api.ObjectSearchOutput, error) {
	ix := s.index()
	if err := ix.ensureLoaded(); err != nil {
		return api.ObjectSearchOutput{}, err
	}
	var within map[string]bool
	if in.Within != "" {
		if _, ok := ix.get(in.Within); !ok {
			return api.ObjectSearchOutput{}, fmt.Errorf("%w: within: object %q", api.ErrNotFound, in.Within)
		}
		within = ix.subtree(in.Within)
	}
	// admit applies every filter but the query.
	admit := func(o api.Object) bool {
		if within != nil && !within[o.ID] {
			return false
		}
		if len(in.Types) > 0 && !sliceContains(in.Types, o.Type) {
			return false
		}
		if len(in.Modules) > 0 {
			m, _ := api.ModuleOfObjectID(o.ID)
			if !sliceContains(in.Modules, m) {
				return false
			}
		}
		for _, c := range in.Capabilities {
			if !sliceContains(s.effectiveCapabilities(o), c) {
				return false
			}
		}
		if len(in.States) > 0 {
			st, ok := s.shownState(o)
			if !ok || !sliceContains(in.States, st) {
				return false
			}
		}
		return true
	}
	// filtered applies the ObjectFilter, last and capped: it may read edges
	// and evaluate a `when`.
	evaluated, capped := 0, false
	filtered := func(o api.Object) bool {
		if in.Filter == nil {
			return true
		}
		if evaluated >= api.SearchFilterCap {
			capped = true
			return false
		}
		evaluated++
		ok, _ := in.Filter.Matches(s.filterSubject(o, s.effectiveRefs(o)))
		return ok
	}

	switch in.Mode {
	case "", api.SearchSubstring:
	case api.SearchFullText:
		if strings.TrimSpace(in.Query) == "" {
			return api.ObjectSearchOutput{}, fmt.Errorf("%w: fulltext search needs a query", api.ErrInvalidInput)
		}
		ranked := ix.fulltext(in.Query)
		hits := ranked[:0:0]
		for _, h := range ranked {
			if o, ok := ix.get(h.ID); ok && admit(o) && filtered(o) {
				hits = append(hits, h)
			}
		}
		out := api.ObjectSearchOutput{Total: len(hits), Capped: capped}
		if in.Limit > 0 && len(hits) > in.Limit {
			hits = hits[:in.Limit]
		}
		out.Hits = hits
		out.Objects = make([]api.Object, 0, len(hits))
		for _, h := range hits {
			o, _ := ix.get(h.ID)
			out.Objects = append(out.Objects, o)
		}
		out.InError = s.inError(objectIDs(out.Objects))
		if in.WithStates {
			out.States = s.statesOf(out.Objects)
		}
		return out, nil
	default:
		return api.ObjectSearchOutput{}, fmt.Errorf("%w: search mode %q (substring or fulltext)", api.ErrInvalidInput, in.Mode)
	}

	q := strings.ToLower(strings.TrimSpace(in.Query))
	var hits []api.Object
	for _, o := range ix.list(api.ObjectListInput{}) {
		if !admit(o) {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(searchText(o)), q) {
			continue
		}
		if !filtered(o) {
			if capped {
				break
			}
			continue
		}
		hits = append(hits, o)
	}
	out := api.ObjectSearchOutput{Total: len(hits), Capped: capped}
	if in.Limit > 0 && len(hits) > in.Limit {
		hits = hits[:in.Limit]
	}
	out.Objects = hits
	out.InError = s.inError(objectIDs(hits))
	if in.WithStates {
		out.States = s.statesOf(hits)
	}
	return out, nil
}

// shownState is the lifecycle state an object is in: the stored one, else
// the initial state of its workflow (the object's own, else its type's) — an
// object that never moved is in it. False when it follows no declared
// workflow.
func (s *Service) shownState(o api.Object) (string, bool) {
	slot, err := s.lifecycleSlotOf(o, api.CapLifecycle)
	if err != nil {
		return "", false
	}
	if slot.state != "" {
		return slot.state, true
	}
	if wf, ok := s.workflowNamed(slot.workflow); ok && wf.Initial != "" {
		return wf.Initial, true
	}
	return "", false
}

// statesOf is each object's shown state, coloured as the lifecycle
// capability's state badge colours it.
func (s *Service) statesOf(objs []api.Object) map[string]api.ObjectState {
	var colors map[string]string
	if c, ok := s.capabilitiesByName()[api.CapLifecycle]; ok {
		for _, b := range c.Badges {
			if b.Field == "state" {
				colors = b.Colors
			}
		}
	}
	out := map[string]api.ObjectState{}
	for _, o := range objs {
		st, ok := s.shownState(o)
		if !ok {
			continue
		}
		color := colors[st]
		if color == "" {
			color = colors["*"]
		}
		out[o.ID] = api.ObjectState{State: st, Color: color}
	}
	return out
}

// objectIDs lists the objects' ids, in order.
func objectIDs(objs []api.Object) []string {
	ids := make([]string, len(objs))
	for i, o := range objs {
		ids[i] = o.ID
	}
	return ids
}

// markTreeErrors sets HasErrors on every node of a tree whose object carries
// errors — the tree draws those red.
func (s *Service) markTreeErrors(nodes []api.ObjectTreeNode) {
	s.ensureProblems()
	s.problems.mu.Lock()
	defer s.problems.mu.Unlock()
	var walk func([]api.ObjectTreeNode)
	walk = func(ns []api.ObjectTreeNode) {
		for i := range ns {
			id := ns[i].Object.ID
			ns[i].HasErrors = len(s.problems.validity[id]) > 0 || len(s.problems.links[id]) > 0
			walk(ns[i].Children)
		}
	}
	walk(nodes)
}

// searchText is the indexed text of an object: its id, container name and
// description, content title and tags — the resident properties
// (identity-and-index.md §4), never the body.
func searchText(o api.Object) string {
	parts := append([]string{o.ID, o.Name, o.Description}, o.Tags...)
	return strings.Join(parts, " ")
}

// ObjectTree returns a containment subtree as nested data — one call, no
// per-object round trip, no strategy invocation.
func (s *Service) ObjectTree(ctx stdctx.Context, in api.ObjectTreeInput) (api.ObjectTreeOutput, error) {
	ix := s.index()
	if err := ix.ensureLoaded(); err != nil {
		return api.ObjectTreeOutput{}, err
	}
	out, err := ix.tree(in)
	if err != nil {
		return out, err
	}
	if in.DrawnOnly {
		out.Roots, out.Total = s.pruneToDrawn(out.Roots)
	}
	s.markTreeErrors(out.Roots)
	s.markTreeSystem(ctx, out.Roots)
	s.markTreeLevel(ctx, out.Roots)
	return out, nil
}

// markTreeLevel sets Level on every node from its type's level capability
// : each type read once.
func (s *Service) markTreeLevel(ctx stdctx.Context, nodes []api.ObjectTreeNode) {
	levels := map[string]string{}
	var walk func([]api.ObjectTreeNode)
	walk = func(ns []api.ObjectTreeNode) {
		for i := range ns {
			typ := ns[i].Object.Type
			level, seen := levels[typ]
			if !seen {
				if got, err := s.ObjectTypeGet(ctx, api.ObjectTypeGetInput{Name: typ}); err == nil {
					level = got.ObjectType.Level()
				}
				levels[typ] = level
			}
			ns[i].Level = level
			walk(ns[i].Children)
		}
	}
	walk(nodes)
}

// markTreeSystem sets System on every node that is a system module or lies in
// one - read from the system-module capability, never from a name.
func (s *Service) markTreeSystem(ctx stdctx.Context, nodes []api.ObjectTreeNode) {
	sys, err := s.ObjectList(ctx, api.ObjectListInput{Capability: api.CapSystemModule})
	if err != nil || len(sys.Objects) == 0 {
		return
	}
	system := make(map[string]bool, len(sys.Objects))
	for _, o := range sys.Objects {
		system[o.ID] = true
	}
	var walk func([]api.ObjectTreeNode)
	walk = func(ns []api.ObjectTreeNode) {
		for i := range ns {
			id := ns[i].Object.ID
			module, _ := api.ModuleOfObjectID(id)
			ns[i].System = system[id] || system[module]
			walk(ns[i].Children)
		}
	}
	walk(nodes)
}

// pruneToDrawn removes objects whose type is not drawn (`tree:` root or node).
//
// A HIDDEN OBJECT DOES NOT HIDE ITS DESCENDANTS. Its drawn children are lifted
// to its place rather than dropped with it: "do not draw this row" and "do not
// draw this subtree" are different instructions, and only the first is what
// the capability says. Dropping the subtree would make a single undeclared
// type silently erase everything filed beneath it.
//
// An UNKNOWN type is DRAWN. A type nobody declared is a problem to see, not to
// hide — the opposite default would make a corpus with a typo in its type
// render as an empty tree.
func (s *Service) pruneToDrawn(nodes []api.ObjectTreeNode) ([]api.ObjectTreeNode, int) {
	var kept []api.ObjectTreeNode
	total := 0
	for _, n := range nodes {
		children, sub := s.pruneToDrawn(n.Children)
		if !s.isDrawn(n.Object) {
			kept = append(kept, children...)
			total += sub
			continue
		}
		n.Children = children
		kept = append(kept, n)
		total += 1 + sub
	}
	return kept, total
}

// ObjectScan rebuilds the index from disk and reports what changed — the
// ObserveActual L0 diff that drives staleness and re-check (G4).
//
// Force re-reads every file from an empty index, so everything reads as Added.
// That is the honest answer to "pretend you knew nothing": it is a rebuild, not
// a diff.
func (s *Service) ObjectScan(_ stdctx.Context, in api.ObjectScanInput) (api.ObjectScanOutput, error) {
	return s.objectScan(in, false)
}

// rescanOwnWrites rebuilds the index after a conversion wrote the object
// files (migrate, a project group's join): what it wrote is CWM's own, never
// an outside change, so the catch-up cursor takes it as its baseline and
// nothing is announced.
func (s *Service) rescanOwnWrites() error {
	_, err := s.objectScan(api.ObjectScanInput{Force: true}, true)
	return err
}

// objectScan is ObjectScan; own takes what the scan reads as the catch-up's
// baseline instead of comparing it.
func (s *Service) objectScan(in api.ObjectScanInput, own bool) (api.ObjectScanOutput, error) {
	ix := s.index()
	if in.Force {
		ix.mu.Lock()
		ix.byID = map[string]indexEntry{}
		ix.incoming = nil
		ix.loaded = false
		ix.mu.Unlock()
	}
	out, err := ix.scan()
	switch {
	case err == nil && own:
		err = ix.rebaseline()
	case err == nil:
		_, _ = ix.catchUp() // what moved outside the verbs (catchup.go)
	}
	// The ended ids are read again too: an outside edit of a history folder
	// is seen at the scan (ended.go).
	s.forgetEnded()
	if err != nil || !in.Validate {
		return out, err
	}
	out.Invalid, out.Checked = s.invalidObjects()
	return out, nil
}

// invalidObjects is the objects' errors (problems.go) as the scan reports
// them — the same computation the global Errors list reads, never a second copy
// of the checks — plus the files that could not be filed under an id.
func (s *Service) invalidObjects() ([]api.InvalidObject, int) {
	ix := s.index()
	s.ensureProblems()
	var out []api.InvalidObject
	objs := ix.list(api.ObjectListInput{})
	for _, o := range objs {
		errs := s.problemsOf(o.ID)
		if len(errs) == 0 {
			continue
		}
		path, _ := ix.pathOf(o.ID)
		problems := make([]string, 0, len(errs))
		for _, d := range errs {
			problems = append(problems, d.Description)
		}
		out = append(out, api.InvalidObject{ID: o.ID, Path: ix.rel(path), Problems: problems})
	}
	out = append(out, ix.unreadableFiles()...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].ID != out[j].ID {
			return out[i].ID < out[j].ID
		}
		return out[i].Path < out[j].Path
	})
	return out, len(objs)
}

// lifecycleProblem reports a workflow state — the object's own lifecycle or a
// capability's — that is not a state of its workflow ("" when all hold).
func (s *Service) lifecycleProblem(obj api.Object) string {
	states := s.workflowStates(obj)
	slots := make([]string, 0, len(states))
	for slot := range states {
		slots = append(slots, slot)
	}
	sort.Strings(slots)
	for _, slot := range slots {
		state := states[slot]
		if state == "" {
			continue
		}
		ls, err := s.lifecycleSlotOf(obj, slot)
		if err != nil {
			if slot == "" {
				continue // an object with a state and no workflow names none to check
			}
			return err.Error()
		}
		wf, ok := s.workflowNamed(ls.workflow)
		what := "lifecycle"
		if slot != "" {
			what = "capability " + slot
		}
		if !ok {
			return fmt.Sprintf("%s: workflow %q is not declared", what, ls.workflow)
		}
		if _, ok := wf.StateNamed(state); !ok {
			return fmt.Sprintf("%s: %q is not a state of workflow %s", what, state, wf.Name)
		}
	}
	return ""
}
