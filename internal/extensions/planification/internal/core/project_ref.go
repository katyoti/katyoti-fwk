// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"errors"
	"strings"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// A REQUEST'S PROJECT — a universe may serve several projects, two of them
// holding a module of one name and a request of one id. Every
// to-do of a request is put in the request's project: its module is named
// <key>:<module> when several projects are served, bare when one is (as
// before). This file is the one place planification turns a project into a
// reference; the scheduler keys the row by the project the qualified module
// resolves to and keeps the module bare beside it (Todo.Repository). A to-do's
// Request and Target are always bare: a row keeps references of its own.

// requestRef is a request's reference: an id given qualified stays so; a bare
// one is qualified by the project the CWM names holding it when several
// projects are served (ObjectGetOutput.Project), kept bare when one is - or
// when the CWM cannot name one project (its read says why, where it is used).
func requestRef(ctx context.Context, id string) string {
	ref, _ := requestRefOf(ctx, id)
	return ref
}

// requestRefOf is requestRef naming the CWM's refusal of a bare id it cannot
// take as it is - the one several served projects hold (Ambiguous, invalid
// input): a reader of the request's to-dos answers it rather than mixing the
// projects' rows. Any other failure to read (the request gone) keeps the id
// bare, with no error.
func requestRefOf(ctx context.Context, id string) (string, error) {
	if id == "" || strings.Contains(id, ":") {
		return id, nil
	}
	key, err := projectOf(ctx, id)
	if err != nil {
		if errors.Is(err, cwmapi.ErrInvalidInput) {
			return id, err
		}
		return id, nil
	}
	if key != "" {
		return cwmapi.QualifiedID(key, id), nil
	}
	return id, nil
}

// todoRequestRef is the reference of the request a to-do read back works
// for: its Request is bare, its project the uuid in Repository. One project
// served keeps it bare; several qualify it by the project's key - named by
// the CWM for an id one project holds, resolved from the to-do's Repository
// for an id two projects hold.
func todoRequestRef(ctx context.Context, t schedapi.Todo) string {
	if t.Request == "" || strings.Contains(t.Request, ":") {
		return t.Request
	}
	key, err := projectOf(ctx, t.Request)
	if err == nil {
		if key == "" {
			return t.Request
		}
		return cwmapi.QualifiedID(key, t.Request)
	}
	// Refused bare: several projects hold the id - the to-do's own names it.
	if t.Repository == "" {
		return t.Request
	}
	cwm, cerr := cwmOf(ctx)
	if cerr != nil {
		return t.Request
	}
	got, rerr := cwm.ObjectResolve(ctx, cwmapi.ObjectResolveInput{Ref: cwmapi.FullName(t.Repository, t.Request)})
	if rerr != nil || got.Project == "" {
		return t.Request
	}
	return cwmapi.QualifiedID(got.Project, t.Request)
}

// todoModuleRef is the module a follow-on to-do of t is put in: t's module in
// the project of t's request.
func todoModuleRef(ctx context.Context, t schedapi.Todo) string {
	return moduleIn(todoRequestRef(ctx, t), t.Module)
}

// moduleIn is module named in the project of ref (a qualified id or
// module): qualified when ref is, bare otherwise; a module already qualified
// stays as it is.
func moduleIn(ref, module string) string {
	if strings.Contains(module, ":") {
		return module
	}
	if key := keyOf(ref); key != "" {
		return key + ":" + module
	}
	return module
}

// requestModule is the module a request's to-dos are in: its module in its
// project (qualified when ref is).
func requestModule(ref string) (string, error) {
	return cwmapi.ModuleOfObjectID(ref)
}

// keyOf is the project key a reference names, "" when bare.
func keyOf(ref string) string {
	if i := strings.Index(ref, ":"); i > 0 {
		return ref[:i]
	}
	return ""
}

// bareID is a reference's bare id: what a to-do's Request and Target keep.
func bareID(ref string) string {
	if i := strings.Index(ref, ":"); i >= 0 {
		return ref[i+1:]
	}
	return ref
}

// inProject is an object read by a qualified reference, named by it: what
// planification reads, writes and puts for it next stays in its project. An
// object read bare keeps its bare id.
func inProject(o cwmapi.Object, ref string) cwmapi.Object {
	if o.ID != "" && keyOf(ref) != "" && !strings.Contains(o.ID, ":") {
		o.ID = cwmapi.QualifiedID(keyOf(ref), o.ID)
	}
	return o
}

// bareModules is modules each named bare (a module reference's module).
func bareModules(modules []string) []string {
	for i, m := range modules {
		if strings.Contains(m, ":") {
			modules = append([]string(nil), modules...)
			for j := i; j < len(modules); j++ {
				modules[j] = bareID(modules[j])
			}
			return modules
		}
	}
	return modules
}

// projectOf is the key of the project holding id when several projects are
// served, "" when one is - read with the object, once for a
// view carrying the read cache.
func projectOf(ctx context.Context, id string) (string, error) {
	cwm, err := cwmOf(ctx)
	if err != nil {
		return "", err
	}
	module, err := cwmapi.ModuleOfObjectID(id)
	if err != nil {
		return "", err
	}
	r := readObject(ctx, cwm, module, id)
	return r.project, r.err
}

// repositoryOfKey is the uuid of the project a key names - what a to-do of
// that project carries in Repository; "" when it cannot be read. A view
// carrying the read cache resolves each key once (readCache.repos, the
// counterpart of projectRefs' uuid->key map): it filters the to-dos of many
// requests of one project.
func repositoryOfKey(ctx context.Context, key, id string) string {
	c, cached := ctx.Value(readCacheKey{}).(*readCache)
	if cached {
		c.mu.Lock()
		repo, hit := c.repos[key]
		c.mu.Unlock()
		if hit {
			return repo
		}
	}
	repo := resolveRepository(ctx, key, id)
	if cached && repo != "" { // a key not resolved is asked again with the next id
		c.mu.Lock()
		if c.repos == nil {
			c.repos = map[string]string{}
		}
		c.repos[key] = repo
		c.mu.Unlock()
	}
	return repo
}

// resolveRepository asks the CWM the uuid of the project of key holding id.
func resolveRepository(ctx context.Context, key, id string) string {
	cwm, err := cwmOf(ctx)
	if err != nil {
		return ""
	}
	got, err := cwm.ObjectResolve(ctx, cwmapi.ObjectResolveInput{Ref: cwmapi.QualifiedID(key, id)})
	if err != nil {
		return ""
	}
	return got.ProjectID
}

// requestFilter is what keeps the to-dos of the request ref names: its bare
// id, and - ref qualified - its project's uuid (once per key for a view). A
// qualified ref keeps only its project's rows: a row with no Repository is
// one of a store serving one project, which names its requests bare - with
// several served it would belong to none, and is not mixed into any. A
// project whose uuid cannot be read filters by the bare id alone.
func requestFilter(ctx context.Context, ref string) func(api.WorkTodo) bool {
	bare, repo := bareID(ref), ""
	if key := keyOf(ref); key != "" {
		repo = repositoryOfKey(ctx, key, bare)
	}
	return func(t api.WorkTodo) bool {
		return t.Request == bare && (repo == "" || t.Repository == repo)
	}
}

// projectCWM is the CWM as planification asks it: an object it names by a
// qualified reference answers named so - read, updated, moved or given a
// capability - so the request it plans stays in its project through every
// write that follows. Bare references pass through unchanged: a universe
// serving one project sees the CWM itself.
type projectCWM struct{ cwmapi.CWMService }

func (c projectCWM) ObjectGet(ctx context.Context, in cwmapi.ObjectGetInput) (cwmapi.ObjectGetOutput, error) {
	out, err := c.CWMService.ObjectGet(ctx, in)
	out.Object = inProject(out.Object, in.ID)
	return out, err
}

func (c projectCWM) ObjectUpdate(ctx context.Context, in cwmapi.ObjectUpdateInput) (cwmapi.ObjectUpdateOutput, error) {
	ref := in.Object.ID
	if in.Revision != "" || in.Force {
		out, err := c.CWMService.ObjectUpdate(ctx, in)
		out.Object = inProject(out.Object, ref)
		return out, err
	}
	out, err := c.updateKeepingMoves(ctx, in)
	out.Object = inProject(out.Object, ref)
	return out, err
}

// keptOnUpdate are the capabilities planification changes only through
// CWM's verbs (a move, a gate wait added or ended): a write of a whole
// object never carries its own copy of them.
var keptOnUpdate = []string{cwmapi.CapLifecycle, api.CapGateWait}

// updateWriteAttempts bounds the rereads of a write a concurrent one refused.
const updateWriteAttempts = 8

// updateKeepingMoves writes an object planification read earlier, with no
// revision: the stored lifecycle and gate wait are taken over the copy's and
// the write names the revision read, retried on a conflict. A move made since
// the copy was read - a person forcing a gate, a reaction ending a wait - is
// never undone by a planner's late write. An object
// not stored yet is written as given.
func (c projectCWM) updateKeepingMoves(ctx context.Context, in cwmapi.ObjectUpdateInput) (cwmapi.ObjectUpdateOutput, error) {
	module := in.Module
	if module == "" {
		m, err := cwmapi.ModuleOfObjectID(in.Object.ID)
		if err != nil {
			return c.CWMService.ObjectUpdate(ctx, in)
		}
		module = m
	}
	for attempt := 0; ; attempt++ {
		got, err := c.CWMService.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: module, ID: in.Object.ID})
		if err != nil || got.Revision == "" {
			return c.CWMService.ObjectUpdate(ctx, in)
		}
		next := in
		next.Object = withStoredMoves(in.Object, got.Object)
		next.Revision = got.Revision
		out, err := c.CWMService.ObjectUpdate(ctx, next)
		if !errors.Is(err, cwmapi.ErrConflict) || attempt >= updateWriteAttempts {
			return out, err
		}
	}
}

// withStoredMoves is o with stored's kept capabilities in place of its own.
func withStoredMoves(o, stored cwmapi.Object) cwmapi.Object {
	caps := make(map[string]map[string]any, len(o.Capabilities))
	for k, v := range o.Capabilities {
		caps[k] = v
	}
	for _, k := range keptOnUpdate {
		if v, ok := stored.Capabilities[k]; ok {
			caps[k] = v
		} else {
			delete(caps, k)
		}
	}
	o.Capabilities = caps
	return o
}

func (c projectCWM) ObjectTransition(ctx context.Context, in cwmapi.ObjectTransitionInput) (cwmapi.ObjectTransitionOutput, error) {
	out, err := c.CWMService.ObjectTransition(ctx, in)
	out.Object = inProject(out.Object, in.ID)
	return out, err
}

func (c projectCWM) ObjectAddCapability(ctx context.Context, in cwmapi.ObjectAddCapabilityInput) (cwmapi.ObjectAddCapabilityOutput, error) {
	out, err := c.CWMService.ObjectAddCapability(ctx, in)
	out.Object = inProject(out.Object, in.ID)
	return out, err
}

func (c projectCWM) ObjectRemoveCapability(ctx context.Context, in cwmapi.ObjectRemoveCapabilityInput) (cwmapi.ObjectRemoveCapabilityOutput, error) {
	out, err := c.CWMService.ObjectRemoveCapability(ctx, in)
	out.Object = inProject(out.Object, in.ID)
	return out, err
}

// THE PROJECTS « À FAIRE » READS - a universe serving
// several projects lists each object with the key of the project holding it
// (ObjectList's Projects). The demands, the person's items and the to-dos
// joining them are then keyed by the qualified id <key>:<id>: two projects may
// hold one request id, and neither collapses into the other.

// qualifyListed is objects named in the project each is listed in: its id
// and its bare edges qualified by the key (edges across projects are not
// supported). One project served (no Projects): as listed.
func qualifyListed(objects []cwmapi.Object, projects []string) []cwmapi.Object {
	if len(projects) == 0 || len(projects) != len(objects) {
		return objects
	}
	out := make([]cwmapi.Object, len(objects))
	for i, o := range objects {
		out[i] = inKey(o, projects[i])
	}
	return out
}

// inKey is o named in the project of key: its id and its bare edges to an
// object qualified; o as it is when key is empty or its id is qualified.
func inKey(o cwmapi.Object, key string) cwmapi.Object {
	if key == "" || o.ID == "" || strings.Contains(o.ID, ":") {
		return o
	}
	o.ID = cwmapi.QualifiedID(key, o.ID)
	if len(o.Edges) > 0 {
		edges := make([]cwmapi.ObjectEdge, len(o.Edges))
		for i, e := range o.Edges {
			edges[i] = e
			if e.To != "" && strings.Contains(e.To, ".") && !strings.Contains(e.To, ":") {
				edges[i].To = cwmapi.QualifiedID(key, e.To)
			}
		}
		o.Edges = edges
	}
	return o
}

// inKeyOf is id named in the project ref names: qualified when ref is and id
// is bare, as it is otherwise.
func inKeyOf(ref, id string) string {
	if key := keyOf(ref); key != "" && id != "" && !strings.Contains(id, ":") {
		return cwmapi.QualifiedID(key, id)
	}
	return id
}

// projectRefs names the request of a to-do (its Request bare, its project
// the uuid in Repository) the way the demands are keyed: qualified by the
// project's key when several projects are served, bare when one is. Each
// project's key is resolved once (ObjectResolve on the full name).
type projectRefs struct {
	ctx     context.Context
	several bool
	keys    map[string]string // by project uuid
}

// newProjectRefs reads the to-dos' requests for several served projects when
// several is set, bare otherwise.
func newProjectRefs(ctx context.Context, several bool) *projectRefs {
	return &projectRefs{ctx: ctx, several: several, keys: map[string]string{}}
}

// ref is the reference of request in the project of uuid repository.
func (p *projectRefs) ref(request, repository string) string {
	if p == nil || !p.several || request == "" || repository == "" || strings.Contains(request, ":") {
		return request
	}
	key, ok := p.keys[repository]
	if !ok {
		if cwm, err := cwmOf(p.ctx); err == nil {
			if got, err := cwm.ObjectResolve(p.ctx, cwmapi.ObjectResolveInput{Ref: cwmapi.FullName(repository, request)}); err == nil {
				key = got.Project
			}
		}
		if key != "" {
			p.keys[repository] = key // a request not found resolves again with the next one
		}
	}
	if key == "" {
		return request
	}
	return cwmapi.QualifiedID(key, request)
}

// todo is the reference of the request a to-do of planification works for.
func (p *projectRefs) todo(t api.WorkTodo) string { return p.ref(t.Request, t.Repository) }

// module is the reference of a to-do's module the way the scheduler's Modes
// names it: <key>:<module> in the project of its request when several
// projects are served, bare when one is.
func (p *projectRefs) module(t api.WorkTodo) string { return moduleIn(p.todo(t), t.Module) }

// modesByRef is the scheduler's modes by the reference each module is named
// by, and whether several projects are served (a reference is qualified).
func modesByRef(modules []schedapi.ModeOutput) (map[string]string, bool) {
	modes, several := make(map[string]string, len(modules)), false
	for _, m := range modules {
		modes[m.Module] = m.Mode
		several = several || keyOf(m.Module) != ""
	}
	return modes, several
}

// offFor reports a to-do whose module is off, its mode read by the module's
// reference in the to-do's project.
func offFor(modes map[string]string, refs *projectRefs, t api.WorkTodo) bool {
	return modes[refs.module(t)] == schedapi.ModeOff
}
