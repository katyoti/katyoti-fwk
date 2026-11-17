// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"fmt"
	"sort"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// EdgeCreate adds a typed directed edge from one object. The source object
// owns its outgoing edges: the verb appends to its `edges:` and rewrites the
// file, exactly what an author would do by hand, so the two ways of adding an
// edge cannot disagree. `parent_id` is refused — containment is the object's
// `parent`. Idempotent: re-adding the same edge is a no-op success.
func (s *Service) EdgeCreate(ctx stdctx.Context, in api.EdgeCreateInput) (api.EdgeCreateOutput, error) {
	if in.From == "" || in.To == "" {
		return api.EdgeCreateOutput{}, fmt.Errorf("%w: EdgeCreate: from/to required", api.ErrInvalidInput)
	}
	if in.Kind == "" {
		return api.EdgeCreateOutput{}, fmt.Errorf("%w: EdgeCreate: kind required", api.ErrInvalidInput)
	}
	if in.Kind == api.EdgeTypeParentID {
		return api.EdgeCreateOutput{}, fmt.Errorf(
			"%w: EdgeCreate: parent_id is reserved — containment is parent (ObjectUpdate, ObjectMove)",
			api.ErrTypeRuleViolation)
	}
	obj, ok := s.objectByIDFor(ctx, in.From)
	if !ok {
		return api.EdgeCreateOutput{}, fmt.Errorf("%w: EdgeCreate: object %q", api.ErrNotFound, in.From)
	}
	// A bare target is resolved from the SOURCE's module (qualifyTarget): its
	// own object first, else the object with that bare id (a module).
	if m, err := api.ModuleOfObjectID(in.From); err == nil {
		in.To = qualifyTarget(m, in.To, s.existsFor(ctx))
	}
	added := api.ObjectEdge{Kind: string(in.Kind), To: in.To}
	err := s.rewriteEdges(ctx, obj.ID, "EdgeCreate", func(edges []api.ObjectEdge) ([]api.ObjectEdge, bool, error) {
		for _, e := range edges {
			if e == added {
				return nil, false, nil // idempotent
			}
		}
		next := append(append([]api.ObjectEdge(nil), edges...), added)
		sortObjectEdges(next)
		return next, true, nil
	})
	if err != nil {
		return api.EdgeCreateOutput{}, err
	}
	return api.EdgeCreateOutput{Edge: api.Edge{From: in.From, To: in.To, Kind: in.Kind}}, nil
}

// EdgeList returns the edges incident to an object (or every edge when NodeID
// is empty), optionally filtered by Kind + Direction, answered from the index.
// Sorted (from, to, kind) for byte-stable output across runs.
func (s *Service) EdgeList(ctx stdctx.Context, in api.EdgeListInput) (api.EdgeListOutput, error) {
	if err := s.index().ensureLoaded(); err != nil {
		return api.EdgeListOutput{}, err
	}
	edges := s.index().edges(in.ObjectID, in.Kind, in.Direction)
	if edges == nil {
		edges = []api.Edge{}
	}
	for i := range edges {
		decl := s.edgeDecl(edges[i].Kind)
		edges[i].Inverse = decl.InverseName // served-by, realized-by ...
		edges[i].Label, edges[i].InverseLabel = decl.Label, decl.InverseLabel
		// A target that left for its module's history has ended: the link is
		// valid, and a reader shows it as ended (ended.go).
		if _, indexed := s.index().get(edges[i].To); !indexed {
			_, edges[i].Ended = s.isEnded(edges[i].To)
		}
	}
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].From != edges[j].From {
			return edges[i].From < edges[j].From
		}
		if edges[i].To != edges[j].To {
			return edges[i].To < edges[j].To
		}
		return edges[i].Kind < edges[j].Kind
	})
	return api.EdgeListOutput{Edges: edges}, nil
}

// EdgeDelete removes a specific (From, To, Kind) edge from the source object.
// An absent edge is refused as not found — a wrong id or kind must not look
// like success.
func (s *Service) EdgeDelete(ctx stdctx.Context, in api.EdgeDeleteInput) (api.EdgeDeleteOutput, error) {
	if in.From == "" || in.To == "" {
		return api.EdgeDeleteOutput{}, fmt.Errorf("%w: EdgeDelete: from/to required", api.ErrInvalidInput)
	}
	if in.Kind == "" {
		return api.EdgeDeleteOutput{}, fmt.Errorf("%w: EdgeDelete: kind required", api.ErrInvalidInput)
	}
	if in.Kind == api.EdgeTypeParentID {
		return api.EdgeDeleteOutput{}, fmt.Errorf(
			"%w: EdgeDelete: parent_id is reserved — containment is parent",
			api.ErrTypeRuleViolation)
	}
	obj, ok := s.objectByIDFor(ctx, in.From)
	if !ok {
		return api.EdgeDeleteOutput{}, fmt.Errorf("%w: EdgeDelete: object %q", api.ErrNotFound, in.From)
	}
	err := s.rewriteEdges(ctx, obj.ID, "EdgeDelete", func(edges []api.ObjectEdge) ([]api.ObjectEdge, bool, error) {
		kept := make([]api.ObjectEdge, 0, len(edges))
		for _, e := range edges {
			if e.Kind == string(in.Kind) && e.To == in.To {
				continue
			}
			kept = append(kept, e)
		}
		if len(kept) == len(edges) {
			return nil, false, fmt.Errorf("%w: EdgeDelete: no edge %s -%s-> %s on object", api.ErrNotFound, in.From, in.Kind, in.To)
		}
		return kept, true, nil
	})
	if err != nil {
		return api.EdgeDeleteOutput{}, err
	}
	return api.EdgeDeleteOutput{}, nil
}

// objectByID answers "is this id an object the index holds".
// objectExists reports an object of the project with this id.
func (s *Service) objectExists(id string) bool {
	_, ok := s.objectByID(id)
	return ok
}

func (s *Service) objectByID(id string) (api.Object, bool) {
	if id == "" {
		return api.Object{}, false
	}
	if err := s.index().ensureLoaded(); err != nil {
		return api.Object{}, false
	}
	return s.index().get(id)
}

// rewriteEdges changes an object's edges through ObjectUpdate — the one write
// path: the same checks, before_update / on_update, the caller's context on
// the event. It changes the STORED edges (read from the file, which keeps the
// body) on the revision read, and redoes the change when another write landed
// in between.
func (s *Service) rewriteEdges(ctx stdctx.Context, id, verb string, change func([]api.ObjectEdge) ([]api.ObjectEdge, bool, error)) error {
	_, err := retryOnConflict(func() (struct{}, error) {
		obj, module, rev, err := s.fullObjectRev(ctx, id)
		if err != nil {
			return struct{}{}, err
		}
		edges, changed, err := change(obj.Edges)
		if err != nil || !changed {
			return struct{}{}, err
		}
		obj.Edges = edges
		if _, err := s.ObjectUpdate(ctx, api.ObjectUpdateInput{Module: module, Object: obj, Revision: rev}); err != nil {
			return struct{}{}, fmt.Errorf("%s: %w", verb, err)
		}
		return struct{}{}, nil
	})
	return err
}

// sortObjectEdges keeps an object's edge list byte-stable: by kind, then
// target — the order nodeobj writes, so a verb edit and a transform never
// disagree on the file.
func sortObjectEdges(edges []api.ObjectEdge) {
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].Kind != edges[j].Kind {
			return edges[i].Kind < edges[j].Kind
		}
		return edges[i].To < edges[j].To
	})
}
