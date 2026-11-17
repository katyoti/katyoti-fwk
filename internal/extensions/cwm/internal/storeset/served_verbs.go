// SPDX-License-Identifier: Apache-2.0

package storeset

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"sync"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/internal/core"
)

// The verbs of api.CWMService beyond the seven of served.go, each routed
// . Served embeds no store: a verb missing here does not compile.

// --- Routing ----------------------------------------------------------------

// byID is the store holding an id written bare or qualified, and the id bare.
func (s *Served) byID(id string) (string, *core.Service, string, error) {
	key, bare, err := splitID(id)
	if err != nil {
		return "", nil, "", err
	}
	k, st, err := s.set.holdingID(key, bare)
	if err != nil {
		return "", nil, "", err
	}
	return k, st, bare, nil
}

// inProject reads an id that must be of project key: bare, or qualified with
// key. Qualified with another key it is refused - what crosses a project is
// not a write of one store.
func inProject(key, what, id string) (string, error) {
	k, bare, err := splitID(id)
	if err != nil {
		return "", err
	}
	if k != "" && k != key {
		return "", fmt.Errorf("%w: %s %q is in project %q, not in %q: a write never crosses projects", api.ErrInvalidInput, what, id, k, key)
	}
	return bare, nil
}

// routed runs a verb of one object on the store holding it: *id names the
// object, bare or qualified, and is made bare.
func routed[I, O any](s *Served, ctx context.Context, id *string, in *I, verb func(*core.Service, context.Context, I) (O, error)) (O, error) {
	_, st, bare, err := s.byID(*id)
	if err != nil {
		var zero O
		return zero, err
	}
	*id = bare
	return verb(st, ctx, *in)
}

// joined is each store's answer to a verb asked of every store, in the set's
// order; a failure names its project.
func joined[I, O any](s *Served, ctx context.Context, in I, verb func(*core.Service, context.Context, I) (O, error)) ([]string, []O, error) {
	outs := make([]O, 0, len(s.set.keys))
	for _, k := range s.set.keys {
		out, err := verb(s.set.byKey[k], ctx, in)
		if err != nil {
			return nil, nil, fmt.Errorf("project %q: %w", k, err)
		}
		outs = append(outs, out)
	}
	return s.set.Keys(), outs, nil
}

// refusedWithSeveral refuses a verb of one served project when the set serves
// several.
func (s *Served) refusedWithSeveral(verb, why string) error {
	return fmt.Errorf("%w: %s: this universe serves several projects (%s): %s", api.ErrInvalidInput, verb, strings.Join(s.set.keys, ", "), why)
}

func qualifyAll(key string, ids []string) []string {
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = api.QualifiedID(key, id)
	}
	return out
}

// --- A. By the store holding the object -------------------------------------

func (s *Served) ObjectArchive(ctx context.Context, in api.ObjectArchiveInput) (api.ObjectArchiveOutput, error) {
	return routed(s, ctx, &in.ID, &in, (*core.Service).ObjectArchive)
}

func (s *Served) ObjectEffectiveCapabilities(ctx context.Context, in api.ObjectEffectiveCapabilitiesInput) (api.ObjectEffectiveCapabilitiesOutput, error) {
	return routed(s, ctx, &in.ID, &in, (*core.Service).ObjectEffectiveCapabilities)
}

func (s *Served) ObjectEffectiveSurface(ctx context.Context, in api.ObjectEffectiveSurfaceInput) (api.ObjectEffectiveSurfaceOutput, error) {
	return routed(s, ctx, &in.ID, &in, (*core.Service).ObjectEffectiveSurface)
}

// ObjectAllowedChildren answers under the store holding the parent; with no
// parent, the project root's children - the same types in every project (one
// namespace), read from the first store.
func (s *Served) ObjectAllowedChildren(ctx context.Context, in api.ObjectAllowedChildrenInput) (api.ObjectAllowedChildrenOutput, error) {
	if in.ParentID == "" {
		return s.primary.ObjectAllowedChildren(ctx, in)
	}
	return routed(s, ctx, &in.ParentID, &in, (*core.Service).ObjectAllowedChildren)
}

func (s *Served) ObjectFileRead(ctx context.Context, in api.ObjectFileReadInput) (api.ObjectFileReadOutput, error) {
	return routed(s, ctx, &in.ID, &in, (*core.Service).ObjectFileRead)
}

func (s *Served) ObjectFileWrite(ctx context.Context, in api.ObjectFileWriteInput) (api.ObjectFileWriteOutput, error) {
	return routed(s, ctx, &in.ID, &in, (*core.Service).ObjectFileWrite)
}

func (s *Served) ObjectFileList(ctx context.Context, in api.ObjectFileListInput) (api.ObjectFileListOutput, error) {
	return routed(s, ctx, &in.ID, &in, (*core.Service).ObjectFileList)
}

func (s *Served) ObjectFileDelete(ctx context.Context, in api.ObjectFileDeleteInput) (api.ObjectFileDeleteOutput, error) {
	return routed(s, ctx, &in.ID, &in, (*core.Service).ObjectFileDelete)
}

func (s *Served) ObjectFileStat(ctx context.Context, in api.ObjectFileStatInput) (api.ObjectFileStatOutput, error) {
	return routed(s, ctx, &in.ID, &in, (*core.Service).ObjectFileStat)
}

func (s *Served) ObjectFileAttach(ctx context.Context, in api.ObjectFileAttachInput) (api.ObjectFileAttachOutput, error) {
	return routed(s, ctx, &in.ID, &in, (*core.Service).ObjectFileAttach)
}

func (s *Served) ObjectHooks(ctx context.Context, in api.ObjectHooksInput) (api.ObjectHooksOutput, error) {
	return routed(s, ctx, &in.ID, &in, (*core.Service).ObjectHooks)
}

func (s *Served) ObjectTransition(ctx context.Context, in api.ObjectTransitionInput) (api.ObjectTransitionOutput, error) {
	return routed(s, ctx, &in.ID, &in, (*core.Service).ObjectTransition)
}

func (s *Served) ObjectEvaluate(ctx context.Context, in api.ObjectEvaluateInput) (api.ObjectEvaluateOutput, error) {
	return routed(s, ctx, &in.ID, &in, (*core.Service).ObjectEvaluate)
}

func (s *Served) ObjectAddCapability(ctx context.Context, in api.ObjectAddCapabilityInput) (api.ObjectAddCapabilityOutput, error) {
	return routed(s, ctx, &in.ID, &in, (*core.Service).ObjectAddCapability)
}

func (s *Served) ObjectRemoveCapability(ctx context.Context, in api.ObjectRemoveCapabilityInput) (api.ObjectRemoveCapabilityOutput, error) {
	return routed(s, ctx, &in.ID, &in, (*core.Service).ObjectRemoveCapability)
}

func (s *Served) ObjectNamedFileList(ctx context.Context, in api.ObjectNamedFileListInput) (api.ObjectNamedFileListOutput, error) {
	return routed(s, ctx, &in.ID, &in, (*core.Service).ObjectNamedFileList)
}

func (s *Served) ObjectNamedFileRead(ctx context.Context, in api.ObjectNamedFileReadInput) (api.ObjectNamedFileReadOutput, error) {
	return routed(s, ctx, &in.ID, &in, (*core.Service).ObjectNamedFileRead)
}

func (s *Served) ObjectNamedFileWrite(ctx context.Context, in api.ObjectNamedFileWriteInput) (api.ObjectNamedFileWriteOutput, error) {
	return routed(s, ctx, &in.ID, &in, (*core.Service).ObjectNamedFileWrite)
}

func (s *Served) ObjectNamedFileDeclare(ctx context.Context, in api.ObjectNamedFileDeclareInput) (api.ObjectNamedFileDeclareOutput, error) {
	return routed(s, ctx, &in.ID, &in, (*core.Service).ObjectNamedFileDeclare)
}

func (s *Served) ObjectRetype(ctx context.Context, in api.ObjectRetypeInput) (api.ObjectRetypeOutput, error) {
	return routed(s, ctx, &in.ID, &in, (*core.Service).ObjectRetype)
}

// ObjectSplit splits in the store holding the source: its successors and the
// children they adopt stay in the source's project.
func (s *Served) ObjectSplit(ctx context.Context, in api.ObjectSplitInput) (api.ObjectSplitOutput, error) {
	key, st, bare, err := s.byID(in.ID)
	if err != nil {
		return api.ObjectSplitOutput{}, err
	}
	in.ID = bare
	succ := make([]api.ObjectSplitTarget, len(in.Successors))
	for i, t := range in.Successors {
		if t.ID != "" {
			if t.ID, err = inProject(key, "successor", t.ID); err != nil {
				return api.ObjectSplitOutput{}, err
			}
		}
		kids := make([]string, len(t.Children))
		for j, c := range t.Children {
			if kids[j], err = inProject(key, "child", c); err != nil {
				return api.ObjectSplitOutput{}, err
			}
		}
		t.Children = kids
		succ[i] = t
	}
	in.Successors = succ
	return st.ObjectSplit(ctx, in)
}

func (s *Served) EventRaise(ctx context.Context, in api.EventRaiseInput) (api.EventRaiseOutput, error) {
	return routed(s, ctx, &in.ObjectID, &in, (*core.Service).EventRaise)
}

// EdgeList lists the edges of one object from the store holding it; with no
// object, every store's edges.
func (s *Served) EdgeList(ctx context.Context, in api.EdgeListInput) (api.EdgeListOutput, error) {
	if in.ObjectID != "" {
		return routed(s, ctx, &in.ObjectID, &in, (*core.Service).EdgeList)
	}
	_, outs, err := joined(s, ctx, in, (*core.Service).EdgeList)
	if err != nil {
		return api.EdgeListOutput{}, err
	}
	out := api.EdgeListOutput{Edges: []api.Edge{}}
	for _, o := range outs {
		out.Edges = append(out.Edges, o.Edges...)
	}
	return out, nil
}

// --- B. By the source's store -----------------------------------------------

// EdgeCreate writes the edge in the store holding its source; its target is in
// the same project (an edge never crosses a project).
func (s *Served) EdgeCreate(ctx context.Context, in api.EdgeCreateInput) (api.EdgeCreateOutput, error) {
	key, st, bare, err := s.byID(in.From)
	if err != nil {
		return api.EdgeCreateOutput{}, err
	}
	in.From = bare
	if in.To, err = inProject(key, "edge target", in.To); err != nil {
		return api.EdgeCreateOutput{}, err
	}
	return st.EdgeCreate(ctx, in)
}

// EdgeDelete removes the edge from the store holding its source.
func (s *Served) EdgeDelete(ctx context.Context, in api.EdgeDeleteInput) (api.EdgeDeleteOutput, error) {
	key, st, bare, err := s.byID(in.From)
	if err != nil {
		return api.EdgeDeleteOutput{}, err
	}
	in.From = bare
	if in.To, err = inProject(key, "edge target", in.To); err != nil {
		return api.EdgeDeleteOutput{}, err
	}
	return st.EdgeDelete(ctx, in)
}

// --- C. A move stays in its project -----------------------------------------

// ObjectMove moves an object in the store holding it. A new module or parent
// of another project is refused, naming both: a move across repositories is
// not a move.
func (s *Served) ObjectMove(ctx context.Context, in api.ObjectMoveInput) (api.ObjectMoveOutput, error) {
	key, st, bare, err := s.byID(in.ID)
	if err != nil {
		return api.ObjectMoveOutput{}, err
	}
	in.ID = bare
	if in.NewParent != "" {
		if in.NewParent, err = inProject(key, "new parent", in.NewParent); err != nil {
			return api.ObjectMoveOutput{}, err
		}
	}
	if in.NewModule != "" {
		mk, m, err := splitModule(in.NewModule)
		if err != nil {
			return api.ObjectMoveOutput{}, err
		}
		if mk != "" && mk != key {
			return api.ObjectMoveOutput{}, fmt.Errorf("%w: move %q (project %q) into module %q of project %q: a move never crosses projects", api.ErrInvalidInput, in.ID, key, m, mk)
		}
		in.NewModule = m
		if mk == "" {
			if err := s.moduleStaysIn(key, st, m, in.ID); err != nil {
				return api.ObjectMoveOutput{}, err
			}
		}
	}
	return st.ObjectMove(ctx, in)
}

// moduleStaysIn refuses a move of id (project key) into a bare module its own
// project lacks and another project holds.
func (s *Served) moduleStaysIn(key string, st *core.Service, module, id string) error {
	if ok, err := st.HoldsModule(module); err != nil || ok {
		return err
	}
	var others []string
	for _, k := range s.set.keys {
		if k == key {
			continue
		}
		if ok, err := s.set.byKey[k].HoldsModule(module); err != nil {
			return fmt.Errorf("project %q: %w", k, err)
		} else if ok {
			others = append(others, k)
		}
	}
	if len(others) > 0 {
		return fmt.Errorf("%w: move %q (project %q) into module %q, held by project %s: a move never crosses projects", api.ErrInvalidInput, id, key, module, strings.Join(others, ", "))
	}
	return nil
}

// --- D. By module, object or every store -------------------------------------

// modulesStores are the stores holding a module written bare or qualified.
func (s *Served) moduleStores(module string) ([]string, error) {
	mk, m, err := splitModule(module)
	if err != nil {
		return nil, err
	}
	if mk != "" {
		if _, _, err := s.set.byProject(mk); err != nil {
			return nil, err
		}
		return []string{mk}, nil
	}
	var keys []string
	for _, k := range s.set.keys {
		ok, err := s.set.byKey[k].HoldsModule(m)
		if err != nil {
			return nil, fmt.Errorf("project %q: %w", k, err)
		}
		if ok {
			keys = append(keys, k)
		}
	}
	if len(keys) == 0 {
		return nil, fmt.Errorf("%w: module %q is in no served project (%s)", api.ErrNotFound, m, strings.Join(s.set.keys, ", "))
	}
	return keys, nil
}

func bareModule(module string) string {
	if i := strings.Index(module, ":"); i >= 0 {
		return module[i+1:]
	}
	return module
}

// ObjectErrorList lists the objects in error under Within in the store
// holding it, of Module in every store holding it, else of every store; each
// object by its qualified id.
func (s *Served) ObjectErrorList(ctx context.Context, in api.ObjectErrorListInput) (api.ObjectErrorListOutput, error) {
	keys := s.set.keys
	if in.Within != "" {
		key, _, bare, err := s.byID(in.Within)
		if err != nil {
			return api.ObjectErrorListOutput{}, err
		}
		keys, in.Within = []string{key}, bare
	}
	if in.Module != "" {
		mkeys, err := s.moduleStores(in.Module)
		if err != nil {
			return api.ObjectErrorListOutput{}, err
		}
		keys = intersect(keys, mkeys)
		in.Module = bareModule(in.Module)
	}
	out := api.ObjectErrorListOutput{Objects: []api.ObjectErrors{}}
	for _, k := range keys {
		got, err := s.set.byKey[k].ObjectErrorList(ctx, in)
		if err != nil {
			return api.ObjectErrorListOutput{}, fmt.Errorf("project %q: %w", k, err)
		}
		for _, o := range got.Objects {
			o.ID = api.QualifiedID(k, o.ID)
			out.Objects = append(out.Objects, o)
		}
		out.Unreadable = append(out.Unreadable, got.Unreadable...)
		out.Total += got.Total
	}
	return out, nil
}

func intersect(a, b []string) []string {
	var out []string
	for _, x := range a {
		if slices.Contains(b, x) {
			out = append(out, x)
		}
	}
	return out
}

// ObjectRunHooks runs a point over one object in the store holding it, over a
// module in every store holding it, else over every store; the runs joined.
func (s *Served) ObjectRunHooks(ctx context.Context, in api.ObjectRunHooksInput) (api.ObjectRunHooksOutput, error) {
	if in.ID != "" {
		in.Module = bareModule(in.Module)
		return routed(s, ctx, &in.ID, &in, (*core.Service).ObjectRunHooks)
	}
	keys := s.set.keys
	if in.Module != "" {
		var err error
		if keys, err = s.moduleStores(in.Module); err != nil {
			return api.ObjectRunHooksOutput{}, err
		}
		in.Module = bareModule(in.Module)
	}
	var out api.ObjectRunHooksOutput
	for i, k := range keys {
		got, err := s.set.byKey[k].ObjectRunHooks(ctx, in)
		if err != nil {
			return api.ObjectRunHooksOutput{}, fmt.Errorf("project %q: %w", k, err)
		}
		if i == 0 {
			out.Point, out.Timing, out.Answer = got.Point, got.Timing, got.Answer
		}
		out.Objects += got.Objects
		out.Runs = append(out.Runs, got.Runs...)
	}
	return out, nil
}

// ModuleSubscribers answers from the store holding the module: a bare module
// several projects hold is refused, naming them.
func (s *Served) ModuleSubscribers(ctx context.Context, in api.ModuleSubscribersInput) (api.ModuleSubscribersOutput, error) {
	key, m, err := splitModule(in.Module)
	if err != nil {
		return api.ModuleSubscribersOutput{}, err
	}
	_, st, err := s.set.holdingModule(key, m)
	if err != nil {
		return api.ModuleSubscribersOutput{}, err
	}
	in.Module = m
	return st.ModuleSubscribers(ctx, in)
}

// --- E. Every store, joined -------------------------------------------------

// ObjectScan rescans every store; the ids it reports are qualified.
func (s *Served) ObjectScan(ctx context.Context, in api.ObjectScanInput) (api.ObjectScanOutput, error) {
	keys, outs, err := joined(s, ctx, in, (*core.Service).ObjectScan)
	if err != nil {
		return api.ObjectScanOutput{}, err
	}
	var out api.ObjectScanOutput
	for i, o := range outs {
		k := keys[i]
		out.Added = append(out.Added, qualifyAll(k, o.Added)...)
		out.Modified = append(out.Modified, qualifyAll(k, o.Modified)...)
		out.Removed = append(out.Removed, qualifyAll(k, o.Removed)...)
		for _, inv := range o.Invalid {
			inv.ID = api.QualifiedID(k, inv.ID)
			out.Invalid = append(out.Invalid, inv)
		}
		out.Total += o.Total
		out.Checked += o.Checked
	}
	return out, nil
}

// ObjectTree reads the subtree of Root from the store holding it; with no
// root, each store's roots in the set's order.
func (s *Served) ObjectTree(ctx context.Context, in api.ObjectTreeInput) (api.ObjectTreeOutput, error) {
	if in.Root != "" {
		return routed(s, ctx, &in.Root, &in, (*core.Service).ObjectTree)
	}
	_, outs, err := joined(s, ctx, in, (*core.Service).ObjectTree)
	if err != nil {
		return api.ObjectTreeOutput{}, err
	}
	out := api.ObjectTreeOutput{Roots: []api.ObjectTreeNode{}}
	for _, o := range outs {
		out.Roots = append(out.Roots, o.Roots...)
		out.Total += o.Total
	}
	return out, nil
}

// EventSubscriptions joins every store's subscriptions by event type, each
// declaration once.
func (s *Served) EventSubscriptions(ctx context.Context, in api.EventSubscriptionsInput) (api.EventSubscriptionsOutput, error) {
	_, outs, err := joined(s, ctx, in, (*core.Service).EventSubscriptions)
	if err != nil {
		return api.EventSubscriptionsOutput{}, err
	}
	out := api.EventSubscriptionsOutput{Events: []api.EventTypeSubscriptions{}}
	at := map[string]int{}
	seen := map[string]map[string]bool{}
	for _, o := range outs {
		for _, e := range o.Events {
			i, ok := at[e.Event]
			if !ok {
				i = len(out.Events)
				at[e.Event] = i
				seen[e.Event] = map[string]bool{}
				subs := e.Subscriptions
				e.Subscriptions = nil
				out.Events = append(out.Events, e)
				e.Subscriptions = subs
			}
			for _, sub := range e.Subscriptions {
				b, _ := json.Marshal(sub)
				if seen[e.Event][string(b)] {
					continue
				}
				seen[e.Event][string(b)] = true
				out.Events[i].Subscriptions = append(out.Events[i].Subscriptions, sub)
			}
			if out.Events[i].Subscriptions == nil {
				out.Events[i].Subscriptions = []api.EventSubscription{}
			}
		}
	}
	return out, nil
}

// ProjectsKnown lists every served project first, in the set's order, then
// the installation's other projects once each, by name.
func (s *Served) ProjectsKnown(ctx context.Context, in api.ProjectsKnownInput) (api.ProjectsKnownOutput, error) {
	_, outs, err := joined(s, ctx, in, (*core.Service).ProjectsKnown)
	if err != nil {
		return api.ProjectsKnownOutput{}, err
	}
	var served, others []api.KnownProject
	servedDir := map[string]bool{}
	for _, o := range outs {
		for _, p := range o.Projects {
			if p.Served {
				served = append(served, p)
				servedDir[cleanRoot(p.Dir)] = true
			}
		}
	}
	seen := map[string]bool{}
	for _, o := range outs {
		for _, p := range o.Projects {
			d := cleanRoot(p.Dir)
			if p.Served || servedDir[d] || seen[d] {
				continue
			}
			seen[d] = true
			others = append(others, p)
		}
	}
	slices.SortStableFunc(others, func(a, b api.KnownProject) int {
		return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
	})
	return api.ProjectsKnownOutput{Projects: append(append([]api.KnownProject{}, served...), others...)}, nil
}

// migrateJoin joins the stores' conversions: the oldest version from, the
// steps of each, every path qualified by its project.
func migrateJoin(keys []string, outs []api.MigrateOutput) api.MigrateOutput {
	var out api.MigrateOutput
	for i, o := range outs {
		k := keys[i]
		if i == 0 || o.From < out.From {
			out.From = o.From
		}
		if o.To > out.To {
			out.To = o.To
		}
		out.DryRun = out.DryRun || o.DryRun
		if len(outs) == 1 {
			out.GroupMerge = o.GroupMerge // one project: its join as it is
		}
		for _, st := range o.Steps {
			st.Changed = qualifyAll(k, st.Changed)
			st.Removed = qualifyAll(k, st.Removed)
			out.Steps = append(out.Steps, st)
		}
		out.Unreadable = append(out.Unreadable, qualifyAll(k, o.Unreadable)...)
		if o.Tree != nil {
			if out.Tree == nil {
				out.Tree = &api.MigrateTree{Repo: true, Clean: true}
			}
			out.Tree.Repo = out.Tree.Repo && o.Tree.Repo
			out.Tree.Clean = out.Tree.Clean && o.Tree.Clean
			out.Tree.TouchedDirty = append(out.Tree.TouchedDirty, qualifyAll(k, o.Tree.TouchedDirty)...)
		}
	}
	return out
}

// Migrate brings every store's data to this binary's version, each in its
// own repository; the reports joined, each path qualified by its project.
func (s *Served) Migrate(ctx context.Context, in api.MigrateInput) (api.MigrateOutput, error) {
	keys, outs, err := joined(s, ctx, in, (*core.Service).Migrate)
	if err != nil {
		return api.MigrateOutput{}, err
	}
	return migrateJoin(keys, outs), nil
}

// MigrateCommit converts and commits, each in its own repository, the stores
// whose data waits. Refused writing nothing when none waits or a file one of
// them writes is not as its last commit has it.
func (s *Served) MigrateCommit(ctx context.Context, in api.MigrateCommitInput) (api.MigrateCommitOutput, error) {
	keys, dry, err := joined(s, ctx, in.MigrateInput(true), (*core.Service).Migrate)
	if err != nil {
		return api.MigrateCommitOutput{}, err
	}
	var waiting []string
	var dirty []string
	for i, d := range dry {
		if d.From == d.To && d.GroupMerge == nil {
			continue
		}
		waiting = append(waiting, keys[i])
		if d.Tree != nil && d.Tree.Repo && !d.Tree.Clean {
			dirty = append(dirty, qualifyAll(keys[i], d.Tree.TouchedDirty)...)
		}
	}
	joinedDry := migrateJoin(keys, dry)
	if len(waiting) == 0 {
		return api.MigrateCommitOutput{MigrateOutput: joinedDry}, fmt.Errorf("%w: MigrateCommit: the data of every served project (%s) is at version %d already - nothing waits for a conversion", api.ErrInvalidInput, strings.Join(keys, ", "), joinedDry.To)
	}
	if len(dirty) > 0 {
		return api.MigrateCommitOutput{MigrateOutput: joinedDry}, fmt.Errorf("%w: MigrateCommit: files the conversion writes differ from the last commit - commit or discard them first: %s", api.ErrConflict, strings.Join(dirty, ", "))
	}
	var (
		outs          []api.MigrateOutput
		shas, reasons []string
		committedKeys []string
	)
	for _, k := range waiting {
		got, err := s.set.byKey[k].MigrateCommit(ctx, in)
		if err != nil {
			return api.MigrateCommitOutput{MigrateOutput: migrateJoin(committedKeys, outs)}, fmt.Errorf("project %q: %w", k, err)
		}
		committedKeys = append(committedKeys, k)
		outs = append(outs, got.MigrateOutput)
		if got.Commit.Sha != "" {
			shas = append(shas, k+":"+got.Commit.Sha)
		}
		if got.Commit.Reason != "" {
			reasons = append(reasons, k+": "+got.Commit.Reason)
		}
	}
	out := api.MigrateCommitOutput{MigrateOutput: migrateJoin(committedKeys, outs)}
	if len(waiting) == 1 {
		// One repository converted: its commit as it is.
		out.Commit.Sha = strings.TrimPrefix(strings.Join(shas, ""), waiting[0]+":")
		out.Commit.Reason = strings.TrimPrefix(strings.Join(reasons, ""), waiting[0]+": ")
		return out, nil
	}
	out.Commit = api.MigrateCommit{Sha: strings.Join(shas, " "), Reason: strings.Join(reasons, "; ")}
	return out, nil
}

// CatchUpRelease releases the held events in the store whose catch-up holds
// them; not found when none does.
func (s *Served) CatchUpRelease(ctx context.Context, in api.CatchUpReleaseInput) (api.CatchUpReleaseOutput, error) {
	var notFound error
	for _, k := range s.set.keys {
		out, err := s.set.byKey[k].CatchUpRelease(ctx, in)
		if err == nil {
			return out, nil
		}
		if !errors.Is(err, api.ErrNotFound) {
			return api.CatchUpReleaseOutput{}, fmt.Errorf("project %q: %w", k, err)
		}
		if notFound == nil {
			notFound = err
		}
	}
	return api.CatchUpReleaseOutput{}, notFound
}

// OutboxRelease releases a reaction's outbox in every store: a reaction may
// have written in several.
func (s *Served) OutboxRelease(ctx context.Context, in api.OutboxReleaseInput) (api.OutboxReleaseOutput, error) {
	_, outs, err := joined(s, ctx, in, (*core.Service).OutboxRelease)
	var out api.OutboxReleaseOutput
	for _, o := range outs {
		out.Matches += o.Matches
		out.Hooks += o.Hooks
	}
	return out, err
}

// OutboxDrop drops a reaction's outbox in every store.
func (s *Served) OutboxDrop(ctx context.Context, in api.OutboxDropInput) (api.OutboxDropOutput, error) {
	_, outs, err := joined(s, ctx, in, (*core.Service).OutboxDrop)
	if err != nil {
		return api.OutboxDropOutput{}, err
	}
	out := api.OutboxDropOutput{Writes: []api.MatchedEvent{}}
	for _, o := range outs {
		out.Writes = append(out.Writes, o.Writes...)
		out.NotWritten = append(out.NotWritten, o.NotWritten...)
	}
	return out, nil
}

// OutboxList lists every store's outboxes; a reaction that wrote in several
// stores is listed once, its counts summed.
func (s *Served) OutboxList(ctx context.Context, in api.OutboxListInput) (api.OutboxListOutput, error) {
	_, outs, err := joined(s, ctx, in, (*core.Service).OutboxList)
	if err != nil {
		return api.OutboxListOutput{}, err
	}
	out := api.OutboxListOutput{Outboxes: []api.OutboxSummary{}}
	at := map[string]int{}
	for _, o := range outs {
		for _, b := range o.Outboxes {
			i, ok := at[b.Reaction]
			if !ok {
				at[b.Reaction] = len(out.Outboxes)
				out.Outboxes = append(out.Outboxes, b)
				continue
			}
			got := &out.Outboxes[i]
			if b.At.Before(got.At) {
				got.At = b.At
			}
			got.Writes += b.Writes
			got.Matches += b.Matches
			got.Hooks += b.Hooks
		}
	}
	return out, nil
}

// ChangeSetBegin opens the reaction's or the caller's change set in every store: a reaction
// may write in several. The sets are not one atomic commit across stores -
// each store commits its own (one repository, one commit). A store refusing
// to begin discards the sets already begun in the stores before it: the
// reaction is open in all of them or in none.
func (s *Served) ChangeSetBegin(ctx context.Context, in api.ChangeSetBeginInput) (api.ChangeSetBeginOutput, error) {
	out := api.ChangeSetBeginOutput{Discarded: []api.ChangeSetWrite{}}
	for i, k := range s.set.keys {
		got, err := s.set.byKey[k].ChangeSetBegin(ctx, in)
		if err != nil {
			for _, begun := range s.set.keys[:i] {
				_, _ = s.set.byKey[begun].ChangeSetDiscard(ctx, api.ChangeSetDiscardInput{Reaction: in.Reaction, Set: in.Set})
			}
			return api.ChangeSetBeginOutput{}, fmt.Errorf("project %q: %w", k, err)
		}
		out.Discarded = append(out.Discarded, got.Discarded...)
	}
	return out, nil
}

// ChangeSetCommit commits the reaction's change set in every store, one after
// the other: a failure leaves the stores before it committed.
func (s *Served) ChangeSetCommit(ctx context.Context, in api.ChangeSetCommitInput) (api.ChangeSetCommitOutput, error) {
	_, outs, err := joined(s, ctx, in, (*core.Service).ChangeSetCommit)
	if err != nil {
		return api.ChangeSetCommitOutput{}, err
	}
	out := api.ChangeSetCommitOutput{Writes: []api.ChangeSetWrite{}}
	for _, o := range outs {
		out.Writes = append(out.Writes, o.Writes...)
	}
	return out, nil
}

// ChangeSetDiscard discards the reaction's change set in every store.
func (s *Served) ChangeSetDiscard(ctx context.Context, in api.ChangeSetDiscardInput) (api.ChangeSetDiscardOutput, error) {
	_, outs, err := joined(s, ctx, in, (*core.Service).ChangeSetDiscard)
	if err != nil {
		return api.ChangeSetDiscardOutput{}, err
	}
	out := api.ChangeSetDiscardOutput{Discarded: []api.ChangeSetWrite{}}
	for _, o := range outs {
		out.Discarded = append(out.Discarded, o.Discarded...)
	}
	return out, nil
}

// ChangeSetList lists every store's open change sets; a reaction open in
// several stores (a caller's set alike) is listed once, its writes joined.
func (s *Served) ChangeSetList(ctx context.Context, in api.ChangeSetListInput) (api.ChangeSetListOutput, error) {
	_, outs, err := joined(s, ctx, in, (*core.Service).ChangeSetList)
	if err != nil {
		return api.ChangeSetListOutput{}, err
	}
	out := api.ChangeSetListOutput{Sets: []api.ChangeSetSummary{}}
	at := map[string]int{}
	for _, o := range outs {
		for _, c := range o.Sets {
			k := c.Reaction + "|" + c.Set
			i, ok := at[k]
			if !ok {
				at[k] = len(out.Sets)
				out.Sets = append(out.Sets, c)
				continue
			}
			got := &out.Sets[i]
			if c.At.Before(got.At) {
				got.At = c.At
			}
			got.Writes = append(got.Writes, c.Writes...)
		}
	}
	return out, nil
}

// EventsSubscribe subscribes once per bus the stores publish on - stores
// sharing one bus would deliver each event twice - and fans the channels in;
// the joined channel closes once every one has, and the cancel cancels each.
func (s *Served) EventsSubscribe(ctx context.Context, in api.EventsSubscribeInput) (<-chan api.EventEnvelope, func(), error) {
	var (
		chans   []<-chan api.EventEnvelope
		cancels []func()
		buses   []any
	)
	cancelAll := func() {
		for _, c := range cancels {
			c()
		}
	}
	for _, k := range s.set.keys {
		st := s.set.byKey[k]
		if bus := busOf(st); bus != nil {
			if slices.ContainsFunc(buses, func(b any) bool { return b == bus }) {
				continue
			}
			buses = append(buses, bus)
		}
		ch, cancel, err := st.EventsSubscribe(ctx, in)
		if err != nil {
			cancelAll()
			return nil, nil, fmt.Errorf("project %q: %w", k, err)
		}
		chans = append(chans, ch)
		cancels = append(cancels, cancel)
	}
	if len(chans) == 1 {
		return chans[0], cancelAll, nil
	}
	out := make(chan api.EventEnvelope, 64)
	done := make(chan struct{})
	var once sync.Once
	cancel := func() {
		once.Do(func() {
			close(done)
			cancelAll()
		})
	}
	var wg sync.WaitGroup
	for _, ch := range chans {
		wg.Add(1)
		go func(ch <-chan api.EventEnvelope) {
			defer wg.Done()
			for env := range ch {
				select {
				case out <- env:
				case <-done:
					return // cancelled: the reader may be gone
				}
			}
		}(ch)
	}
	go func() {
		wg.Wait()
		close(out)
	}()
	return out, cancel, nil
}

// busOf is the in-process bus a store publishes on, when it is one value can
// be compared by (a pointer); nil otherwise.
func busOf(st *core.Service) any {
	if st.Config == nil || st.Config.EventBus == nil {
		return nil
	}
	b := any(st.Config.EventBus)
	if !reflect.TypeOf(b).Comparable() {
		return nil
	}
	return b
}

// --- F. One namespace: the first store answers ------------------------------

// ObjectTypeGet: types are one namespace - every store reads the same
// declarations - so the first store answers.
func (s *Served) ObjectTypeGet(ctx context.Context, in api.ObjectTypeGetInput) (api.ObjectTypeGetOutput, error) {
	return s.primary.ObjectTypeGet(ctx, in)
}

func (s *Served) ObjectTypeList(ctx context.Context, in api.ObjectTypeListInput) (api.ObjectTypeListOutput, error) {
	return s.primary.ObjectTypeList(ctx, in)
}

func (s *Served) ObjectAspectList(ctx context.Context, in api.ObjectAspectListInput) (api.ObjectAspectListOutput, error) {
	return s.primary.ObjectAspectList(ctx, in)
}

func (s *Served) DeclarationFindings(ctx context.Context, in api.DeclarationFindingsInput) (api.DeclarationFindingsOutput, error) {
	return s.primary.DeclarationFindings(ctx, in)
}

// --- G. One project's own verbs: refused with several -----------------------

func (s *Served) ProjectInit(context.Context, api.ProjectInitInput) (api.ProjectInitOutput, error) {
	return api.ProjectInitOutput{}, s.refusedWithSeveral("ProjectInit", "each has its project object already - init a project in its own folder")
}
