// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/objectcontext/api"
)

// declarations are what the catalog declares that an agent context reads: the
// capabilities (their `agent:` and the fields carried into it), the policies
// and the `agent-context` contributions.
type declarations struct {
	capabilities map[string]cwmapi.Capability
	policies     []cwmapi.Policy
	framings     []api.AgentFraming
}

// catalog holds the declarations, read once and again after a change.
type catalog struct {
	mu   sync.Mutex
	decl *declarations
}

func (c *catalog) invalidate() {
	c.mu.Lock()
	c.decl = nil
	c.mu.Unlock()
}

func (c *catalog) get(ctx context.Context) declarations {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.decl == nil {
		d := readDeclarations(ctx)
		c.decl = &d
	}
	return *c.decl
}

// readDeclarations reads the capabilities and policies as CWM reads them
// (DeclarationsFromCatalog) and this service's own `agent-context`
// declarations (QueryTyped) — the shared readers, so two copies of a loop
// never disagree on what is declared.
func readDeclarations(ctx context.Context) declarations {
	d := declarations{capabilities: map[string]cwmapi.Capability{}}
	cat, err := application.UseService[mbapi.Catalog](ctx)
	if err != nil || cat == nil {
		slog.Warn("object-context: no catalog: no declaration is read", "error", err)
		return d
	}
	decl := cwmapi.DeclarationsFromCatalog(ctx, cat)
	for _, c := range decl.Capabilities {
		d.capabilities[c.Name] = c
	}
	d.policies = decl.Policies
	// Each framing decodes on its own: one that does not (a filter key no
	// field takes) is skipped with the reason, never all of them.
	framings, err := mbapi.QueryTyped[json.RawMessage](ctx, mbapi.Q{Reach: mbapi.ReachUniverse, Kind: api.AgentContextKind, Catalog: cat})
	if err != nil {
		slog.Warn("object-context: agent-context declarations not read", "error", err)
		return d
	}
	for _, f := range framings {
		var fr api.AgentFraming
		if len(f.Body) > 0 {
			if err := json.Unmarshal(f.Body, &fr); err != nil {
				slog.Warn("object-context: agent-context skipped", "uri", f.URI, "error", err)
				continue
			}
		}
		if fr.Name == "" {
			continue
		}
		fr.Provider = f.Provider
		d.framings = append(d.framings, fr)
	}
	return d
}

// policiesFor is every policy that applies to an object of type typ carrying
// caps (its effective capabilities), once each.
func (d declarations) policiesFor(typ string, caps []string) []cwmapi.Policy {
	var out []cwmapi.Policy
	seen := map[string]bool{}
	for _, p := range d.policies {
		if ok, _ := p.AppliesTo(typ, caps); ok && !seen[p.Name] {
			seen[p.Name] = true
			out = append(out, p)
		}
	}
	return out
}

// reader is CWM as this service reads it.
type reader struct {
	cwm cwmapi.CWMService
}

func readerOf(ctx context.Context) (reader, error) {
	cwm, err := application.UseService[cwmapi.CWMService](ctx)
	if err != nil || cwm == nil {
		return reader{}, fmt.Errorf("object-context: no CWMService: %v", err)
	}
	return reader{cwm}, nil
}

// get is one object, whole (body and fields).
func (r reader) get(ctx context.Context, id string) (cwmapi.Object, error) {
	module, _ := cwmapi.ModuleOfObjectID(id)
	if module == "" {
		module = id // a module object's id is its folder
	}
	out, err := r.cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: module, ID: id})
	return out.Object, err
}

// children are an object's direct children, of one type when typ is set.
func (r reader) children(ctx context.Context, id, typ string) []cwmapi.Object {
	out, err := r.cwm.ObjectList(ctx, cwmapi.ObjectListInput{Parent: id, Type: typ})
	if err != nil {
		return nil
	}
	return out.Objects
}

// objectType is a declared type, or false.
func (r reader) objectType(ctx context.Context, name string) (cwmapi.ObjectType, bool) {
	out, err := r.cwm.ObjectTypeGet(ctx, cwmapi.ObjectTypeGetInput{Name: name})
	return out.ObjectType, err == nil
}

// evaluator evaluates expressions over one object in the plain object
// vocabulary (no functions), each once: what is known ahead is asked in one
// call, the rest as it comes.
type evaluator struct {
	ctx  context.Context
	cwm  cwmapi.CWMService
	id   string
	memo map[string]cwmapi.EvaluateResult
}

func newEvaluator(ctx context.Context, cwm cwmapi.CWMService, id string) *evaluator {
	return &evaluator{ctx: ctx, cwm: cwm, id: id, memo: map[string]cwmapi.EvaluateResult{}}
}

// prefetch evaluates the expressions not yet known, in one call.
func (e *evaluator) prefetch(exprs []string) {
	var ask []string
	seen := map[string]bool{}
	for _, x := range exprs {
		if _, ok := e.memo[x]; !ok && x != "" && !seen[x] {
			seen[x] = true
			ask = append(ask, x)
		}
	}
	if len(ask) == 0 {
		return
	}
	out, err := e.cwm.ObjectEvaluate(e.ctx, cwmapi.ObjectEvaluateInput{ID: e.id, Expressions: ask, NoFunctions: true})
	for i, x := range ask {
		switch {
		case err != nil:
			e.memo[x] = cwmapi.EvaluateResult{Expression: x, Error: err.Error()}
		case i < len(out.Results):
			e.memo[x] = out.Results[i]
		default:
			e.memo[x] = cwmapi.EvaluateResult{Expression: x, Error: "not evaluated"}
		}
	}
}

// value is an expression's value.
func (e *evaluator) value(x string) (any, error) {
	e.prefetch([]string{x})
	r := e.memo[x]
	if r.Error != "" {
		return nil, fmt.Errorf("%s", r.Error)
	}
	return r.Value, nil
}

// predicate is a `when`: it must be a bool — a string or a number is a defect
// in the predicate, never read as truthy.
func (e *evaluator) predicate(x string) (bool, error) {
	v, err := e.value(x)
	if err != nil {
		return false, err
	}
	b, ok := v.(bool)
	if !ok {
		return false, fmt.Errorf("predicate did not return a bool (got %T)", v)
	}
	return b, nil
}
